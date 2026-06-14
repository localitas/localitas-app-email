package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/localitas/localitas-app-email"
	"github.com/localitas/localitas-go"
)

var (
	version = "dev"
	commit  = "unknown"
)

func envOrFileToken() string {
	if t := os.Getenv("LOCALITAS_API_TOKEN"); t != "" {
		return t
	}
	return client.DefaultToken()
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("email-server %s (commit: %s)\n", version, commit)
		os.Exit(0)
	}

	var (
		listen   = flag.String("listen", ":0", "listen address")
		coreURL  = flag.String("core-url", client.DefaultCoreURL(), "base URL of the Localitas core API")
		basePath = flag.String("base-path", "/", "URL prefix for <base href>")
		token    = flag.String("token", envOrFileToken(), "bearer token")
	)
	flag.Parse()

	ctx := context.Background()
	c := client.New(*coreURL)
	if *token != "" {
		c = c.WithToken(*token)
	}

	app := email.New(c, *basePath)
	dbID, err := app.Install(ctx)
	if err != nil {
		log.Fatalf("install: %v", err)
	}
	log.Printf("Email database ready: %s", dbID)

	if err := app.InitStore(*coreURL, dbID, *token); err != nil {
		log.Fatalf("init store: %v", err)
	}
	defer app.Store.Close()
	app.CoreURL = *coreURL
	app.AuthToken = *token

	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	mux.HandleFunc("GET /health.json", email.HandleHealth)

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	fmt.Printf("email-server listening on http://localhost:%d\n", addr.Port)

	selfURL := fmt.Sprintf("http://localhost:%d", addr.Port)
	if err := c.RegisterService(ctx, "email", selfURL); err != nil {
		log.Printf("⚠️  service registry failed: %v", err)
	}

	go email.RegisterSyncAutomation(*coreURL, *token, selfURL)

	shutdown, err := email.BroadcastMDNS(addr.Port, email.DefaultHealth.Name)
	if err != nil {
		log.Printf("⚠️  mDNS broadcast failed: %v", err)
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		if shutdown != nil {
			shutdown()
		}
		os.Exit(0)
	}()

	if err := http.Serve(ln, mux); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
