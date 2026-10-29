package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"listingboard/internal/store"
	"listingboard/internal/worker"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pause, err := stepPause(os.Getenv("STEP_PAUSE"))
	if err != nil {
		log.Fatal(err)
	}
	dataDir := env("DATA_DIR", "data")
	st, err := waitStore(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("worker started, step pause %s", pause)
	worker.New(st, dataDir, pause).Run(ctx)
	log.Print("worker stopped")
}

// stepPause reads STEP_PAUSE as a Go duration. Empty means 2s, 0 is allowed.
func stepPause(value string) (time.Duration, error) {
	if value == "" {
		return 2 * time.Second, nil
	}
	pause, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("STEP_PAUSE must be a Go duration such as 2s: %w", err)
	}
	if pause < 0 {
		return 0, fmt.Errorf("STEP_PAUSE must not be negative, got %s", value)
	}
	return pause, nil
}

func waitStore(dsn string) (*store.Store, error) {
	deadline := time.Now().Add(60 * time.Second)
	var last error
	for {
		st, err := store.Open(dsn)
		if err == nil {
			return st, nil
		}
		last = err
		if time.Now().After(deadline) {
			return nil, last
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
