//go:build frontend

package web

import "embed"

//go:embed all:dist
var AdminFS embed.FS

const AdminDir = "dist"

const AdminEnabled = true
