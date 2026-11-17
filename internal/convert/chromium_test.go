package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	if err := conv.Convert(context.Background(), "in.html", filepath.Join(t.TempDir(), "out.pdf"), map[string]any{"page_size": "a4"}); err == nil {
		t.Fatal("option accepted")
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
	got := injectPrintCSS(html, defaultPrintCSS)
	titleAt := bytes.Index(got, []byte("</title>"))
	headAt := bytes.Index(got, []byte("</head>"))
	cssAt := bytes.Index(got, []byte(defaultPrintCSS))
	if titleAt < 0 || cssAt != titleAt+len("</title>") || headAt != cssAt+len(defaultPrintCSS) {
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
	got := injectPrintCSS(injectCSPFirst(html, cspNoScriptMeta), defaultPrintCSS)
	head := bytes.Index(bytes.ToLower(got), []byte("</head>"))
	css := bytes.Index(got, []byte(defaultPrintCSS))
	if css < 0 || head < 0 || css > head {
		t.Fatalf("css %d head %d", css, head)
	}
	if !bytes.Contains(got, []byte("size: A4 !important")) {
		t.Fatal("missing default page size")
	}
}
