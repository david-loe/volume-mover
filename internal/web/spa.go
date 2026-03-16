package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed spa/dist/* spa/dist/assets/*
var spaFS embed.FS

func spaAssets() http.Handler {
	assets, err := fs.Sub(spaFS, "spa/dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/", http.FileServer(http.FS(assets)))
}

func serveSPAIndex(w http.ResponseWriter, r *http.Request) {
	data, err := spaFS.ReadFile("spa/dist/index.html")
	if err != nil {
		http.Error(w, "spa assets not built", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
