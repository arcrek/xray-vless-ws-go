// Package weblogserver embeds the log viewer's static HTML/CSS/JS assets
// (extracted from logging_site.py's inline LOGGING_HTML_TEMPLATE string) so
// internal/logserver can serve them via go:embed instead of a Go string
// constant, keeping the template editable without touching Go code.
package weblogserver

import "embed"

//go:embed static
var FS embed.FS
