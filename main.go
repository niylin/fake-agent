package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
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

type optionalStringFlag struct {
	set   bool
	value string
}

func (f *optionalStringFlag) Set(value string) error {
	f.set = true
	f.value = value
	return nil
}

func (f *optionalStringFlag) String() string {
	return f.value
}

func (f *optionalStringFlag) IsBoolFlag() bool {
	return true
}

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
	var resetPassword optionalStringFlag
	flag.Var(&resetPassword, "reset-password", "reset panel password; optionally pass a value, otherwise a random password is generated")
	flag.Parse()

	store, err := NewStore(*dataFile)
	if err != nil {
		log.Fatalf("load store: %v", err)
	}
	if resetPassword.set {
		passwordValue := resetPassword.value
		if passwordValue == "true" {
			passwordValue = ""
			if flag.NArg() > 0 {
				passwordValue = flag.Arg(0)
			}
		}
		password, err := store.ResetPanelPassword(passwordValue)
		if err != nil {
			log.Fatalf("reset password: %v", err)
		}
		fmt.Printf("panel password reset: %s\n", password)
		return
	}
	if password := store.GeneratedPanelPassword(); password != "" {
		log.Printf("generated panel password: %s", password)
		log.Printf("password hash saved in %s as panel_password_hash", *dataFile)
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
