package web

import (
	"encoding/json"
	"html"
	"net/http"
	"regexp"
	"strings"
)

// sanitizeHTML keeps a small allowlist of tags (class attribute only) and
// escapes everything else. Plugin HTML is rendered through this. It is
// deliberately dumb; do not enable untrusted remote plugins with tool
// panels until this is replaced by a real sanitiser.
var (
	allowedTags = map[string]bool{"b": true, "i": true, "em": true, "strong": true, "p": true, "ul": true, "ol": true, "li": true,
		"br": true, "span": true, "div": true, "code": true, "pre": true, "h3": true, "h4": true, "small": true, "hr": true}
	tagRe   = regexp.MustCompile(`(?s)<\s*(/?)\s*([a-zA-Z0-9]+)([^>]*)>`)
	classRe = regexp.MustCompile(`class\s*=\s*"([a-zA-Z0-9 _-]*)"`)
)

func sanitizeHTML(in string) string {
	var b strings.Builder
	last := 0
	for _, m := range tagRe.FindAllStringSubmatchIndex(in, -1) {
		b.WriteString(html.EscapeString(in[last:m[0]]))
		last = m[1]
		closing := in[m[2]:m[3]] == "/"
		name := strings.ToLower(in[m[4]:m[5]])
		attrs := in[m[6]:m[7]]
		if !allowedTags[name] {
			b.WriteString(html.EscapeString(in[m[0]:m[1]]))
			continue
		}
		b.WriteString("<")
		if closing {
			b.WriteString("/")
		}
		b.WriteString(name)
		if !closing {
			if cm := classRe.FindStringSubmatch(attrs); cm != nil {
				b.WriteString(` class="` + cm[1] + `"`)
			}
		}
		b.WriteString(">")
	}
	b.WriteString(html.EscapeString(in[last:]))
	return b.String()
}

func writeJSON(w http.ResponseWriter, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(v)
}
