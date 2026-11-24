package convert

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func init() {
	Default.Register(HTMLRenderConverter{})
}

const (
	htmlRenderDefault = 1280
	htmlRenderMin     = 256
	htmlRenderMax     = 2048
	blankChannel      = 250
)

// HTMLRenderConverter draws one HTML file as a primary square PNG.
// A zero value prints with chromium-headless-shell and rasters with pdftoppm.
type HTMLRenderConverter struct{}

// Operation reports the render operation.
func (HTMLRenderConverter) Operation() string { return OperationRender }

// Pairs returns the html to png pair.
func (HTMLRenderConverter) Pairs() []Pair {
	return []Pair{{From: "html", To: "png"}}
}

// Convert writes one square PNG at outPath.
// A nil or empty opts map uses edge 1280. The only accepted key is size.
func (HTMLRenderConverter) Convert(ctx context.Context, inPath, outPath string, opts map[string]any) error {
	if err := renderHTML(ctx, inPath, outPath, opts); err != nil {
		_ = os.Remove(outPath)
		return err
	}
	return nil
}

func renderHTML(ctx context.Context, inPath, outPath string, opts map[string]any) error {
	edge, err := htmlRenderSize(opts)
	if err != nil {
		return err
	}
	workDir, err := os.MkdirTemp(filepath.Dir(outPath), ".html-render-")
	if err != nil {
		return fmt.Errorf("html render: mkdir workspace: %w", err)
	}
	defer os.RemoveAll(workDir)
	pdfPath := filepath.Join(workDir, "sheet.pdf")
	if err := printHTMLPDF(ctx, inPath, pdfPath, workDir); err != nil {
		return err
	}
	img, err := rasterHTMLSquare(ctx, pdfPath, workDir, edge)
	if err != nil {
		return err
	}
	return writeHTMLPNG(outPath, img)
}

func htmlRenderSize(opts map[string]any) (int, error) {
	if len(opts) == 0 {
		return htmlRenderDefault, nil
	}
	if len(opts) != 1 {
		return 0, fmt.Errorf("html render: unsupported option")
	}
	raw, ok := opts["size"]
	if !ok {
		return 0, fmt.Errorf("html render: unsupported option")
	}
	n, err := htmlRenderSizeInt(raw)
	if err != nil {
		return 0, err
	}
	if n < htmlRenderMin || n > htmlRenderMax {
		return 0, fmt.Errorf("html render: size %d out of range", n)
	}
	return n, nil
}

func htmlRenderSizeInt(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float64:
		if v != math.Trunc(v) || math.IsInf(v, 0) || math.IsNaN(v) {
			return 0, fmt.Errorf("html render: size is not an integer")
		}
		return int(v), nil
	default:
		return 0, fmt.Errorf("html render: size is not an integer")
	}
}

func printHTMLPDF(ctx context.Context, inPath, pdfPath, workDir string) error {
	conv, ok := Default.LookupOp(OperationConvert, "html", "pdf")
	if !ok {
		return fmt.Errorf("html render: html to pdf is not registered")
	}
	chrome, ok := conv.(ChromiumConverter)
	if !ok {
		return fmt.Errorf("html render: html to pdf converter is %T", conv)
	}
	if chrome.sharedTmp != "" {
		if err := sweepSharedTmp(chrome.sharedTmp, workDir); err != nil {
			return fmt.Errorf("html render: %w", err)
		}
	}
	input, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("html render: read input: %w", err)
	}
	rendered := injectPrintCSS(injectCSPFirst(input, cspNoScriptMeta), buildPrintCSS(htmlOpts{}))
	renderedPath := filepath.Join(workDir, "rendered.html")
	if err := os.WriteFile(renderedPath, rendered, 0o600); err != nil {
		return fmt.Errorf("html render: write rendered html: %w", err)
	}
	args := chromiumPrintArgs(workDir, pdfPath, "file://"+renderedPath)
	if err := chrome.exec(workDir).run(ctx, "chromium-headless-shell", args...); err != nil {
		return fmt.Errorf("html render: %w", err)
	}
	if err := validatePDF(pdfPath); err != nil {
		return fmt.Errorf("html render: %w", err)
	}
	return nil
}

