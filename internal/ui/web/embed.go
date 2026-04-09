package web

import "embed"

//go:embed static/index.html static/htmx.min.js static/htmx-sse.js
var staticFS embed.FS
