package ui

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/mux"
)

type StandaloneConfig struct {
	NodeURL   *url.URL
	StaticDir string
}

func NewStandaloneHandler(cfg StandaloneConfig) http.Handler {
	rtr := mux.NewRouter()
	uiPath := "/thornado"
	uiAssetsPath := "/thornado/ui/"
	static := Static()
	index := HandleIndex
	if cfg.StaticDir != "" {
		static = http.FileServer(http.Dir(cfg.StaticDir))
		index = func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("content-type", "text/html; charset=utf-8")
			http.ServeFile(w, r, cfg.StaticDir+"/index.html")
		}
	}

	rtr.HandleFunc(uiPath, index).Methods(http.MethodGet, http.MethodHead, http.MethodOptions)
	rtr.HandleFunc(uiPath+"/", index).Methods(http.MethodGet, http.MethodHead, http.MethodOptions)
	rtr.PathPrefix(uiAssetsPath).Handler(http.StripPrefix(uiAssetsPath, static)).Methods(http.MethodGet, http.MethodHead, http.MethodOptions)

	if cfg.NodeURL != nil {
		rtr.PathPrefix("/").Handler(newNodeProxy(cfg.NodeURL))
	}
	return rtr
}

func newNodeProxy(target *url.URL) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(target)
	baseDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		host := r.Host
		baseDirector(r)
		r.Host = target.Host
		if host != "" {
			prior := r.Header.Get("X-Forwarded-Host")
			if prior != "" {
				host = strings.Join([]string{prior, host}, ", ")
			}
			r.Header.Set("X-Forwarded-Host", host)
		}
	}
	return proxy
}

func StaticDirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
