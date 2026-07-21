package html

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// ExtractLinks parses HTML content and returns a slice of extracted URLs/paths
// from tags: a (href), link (href), script (src), img (src), and form (action).
func ExtractLinks(body []byte) []string {
	var links []string
	tokenizer := html.NewTokenizer(bytes.NewReader(body))

	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break
		}

		switch tokenType {
		case html.StartTagToken, html.SelfClosingTagToken:
			tnBytes, hasAttr := tokenizer.TagName()
			if !hasAttr {
				continue
			}

			isA := bytes.EqualFold(tnBytes, []byte("a"))
			isLink := bytes.EqualFold(tnBytes, []byte("link"))
			isScript := bytes.EqualFold(tnBytes, []byte("script"))
			isImg := bytes.EqualFold(tnBytes, []byte("img"))
			isForm := bytes.EqualFold(tnBytes, []byte("form"))

			if !isA && !isLink && !isScript && !isImg && !isForm {
				continue
			}

			for {
				kBytes, vBytes, more := tokenizer.TagAttr()
				k := string(kBytes)
				v := strings.TrimSpace(string(vBytes))

				if v != "" {
					isTargetAttr := false
					if (isA || isLink) && strings.EqualFold(k, "href") {
						isTargetAttr = true
					} else if (isScript || isImg) && strings.EqualFold(k, "src") {
						isTargetAttr = true
					} else if isForm && strings.EqualFold(k, "action") {
						isTargetAttr = true
					}

					if isTargetAttr {
						// Skip fragments, javascript:, mailto:, tel:, etc.
						vLower := strings.ToLower(v)
						if !strings.HasPrefix(vLower, "javascript:") &&
							!strings.HasPrefix(vLower, "mailto:") &&
							!strings.HasPrefix(vLower, "tel:") &&
							!strings.HasPrefix(v, "#") {
							links = append(links, v)
						}
					}
				}

				if !more {
					break
				}
			}
		}
	}
	return links
}
