package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html
var staticFS embed.FS

func Handler() http.Handler {
	sub, _ := fs.Sub(staticFS, ".")
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/admin" || r.URL.Path == "/admin/" {
			r.URL.Path = "/index.html"
		}
		fileServer.ServeHTTP(w, r)
	})
}
