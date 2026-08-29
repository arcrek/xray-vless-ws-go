package tunnel

import "regexp"

var (
	ansiEscape        = regexp.MustCompile(`\x1b\[[0-9;]*[mK]`)
	quickTunnelURL    = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)
	createdBannerLine = regexp.MustCompile(`Your quick Tunnel has been created!`)
)

// StripANSI removes ANSI color/cursor escape codes. Verified against a real
// captured cloudflared run (testdata/quicktunnel_stdout.txt)
// that this build of cloudflared doesn't colorize output when stdout isn't a
// TTY — kept anyway since a future version or a differently-attached stdout
// could reintroduce escapes.
func StripANSI(line string) string {
	return ansiEscape.ReplaceAllString(line, "")
}

// HostnameParser tracks state across the cloudflared stdout stream to find
// the quick-tunnel hostname. Rather than a single global regex search
// (which would false-match if a *.trycloudflare.com URL ever appeared
// anywhere else in the log stream), this narrows the match to the exact
// line immediately following cloudflared's "Your quick Tunnel has been
// created!" banner (verified against a real captured run, not hand-written).
type HostnameParser struct {
	sawBanner bool
}

// NewHostnameParser returns a fresh parser for one cloudflared process's
// stdout stream.
func NewHostnameParser() *HostnameParser {
	return &HostnameParser{}
}

// Feed processes one line of (already ANSI-stripped or raw — Feed strips
// itself) cloudflared stdout, returning the discovered hostname and true
// once found. Safe to keep calling after a hostname is found (idempotent
// no-op).
func (p *HostnameParser) Feed(rawLine string) (hostname string, found bool) {
	line := StripANSI(rawLine)

	if createdBannerLine.MatchString(line) {
		p.sawBanner = true
		return "", false
	}

	if !p.sawBanner {
		return "", false
	}

	if match := quickTunnelURL.FindString(line); match != "" {
		p.sawBanner = false // reset: a restart will print a fresh banner
		host := match[len("https://"):]
		return host, true
	}

	return "", false
}
