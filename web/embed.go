//go:build !frontend

package web

import "embed"

// AdminFS embeds a tiny fallback UI for normal development commands.
//
// Production builds use embed_frontend.go via the "frontend" build tag after
// Vite has generated web/dist.
//
//go:embed all:static
var AdminFS embed.FS

const AdminDir = "static"

const AdminEnabled = false
