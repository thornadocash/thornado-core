package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thornadocash/go-thornado/ui"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:1316", "address for the UI server")
	node := flag.String("node", "http://127.0.0.1:1317", "Thornado node API origin to proxy")
	staticDir := flag.String("static-dir", "", "optional UI static directory to serve instead of embedded assets")
	flag.Parse()

	nodeURL, err := url.Parse(*node)
	if err != nil || nodeURL.Scheme == "" || nodeURL.Host == "" {
		log.Fatalf("invalid --node URL %q", *node)
	}
	if *staticDir != "" && !ui.StaticDirExists(*staticDir) {
		log.Fatalf("invalid --static-dir %q", *staticDir)
	}

	server := &http.Server{
		Addr:              *listen,
		Handler:           ui.NewStandaloneHandler(ui.StandaloneConfig{NodeURL: nodeURL, StaticDir: *staticDir}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if *staticDir != "" {
			log.Printf("serving Thornado UI on http://%s/thornado/ from %s proxying %s", *listen, *staticDir, nodeURL.String())
		} else {
			log.Printf("serving Thornado UI on http://%s/thornado/ proxying %s", *listen, nodeURL.String())
		}
		errCh <- server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		log.Printf("shutting down after %s", sig)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}
