package convert

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHTMLRenderManifest(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "manifest-html-render.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Capabilities []struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
			Legacy  struct {
				Operation string `json:"operation"`
				Engine    string `json:"engine"`
			} `json:"legacy"`
			ClientOperation string         `json:"client_operation"`
			ParameterSchema map[string]any `json:"parameter_schema"`
			Inputs          []struct {
				Role string `json:"role"`
			} `json:"inputs"`
			Outputs []struct {
				Role string `json:"role"`
			} `json:"outputs"`
			Formats []Pair `json:"formats"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Capabilities) != 1 {
		t.Fatalf("capabilities %d", len(doc.Capabilities))
	}
	cap := doc.Capabilities[0]
	if cap.ID != "octoconv.render.html" || cap.Version != 1 || cap.Legacy.Operation != OperationRender || cap.Legacy.Engine != EngineHTML || cap.ClientOperation != OperationRender {
		t.Fatalf("%+v", cap)
	}
	if len(cap.ParameterSchema) != 0 || bytes.Contains(raw, []byte(`"page"`)) || bytes.Contains(raw, []byte(`"page_size"`)) {
		t.Fatal("parameter schema")
	}
	if len(cap.Inputs) != 1 || cap.Inputs[0].Role != "source" || len(cap.Outputs) != 1 || cap.Outputs[0].Role != "primary" {
		t.Fatalf("links %+v %+v", cap.Inputs, cap.Outputs)
	}
	pairs := (HTMLRenderConverter{}).Pairs()
	if len(cap.Formats) != 1 || cap.Formats[0] != (Pair{From: "html", To: "png"}) || len(pairs) != 1 || pairs[0] != cap.Formats[0] {
		t.Fatalf("formats %+v pairs %+v", cap.Formats, pairs)
	}
	conv, ok := Default.LookupOp(OperationRender, "html", "png")
	if !ok {
		t.Fatal("missing render")
	}
	if _, ok := conv.(HTMLRenderConverter); !ok {
		t.Fatalf("converter %T", conv)
	}
	if _, ok := Default.LookupOp(OperationConvert, "html", "png"); ok {
		t.Fatal("convert serves png")
	}
	if MIMEType("png") != "image/png" || MIMEType("pdf") != "application/pdf" || MIMEType("html") != "text/html" {
		t.Fatal("mime")
	}
}

func TestHTMLRenderLetterbox(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 200))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i] = 255
		src.Pix[i+3] = 255
	}
	img := letterboxPage(src, 128)
	if img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
		t.Fatalf("bounds %v", img.Bounds())
	}
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			c := img.NRGBAAt(x, y)
			inside := x >= 32 && x < 96
			if inside {
				if c.R != 255 || c.G != 0 || c.B != 0 || c.A != 255 {
					t.Fatalf("inside %d,%d %+v", x, y, c)
				}
				continue
			}
			if c != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
				t.Fatalf("outside %d,%d %+v", x, y, c)
			}
		}
	}
}

func TestHTMLRenderUnserved(t *testing.T) {
	for _, source := range []string{"pdf", "png"} {
		if _, ok := Default.LookupOp(OperationRender, source, "png"); ok {
			t.Fatalf("served %s", source)
		}
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	out := filepath.Join(dir, "out.png")
	if err := os.WriteFile(out, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, []byte("%PDF-"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (HTMLRenderConverter{}).Convert(context.Background(), in, out, nil); err == nil {
		t.Fatal("drew pdf")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("png left")
	}
}

func TestHTMLRenderContent(t *testing.T) {
	requireHTMLRenderEngines(t)
	img := decodeHTMLPNG(t, renderHTMLFile(t, contentHTML(), nil))
	if img.Bounds().Dx() != 1280 || img.Bounds().Dy() != 1280 || pageIsBlank(img) || !cornersAtLeast(img, blankChannel) {
		t.Fatalf("bounds %v blank %v", img.Bounds(), pageIsBlank(img))
	}
}

func TestHTMLRenderAllBlank(t *testing.T) {
	requireHTMLRenderEngines(t)
	img := decodeHTMLPNG(t, renderHTMLFile(t, `<!DOCTYPE html><html><head></head><body></body></html>`, nil))
	if img.Bounds().Dx() != 1280 || img.Bounds().Dy() != 1280 || !pageIsBlank(img) {
		t.Fatalf("bounds %v blank %v", img.Bounds(), pageIsBlank(img))
	}
}

func TestHTMLRenderLeadingBlank(t *testing.T) {
	requireHTMLRenderEngines(t)
	body := `<!DOCTYPE html><html><head></head><body><div style="break-after:page;height:270mm"></div><p style="font-size:96px;color:#000000">X</p></body></html>`
	img := decodeHTMLPNG(t, renderHTMLFile(t, body, nil))
	if pageIsBlank(img) {
		t.Fatal("leading blank sheet was not skipped")
	}
}

func TestHTMLRenderSize(t *testing.T) {
	if n, err := htmlRenderSize(nil); err != nil || n != 1280 {
		t.Fatalf("nil %d %v", n, err)
	}
	if n, err := htmlRenderSize(map[string]any{}); err != nil || n != 1280 {
		t.Fatalf("empty %d %v", n, err)
	}
	for _, raw := range []any{int(256), int32(2048), int64(1280), float64(256)} {
		opts := map[string]any{"size": raw}
		n, err := htmlRenderSize(opts)
		if err != nil {
			t.Fatal(raw, err)
		}
		switch raw {
		case int(256), float64(256):
			if n != 256 {
				t.Fatal(n)
			}
		case int32(2048):
			if n != 2048 {
				t.Fatal(n)
			}
		case int64(1280):
			if n != 1280 {
				t.Fatal(n)
			}
		}
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.html")
	if err := os.WriteFile(in, []byte(contentHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, opts := range []map[string]any{
		{"size": 255},
		{"size": 2049},
		{"size": 256.5},
		{"size": "1280"},
		{"page_size": "a4"},
		{"margin_mm": 0},
		{"landscape": true},
		{"print_background": true},
		{"size": 1280, "page": 1},
	} {
		out := filepath.Join(dir, "out.png")
		if err := os.WriteFile(out, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := (HTMLRenderConverter{}).Convert(context.Background(), in, out, opts); err == nil {
			t.Fatalf("accepted %#v", opts)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("png left for %#v", opts)
		}
	}
}

func TestHTMLRenderEngineDown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	in := filepath.Join(dir, "in.html")
	out := filepath.Join(dir, "out.png")
	if err := os.WriteFile(in, []byte(contentHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (HTMLRenderConverter{}).Convert(context.Background(), in, out, nil); err == nil {
		t.Fatal("engine started")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("png left")
	}
}

func contentHTML() string {
	return `<!DOCTYPE html><html><head></head><body><p style="font-size:96px;color:#000000">X</p></body></html>`
}

func requireHTMLRenderEngines(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("chromium-headless-shell"); err != nil {
		t.Skip("chromium-headless-shell is not on PATH")
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm is not on PATH")
	}
}

func renderHTMLFile(t *testing.T, body string, opts map[string]any) []byte {
	t.Helper()
	dir := t.TempDir()
	in := filepath.Join(dir, "in.html")
	out := filepath.Join(dir, "out.png")
	if err := os.WriteFile(in, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (HTMLRenderConverter{}).Convert(context.Background(), in, out, opts); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func decodeHTMLPNG(t *testing.T, raw []byte) image.Image {
	t.Helper()
	if !bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("png signature")
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func cornersAtLeast(img image.Image, min uint32) bool {
	b := img.Bounds()
	for _, p := range []image.Point{
		{X: b.Min.X, Y: b.Min.Y},
		{X: b.Max.X - 1, Y: b.Min.Y},
		{X: b.Min.X, Y: b.Max.Y - 1},
		{X: b.Max.X - 1, Y: b.Max.Y - 1},
	} {
		r, g, bl, _ := img.At(p.X, p.Y).RGBA()
		if r>>8 < min || g>>8 < min || bl>>8 < min {
			return false
		}
	}
	return true
}
