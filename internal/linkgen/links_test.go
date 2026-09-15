package linkgen

import (
	"os"
	"strings"
	"testing"

	"github.com/arcrek/xray-vless-ws-go/internal/config"
)

// TestBuildLinksGoldenFixture golden-tests BuildLinks against
// testdata/golden_vless_info.config — a fixed-input fixture, not
// hand-written — so URL-encoding correctness (Go's QueryEscape/PathEscape
// diverge from a naive quote(safe="") on space and '&') is verified
// byte-for-byte rather than assumed.
func TestBuildLinksGoldenFixture(t *testing.T) {
	golden, err := os.ReadFile("testdata/golden_vless_info.config")
	if err != nil {
		t.Fatalf("reading golden fixture: %v", err)
	}
	wantLinks := strings.Split(strings.TrimRight(string(golden), "\n"), "\n")

	sni, err := config.ParseSNIList("api24-normal-alisg.tiktokv.com#Tiktok,vnpt.theworkpc.com#Free VNPT,noremark.example.com,weird#Remark with spaces & stuff")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		WSPath:  "/tiktok4g",
		WSHost:  "trycloudflare.com", // default: links use the detected tunnelHost, not WSHost
		FakeSNI: sni,
	}

	got := BuildLinks(cfg, "5ccad305-e243-4bb2-abf0-1e37189ce4e8", "fin-places-qld-tons.trycloudflare.com")

	if len(got) != len(wantLinks) {
		t.Fatalf("got %d links, want %d\ngot:  %v\nwant: %v", len(got), len(wantLinks), got, wantLinks)
	}
	for i := range got {
		if got[i] != wantLinks[i] {
			t.Errorf("link[%d] mismatch:\n got:  %s\n want: %s", i, got[i], wantLinks[i])
		}
	}
}

func TestBuildLinksNamedTunnelPrefersWSHost(t *testing.T) {
	sni, err := config.ParseSNIList("a.com#A")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		WSPath:  "/p",
		WSHost:  "reverse.example.io.vn", // not the default -> links must use this, not tunnelHost
		FakeSNI: sni,
	}
	got := BuildLinks(cfg, "uuid-x", "some-detected-host.trycloudflare.com")
	for _, link := range got {
		if strings.Contains(link, "some-detected-host") {
			t.Errorf("link used detected tunnelHost instead of WS_HOST override: %s", link)
		}
		if !strings.Contains(link, "reverse.example.io.vn") {
			t.Errorf("link missing WS_HOST override: %s", link)
		}
	}
}

func TestBuildLinksTwoPerSNI(t *testing.T) {
	sni, err := config.ParseSNIList("a.com,b.com")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{WSPath: "/p", WSHost: "trycloudflare.com", FakeSNI: sni}
	got := BuildLinks(cfg, "u", "host.trycloudflare.com")
	if len(got) != 4 {
		t.Fatalf("expected 2 links per SNI entry (TLS + no-TLS), got %d for 2 entries", len(got))
	}
	if !strings.Contains(got[0], ":443") || !strings.Contains(got[0], "security=tls") {
		t.Errorf("first link per entry should be the TLS/443 variant: %s", got[0])
	}
	if !strings.Contains(got[1], ":80") {
		t.Errorf("second link per entry should be the no-TLS/80 variant: %s", got[1])
	}
}

func TestSafeQuoteEncoding(t *testing.T) {
	cases := map[string]string{
		"/tiktok4g":                  "%2Ftiktok4g",
		"Remark with spaces & stuff": "Remark%20with%20spaces%20%26%20stuff",
		"Tiktok":                     "Tiktok",
		"Free VNPT":                  "Free%20VNPT",
	}
	for in, want := range cases {
		if got := safeQuote(in); got != want {
			t.Errorf("safeQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildLinks_XHTTP(t *testing.T) {
	sni, err := config.ParseSNIList("api24-normal-alisg.tiktokv.com#Tiktok")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		WSPath:    "/tiktok4g",
		WSHost:    "trycloudflare.com",
		FakeSNI:   sni,
		Transport: "xhttp",
		XHTTPMode: "auto",
	}

	got := BuildLinks(cfg, "5ccad305-e243-4bb2-abf0-1e37189ce4e8", "my-tunnel.trycloudflare.com")
	if len(got) != 2 {
		t.Fatalf("expected 2 links, got %d", len(got))
	}

	tlsLink := got[0]
	noTLSLink := got[1]

	if !strings.Contains(tlsLink, "type=xhttp") {
		t.Errorf("tlsLink missing type=xhttp: %s", tlsLink)
	}
	if !strings.Contains(tlsLink, "&alpn=h2") {
		t.Errorf("tlsLink missing &alpn=h2: %s", tlsLink)
	}
	if !strings.Contains(tlsLink, "&mode=auto") {
		t.Errorf("tlsLink missing &mode=auto: %s", tlsLink)
	}
	if strings.Contains(tlsLink, "ed=2048") {
		t.Errorf("tlsLink should not contain ed=2048: %s", tlsLink)
	}

	if !strings.Contains(noTLSLink, "type=xhttp") {
		t.Errorf("noTLSLink missing type=xhttp: %s", noTLSLink)
	}
	if strings.Contains(noTLSLink, "alpn=") {
		t.Errorf("noTLSLink should not contain alpn: %s", noTLSLink)
	}
	if !strings.Contains(noTLSLink, "&mode=auto") {
		t.Errorf("noTLSLink missing &mode=auto: %s", noTLSLink)
	}
	if strings.Contains(noTLSLink, "ed=2048") {
		t.Errorf("noTLSLink should not contain ed=2048: %s", noTLSLink)
	}
}
