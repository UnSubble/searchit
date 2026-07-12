package fingerprint

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// detectHTML parses HTML response bodies to extract meta tags, script/link tags,
// comments, and framework attributes, recording them as raw fingerprint signals.
func detectHTML(ctx *Context, fp *Fingerprint) {
	if len(ctx.Body) == 0 {
		return
	}

	if !isHTML(ctx) {
		return
	}

	tokenizer := html.NewTokenizer(bytes.NewReader(ctx.Body))

	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break // EOF or parsing error
		}

		switch tokenType {
		case html.StartTagToken, html.SelfClosingTagToken:
			tnBytes, hasAttr := tokenizer.TagName()
			tagNameLower := strings.ToLower(string(tnBytes))

			var (
				metaName, metaHttpEquiv, metaProperty, metaContent, metaCharset string
				linkRel, linkHref                                               string
				scriptSrc                                                       string
				baseHref                                                        string
			)

			if hasAttr {
				for {
					kBytes, vBytes, more := tokenizer.TagAttr()
					k := strings.ToLower(string(kBytes))
					v := string(vBytes)

					// 1. Collect tag-specific attributes
					switch tagNameLower {
					case "meta":
						switch k {
						case "name":
							metaName = v
						case "http-equiv":
							metaHttpEquiv = v
						case "property":
							metaProperty = v
						case "content":
							metaContent = v
						case "charset":
							metaCharset = v
						}
					case "link":
						if k == "rel" {
							linkRel = strings.ToLower(strings.TrimSpace(v))
						} else if k == "href" {
							linkHref = strings.TrimSpace(v)
						}
					case "script":
						if k == "src" {
							scriptSrc = strings.TrimSpace(v)
						}
					case "base":
						if k == "href" {
							baseHref = strings.TrimSpace(v)
						}
					}

					// 2. Perform general framework attribute checks
					if k == "ng-version" || k == "ng-app" || k == "data-reactroot" || k == "data-react-html" || k == "_nuxt" {
						fp.AddSignal(Signal{
							Source:     "html:attr:" + k,
							Value:      v,
							Confidence: Confidence(1.0),
						})
					} else if strings.HasPrefix(k, "data-v-") || strings.HasPrefix(k, "_nghost-") || strings.HasPrefix(k, "_ngcontent-") {
						fp.AddSignal(Signal{
							Source:     "html:attr:" + k,
							Value:      v,
							Confidence: Confidence(1.0),
						})
					} else if k == "id" {
						vLower := strings.ToLower(strings.TrimSpace(v))
						if vLower == "app" || vLower == "root" || vLower == "__next" || vLower == "__nuxt" || vLower == "main" {
							fp.AddSignal(Signal{
								Source:     "html:id",
								Value:      v,
								Confidence: Confidence(1.0),
							})
						}
					}

					if !more {
						break
					}
				}
			}

			// 3. Process tag-specific actions after processing all attributes
			switch tagNameLower {
			case "meta":
				if metaName != "" && metaContent != "" {
					fp.AddSignal(Signal{
						Source:     "html:meta:name:" + strings.ToLower(metaName),
						Value:      metaContent,
						Confidence: Confidence(1.0),
					})
				}
				if metaHttpEquiv != "" && metaContent != "" {
					fp.AddSignal(Signal{
						Source:     "html:meta:http-equiv:" + strings.ToLower(metaHttpEquiv),
						Value:      metaContent,
						Confidence: Confidence(1.0),
					})
				}
				if metaProperty != "" && metaContent != "" {
					fp.AddSignal(Signal{
						Source:     "html:meta:property:" + strings.ToLower(metaProperty),
						Value:      metaContent,
						Confidence: Confidence(1.0),
					})
				}
				if metaCharset != "" {
					fp.AddSignal(Signal{
						Source:     "html:meta:charset",
						Value:      metaCharset,
						Confidence: Confidence(1.0),
					})
				}
			case "link":
				if linkHref != "" && (linkRel == "stylesheet" || linkRel == "icon" || linkRel == "shortcut icon" || linkRel == "preload") {
					fp.AddSignal(Signal{
						Source:     "html:link:" + linkRel,
						Value:      linkHref,
						Confidence: Confidence(1.0),
					})
				}
			case "script":
				if scriptSrc != "" {
					fp.AddSignal(Signal{
						Source:     "html:script",
						Value:      scriptSrc,
						Confidence: Confidence(1.0),
					})
				}
			case "base":
				if baseHref != "" {
					fp.AddSignal(Signal{
						Source:     "html:base",
						Value:      baseHref,
						Confidence: Confidence(1.0),
					})
				}
			}

		case html.CommentToken:
			commentVal := strings.TrimSpace(string(tokenizer.Text()))
			if commentVal != "" {
				lowerComment := strings.ToLower(commentVal)
				if strings.Contains(lowerComment, "generator") ||
					strings.Contains(lowerComment, "version") ||
					strings.Contains(lowerComment, "build") ||
					strings.Contains(lowerComment, "author") ||
					strings.Contains(lowerComment, "powered by") ||
					strings.Contains(lowerComment, "theme") {

					if len(commentVal) > 256 {
						commentVal = commentVal[:253] + "..."
					}
					fp.AddSignal(Signal{
						Source:     "html:comment",
						Value:      commentVal,
						Confidence: Confidence(1.0),
					})
				}
			}
		}
	}
}

// isHTML checks response context to determine if it is HTML document.
func isHTML(ctx *Context) bool {
	for k, values := range ctx.Header {
		if strings.EqualFold(k, "Content-Type") {
			for _, val := range values {
				if strings.Contains(strings.ToLower(val), "text/html") {
					return true
				}
			}
		}
	}

	path := strings.ToLower(ctx.Path)
	if strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".htm") {
		return true
	}

	return false
}
