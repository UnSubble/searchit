package sitemap

import (
	"encoding/xml"
	"io"
	"strings"
)

// XMLItem represents a parsed sitemap or url entry.
type XMLItem struct {
	IsSitemap  bool // true if it is nested in <sitemap>, false if in <url>
	Loc        string
	LastMod    string
	ChangeFreq string
	Priority   string
}

// ParseStream reads sitemap XML documents from r streamingly.
// It yields items as they are fully parsed.
// It is designed to be highly memory-efficient, not building a DOM.
func ParseStream(r io.Reader, callback func(XMLItem)) error {
	dec := xml.NewDecoder(r)

	var (
		inLoc, inLastMod, inChangeFreq, inPriority bool
		inUrl                                      bool
		item                                       XMLItem
	)

	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch se := t.(type) {
		case xml.StartElement:
			name := strings.ToLower(se.Name.Local)
			switch name {
			case "url":
				inUrl = true
				item = XMLItem{IsSitemap: false}
			case "sitemap":
				item = XMLItem{IsSitemap: true}
			case "loc":
				inLoc = true
			case "lastmod":
				inLastMod = true
			case "changefreq":
				inChangeFreq = true
			case "priority":
				inPriority = true
			}

		case xml.EndElement:
			name := strings.ToLower(se.Name.Local)
			switch name {
			case "url":
				inUrl = false
				item.Loc = strings.TrimSpace(item.Loc)
				item.LastMod = strings.TrimSpace(item.LastMod)
				item.ChangeFreq = strings.TrimSpace(item.ChangeFreq)
				item.Priority = strings.TrimSpace(item.Priority)
				callback(item)
			case "sitemap":
				item.Loc = strings.TrimSpace(item.Loc)
				item.LastMod = strings.TrimSpace(item.LastMod)
				callback(item)
			case "loc":
				inLoc = false
			case "lastmod":
				inLastMod = false
			case "changefreq":
				inChangeFreq = false
			case "priority":
				inPriority = false
			}

		case xml.CharData:
			val := string(se)
			if inLoc {
				item.Loc += val
			} else if inLastMod {
				item.LastMod += val
			} else if inChangeFreq && inUrl {
				item.ChangeFreq += val
			} else if inPriority && inUrl {
				item.Priority += val
			}
		}
	}

	return nil
}
