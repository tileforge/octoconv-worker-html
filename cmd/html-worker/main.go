// Command html-worker runs one approved html capability over Worker Protocol generation 2.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if err := refuseLegacyStores(); err != nil {
		log.Fatalf("html protocol: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	dial, err := loadProtocolDial()
	if err != nil {
		log.Fatalf("html protocol: %v", err)
	}
	log.Printf("html protocol worker starting")
	if err := runProtocol(ctx, dial); err != nil {
		log.Fatalf("html protocol: %v", err)
	}
}

func refuseLegacyStores() error {
	if os.Getenv("DATABASE_URL") != "" || os.Getenv("REDIS_ADDR") != "" {
		return errors.New("DATABASE_URL and REDIS_ADDR are not used")
	}
	return nil
}

func htmlFreeSlots() (int, error) {
	raw := os.Getenv("HTML_WORKER_CONCURRENCY")
	if raw == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(firstField(raw))
	if err != nil || n != 1 {
		return 0, errors.New("HTML_WORKER_CONCURRENCY must be 1")
	}
	return 1, nil
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(firstField(v)); err == nil {
			return d
		}
	}
	return def
}

func firstField(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			return s[:i]
		}
	}
	return s
}
