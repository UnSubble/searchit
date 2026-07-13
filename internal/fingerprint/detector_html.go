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
			isMeta := bytes.EqualFold(tnBytes, []byte("meta"))
			isLink := bytes.EqualFold(tnBytes, []byte("link"))
			isScript := bytes.EqualFold(tnBytes, []byte("script"))
			isBase := bytes.EqualFold(tnBytes, []byte("base"))

			var (
				metaName, metaHttpEquiv, metaProperty, metaContent, metaCharset string
				linkRel, linkHref                                               string
				scriptSrc                                                       string
				baseHref                                                        string
			)

			if hasAttr {
				for {
					kBytes, vBytes, more := tokenizer.TagAttr()

					// Check general attributes first
					isId := bytes.EqualFold(kBytes, []byte("id"))
					isGeneralAttr := bytes.EqualFold(kBytes, []byte("ng-version")) ||
						bytes.EqualFold(kBytes, []byte("ng-app")) ||
						bytes.EqualFold(kBytes, []byte("data-reactroot")) ||
						bytes.EqualFold(kBytes, []byte("data-react-html")) ||
						bytes.EqualFold(kBytes, []byte("_nuxt"))

					isPrefixedAttr := bytesHasPrefixLower(kBytes, []byte("data-v-")) ||
						bytesHasPrefixLower(kBytes, []byte("_nghost-")) ||
						bytesHasPrefixLower(kBytes, []byte("_ngcontent-"))

					// Check tag-specific attributes
					var isName, isHttpEquiv, isProperty, isContent, isCharset bool
					var isRel, isHref, isSrc bool

					if isMeta {
						isName = bytes.EqualFold(kBytes, []byte("name"))
						isHttpEquiv = bytes.EqualFold(kBytes, []byte("http-equiv"))
						isProperty = bytes.EqualFold(kBytes, []byte("property"))
						isContent = bytes.EqualFold(kBytes, []byte("content"))
						isCharset = bytes.EqualFold(kBytes, []byte("charset"))
					} else if isLink {
						isRel = bytes.EqualFold(kBytes, []byte("rel"))
						isHref = bytes.EqualFold(kBytes, []byte("href"))
					} else if isScript {
						isSrc = bytes.EqualFold(kBytes, []byte("src"))
					} else if isBase {
						isHref = bytes.EqualFold(kBytes, []byte("href"))
					}

					if isId || isGeneralAttr || isPrefixedAttr || isName || isHttpEquiv || isProperty || isContent || isCharset || isRel || isHref || isSrc {
						v := string(vBytes)
						if isGeneralAttr {
							fp.AddSignal(Signal{
								Source:     "html:attr:" + string(kBytes),
								Value:      v,
								Confidence: Confidence(1.0),
							})
						} else if isPrefixedAttr {
							fp.AddSignal(Signal{
								Source:     "html:attr:" + string(kBytes),
								Value:      v,
								Confidence: Confidence(1.0),
							})
						} else if isId {
							vLower := strings.ToLower(strings.TrimSpace(v))
							if vLower == "app" || vLower == "root" || vLower == "__next" || vLower == "__nuxt" || vLower == "main" {
								fp.AddSignal(Signal{
									Source:     "html:id",
									Value:      v,
									Confidence: Confidence(1.0),
								})
							}
						} else if isMeta {
							if isName {
								metaName = v
							} else if isHttpEquiv {
								metaHttpEquiv = v
							} else if isProperty {
								metaProperty = v
							} else if isContent {
								metaContent = v
							} else if isCharset {
								metaCharset = v
							}
						} else if isLink {
							if isRel {
								linkRel = strings.ToLower(strings.TrimSpace(v))
							} else if isHref {
								linkHref = strings.TrimSpace(v)
							}
						} else if isScript {
							if isSrc {
								scriptSrc = strings.TrimSpace(v)
							}
						} else if isBase {
							if isHref {
								baseHref = strings.TrimSpace(v)
							}
						}
					}

					if !more {
						break
					}
				}
			}

			// 3. Process tag-specific actions after processing all attributes
			if isMeta {
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
			} else if isLink {
				if linkHref != "" && (linkRel == "stylesheet" || linkRel == "icon" || linkRel == "shortcut icon" || linkRel == "preload") {
					fp.AddSignal(Signal{
						Source:     "html:link:" + linkRel,
						Value:      linkHref,
						Confidence: Confidence(1.0),
					})
				}
			} else if isScript {
				if scriptSrc != "" {
					fp.AddSignal(Signal{
						Source:     "html:script",
						Value:      scriptSrc,
						Confidence: Confidence(1.0),
					})
				}
			} else if isBase {
				if baseHref != "" {
					fp.AddSignal(Signal{
						Source:     "html:base",
						Value:      baseHref,
						Confidence: Confidence(1.0),
					})
				}
			}

		case html.CommentToken:
			commentBytes := tokenizer.Text()
			if len(commentBytes) > 0 {
				if containsCommentKeywords(commentBytes) {
					commentVal := strings.TrimSpace(string(commentBytes))
					if commentVal != "" {
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
}

func bytesHasPrefixLower(s, prefix []byte) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i, p := range prefix {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		if c != p {
			return false
		}
	}
	return true
}

func containsCommentKeywords(b []byte) bool {
	return bytesContainsFold(b, []byte("generator")) ||
		bytesContainsFold(b, []byte("version")) ||
		bytesContainsFold(b, []byte("build")) ||
		bytesContainsFold(b, []byte("author")) ||
		bytesContainsFold(b, []byte("powered by")) ||
		bytesContainsFold(b, []byte("theme"))
}

func bytesContainsFold(s, substr []byte) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1 := s[i+j]
			c2 := substr[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 = c1 + ('a' - 'A')
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
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
