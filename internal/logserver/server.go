// Package logserver embeds the realtime log viewer HTTP server. Static
// assets are go:embed'd from web/logserver rather than kept as an inline Go
// string constant, so the template stays editable without touching Go
// code.
package logserver

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	weblogserver "github.com/arcrek/xray-vless-ws-go/web/logserver"
)

// Server is the embedded log viewer HTTP server.
type Server struct {
	Addr       string       // e.g. "0.0.0.0:9999"
	Password   string       // empty = auth disabled
	Status     *StatusStore // always non-nil
	ConfigPath string       // path to frp_info.config (default: "frp_info.config")
	JSONPath   string       // path to frp_info.json (default: "frp_info.json")

	ring       *Ring
	httpServer *http.Server
	authSecret []byte
}

// New constructs a Server. maxLogs<=0 uses the default (500).
func New(addr, password string, maxLogs int) *Server {
	return &Server{
		Addr:       addr,
		Password:   password,
		Status:     NewStatusStore(),
		ConfigPath: "frp_info.config",
		JSONPath:   "frp_info.json",
		ring:       NewRing(maxLogs),
		authSecret: generateAuthSecret(),
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
	// Public static assets & login page
	mux.Handle("/", http.FileServer(http.FS(assetsFS)))

	// Auth APIs (Public)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)
	mux.HandleFunc("/api/auth-status", s.handleAuthStatus)

	// Protected APIs
	mux.Handle("/logs", s.withAuth(http.HandlerFunc(s.handleLogs)))
	mux.Handle("/stats", s.withAuth(http.HandlerFunc(s.handleStats)))
	mux.Handle("/api/vless-info", s.withAuth(http.HandlerFunc(s.handleVlessInfo)))

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

// isAuthenticated checks whether the given HTTP request has valid authentication.
func (s *Server) isAuthenticated(r *http.Request) bool {
	if s.Password == "" {
		return true
	}

	// 1. Check session cookie
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if validateSessionToken(cookie.Value, s.authSecret) {
			return true
		}
	}

	// 2. Check Authorization header (Bearer or Basic)
	authHdr := r.Header.Get("Authorization")
	if authHdr != "" {
		if strings.HasPrefix(authHdr, "Bearer ") {
			token := strings.TrimPrefix(authHdr, "Bearer ")
			if validateSessionToken(token, s.authSecret) || subtle.ConstantTimeCompare([]byte(token), []byte(s.Password)) == 1 {
				return true
			}
		}

		// Basic auth support for curl / backward compatibility
		_, pass, ok := r.BasicAuth()
		if ok && subtle.ConstantTimeCompare([]byte(pass), []byte(s.Password)) == 1 {
			return true
		}
	}

	return false
}

// withAuth wraps a handler with session cookie / token / basic auth protection.
func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": "unauthorized",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleLogin handles POST /api/login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if s.Password == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "auth_required": false})
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid json"})
			return
		}
	} else {
		_ = r.ParseForm()
		req.Password = r.FormValue("password")
	}

	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.Password)) != 1 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid password"})
		return
	}

	token := createSessionToken(s.authSecret)
	setSessionCookie(w, token)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "token": token})
}

// handleLogout handles POST /api/logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// handleAuthStatus handles GET /api/auth-status
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"auth_required": s.Password != "",
		"authenticated": s.isAuthenticated(r),
	})
}

// handleVlessInfo handles GET /api/vless-info
func (s *Server) handleVlessInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	cfgPath := s.ConfigPath
	if cfgPath == "" {
		cfgPath = "frp_info.config"
	}

	content, err := os.ReadFile(cfgPath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{
			"ready":         false,
			"links":         []string{},
			"raw_config":    "",
			"base64_config": "",
			"message":       "Config file not generated yet",
		})
		return
	}

	rawStr := string(content)
	var links []string
	for _, line := range strings.Split(rawStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			links = append(links, trimmed)
		}
	}
	if links == nil {
		links = []string{}
	}

	b64Config := base64.StdEncoding.EncodeToString(content)

	resp := map[string]any{
		"ready":         len(links) > 0,
		"links":         links,
		"raw_config":    rawStr,
		"base64_config": b64Config,
	}

	jsonPath := s.JSONPath
	if jsonPath == "" {
		jsonPath = "frp_info.json"
	}
	if jContent, err := os.ReadFile(jsonPath); err == nil {
		var meta map[string]any
		if json.Unmarshal(jContent, &meta) == nil {
			resp["meta"] = meta
			if ip, ok := meta["ip"].(string); ok {
				resp["ip"] = ip
			}
			if wshost, ok := meta["wshost"].(string); ok {
				resp["wshost"] = wshost
			}
			if wspath, ok := meta["wspath"].(string); ok {
				resp["wspath"] = wspath
			}
			if transport, ok := meta["transport"].(string); ok {
				resp["transport"] = transport
			}
		}
	}

	json.NewEncoder(w).Encode(resp)
}

// handleLogs serves GET /logs?last_id=N -> {"new_logs":[...],"last_id":N}.
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	lastID := 0
	if v := r.URL.Query().Get("last_id"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			lastID = n
		}
	}

	newLogs, currentSeq := s.ring.Since(lastID)
	if newLogs == nil {
		newLogs = []LogLine{} // "new_logs":[] must serialize as [] not null
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"new_logs": newLogs,
		"last_id":  currentSeq,
	})
}

// handleStats serves GET /stats -> the current StatsSnapshot as JSON.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.Status.Snapshot())
}
