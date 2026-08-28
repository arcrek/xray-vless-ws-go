package tunnel

import (
	"bufio"
	"os"
	"testing"
)

// TestParseQuickTunnelHostnameFixture replays a real captured cloudflared
// run (internal/tunnel/testdata/quicktunnel_stdout.txt, captured with
// `cloudflared tunnel --protocol http2 --url http://127.0.0.1:18899
// --metrics 127.0.0.1:19091` against the pinned CloudflaredVersion) line by
// line through HostnameParser, exactly as the real stdout-reading goroutine
// will.
func TestParseQuickTunnelHostnameFixture(t *testing.T) {
	f, err := os.Open("testdata/quicktunnel_stdout.txt")
	if err != nil {
		t.Fatalf("opening fixture: %v", err)
	}
	defer f.Close()

	p := NewHostnameParser()
	var got string
	var found bool

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if host, ok := p.Feed(scanner.Text()); ok {
			got, found = host, true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning fixture: %v", err)
	}

	if !found {
		t.Fatal("HostnameParser did not find a hostname in the captured fixture")
	}
	want := "fin-places-qld-tons.trycloudflare.com"
	if got != want {
		t.Errorf("got hostname %q, want %q", got, want)
	}
}

func TestHostnameParserIgnoresURLWithoutBanner(t *testing.T) {
	p := NewHostnameParser()
	// A trycloudflare.com URL appearing without the preceding banner line
	// (e.g. in an unrelated log message) must NOT be picked up — this is
	// the exact false-match risk main.py's blind regex-anywhere-in-log had.
	if _, found := p.Feed("some unrelated line mentioning https://not-a-real-tunnel.trycloudflare.com by accident"); found {
		t.Error("Feed matched a URL with no preceding banner line, should not have")
	}
}

func TestHostnameParserBannerThenURL(t *testing.T) {
	p := NewHostnameParser()
	if _, found := p.Feed("INF |  Your quick Tunnel has been created! Visit it at (it may take some time to be reachable):  |"); found {
		t.Fatal("banner line alone should not report found=true")
	}
	host, found := p.Feed("INF |  https://abc-def-ghi.trycloudflare.com                                             |")
	if !found {
		t.Fatal("expected hostname to be found on the line after the banner")
	}
	if host != "abc-def-ghi.trycloudflare.com" {
		t.Errorf("got %q, want abc-def-ghi.trycloudflare.com", host)
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[32mINF\x1b[0m some message"
	want := "INF some message"
	if got := StripANSI(in); got != want {
		t.Errorf("StripANSI(%q) = %q, want %q", in, got, want)
	}
}
