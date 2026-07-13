package sitemap_test

import (
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/sitemap"
)

func TestParseStream_Urlset(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url>
      <loc>http://www.example.com/</loc>
      <lastmod>2005-01-01</lastmod>
      <changefreq>monthly</changefreq>
      <priority>0.8</priority>
   </url>
   <url>
      <loc>http://www.example.com/catalog?item=12&amp;desc=vacation_hawaii</loc>
      <changefreq>weekly</changefreq>
   </url>
</urlset>`

	var items []sitemap.XMLItem
	err := sitemap.ParseStream(strings.NewReader(input), func(item sitemap.XMLItem) {
		items = append(items, item)
	})

	if err != nil {
		t.Fatalf("ParseStream() error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	item1 := items[0]
	if item1.IsSitemap {
		t.Error("item1 should not be IsSitemap")
	}
	if item1.Loc != "http://www.example.com/" {
		t.Errorf("item1 loc expected http://www.example.com/, got %q", item1.Loc)
	}
	if item1.LastMod != "2005-01-01" {
		t.Errorf("item1 lastmod expected 2005-01-01, got %q", item1.LastMod)
	}
	if item1.ChangeFreq != "monthly" {
		t.Errorf("item1 changefreq expected monthly, got %q", item1.ChangeFreq)
	}
	if item1.Priority != "0.8" {
		t.Errorf("item1 priority expected 0.8, got %q", item1.Priority)
	}

	item2 := items[1]
	if item2.Loc != "http://www.example.com/catalog?item=12&desc=vacation_hawaii" {
		t.Errorf("item2 loc expected http://www.example.com/catalog?item=12&desc=vacation_hawaii, got %q", item2.Loc)
	}
}

func TestParseStream_SitemapIndex(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <sitemap>
      <loc>http://www.example.com/sitemap1.xml.gz</loc>
      <lastmod>2004-10-01T18:23:17+00:00</lastmod>
   </sitemap>
   <sitemap>
      <loc>http://www.example.com/sitemap2.xml.gz</loc>
   </sitemap>
</sitemapindex>`

	var items []sitemap.XMLItem
	err := sitemap.ParseStream(strings.NewReader(input), func(item sitemap.XMLItem) {
		items = append(items, item)
	})

	if err != nil {
		t.Fatalf("ParseStream() error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	item1 := items[0]
	if !item1.IsSitemap {
		t.Error("item1 should be IsSitemap")
	}
	if item1.Loc != "http://www.example.com/sitemap1.xml.gz" {
		t.Errorf("item1 loc expected http://www.example.com/sitemap1.xml.gz, got %q", item1.Loc)
	}
	if item1.LastMod != "2004-10-01T18:23:17+00:00" {
		t.Errorf("item1 lastmod expected 2004-10-01T18:23:17+00:00, got %q", item1.LastMod)
	}

	item2 := items[1]
	if !item2.IsSitemap {
		t.Error("item2 should be IsSitemap")
	}
	if item2.Loc != "http://www.example.com/sitemap2.xml.gz" {
		t.Errorf("item2 loc expected http://www.example.com/sitemap2.xml.gz, got %q", item2.Loc)
	}
}

func TestParseStream_Malformed(t *testing.T) {
	input := `<urlset><url><loc>http://valid.com</loc></url><malformed>no-end`
	var items []sitemap.XMLItem
	err := sitemap.ParseStream(strings.NewReader(input), func(item sitemap.XMLItem) {
		items = append(items, item)
	})

	// Malformed XML should still return partial results (the valid url parsed before error)
	if err == nil {
		t.Error("expected parsing error due to malformed XML close tag")
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 parsed item before error, got %d", len(items))
	}
	if items[0].Loc != "http://valid.com" {
		t.Errorf("expected http://valid.com, got %q", items[0].Loc)
	}
}

func TestParseStream_Empty(t *testing.T) {
	input := ``
	var items []sitemap.XMLItem
	err := sitemap.ParseStream(strings.NewReader(input), func(item sitemap.XMLItem) {
		items = append(items, item)
	})

	if err != nil {
		t.Errorf("expected no error parsing empty stream, got %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseStream_MalformedXML_Extreme(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantItems []sitemap.XMLItem
		wantErr   bool
	}{
		{
			name: "Cut-off XML stream / premature EOF",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url>
      <loc>http://example.com/valid</loc>
   </url>
   <url>
      <loc>http://example.com/cut-off`,
			wantItems: []sitemap.XMLItem{
				{Loc: "http://example.com/valid", IsSitemap: false},
			},
			wantErr: true,
		},
		{
			name: "XML namespaces and custom tags ignored",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns:custom="http://custom.org">
   <url>
      <loc>http://example.com/item</loc>
      <custom:meta>some metadata</custom:meta>
      <unknown>ignored element</unknown>
   </url>
</urlset>`,
			wantItems: []sitemap.XMLItem{
				{Loc: "http://example.com/item", IsSitemap: false},
			},
			wantErr: false,
		},
		{
			name: "Invalid UTF-8 encoding in location element",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<urlset>
   <url>
      <loc>http://example.com/path-` + string([]byte{0xff, 0xfe, 0xfd}) + `</loc>
   </url>
</urlset>`,
			wantItems: nil,
			wantErr:   true,
		},
		{
			name: "Extreme whitespace, tabs, and newlines inside tags",
			input: `<?xml version="1.0" encoding="UTF-8"?>
<urlset>
   <url>
      <loc>
		
		http://example.com/whitespace
		
	  </loc>
	  <changefreq>
		monthly
	  </changefreq>
   </url>
</urlset>`,
			wantItems: []sitemap.XMLItem{
				{Loc: "http://example.com/whitespace", ChangeFreq: "monthly", IsSitemap: false},
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotItems []sitemap.XMLItem
			err := sitemap.ParseStream(strings.NewReader(tc.input), func(item sitemap.XMLItem) {
				gotItems = append(gotItems, item)
			})

			if (err != nil) != tc.wantErr {
				t.Errorf("ParseStream() error = %v, wantErr = %v", err, tc.wantErr)
			}

			if len(gotItems) != len(tc.wantItems) {
				t.Fatalf("item count mismatch: got %d, want %d.\nGot: %+v\nWant: %+v",
					len(gotItems), len(tc.wantItems), gotItems, tc.wantItems)
			}

			for i, got := range gotItems {
				want := tc.wantItems[i]
				if got.Loc != want.Loc || got.IsSitemap != want.IsSitemap || got.ChangeFreq != want.ChangeFreq {
					t.Errorf("got[%d] = %+v, want %+v", i, got, want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkParseStream_Standard(b *testing.B) {
	// A standard sitemap entry with 10 URLs
	input := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
   <url><loc>http://www.example.com/1</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
   <url><loc>http://www.example.com/2</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
   <url><loc>http://www.example.com/3</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
   <url><loc>http://www.example.com/4</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
   <url><loc>http://www.example.com/5</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
   <url><loc>http://www.example.com/6</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
   <url><loc>http://www.example.com/7</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
   <url><loc>http://www.example.com/8</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
   <url><loc>http://www.example.com/9</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
   <url><loc>http://www.example.com/10</loc><lastmod>2005-01-01</lastmod><changefreq>monthly</changefreq><priority>0.8</priority></url>
</urlset>`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = sitemap.ParseStream(strings.NewReader(input), func(item sitemap.XMLItem) {})
	}
}
