package main

import (
	"context"
	"embed"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed static/index.html
var staticFiles embed.FS

var indexHTML []byte

func init() {
	var err error
	indexHTML, err = staticFiles.ReadFile("static/index.html")
	if err != nil {
		panic(err)
	}
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8099", "HTTP management listen address")
	dataFile := flag.String("data", "data/agents.json", "JSON database path")
	flag.Parse()

	store, err := NewStore(*dataFile)
	if err != nil {
		log.Fatalf("load store: %v", err)
	}
	manager := NewManager(store)
	manager.StartEnabled()
	defer manager.StopAll()

	server := &http.Server{
		Addr:              *listen,
		Handler:           NewApp(manager).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("fake-agent %s listening on http://%s", appVersion, *listen)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
