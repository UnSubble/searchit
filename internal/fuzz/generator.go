package fuzz

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/stats"
)

// Generator produces RequestDTO instances by replacing placeholders in templates.
type Generator struct {
	urlTemplate           GenCompiledTemplate
	method                string
	bodyTemplate          GenCompiledTemplate
	headerTemplates       []GenCompiledHeader
	cookieTemplate        GenCompiledTemplate
	fuzzedHeaderIndices   []int
	hasCookiePlaceholders bool
	hasBodyPlaceholders   bool
	isJSONBody            bool

	fooWords  []string
	barWords  []string
	bazWords  []string
	buzzWords []string
}

// NewGenerator creates a new Generator.
func NewGenerator(
	urlTemplate string,
	method string,
	bodyTemplate string,
	headerTemplates http.Header,
	cookieTemplate string,
	fooWords []string,
	barWords []string,
	bazWords []string,
	buzzWords []string,
) *Generator {
	if method == "" {
		method = http.MethodGet
	}

	var keys []string
	for k := range headerTemplates {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var compiledHeaders []GenCompiledHeader
	var fuzzedHeaderIndices []int
	for _, k := range keys {
		values := headerTemplates[k]
		ch := GenCompiledHeader{
			Key: CompileGenTemplate(k),
		}
		for _, v := range values {
			ch.Values = append(ch.Values, CompileGenTemplate(v))
		}
		if ch.HasPlaceholders() {
			fuzzedHeaderIndices = append(fuzzedHeaderIndices, len(compiledHeaders))
		}
		compiledHeaders = append(compiledHeaders, ch)
	}

	bodyComp := CompileGenTemplate(bodyTemplate)
	cookieComp := CompileGenTemplate(cookieTemplate)

	return &Generator{
		urlTemplate:           CompileGenTemplate(urlTemplate),
		method:                method,
		bodyTemplate:          bodyComp,
		headerTemplates:       compiledHeaders,
		cookieTemplate:        cookieComp,
		fuzzedHeaderIndices:   fuzzedHeaderIndices,
		hasCookiePlaceholders: cookieComp.HasPlaceholders(),
		hasBodyPlaceholders:   bodyComp.HasPlaceholders(),
		isJSONBody:            isJSONString(bodyTemplate),
		fooWords:              fooWords,
		barWords:              barWords,
		bazWords:              bazWords,
		buzzWords:             buzzWords,
	}
}

// Generate streams fuzzed jobs to the jobs channel.
func (g *Generator) Generate(ctx context.Context, primaryChan <-chan string, jobs chan<- RequestDTO) {
	fooList := g.fooWords
	if len(fooList) == 0 {
		fooList = []string{""}
	}
	barList := g.barWords
	if len(barList) == 0 {
		barList = []string{""}
	}
	bazList := g.bazWords
	if len(bazList) == 0 {
		bazList = []string{""}
	}
	buzzList := g.buzzWords
	if len(buzzList) == 0 {
		buzzList = []string{""}
	}

	if primaryChan != nil {
		for {
			select {
			case <-ctx.Done():
				return
			case w, ok := <-primaryChan:
				if !ok {
					return
				}
				g.generatePermutations(ctx, w, fooList, barList, bazList, buzzList, jobs)
			}
		}
	} else {
		g.generatePermutations(ctx, "", fooList, barList, bazList, buzzList, jobs)
	}
}

func (g *Generator) generatePermutations(
	ctx context.Context,
	fuzzVal string,
	fooList, barList, bazList, buzzList []string,
	jobs chan<- RequestDTO,
) {
	for _, fooVal := range fooList {
		for _, barVal := range barList {
			for _, bazVal := range bazList {
				for _, buzzVal := range buzzList {
					select {
					case <-ctx.Done():
						return
					default:
					}

					values := [5]string{fuzzVal, fooVal, barVal, bazVal, buzzVal}

					urlStr := g.urlTemplate.Render(values)
					if _, err := url.Parse(urlStr); err != nil {
						atomic.AddInt64(&stats.GlobalInstrumentation.InvalidWords, 1)
						continue
					}

					var bodyStr string
					if len(g.bodyTemplate.Segments) > 0 {
						bodyStr = g.bodyTemplate.Render(values)
					}

					headers := make(http.Header)
					for _, ch := range g.headerTemplates {
						newK := ch.Key.Render(values)
						var newValues []string
						for _, val := range ch.Values {
							newValues = append(newValues, val.Render(values))
						}
						headers[newK] = newValues
					}

					var cookieStr string
					if len(g.cookieTemplate.Segments) > 0 {
						cookieStr = g.cookieTemplate.Render(values)
					}

					var fuzzData *engine.FuzzData
					if len(g.fuzzedHeaderIndices) > 0 || g.hasCookiePlaceholders || g.hasBodyPlaceholders {
						var fields []engine.FuzzField
						for _, idx := range g.fuzzedHeaderIndices {
							ch := g.headerTemplates[idx]
							newK := ch.Key.Render(values)
							for _, val := range ch.Values {
								newV := val.Render(values)
								fields = append(fields, engine.FuzzField{
									Location: engine.LocationHeader,
									Name:     newK,
									Value:    newV,
								})
							}
						}
						if g.hasCookiePlaceholders && cookieStr != "" {
							name, val := parseCookieField(cookieStr)
							fields = append(fields, engine.FuzzField{
								Location: engine.LocationCookie,
								Name:     name,
								Value:    val,
							})
						}
						if g.hasBodyPlaceholders && bodyStr != "" {
							loc := engine.LocationBody
							if g.isJSONBody {
								loc = engine.LocationJSON
							}
							fields = append(fields, engine.FuzzField{
								Location: loc,
								Value:    bodyStr,
							})
						}
						if len(fields) > 0 {
							fuzzData = &engine.FuzzData{Fields: fields}
						}
					}

					select {
					case <-ctx.Done():
						return
					case jobs <- RequestDTO{
						URL:      urlStr,
						Method:   g.method,
						Body:     bodyStr,
						Headers:  headers,
						Cookies:  []string{cookieStr},
						FuzzData: fuzzData,
					}:
					}
				}
			}
		}
	}
}

func parseCookieField(cookieStr string) (string, string) {
	if idx := strings.Index(cookieStr, "="); idx != -1 {
		return strings.TrimSpace(cookieStr[:idx]), strings.TrimSpace(cookieStr[idx+1:])
	}
	return "", strings.TrimSpace(cookieStr)
}

func isJSONString(s string) bool {
	trimmed := strings.TrimSpace(s)
	return (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"))
}
