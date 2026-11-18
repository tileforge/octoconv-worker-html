// Command html-worker runs one approved html capability over Worker Protocol generation 2.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tileforge/octoconv-worker-html/internal/convert"
	"github.com/tileforge/octoconv-worker-html/internal/sandbox"
)

const htmlSandboxDisabledEnv = "HTML_SANDBOX_DISABLED"

func main() {
	if sandbox.RunExecModeIfRequested(os.Args) {
		return
	}
	if err := refuseLegacyStores(); err != nil {
		log.Fatalf("html protocol: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sandboxHelper, err := sandbox.ResolveHelper(htmlSandboxDisabledEnv)
	if err != nil {
		log.Fatalf("html sandbox: %v", err)
	}
	concurrency, err := htmlFreeSlots()
	if err != nil {
		log.Fatalf("html protocol: %v", err)
	}
	conv, err := convert.ConfinedChromium(sandboxHelper, concurrency, "HTML_WORKER_CONCURRENCY")
	if err != nil {
		log.Fatalf("html sandbox: %v", err)
	}
	convert.Default.Register(conv)
	if len(sandboxHelper) == 0 {
		log.Printf("%s=true: the engine can read every file in this container", htmlSandboxDisabledEnv)
	} else {
		log.Printf("html sandbox: landlock via %s", strings.Join(sandboxHelper, " "))
	}

	if len(sandboxHelper) > 0 {
		if err := refuseSweptProtocolFiles(); err != nil {
			log.Fatalf("html sandbox: %v", err)
		}
	}

	preflightCtx, cancelPreflight := context.WithTimeout(ctx, 60*time.Second)
	proof, err := convert.HTMLPreflight(preflightCtx, conv)
	cancelPreflight()
	if err != nil {
		log.Fatalf("html sandbox preflight: %v", err)
	}
	if len(sandboxHelper) > 0 {
		if proof.OutsideBytes > 0 {
			log.Printf("html sandbox proven: the engine rendered inside the job directory and was denied a file outside every grant (inside pdf %d bytes, outside pdf %d bytes, marker absent)", proof.InsideBytes, proof.OutsideBytes)
		} else {
			log.Printf("html sandbox proven: the engine rendered inside the job directory and was denied a file outside every grant (inside pdf %d bytes, chromium net::ERR_ACCESS_DENIED, no outside pdf)", proof.InsideBytes)
		}
	}

	dial, err := loadProtocolDial()
	if err != nil {
		log.Fatalf("html protocol: %v", err)
	}
	log.Printf("html protocol worker starting")
	if err := runProtocol(ctx, dial); err != nil {
		log.Fatalf("html protocol: %v", err)
	}
}

func refuseSweptProtocolFiles() error {
	files := []struct {
		name string
		path string
	}{
		{envProtocolCert, os.Getenv(envProtocolCert)},
		{envProtocolKey, os.Getenv(envProtocolKey)},
		{envProtocolCA, os.Getenv(envProtocolCA)},
		{envProtocolManifest, os.Getenv(envProtocolManifest)},
	}
	for _, file := range files {
		if err := convert.RefuseSharedTmpPath(file.name, file.path); err != nil {
			return err
		}
	}
	return nil
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
