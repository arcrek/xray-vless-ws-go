// Package logserver embeds the realtime log viewer HTTP server, ported from
// logging_site.py's RealtimeLogger. Static assets are go:embed'd from
// web/logserver rather than kept as an inline Go string constant, so the
// template stays editable without touching Go code.
package logserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	weblogserver "github.com/arcrek/xray-vless-ws-go/web/logserver"
)

// Server is the embedded log viewer HTTP server.
type Server struct {
	Addr     string // e.g. "0.0.0.0:9999"
	Password string // empty = auth disabled, matching Python's password=None default

	ring       *Ring
	httpServer *http.Server
}

// New constructs a Server. maxLogs<=0 uses the default (500, matching
// logging_site.py).
func New(addr, password string, maxLogs int) *Server {
	return &Server{
		Addr:     addr,
		Password: password,
		ring:     NewRing(maxLogs),
	}
}

// Push adds one log line to the ring buffer, tagged by source ("XRAY" /
// "CLOUDFLARE" / etc.) — the sink both Phase 2's xray-core log channel and
// Phase 3's tunnel supervisor feed into.
func (s *Server) Push(source, text string) {
	s.ring.Push(source, text)
}

// Start begins serving until ctx is cancelled. It returns once the server
// has shut down (or immediately with an error if it never started
// listening).
func (s *Server) Start(ctx context.Context) error {
	assetsFS, err := fs.Sub(weblogserver.FS, "static")
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/", s.withAuth(http.FileServer(http.FS(assetsFS))))
	mux.Handle("/logs", s.withAuth(http.HandlerFunc(s.handleLogs)))

	s.httpServer = &http.Server{Addr: s.Addr, Handler: mux}

	errCh := make(chan error, 1)
	go func() { errCh <- s.httpServer.ListenAndServe() }()

	select {
	case <-ctx.Done():
		// Bounded, not context.Background(): a graceful Shutdown() waits
		// for in-flight requests (e.g. a client whose /logs long-poll
		// hasn't returned yet) and would otherwise block indefinitely,
		// which — via main.go's own bounded wait — is exactly what that
		// bound exists to catch, but closing cleanly here is cheaper than
		// relying on the outer backstop.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

// withAuth wraps a handler with optional HTTP Basic Auth, ported from
// logging_site.py:103-110's check_auth(). Off (any request allowed) when
// Password is empty, matching Python's password=None default.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Password != "" {
			_, pass, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(pass), []byte(s.Password)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Login Required"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// handleLogs serves GET /logs?last_id=N -> {"new_logs":[...],"last_id":N},
// matching logging_site.py:127-138 exactly.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	lastID := 0
	if v := r.URL.Query().Get("last_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			lastID = n
		}
	}

	newLogs, currentSeq := s.ring.Since(lastID)
	if newLogs == nil {
		newLogs = []LogLine{} // "new_logs":[] not null, matching Python's json.dumps of an empty list
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"new_logs": newLogs,
		"last_id":  currentSeq,
	})
}
