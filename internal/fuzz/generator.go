package fuzz

import (
	"context"
	"net/http"
	"strings"
)

// Generator produces Job instances by replacing placeholders in templates.
type Generator struct {
	urlTemplate     string
	method          string
	bodyTemplate    string
	headerTemplates http.Header

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
	fooWords []string,
	barWords []string,
	buzzWords []string,
) *Generator {
	if method == "" {
		method = http.MethodGet
	}
	return &Generator{
		urlTemplate:     urlTemplate,
		method:          method,
		bodyTemplate:    bodyTemplate,
		headerTemplates: headerTemplates,
		fooWords:        fooWords,
		barWords:        barWords,
		buzzWords:       buzzWords,
	}
}

// Generate streams fuzzed jobs to the jobs channel.
func (g *Generator) Generate(ctx context.Context, primaryChan <-chan string, jobs chan<- Job) {
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
	jobs chan<- Job,
) {
	for _, fooVal := range fooList {
		for _, barVal := range barList {
			for _, buzzVal := range buzzList {
				select {
				case <-ctx.Done():
					return
				default:
				}

				urlStr := g.replacePlaceholders(g.urlTemplate, fuzzVal, fooVal, barVal, buzzVal)

				var bodyBytes []byte
				if g.bodyTemplate != "" {
					bodyStr := g.replacePlaceholders(g.bodyTemplate, fuzzVal, fooVal, barVal, buzzVal)
					bodyBytes = []byte(bodyStr)
				}

				headers := make(http.Header)
				for k, values := range g.headerTemplates {
					newK := g.replacePlaceholders(k, fuzzVal, fooVal, barVal, buzzVal)
					var newValues []string
					for _, val := range values {
						newValues = append(newValues, g.replacePlaceholders(val, fuzzVal, fooVal, barVal, buzzVal))
					}
					headers[newK] = newValues
				}

				select {
				case <-ctx.Done():
					return
				case jobs <- Job{
					URL:     urlStr,
					Method:  g.method,
					Body:    bodyBytes,
					Headers: headers,
				}:
				}
			}
		}
	}
}

func (g *Generator) replacePlaceholders(template, fuzzVal, fooVal, barVal, buzzVal string) string {
	res := template
	res = strings.ReplaceAll(res, "FUZZ", fuzzVal)
	res = strings.ReplaceAll(res, "FOO", fooVal)
	res = strings.ReplaceAll(res, "BAR", barVal)
	res = strings.ReplaceAll(res, "BUZZ", buzzVal)
	return res
}
