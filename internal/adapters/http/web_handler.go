package http

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed web/*
var webFS embed.FS

// RegisterWebRoutes registra as rotas estáticas do painel web SPA embutido no roteador Chi.
func RegisterWebRoutes(r chi.Router) {
	assetsFS, err := fs.Sub(webFS, "web")
	if err == nil {
		fileServer := http.FileServer(http.FS(assetsFS))
		r.Handle("/assets/*", http.StripPrefix("/assets/", fileServer))
	}

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		indexContent, err := webFS.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "Dashboard indisponivel", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexContent)
	})

	r.Get("/ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	r.Get("/ui/*", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
}
