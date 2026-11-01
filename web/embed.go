// Package web embeds the built user interface (web/dist).
package web

import "embed"

// Dist contains the files produced by "npm run build".
//
//go:embed all:dist
var Dist embed.FS
