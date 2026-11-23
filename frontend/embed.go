package frontend

import "embed"

// DistFS embeds the built Vite SPA assets from frontend/dist.
//
//go:embed all:dist
var DistFS embed.FS
