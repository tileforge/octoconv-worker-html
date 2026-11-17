// Package convert turns one HTML file into a PDF with chromium-headless-shell.
package convert

import (
	"context"
	"strings"
	"time"
)

const (
	EngineHTML           = "html"
	OperationConvert     = "convert"
	DefaultEngineTimeout = 60 * time.Second
	MaxFileBytes         = 100 << 20
)

// Pair is one source format and one target format.
type Pair struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Converter converts the file at inPath into outPath.
type Converter interface {
	Pairs() []Pair
	Convert(ctx context.Context, inPath, outPath string, opts map[string]any) error
}

// MIMEType is the content type the core stores for format.
func MIMEType(format string) string {
	switch NormalizeFormat(format) {
	case "pdf":
		return "application/pdf"
	case "html":
		return "text/html"
	default:
		return "application/octet-stream"
	}
}

// NormalizeFormat lowercases a format or extension and strips one leading dot.
func NormalizeFormat(f string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(f), "."))
}

type opKey struct {
	Operation string
	From      string
	To        string
}

// Registry maps an operation and a format pair to a converter.
type Registry struct {
	m map[opKey]Converter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{m: make(map[opKey]Converter)}
}

// Register indexes each pair c reports.
func (r *Registry) Register(c Converter) {
	for _, pair := range c.Pairs() {
		r.m[opKey{
			Operation: OperationConvert,
			From:      NormalizeFormat(pair.From),
			To:        NormalizeFormat(pair.To),
		}] = c
	}
}

// LookupOp reports the converter for op, from, and to.
func (r *Registry) LookupOp(op, from, to string) (Converter, bool) {
	c, ok := r.m[opKey{
		Operation: op,
		From:      NormalizeFormat(from),
		To:        NormalizeFormat(to),
	}]
	return c, ok
}

// Default is the process registry. init registers Chromium.
var Default = NewRegistry()
