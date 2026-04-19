// Package moc is the Market of Choice adapter.
//
// MOC has no dedicated product API. Their weekly specials page is rendered
// by WordPress as a static HTML table inside a page whose content we can
// pull as JSON from /wp-json/wp/v2/pages/<id>. Staff hand-edit the table
// weekly, so formatting is inconsistent — expect leading spaces, typos,
// and multi-line cells.
package moc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/grunt3714-lgtm/pricewise/store"
)

const (
	name    = "moc"
	apiURL  = "https://marketofchoice.com/wp-json/wp/v2/pages/45481"
	referer = "https://marketofchoice.com/specials/weekly/"
)

type Store struct{}

func New() *Store { return &Store{} }

func (*Store) Name() string           { return name }
func (*Store) Backend() store.Backend { return store.BackendDirect }

// ServesZIP reports whether this adapter is relevant for the given ZIP.
// Market of Choice has 11 stores, all in Oregon — so any 97xxx ZIP.
func (*Store) ServesZIP(zip string) bool {
	return strings.HasPrefix(strings.TrimSpace(zip), "97")
}

type page struct {
	Modified    string `json:"modified_gmt"`
	Link        string `json:"link"`
	Title       struct{ Rendered string } `json:"title"`
	Content     struct{ Rendered string } `json:"content"`
}

func (s *Store) Fetch(ctx context.Context) ([]store.Item, error) {
	req, err := store.NewRequest(ctx, apiURL, "application/json", referer)
	if err != nil {
		return nil, err
	}
	resp, err := store.DefaultClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("moc: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, fmt.Errorf("moc: decode: %w", err)
	}
	return parse(p.Content.Rendered), nil
}

var (
	rowRE  = regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	cellRE = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
	wsRE   = regexp.MustCompile(`\s+`)
)

func parse(html string) []store.Item {
	var items []store.Item
	for _, row := range rowRE.FindAllStringSubmatch(html, -1) {
		var cells []string
		for _, c := range cellRE.FindAllStringSubmatch(row[1], -1) {
			text := store.StripTags(c[1])
			text = store.DecodeEntities(text)
			text = wsRE.ReplaceAllString(text, " ")
			cells = append(cells, strings.TrimSpace(text))
		}
		for len(cells) < 6 {
			cells = append(cells, "")
		}
		if len(cells) < 5 || cells[0] == "" {
			continue
		}
		var attrs []string
		if cells[5] != "" {
			for _, a := range strings.Split(cells[5], ",") {
				if t := strings.TrimSpace(a); t != "" {
					attrs = append(attrs, t)
				}
			}
		}
		items = append(items, store.Item{
			Store:        name,
			Name:         cells[0],
			Size:         cells[1],
			SalePrice:    cells[2],
			RegularPrice: cells[3],
			Savings:      cells[4],
			Attributes:   attrs,
			URL:          referer,
		})
	}
	return items
}
