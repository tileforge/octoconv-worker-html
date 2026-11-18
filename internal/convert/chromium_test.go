package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPairsMatchTheManifest(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "manifest-html-convert.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Capabilities []struct {
			ID     string `json:"id"`
			Legacy struct {
				Operation string `json:"operation"`
				Engine    string `json:"engine"`
			} `json:"legacy"`
			Formats []Pair `json:"formats"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Capabilities) != 1 {
		t.Fatalf("capabilities %d", len(doc.Capabilities))
	}
	gotCap := doc.Capabilities[0]
	if gotCap.ID != "octoconv.convert.html" || gotCap.Legacy.Engine != EngineHTML || gotCap.Legacy.Operation != OperationConvert {
		t.Fatalf("capability %+v", gotCap)
	}
	pairs := (ChromiumConverter{}).Pairs()
	if len(pairs) != 1 || len(gotCap.Formats) != 1 || gotCap.Formats[0] != pairs[0] {
		t.Fatalf("formats %+v pairs %+v", gotCap.Formats, pairs)
	}
	conv, ok := Default.LookupOp(OperationConvert, "html", "pdf")
	if !ok {
		t.Fatal("missing html to pdf")
	}
	if _, err := htmlOptsFromMap(map[string]any{"page_size": "a4"}); err != nil {
		t.Fatal(err)
	}
	if err := conv.Convert(context.Background(), "in.html", filepath.Join(t.TempDir(), "out.pdf"), map[string]any{"zoom": 2}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown option: %v", err)
	}
}

func TestValidatePDF(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "out.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validatePDF(pdf); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "plain.pdf")
	if err := os.WriteFile(plain, []byte("not a pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validatePDF(plain); err == nil {
		t.Fatal("plain pdf accepted")
	}
	empty := filepath.Join(dir, "empty.pdf")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validatePDF(empty); err == nil {
		t.Fatal("empty pdf accepted")
	}
}

func TestInjectCSPFirst(t *testing.T) {
	withDoctype := []byte("<!doctype html><html><head></head><body>x</body></html>")
	got := injectCSPFirst(withDoctype, cspNoScriptMeta)
	wantAt := len("<!doctype html>")
	if !bytes.Equal(got[:wantAt], withDoctype[:wantAt]) || !bytes.HasPrefix(got[wantAt:], []byte(cspNoScriptMeta)) {
		t.Fatalf("csp not after doctype: %s", got)
	}
	noDoctype := []byte("<html><body>x</body></html>")
	got = injectCSPFirst(noDoctype, cspNoScriptMeta)
	if !bytes.HasPrefix(got, []byte(cspNoScriptMeta)) {
		t.Fatalf("csp not first: %s", got)
	}
	vertical := append([]byte{'\v'}, withDoctype...)
	got = injectCSPFirst(vertical, cspNoScriptMeta)
	if !bytes.HasPrefix(got, []byte(cspNoScriptMeta)) {
		t.Fatalf("vertical tab skipped: %s", got)
	}
}

func TestInjectPrintCSSKeepsByteOffsets(t *testing.T) {
	html := []byte("<html><head><title>İstanbul</title></head><body>x</body></html>")
	got := injectPrintCSS(html, buildPrintCSS(htmlOpts{}))
	titleAt := bytes.Index(got, []byte("</title>"))
	headAt := bytes.Index(got, []byte("</head>"))
	css := buildPrintCSS(htmlOpts{})
	cssAt := bytes.Index(got, []byte(css))
	if titleAt < 0 || cssAt != titleAt+len("</title>") || headAt != cssAt+len(css) {
		t.Fatalf("css %d title %d head %d\n%s", cssAt, titleAt, headAt, got)
	}

	upper := []byte("<HTML><HEAD><TITLE>x</TITLE></HEAD><BODY>x</BODY></HTML>")
	got = injectPrintCSS(upper, "<style>x</style>")
	if !bytes.Contains(got, []byte("</TITLE><style>x</style></HEAD>")) {
		t.Fatalf("upper head: %s", got)
	}

	noHead := []byte("<HTML lang=\"tr\"><body>x</body></HTML>")
	got = injectPrintCSS(noHead, "<style>x</style>")
	if !bytes.HasPrefix(got, []byte("<HTML lang=\"tr\"><style>x</style>")) {
		t.Fatalf("html open: %s", got)
	}
}

func TestDefaultPrintCSSGoesBeforeHeadClose(t *testing.T) {
	html := []byte("<!doctype html><html><head><title>x</title></head><body>x</body></html>")
	css := buildPrintCSS(htmlOpts{})
	got := injectPrintCSS(injectCSPFirst(html, cspNoScriptMeta), css)
	head := bytes.Index(bytes.ToLower(got), []byte("</head>"))
	cssAt := bytes.Index(got, []byte(css))
	if cssAt < 0 || head < 0 || cssAt > head {
		t.Fatalf("css %d head %d", cssAt, head)
	}
	if !bytes.Contains(got, []byte("size: A4 !important")) {
		t.Fatal("missing default page size")
	}
	if bytes.Contains(got, []byte("margin:")) {
		t.Fatal("default css sets a margin")
	}
}

func TestPrintCSS(t *testing.T) {
	zero, err := htmlOptsFromMap(nil)
	if err != nil {
		t.Fatal(err)
	}
	if zero.MarginMM != nil {
		t.Fatal("nil margin became explicit")
	}
	css := buildPrintCSS(zero)
	if !strings.Contains(css, "size: A4 !important") || strings.Contains(css, "margin:") {
		t.Fatalf("default %s", css)
	}
	if !strings.Contains(css, "background: none !important") {
		t.Fatalf("default background %s", css)
	}

	explicit, err := htmlOptsFromMap(map[string]any{
		"page_size":        "letter",
		"margin_mm":        float64(0),
		"landscape":        true,
		"print_background": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.MarginMM == nil || *explicit.MarginMM != 0 {
		t.Fatalf("explicit zero %+v", explicit.MarginMM)
	}
	css = buildPrintCSS(explicit)
	if !strings.Contains(css, "size: letter landscape !important") || !strings.Contains(css, "margin: 0mm !important") {
		t.Fatalf("explicit %s", css)
	}
	if !strings.Contains(css, "print-color-adjust: exact !important") || strings.Contains(css, "background: none") {
		t.Fatalf("background %s", css)
	}

	ten, err := htmlOptsFromMap(map[string]any{"margin_mm": float64(10)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buildPrintCSS(ten), "margin: 10mm !important") {
		t.Fatal(buildPrintCSS(ten))
	}
	if _, err := htmlOptsFromMap(map[string]any{"page_size": "tabloid"}); err == nil {
		t.Fatal("tabloid accepted")
	}
	if _, err := htmlOptsFromMap(map[string]any{"margin_mm": float64(51)}); err == nil {
		t.Fatal("margin 51 accepted")
	}
	if _, err := htmlOptsFromMap(map[string]any{"margin_mm": float64(10.5)}); err == nil {
		t.Fatal("fractional margin accepted")
	}
	if _, err := htmlOptsFromMap(map[string]any{"unknown": true}); err == nil {
		t.Fatal("unknown accepted")
	}
}
