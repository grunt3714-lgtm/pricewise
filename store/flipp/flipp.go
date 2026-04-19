// Package flipp is a shared backend that speaks to the public Flipp API
// (backflipp.wishabi.com). Flipp aggregates weekly ads from many
// retailers; per-ZIP availability varies by chain.
//
// One flipp.Store value represents one merchant at one ZIP. The adapter:
//  1. GET /flipp/flyers?postal_code=ZIP             → list active flyers
//  2. Filter flyers whose merchant matches ours
//  3. GET /flipp/flyers/{flyer_id}?postal_code=ZIP  → items per flyer
//  4. Normalize each priced item into store.Item
//
// Flyer items are sparse: many records are banner ads or category
// headers with empty price. We drop those. Unlike a store's own API the
// flyer JSON does not include pack size, regular price, or savings
// amount — those fields stay empty.
package flipp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/grunt3714-lgtm/pricewise/store"
)

const (
	apiBase       = "https://backflipp.wishabi.com/flipp"
	defaultLocale = "en-US"
)

// Store is a Flipp-backed adapter for a single merchant at one ZIP.
type Store struct {
	id       string // short ID surfaced in CLI output (e.g. "fredmeyer")
	merchant string // exact merchant name from the Flipp API ("Fred Meyer")
	zip      string
	locale   string
}

// Option lets callers override defaults.
type Option func(*Store)

func WithLocale(locale string) Option { return func(s *Store) { s.locale = locale } }

// New constructs a Flipp-backed Store for the given merchant + ZIP.
//
// `id` is the short name surfaced in CLI output ("fredmeyer"). `merchant`
// must match Flipp's merchant field (case-insensitive matching is used).
// `zip` is the postal code to query Flipp with — Flipp scopes availability
// by ZIP, so two shoppers in different cities see different flyer sets.
func New(id, merchant, zip string, opts ...Option) *Store {
	s := &Store{id: id, merchant: merchant, zip: zip, locale: defaultLocale}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *Store) Name() string           { return s.id }
func (s *Store) Backend() store.Backend { return store.BackendFlipp }

type flyerListResp struct {
	Flyers []flyerMeta `json:"flyers"`
}

type flyerMeta struct {
	ID        int64  `json:"id"`
	Merchant  string `json:"merchant"`
	Name      string `json:"name"`
	ValidFrom string `json:"valid_from"`
	ValidTo   string `json:"valid_to"`
}

type flyerItemsResp struct {
	Items []flyerItem `json:"items"`
}

type flyerItem struct {
	ID        int64           `json:"id"`
	FlyerID   int64           `json:"flyer_id"`
	Name      string          `json:"name"`
	ShortName string          `json:"short_name"`
	Brand     string          `json:"brand"`
	Price     json.RawMessage `json:"price"`    // sometimes string, sometimes number
	Discount  json.RawMessage `json:"discount"` // sometimes string, sometimes number
	SaleStory string          `json:"sale_story,omitempty"`
	ValidFrom string          `json:"valid_from"`
	ValidTo   string          `json:"valid_to"`
}

// rawToString coerces a JSON value that might be a string, number, or
// null into a plain string. Empty for null.
func rawToString(r json.RawMessage) string {
	s := strings.TrimSpace(string(r))
	if s == "" || s == "null" {
		return ""
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var out string
		if err := json.Unmarshal(r, &out); err == nil {
			return out
		}
	}
	return s
}

func (s *Store) listFlyers(ctx context.Context) ([]flyerMeta, error) {
	if s.zip == "" {
		return nil, fmt.Errorf("flipp %s: ZIP is required", s.id)
	}
	u := fmt.Sprintf("%s/flyers?postal_code=%s&locale=%s",
		apiBase, url.QueryEscape(s.zip), url.QueryEscape(s.locale))
	req, err := store.NewRequest(ctx, u, "application/json", "https://flipp.com/")
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
		return nil, fmt.Errorf("flipp %s: flyers list http %d: %s",
			s.id, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out flyerListResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("flipp %s: decode flyers: %w", s.id, err)
	}
	var mine []flyerMeta
	want := strings.TrimSpace(strings.ToLower(s.merchant))
	for _, f := range out.Flyers {
		if strings.TrimSpace(strings.ToLower(f.Merchant)) == want {
			mine = append(mine, f)
		}
	}
	return mine, nil
}

func (s *Store) fetchFlyer(ctx context.Context, flyerID int64) ([]flyerItem, error) {
	u := fmt.Sprintf("%s/flyers/%d?postal_code=%s&locale=%s",
		apiBase, flyerID, url.QueryEscape(s.zip), url.QueryEscape(s.locale))
	req, err := store.NewRequest(ctx, u, "application/json", "https://flipp.com/")
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
		return nil, fmt.Errorf("flipp %s: flyer %d http %d: %s",
			s.id, flyerID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out flyerItemsResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("flipp %s: decode flyer %d: %w", s.id, flyerID, err)
	}
	return out.Items, nil
}

func (s *Store) Fetch(ctx context.Context) ([]store.Item, error) {
	flyers, err := s.listFlyers(ctx)
	if err != nil {
		return nil, err
	}
	if len(flyers) == 0 {
		return nil, nil
	}

	var (
		mu    sync.Mutex
		items []store.Item
		errs  []error
		wg    sync.WaitGroup
	)
	for _, f := range flyers {
		f := f
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := s.fetchFlyer(ctx, f.ID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			for _, r := range raw {
				it, ok := normalize(s.id, f, r)
				if ok {
					items = append(items, it)
				}
			}
		}()
	}
	wg.Wait()
	if len(items) == 0 && len(errs) > 0 {
		return nil, errs[0]
	}
	return items, nil
}

func normalize(storeID string, f flyerMeta, r flyerItem) (store.Item, bool) {
	price := strings.TrimSpace(rawToString(r.Price))
	name := strings.TrimSpace(r.Name)
	if name == "" || price == "" {
		return store.Item{}, false
	}
	// Drop banner tiles whose "name" is literally the merchant name.
	if strings.EqualFold(name, f.Merchant) {
		return store.Item{}, false
	}
	var attrs []string
	if r.Brand != "" && !strings.EqualFold(r.Brand, name) {
		attrs = append(attrs, r.Brand)
	}
	if d := rawToString(r.Discount); d != "" && d != "0" {
		// Bare numeric discount is a percent off; stringify it.
		if _, err := json.Number(d).Int64(); err == nil {
			d = d + "% off"
		}
		attrs = append(attrs, d)
	}
	if r.SaleStory != "" {
		attrs = append(attrs, r.SaleStory)
	}
	if len(f.ValidTo) >= 10 {
		attrs = append(attrs, "thru "+f.ValidTo[:10])
	}
	return store.Item{
		Store:      storeID,
		Name:       name,
		SalePrice:  price,
		Category:   f.Name, // e.g. "Weekly Ad", "Big Book of Savings"
		Attributes: attrs,
	}, true
}
