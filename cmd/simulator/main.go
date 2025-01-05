package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tb0hdan/simulator/pkg/server"
)

const (
	ListenAddr      = "localhost:8080"
	GracePeriod     = 3 * time.Second
	ShutdownTimeout = 5 * time.Second
)

func main() {
	var (
		listenAddr  = flag.String("listen", ListenAddr, "server listen address")
		gracePeriod = flag.Duration("grace-period", GracePeriod, "grace period for server shutdown")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(log.New(os.Stdout, "", log.LstdFlags))
	go func() {
		if err := srv.Start(*listenAddr, *gracePeriod); !errors.Is(err, server.ErrServerClosed) {
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
		log.Printf("error shutting down server: %v", err)
	}
}
