package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tb0hdan/simulator/pkg/server"
)

const (
	ShutdownTimeout = 5 * time.Second
)

func main() {
	var (
		listenAddr = flag.String("listen", "localhost:8080", "server listen address")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New()
	go func() {
		if err := srv.Start(*listenAddr); err != nil {
			log.Println("Error starting server:", err)
			stop()
		}
	}()

	log.Println("Server started on", *listenAddr)
	<-ctx.Done()
	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("error shutting down server: %v", err)
	}
}
