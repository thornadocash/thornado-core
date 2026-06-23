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
			setDevNoStore(w)
			w.Header().Set("content-type", "text/html; charset=utf-8")
			http.ServeFile(w, r, cfg.StaticDir+"/index.html")
		}
	}

	rtr.HandleFunc(uiPath, index).Methods(http.MethodGet, http.MethodHead, http.MethodOptions)
	rtr.HandleFunc(uiPath+"/", index).Methods(http.MethodGet, http.MethodHead, http.MethodOptions)
	if cfg.StaticDir != "" {
		rtr.HandleFunc(uiAssetsPath+"prover/withdraw/prove", handleDevWithdrawProof(cfg.StaticDir)).Methods(http.MethodPost, http.MethodOptions)
	}
	rtr.PathPrefix(uiAssetsPath).Handler(http.StripPrefix(uiAssetsPath, noStoreHandler(static))).Methods(http.MethodGet, http.MethodHead, http.MethodOptions)

	if cfg.NodeURL != nil {
		rtr.PathPrefix("/").Handler(newNodeProxy(cfg.NodeURL))
	}
	return rtr
}

func setDevNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func noStoreHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setDevNoStore(w)
		next.ServeHTTP(w, r)
	})
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
		repoRoot := findRepoRootForWithdrawProof(staticDir)
		script := filepath.Join(repoRoot, "circuits", "tornado", "scripts", "prove-withdraw.mjs")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		nodeBinary, err := findNodeBinary()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		cmd := exec.CommandContext(ctx, nodeBinary, "--stack_size=65500", script)
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

func findNodeBinary() (string, error) {
	if override := strings.TrimSpace(os.Getenv("NODE_BINARY")); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", err
		} else {
			return override, nil
		}
	}
	if node, err := exec.LookPath("node"); err == nil {
		return node, nil
	}
	for _, candidate := range []string{"/opt/homebrew/bin/node", "/usr/local/bin/node", "/usr/bin/node"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func findRepoRootForWithdrawProof(staticDir string) string {
	dir := filepath.Clean(staticDir)
	for {
		script := filepath.Join(dir, "circuits", "tornado", "scripts", "prove-withdraw.mjs")
		if _, err := os.Stat(script); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Clean(filepath.Join(staticDir, "..", "..", ".."))
		}
		dir = parent
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
