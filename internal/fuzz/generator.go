package fuzz

import (
	"context"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/unsubble/searchit/internal/stats"
)

// Generator produces RequestDTO instances by replacing placeholders in templates.
type Generator struct {
	urlTemplate     GenCompiledTemplate
	method          string
	bodyTemplate    GenCompiledTemplate
	headerTemplates []GenCompiledHeader
	cookieTemplate  GenCompiledTemplate

	fooWords  []string
	barWords  []string
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
	buzzWords []string,
) *Generator {
	if method == "" {
		method = http.MethodGet
	}

	var compiledHeaders []GenCompiledHeader
	for k, values := range headerTemplates {
		ch := GenCompiledHeader{
			Key: CompileGenTemplate(k),
		}
		for _, v := range values {
			ch.Values = append(ch.Values, CompileGenTemplate(v))
		}
		compiledHeaders = append(compiledHeaders, ch)
	}

	return &Generator{
		urlTemplate:     CompileGenTemplate(urlTemplate),
		method:          method,
		bodyTemplate:    CompileGenTemplate(bodyTemplate),
		headerTemplates: compiledHeaders,
		cookieTemplate:  CompileGenTemplate(cookieTemplate),
		fooWords:        fooWords,
		barWords:        barWords,
		buzzWords:       buzzWords,
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
				g.generatePermutations(ctx, w, fooList, barList, buzzList, jobs)
			}
		}
	} else {
		g.generatePermutations(ctx, "", fooList, barList, buzzList, jobs)
	}
}

func (g *Generator) generatePermutations(
	ctx context.Context,
	fuzzVal string,
	fooList, barList, buzzList []string,
	jobs chan<- RequestDTO,
) {
	for _, fooVal := range fooList {
		for _, barVal := range barList {
			for _, buzzVal := range buzzList {
				select {
				case <-ctx.Done():
					return
				default:
				}

				values := [4]string{fuzzVal, fooVal, barVal, buzzVal}

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

				select {
				case <-ctx.Done():
					return
				case jobs <- RequestDTO{
					URL:     urlStr,
					Method:  g.method,
					Body:    bodyStr,
					Headers: headers,
					Cookies: []string{cookieStr},
				}:
				}
			}
		}
	}
}
