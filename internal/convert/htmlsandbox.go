package convert

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Chromium puts its shared memory in /tmp regardless of TMPDIR.
var htmlReadWrite = []string{"/tmp", "/dev/null"}

// /usr/bin/chromium-headless-shell is a 61-byte "#!/bin/sh" wrapper. Looking
// it up grants the wrapper, not the interpreter, so /usr/bin/dash is named
// literally. The real binary is under /usr/lib.
var htmlReadOnly = []string{
	"/usr/bin/dash",
	"/lib",
	"/usr/lib",
	"/usr/share",
	"/usr/local/share",
	"/etc/fonts",
	"/etc/ld.so.cache",
	"/etc/localtime",
	"/etc/nsswitch.conf",
	"/etc/host.conf",
	"/etc/hosts",
	"/etc/resolv.conf",
	"/proc",
	"/sys/devices",
	"/dev/urandom",
}

var htmlEngines = []string{"chromium-headless-shell"}

// /tmp is itself a grant, so the outside fixture cannot be created there.
const htmlPreflightOutsideBase = "/var/tmp"

// ConfinedChromium returns a converter that runs chromium under helper.
// helper empty leaves the zero value: no ruleset and no /tmp sweep.
func ConfinedChromium(helper []string, concurrency int, concurrencyEnv string) (ChromiumConverter, error) {
	c := ChromiumConverter{sandbox: newEngineSandbox(helper)}
	if err := sharedTmpGuard(htmlReadWrite, c.sandbox, concurrency, concurrencyEnv); err != nil {
		return ChromiumConverter{}, err
	}
	if c.sandbox.confines() && slices.Contains(htmlReadWrite, sharedTmpDir) {
		c.sharedTmp = sharedTmpDir
	}
	return c, nil
}

// os.RemoveAll on a symlink removes the symlink, not its target.
func sweepSharedTmp(dir, keep string) error {
	cleanDir := filepath.Clean(dir)
	cleanKeep := filepath.Clean(keep)
	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return fmt.Errorf("%w: sweep %s before the task: %w", ErrEngineSandboxUnavailable, cleanDir, err)
	}
	for _, e := range entries {
		p := filepath.Join(cleanDir, e.Name())
		if p == cleanKeep || strings.HasPrefix(cleanKeep, p+string(filepath.Separator)) {
			continue
		}
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("%w: sweep %s before the task: %w", ErrEngineSandboxUnavailable, cleanDir, err)
		}
	}
	return nil
}

// PreflightProof is what HTMLPreflight observed. OutsideBytes is 0 when
// Chromium writes no file.
type PreflightProof struct {
	InsideBytes  int
	OutsideBytes int
}

// HTMLPreflight proves the ruleset both ways. The inside render must carry
// the marker. The outside file must not: either as a PDF that lacks the
// marker, or, on Chromium 156, as exit 1 with net::ERR_ACCESS_DENIED and no
// PDF. A missing file after a helper setup failure or a missing binary is
// not a denial. An unconfined converter returns a zero proof.
func HTMLPreflight(ctx context.Context, c ChromiumConverter) (PreflightProof, error) {
	if !c.sandbox.confines() {
		return PreflightProof{}, nil
	}
	if slices.Contains(htmlReadWrite, sharedTmpDir) && c.sharedTmp == "" {
		return PreflightProof{}, fmt.Errorf("a confined html converter must sweep /tmp before every task")
	}

	w, cleanup, err := newPreflightWorkspaceOutside("html", htmlPreflightOutsideBase)
	if err != nil {
		return PreflightProof{}, err
	}
	defer cleanup()

	if err := w.requireOutsideGrants(htmlReadWrite, htmlReadOnly); err != nil {
		return PreflightProof{}, err
	}
	marker, err := preflightMarker()
	if err != nil {
		return PreflightProof{}, err
	}
	html := []byte("<html><head><title>" + marker + "</title></head><body><p>" + marker + "</p></body></html>")
	inside, outside, err := w.place("in.html", html)
	if err != nil {
		return PreflightProof{}, err
	}

	outInside := filepath.Join(w.jobDir, "out.pdf")
	if err := c.Convert(ctx, inside, outInside, nil); err != nil {
		return PreflightProof{}, fmt.Errorf("the engine cannot convert under the sandbox: %w", err)
	}
	if err := requireOutput(outInside, []byte("/Title ("+marker+")")); err != nil {
		return PreflightProof{}, fmt.Errorf("the engine cannot convert under the sandbox: %w", err)
	}
	insideInfo, err := os.Stat(outInside)
	if err != nil {
		return PreflightProof{}, fmt.Errorf("the engine cannot convert under the sandbox: %w", err)
	}

	// Convert copies its input into the job directory first, so the negative
	// run points chromium itself at the outside file with the same argv and
	// the same ruleset.
	outOutside := filepath.Join(w.jobDir, "neg.pdf")
	convErr := c.exec(w.jobDir).run(ctx, "chromium-headless-shell",
		chromiumPrintArgs(w.jobDir, outOutside, "file://"+outside)...)
	n, err := requireDeniedPDF(outOutside, []byte(marker))
	if err == nil {
		return PreflightProof{InsideBytes: int(insideInfo.Size()), OutsideBytes: n}, nil
	}
	if errors.Is(err, errDeniedPDFMissing) && outsideAccessDenied(convErr, marker) {
		return PreflightProof{InsideBytes: int(insideInfo.Size())}, nil
	}
	if convErr != nil {
		return PreflightProof{}, fmt.Errorf("%w; engine: %v", err, convErr)
	}
	return PreflightProof{}, err
}

// Chromium 156 exits 1 with this string and writes no PDF when Landlock
// denies the page. A dbus "no such file" line in the same log is not a denial.
func outsideAccessDenied(convErr error, marker string) bool {
	if convErr == nil || errors.Is(convErr, ErrEngineSandboxUnavailable) {
		return false
	}
	msg := convErr.Error()
	if strings.Contains(msg, marker) || strings.Contains(msg, "locate engine ") {
		return false
	}
	return strings.Contains(msg, "net::ERR_ACCESS_DENIED")
}

func (c ChromiumConverter) exec(workDir string) engineExec {
	return c.sandbox.forJobRW(workDir, htmlReadWrite, htmlReadOnly, htmlEngines).withEnv(engineEnv(""))
}
