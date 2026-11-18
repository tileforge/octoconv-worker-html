package convert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTMLPreflightUnconfined(t *testing.T) {
	proof, err := HTMLPreflight(context.Background(), ChromiumConverter{})
	if err != nil || proof != (PreflightProof{}) {
		t.Fatalf("unconfined preflight = %+v, %v", proof, err)
	}
}

func TestConfinedChromiumRejectsSharedTmpConcurrency(t *testing.T) {
	helper := []string{"/usr/local/bin/html-worker", "__sandbox-exec"}
	_, err := ConfinedChromium(helper, 2, "HTML_WORKER_CONCURRENCY")
	if err == nil || !strings.Contains(err.Error(), "HTML_WORKER_CONCURRENCY=2") || !strings.Contains(err.Error(), "/tmp") {
		t.Fatalf("concurrency 2: %v", err)
	}
	c, err := ConfinedChromium(helper, 1, "HTML_WORKER_CONCURRENCY")
	if err != nil {
		t.Fatal(err)
	}
	if c.sharedTmp != "/tmp" || !c.sandbox.confines() {
		t.Fatalf("sharedTmp %q confines %v", c.sharedTmp, c.sandbox.confines())
	}
	if _, err := ConfinedChromium(nil, 4, "HTML_WORKER_CONCURRENCY"); err != nil {
		t.Fatalf("unconfined: %v", err)
	}
}

func TestHTMLPreflightRefusesConfinedWithoutSweep(t *testing.T) {
	c := ChromiumConverter{sandbox: newEngineSandbox([]string{"/bin/html-worker", "__sandbox-exec"})}
	if _, err := HTMLPreflight(context.Background(), c); err == nil || !strings.Contains(err.Error(), "/tmp") {
		t.Fatal(err)
	}
}

func TestOutsideAccessDenied(t *testing.T) {
	marker := "octoconv-preflight-abc"
	access := errors.New("headless_command_handler.cc:403] Page load failed: net::ERR_ACCESS_DENIED")
	if !outsideAccessDenied(access, marker) {
		t.Fatal("access denied rejected")
	}
	noisy := errors.New("Failed to connect to socket /run/dbus/system_bus_socket: No such file or directory\n" + access.Error())
	if !outsideAccessDenied(noisy, marker) {
		t.Fatal("access denied with dbus noise rejected")
	}
	if outsideAccessDenied(nil, marker) {
		t.Fatal("nil error accepted")
	}
	if outsideAccessDenied(fmt.Errorf("%w: exit status 78", ErrEngineSandboxUnavailable), marker) {
		t.Fatal("setup failure accepted")
	}
	if outsideAccessDenied(errors.New("locate engine chromium-headless-shell: exec: not found"), marker) {
		t.Fatal("missing engine accepted")
	}
	if outsideAccessDenied(errors.New("net::ERR_ACCESS_DENIED "+marker), marker) {
		t.Fatal("marker in the engine output accepted")
	}
	if outsideAccessDenied(errors.New("No such file or directory"), marker) {
		t.Fatal("missing file without access denied accepted")
	}
}

func TestRequireDeniedPDF(t *testing.T) {
	dir := t.TempDir()
	if _, err := requireDeniedPDF(filepath.Join(dir, "missing.pdf"), []byte("m")); err == nil {
		t.Fatal("missing pdf accepted")
	}
	leaked := filepath.Join(dir, "leaked.pdf")
	if err := os.WriteFile(leaked, []byte("%PDF-1.4\n/Title (octoconv-preflight-abc)"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := requireDeniedPDF(leaked, []byte("octoconv-preflight-abc")); err == nil {
		t.Fatal("marker accepted")
	}
	blank := filepath.Join(dir, "blank.pdf")
	body := []byte("%PDF-1.4\nblank")
	if err := os.WriteFile(blank, body, 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := requireDeniedPDF(blank, []byte("octoconv-preflight-abc"))
	if err != nil || n != len(body) {
		t.Fatalf("blank = %d, %v", n, err)
	}
}

func TestWrapEngineUsesAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "chromium-headless-shell")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	s := &engineSandbox{Helper: []string{"/usr/local/bin/html-worker", "__sandbox-exec"}, Confine: true}
	name, args, err := s.wrapEngine("chromium-headless-shell", []string{"rw:/job"}, []string{"--headless"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "/usr/local/bin/html-worker" {
		t.Fatal(name)
	}
	want := []string{"__sandbox-exec", "rw:/job", "--", bin, "--headless"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args %q", args)
	}
}

func TestForJobRWGrantsMeasuredPaths(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "chromium-headless-shell")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	s := newEngineSandbox([]string{"/usr/local/bin/html-worker", "__sandbox-exec"})
	spec := strings.Join(s.forJobRW("/job", htmlReadWrite, htmlReadOnly, htmlEngines).spec, "\n")
	for _, want := range []string{"rw:/job", "rw:/tmp", "rw:/dev/null", "ro:/usr/bin/dash", "ro:" + bin} {
		if !strings.Contains(spec, want) {
			t.Errorf("missing %s in %s", want, spec)
		}
	}
	var bare *engineSandbox
	if got := bare.forJobRW("/job", htmlReadWrite, htmlReadOnly, htmlEngines).spec; len(got) != 0 {
		t.Fatalf("unconfined spec %q", got)
	}
}
