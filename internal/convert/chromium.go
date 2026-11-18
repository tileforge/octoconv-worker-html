package convert

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func init() {
	Default.Register(ChromiumConverter{})
}

// ChromiumConverter shells out to chromium-headless-shell for html to pdf.
// A zero value runs the binary directly. It does not apply Landlock.
type ChromiumConverter struct{}

// Pairs returns the single html to pdf pair.
func (ChromiumConverter) Pairs() []Pair {
	return []Pair{{From: "html", To: "pdf"}}
}

const cspNoScriptMeta = `<meta http-equiv="Content-Security-Policy" content="script-src 'none'; object-src 'none'; frame-src 'none'; child-src 'none'">`

// Convert renders inPath to outPath. opts are the assignment parameters.
func (ChromiumConverter) Convert(ctx context.Context, inPath, outPath string, opts map[string]any) error {
	parsed, err := htmlOptsFromMap(opts)
	if err != nil {
		return fmt.Errorf("chromium: %w", err)
	}
	input, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("chromium: read input: %w", err)
	}
	workDir := filepath.Dir(outPath)
	rendered := injectPrintCSS(injectCSPFirst(input, cspNoScriptMeta), buildPrintCSS(parsed))
	renderedPath := filepath.Join(workDir, "rendered.html")
	if err := os.WriteFile(renderedPath, rendered, 0o600); err != nil {
		return fmt.Errorf("chromium: write rendered html: %w", err)
	}
	args := chromiumPrintArgs(workDir, outPath, "file://"+renderedPath)
	if err := runCommand(ctx, engineEnv(""), "chromium-headless-shell", args...); err != nil {
		return fmt.Errorf("chromium: %w", err)
	}
	if err := validatePDF(outPath); err != nil {
		return fmt.Errorf("chromium: %w", err)
	}
	return nil
}

// chromium-headless-shell under nobody exits with "No usable sandbox!" unless
// --no-sandbox is set. That flag is the browser's own sandbox switch.
func chromiumPrintArgs(workDir, outPath, sourceURL string) []string {
	return []string{
		"--headless",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--no-pdf-header-footer",
		"--user-data-dir=" + filepath.Join(workDir, "chrome-profile"),
		"--proxy-server=127.0.0.1:9",
		"--proxy-bypass-list=<-loopback>",
		"--host-resolver-rules=MAP * ~NOTFOUND",
		"--print-to-pdf=" + outPath,
		sourceURL,
	}
}

var pdfMagic = []byte("%PDF-")

func validatePDF(path string) error {
	info, err := statOutput(path)
	if err != nil {
		return fmt.Errorf("stat output: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("output is empty")
	}
	header, err := readHeader(path, len(pdfMagic))
	if err != nil || !bytes.Equal(header, pdfMagic) {
		return fmt.Errorf("output missing %%PDF- magic bytes")
	}
	return nil
}

var (
	headCloseTag     = []byte("</head>")
	htmlOpenPrefix   = []byte("<html")
	cspDoctypePrefix = []byte("<!doctype")
)

// bytes.ToLower follows Unicode case mapping. U+0130 (İ) shrinks from two
// UTF-8 bytes to one, so an index into that copy misses the original.
func asciiLower(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}

func injectPrintCSS(html []byte, css string) []byte {
	lower := asciiLower(html)
	if idx := bytes.Index(lower, headCloseTag); idx >= 0 {
		return spliceAt(html, idx, css)
	}
	if idx := bytes.Index(lower, htmlOpenPrefix); idx >= 0 {
		if end := bytes.IndexByte(html[idx:], '>'); end >= 0 {
			return spliceAt(html, idx+end+1, css)
		}
	}
	return spliceAt(html, 0, css)
}

// HTML whitespace here is tab, LF, FF, CR, and space. A vertical tab is not
// in that set: chromium does not skip it, and treating it as whitespace
// would place the CSP after the doctype and outside head.
func injectCSPFirst(html []byte, csp string) []byte {
	p := 0
	if bytes.HasPrefix(html, []byte{0xEF, 0xBB, 0xBF}) {
		p = 3
	}
	for p < len(html) {
		switch html[p] {
		case '\t', '\n', '\f', '\r', ' ':
			p++
			continue
		}
		break
	}
	if p+len(cspDoctypePrefix) <= len(html) && bytes.EqualFold(html[p:p+len(cspDoctypePrefix)], cspDoctypePrefix) {
		if end := bytes.IndexByte(html[p:], '>'); end >= 0 {
			return spliceAt(html, p+end+1, csp)
		}
	}
	return spliceAt(html, p, csp)
}

func spliceAt(html []byte, at int, css string) []byte {
	out := make([]byte, 0, len(html)+len(css))
	out = append(out, html[:at]...)
	out = append(out, css...)
	out = append(out, html[at:]...)
	return out
}
