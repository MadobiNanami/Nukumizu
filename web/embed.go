package web

import (
	"embed"
	"io/fs"
)

// distFS holds the built console. Vite writes it to web/dist (see the outDir
// in frontend/vite.config.js) and it is compiled into the binary here, so a
// running executable serves the whole frontend on its own: neither the
// frontend sources nor web/dist need to exist on the machine that runs it.
//
// The all: prefix also picks up files whose names start with "_" or ".", which
// the default pattern skips.
//
//go:embed all:dist
var distFS embed.FS

// StaticFiles is the built frontend, rooted at the directory Vite writes to,
// so paths inside it are relative to that directory, e.g. "index.html" or
// "assets/app.js".
//
// web/dist is a build artifact and is not in a fresh checkout, so the console
// has to be built before the backend compiles — any build-*.sh / build-*.bat
// does it first, or run `npm run build` in frontend/ yourself. Compiling
// without it fails with "pattern all:dist: no matching files found".
var StaticFiles fs.FS

func init() {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("web: embedded frontend is unreadable: " + err.Error())
	}
	StaticFiles = sub
}
