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

func TestSweepRemovesSiblingsUnderAncestor(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "jobs", "current")
	if err := os.MkdirAll(keep, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keep, "in.html"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, "jobs", "old")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "secret"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	junk := filepath.Join(root, "junk")
	if err := os.WriteFile(junk, []byte("j"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sweepSharedTmp(root, keep); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(keep, "in.html")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old job survived: %v", err)
	}
	if _, err := os.Stat(junk); !os.IsNotExist(err) {
		t.Fatalf("junk survived: %v", err)
	}
}

func TestSweepKeepsJobThroughTmpAlias(t *testing.T) {
	root := t.TempDir()
	jobs := filepath.Join(root, "jobs")
	if err := os.MkdirAll(jobs, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "html-tmp")
	if err := os.Symlink(jobs, alias); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(alias, "current")
	if err := os.MkdirAll(keep, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keep, "in.html"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(jobs, "old")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "secret"), []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sweepSharedTmp(root, keep); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(keep, "in.html")); err != nil {
		t.Fatalf("current job: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old job survived: %v", err)
	}
}

func TestSweepRemovesSymlinkNotItsTarget(t *testing.T) {
	root := t.TempDir()
	keep := filepath.Join(root, "current")
	if err := os.MkdirAll(keep, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(target, []byte("s"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := sweepSharedTmp(root, keep); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("symlink survived: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target: %v", err)
	}
}

func TestPathUnderFollowsSymlinks(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "a", "key")
	if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	under, err := pathUnder(inside, root)
	if err != nil || !under {
		t.Fatalf("inside = %v, %v", under, err)
	}
	missing := filepath.Join(root, "missing.key")
	under, err = pathUnder(missing, root)
	if err != nil || !under {
		t.Fatalf("missing = %v, %v", under, err)
	}
	other := t.TempDir()
	link := filepath.Join(other, "link")
	if err := os.Symlink(inside, link); err != nil {
		t.Fatal(err)
	}
	under, err = pathUnder(link, root)
	if err != nil || !under {
		t.Fatalf("symlink = %v, %v", under, err)
	}
	under, err = pathUnder(filepath.Join(other, "plain.key"), root)
	if err != nil || under {
		t.Fatalf("outside = %v, %v", under, err)
	}
}

func TestRefuseSharedTmpPath(t *testing.T) {
	if err := RefuseSharedTmpPath("HTML_PROTOCOL_CLIENT_KEY", ""); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(sharedTmpDir, "html-sweep-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	key := filepath.Join(dir, "client.key")
	if err := os.WriteFile(key, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RefuseSharedTmpPath("HTML_PROTOCOL_CLIENT_KEY", key); err == nil || !strings.Contains(err.Error(), sharedTmpDir) {
		t.Fatalf("key under shared tmp: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "client.key")
	if under, err := pathUnder(outside, sharedTmpDir); err != nil {
		t.Fatal(err)
	} else if under {
		t.Skip("temp dir is under the shared tmp")
	}
	if err := RefuseSharedTmpPath("HTML_PROTOCOL_CLIENT_KEY", outside); err != nil {
		t.Fatal(err)
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
