// Package capella is the Capella Market adapter.
//
// Capella publishes a "text version" of their biweekly flyer at a stable URL
// (flyertext.html). It's a single <table> with columns:
//   Location | Thru | Brand | Type | Detail | Size | Sale
// The file is hand-maintained and uses HTML 3.2. Unlike the graphical
// flyer PDF it's lightweight and trivially parseable.
//
// Capella doesn't publish regular prices or savings — only sale prices —
// so those fields stay empty.
package capella

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/grunt3714-lgtm/pricewise/store"
)

const (
	name   = "capella"
	source = "https://www.capellamarket.com/flyertext.html"
)

type Store struct{}

func New() *Store { return &Store{} }

func (*Store) Name() string           { return name }
func (*Store) Backend() store.Backend { return store.BackendDirect }

// ServesZIP reports whether this adapter is relevant for the given ZIP.
// Capella has one store and is most useful to shoppers in the 974xx
// range (Lane County, Oregon).
func (*Store) ServesZIP(zip string) bool {
	return strings.HasPrefix(strings.TrimSpace(zip), "974")
}

func (s *Store) Fetch(ctx context.Context) ([]store.Item, error) {
	req, err := store.NewRequest(ctx, source, "text/html", "https://www.capellamarket.com/flyer/")
	if err != nil {
		return nil, err
	}
	resp, err := store.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("capella: http %d", resp.StatusCode)
	}
	html, err := store.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parse(html), nil
}

var (
	// The table is on one big line in flyertext.html; tolerate arbitrary
	// whitespace around the tags just in case the export format changes.
	rowRE  = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	cellRE = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	wsRE   = regexp.MustCompile(`\s+`)
)

func parse(html string) []store.Item {
	_ = io.Discard
	var items []store.Item
	for _, row := range rowRE.FindAllStringSubmatch(html, -1) {
		var cells []string
		for _, c := range cellRE.FindAllStringSubmatch(row[1], -1) {
			text := store.StripTags(c[1])
			text = store.DecodeEntities(text)
			text = wsRE.ReplaceAllString(text, " ")
			cells = append(cells, strings.TrimSpace(text))
		}
		if len(cells) != 7 {
			continue
		}
		// Skip header rows.
		if strings.EqualFold(cells[0], "Location") || strings.EqualFold(cells[6], "Sale") {
			continue
		}
		location, thru, brand, kind, detail, size, sale := cells[0], cells[1], cells[2], cells[3], cells[4], cells[5], cells[6]
		nameParts := []string{}
		for _, p := range []string{brand, kind, detail} {
			if p != "" && p != "-" {
				nameParts = append(nameParts, p)
			}
		}
		itemName := strings.Join(nameParts, " ")
		if itemName == "" {
			continue
		}
		attrs := []string{}
		if thru != "" {
			attrs = append(attrs, "thru "+thru)
		}
		items = append(items, store.Item{
			Store:     name,
			Name:      itemName,
			Size:      size,
			SalePrice: sale,
			Category:  location,
			Attributes: attrs,
			URL:       "https://www.capellamarket.com/flyer/",
		})
	}
	return items
}
