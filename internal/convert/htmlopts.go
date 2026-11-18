package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

var htmlPageSizeCSS = map[string]string{
	"a4":     "A4",
	"letter": "letter",
	"legal":  "legal",
	"a3":     "A3",
	"a5":     "A5",
}

type htmlOpts struct {
	PageSize string `json:"page_size,omitempty"`
	// MarginMM is nil when the key is absent. An explicit 0 still sets the margin.
	MarginMM        *int `json:"margin_mm,omitempty"`
	Landscape       bool `json:"landscape,omitempty"`
	PrintBackground bool `json:"print_background,omitempty"`
}

func htmlOptsFromMap(m map[string]any) (htmlOpts, error) {
	if len(m) == 0 {
		return htmlOpts{}, nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return htmlOpts{}, fmt.Errorf("marshal opts: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var o htmlOpts
	if err := dec.Decode(&o); err != nil {
		return htmlOpts{}, fmt.Errorf("parse opts: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return htmlOpts{}, fmt.Errorf("parse opts: trailing data")
	}
	if o.PageSize != "" {
		if _, ok := htmlPageSizeCSS[o.PageSize]; !ok {
			return htmlOpts{}, fmt.Errorf("unsupported page_size %q", o.PageSize)
		}
	}
	if o.MarginMM != nil && (*o.MarginMM < 0 || *o.MarginMM > 50) {
		return htmlOpts{}, fmt.Errorf("margin_mm %d out of range [0,50]", *o.MarginMM)
	}
	return o, nil
}

func buildPrintCSS(o htmlOpts) string {
	size := htmlPageSizeCSS[o.PageSize]
	if size == "" {
		size = htmlPageSizeCSS["a4"]
	}
	if o.Landscape {
		size += " landscape"
	}
	css := fmt.Sprintf("@page { size: %s !important; }\n", size)
	if o.MarginMM != nil {
		css = fmt.Sprintf("@page { size: %s !important; margin: %dmm !important; }\n", size, *o.MarginMM)
	}
	if o.PrintBackground {
		css += "*, *::before, *::after { -webkit-print-color-adjust: exact !important; print-color-adjust: exact !important; }\n"
		return "<style>" + css + "</style>"
	}
	css += "*, *::before, *::after { -webkit-print-color-adjust: economy !important; print-color-adjust: economy !important; " +
		"background: none !important; background-color: transparent !important; background-image: none !important; }\n"
	return "<style>" + css + "</style>"
}
