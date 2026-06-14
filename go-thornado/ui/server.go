package ui

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	if cfg.StaticDir != "" {
		rtr.HandleFunc(uiAssetsPath+"prover/withdraw/prove", handleDevWithdrawProof(cfg.StaticDir)).Methods(http.MethodPost, http.MethodOptions)
	}
	rtr.PathPrefix(uiAssetsPath).Handler(http.StripPrefix(uiAssetsPath, static)).Methods(http.MethodGet, http.MethodHead, http.MethodOptions)

	if cfg.NodeURL != nil {
		rtr.PathPrefix("/").Handler(newNodeProxy(cfg.NodeURL))
	}
	return rtr
}

func handleDevWithdrawProof(staticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body := http.MaxBytesReader(w, r.Body, 2<<20)
		defer body.Close()
		input := new(bytes.Buffer)
		if _, err := input.ReadFrom(body); err != nil {
			http.Error(w, "invalid witness payload", http.StatusBadRequest)
			return
		}
		repoRoot := filepath.Clean(filepath.Join(staticDir, "..", "..", ".."))
		script := filepath.Join(repoRoot, "circuits", "tornado", "scripts", "prove-withdraw.mjs")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "node", "--stack_size=65500", script)
		cmd.Dir = repoRoot
		cmd.Stdin = bytes.NewReader(input.Bytes())
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = err.Error()
			}
			http.Error(w, message, http.StatusBadGateway)
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write(out)
	}
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