func rasterHTMLSquare(ctx context.Context, pdfPath, workDir string, edge int) (image.Image, error) {
	conv, ok := Default.LookupOp(OperationConvert, "html", "pdf")
	if !ok {
		return nil, fmt.Errorf("html render: html to pdf is not registered")
	}
	chrome, ok := conv.(ChromiumConverter)
	if !ok {
		return nil, fmt.Errorf("html render: html to pdf converter is %T", conv)
	}
	base := filepath.Join(workDir, "page")
	err := chrome.exec(workDir).run(ctx, "pdftoppm", "-png", "-scale-to", strconv.Itoa(edge), pdfPath, base)
	if err != nil {
		return nil, fmt.Errorf("html render: %w", err)
	}
	return squareFromHTMLPages(workDir, base, edge)
}

func squareFromHTMLPages(workDir, base string, edge int) (image.Image, error) {
	paths, err := htmlRasterPagePaths(workDir, base)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("html render: pdftoppm wrote no page")
	}
	for _, path := range paths {
		img, err := decodeHTMLPage(path)
		if err != nil {
			return nil, err
		}
		if pageIsBlank(img) {
			continue
		}
		return letterboxPage(img, edge), nil
	}
	return whiteSquare(edge), nil
}

func decodeHTMLPage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("html render: open page: %w", err)
	}
	img, decErr := png.Decode(f)
	closeErr := f.Close()
	if decErr != nil {
		return nil, fmt.Errorf("html render: decode page: %w", decErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("html render: close page: %w", closeErr)
	}
	return img, nil
}

func htmlRasterPagePaths(workDir, base string) ([]string, error) {
	prefix := filepath.Base(base) + "-"
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return nil, fmt.Errorf("html render: read pages: %w", err)
	}
	type pageFile struct {
		n    int
		path string
	}
	found := make([]pageFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".png") {
			continue
		}
		numText := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".png")
		n, err := strconv.Atoi(numText)
		if err != nil || n < 1 {
			continue
		}
		found = append(found, pageFile{n: n, path: filepath.Join(workDir, name)})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].n < found[j].n })
	paths := make([]string, len(found))
	for i, page := range found {
		paths[i] = page.path
	}
	return paths, nil
}

func pageIsBlank(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 < blankChannel || g>>8 < blankChannel || bl>>8 < blankChannel {
				return false
			}
		}
	}
	return true
}

func letterboxPage(src image.Image, edge int) *image.NRGBA {
	dst := whiteSquare(edge)
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 || edge <= 0 {
		return dst
	}
	longer := sw
	if sh > longer {
		longer = sh
	}
	dw := sw * edge / longer
	dh := sh * edge / longer
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	ox := (edge - dw) / 2
	oy := (edge - dh) / 2
	for y := 0; y < dh; y++ {
		sy := sb.Min.Y + y*sh/dh
		for x := 0; x < dw; x++ {
			sx := sb.Min.X + x*sw/dw
			dst.SetNRGBA(ox+x, oy+y, color.NRGBAModel.Convert(src.At(sx, sy)).(color.NRGBA))
		}
	}
	return dst
}

func whiteSquare(edge int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, edge, edge))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
		img.Pix[i+1] = 255
		img.Pix[i+2] = 255
		img.Pix[i+3] = 255
	}
	return img
}

func writeHTMLPNG(path string, img image.Image) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("html render: create png: %w", err)
	}
	encErr := png.Encode(f, img)
	closeErr := f.Close()
	if encErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("html render: encode png: %w", encErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("html render: close png: %w", closeErr)
	}
	return nil
}
