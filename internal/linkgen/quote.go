package linkgen

import "strings"

// safeQuote percent-encodes s, leaving only a fixed "always safe" set —
// ASCII letters, digits, and "_.-~" — unescaped; everything else (including
// space and '&') is percent-encoded. Neither Go stdlib escaper matches this
// exactly: url.QueryEscape encodes space as '+' instead of %20, and
// url.PathEscape leaves '&' (and a few other RFC 3986 sub-delims)
// unescaped. This is the one documented URL-encoding divergence risk in
// Phase 4's plan; safeQuote closes it by reimplementing the exact
// byte-safe set rather than reaching for either Go escaper.
func safeQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isPyUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexDigit(c >> 4))
		b.WriteByte(hexDigit(c & 0x0f))
	}
	return b.String()
}

func isPyUnreserved(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '_' || c == '.' || c == '-' || c == '~':
		return true
	}
	return false
}

func hexDigit(n byte) byte {
	const hex = "0123456789ABCDEF"
	return hex[n]
}
