package linkgen

import "strings"

// pyQuote percent-encodes exactly like Python's urllib.parse.quote(s, safe="")
// - the function main.py actually uses (main.py:344-345, 367). Neither Go
// stdlib escaper matches it exactly: url.QueryEscape encodes space as '+'
// (Python encodes it as %20), and url.PathEscape leaves '&' (and a few
// other RFC 3986 sub-delims) unescaped (Python percent-encodes them too,
// since safe="" means only its fixed "always safe" set - ASCII letters,
// digits, and "_.-~" - is left alone). This is the one documented
// divergence risk in Phase 4's plan; pyQuote closes it by reimplementing
// Python's exact byte-safe set rather than reaching for either Go escaper.
func pyQuote(s string) string {
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
