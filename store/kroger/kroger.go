// Package kroger is the official Kroger API adapter, used as the direct
// backend for Fred Meyer (a Kroger banner). Unlike the Flipp adapter
// this pulls the full SKU catalog with store-level pricing, pack sizes,
// and real promo data — not just weekly-ad-featured items.
//
// Authentication is OAuth 2 client-credentials. Register a free
// developer app at https://developer.kroger.com/ to obtain a client ID
// and secret, then export them:
//
//	export KROGER_CLIENT_ID=...
//	export KROGER_CLIENT_SECRET=...
//
// Without those vars the adapter returns store.ErrBackendUnavailable,
// which the MultiStore fallback chain handles gracefully by trying
// the next backend (typically Flipp).
//
// The location is auto-resolved from the supplied ZIP via the Locations
// API. Override with KROGER_LOCATION_ID to pin a specific store.
package kroger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/grunt3714-lgtm/pricewise/store"
)

const (
	name = "fredmeyer"

	tokenURL     = "https://api.kroger.com/v1/connect/oauth2/token"
	productsURL  = "https://api.kroger.com/v1/products"
	locationsURL = "https://api.kroger.com/v1/locations"

	productCompactScope = "product.compact"

	// chainFRED is the Kroger chain filter for Fred Meyer. Other banners
	// would use different strings (e.g. KROGER, HARRIS, RALPHS).
	chainFRED = "FRED"
)

// Store is the Kroger-backed adapter for Fred Meyer.
type Store struct {
	clientID     string
	clientSecret string
	zip          string

	mu         sync.Mutex
	token      string
	tokenExp   time.Time
	locationID string // resolved on first use (or from KROGER_LOCATION_ID)
}

// New reads credentials from the environment and returns a Store bound
// to the given ZIP. Fetch resolves the ZIP to a Fred Meyer location ID
// on first use (cached thereafter).
func New(zip string) *Store {
	return &Store{
		clientID:     strings.TrimSpace(os.Getenv("KROGER_CLIENT_ID")),
		clientSecret: strings.TrimSpace(os.Getenv("KROGER_CLIENT_SECRET")),
		zip:          zip,
		locationID:   strings.TrimSpace(os.Getenv("KROGER_LOCATION_ID")),
	}
}

func (*Store) Name() string           { return name }
func (*Store) Backend() store.Backend { return store.BackendDirect }

// configured reports whether the adapter has credentials available.
func (s *Store) configured() bool {
	return s.clientID != "" && s.clientSecret != ""
}

// Fetch returns current promo products for the ZIP's nearest Fred Meyer.
// Kroger's Products API is query-driven (no "all items" endpoint), so we
// sweep a set of broad seed terms and deduplicate.
func (s *Store) Fetch(ctx context.Context) ([]store.Item, error) {
	if !s.configured() {
		return nil, fmt.Errorf("%w: set KROGER_CLIENT_ID and KROGER_CLIENT_SECRET to enable Kroger direct backend",
			store.ErrBackendUnavailable)
	}
	if err := s.ensureToken(ctx); err != nil {
		return nil, err
	}
	if err := s.ensureLocation(ctx); err != nil {
		return nil, err
	}

	seeds := []string{
		"milk", "egg", "bread", "cheese", "yogurt", "butter", "chicken",
		"beef", "pork", "fish", "banana", "apple", "orange", "potato",
		"onion", "tomato", "pepper", "lettuce", "carrot", "berry",
		"coffee", "tea", "juice", "water", "soda", "beer", "wine",
		"pasta", "rice", "cereal", "oil", "sauce", "soup", "chips",
		"ice cream", "pizza",
	}

	var (
		mu    sync.Mutex
		found = make(map[string]store.Item)
		errs  []error
		sem   = make(chan struct{}, 6) // 6 concurrent requests max
		wg    sync.WaitGroup
	)
	for _, seed := range seeds {
		seed := seed
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			items, err := s.searchProducts(ctx, seed)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			for _, it := range items {
				if _, ok := found[it.URL]; !ok {
					found[it.URL] = it
				}
			}
		}()
	}
	wg.Wait()
	if len(found) == 0 && len(errs) > 0 {
		return nil, errs[0]
	}
	out := make([]store.Item, 0, len(found))
	for _, it := range found {
		out = append(out, it)
	}
	return out, nil
}

// ensureToken fetches a new OAuth token if one isn't cached or the cached
// one is within 30s of expiring.
func (s *Store) ensureToken(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && time.Now().Add(30*time.Second).Before(s.tokenExp) {
		return nil
	}
	body := strings.NewReader(url.Values{
		"grant_type": {"client_credentials"},
		"scope":      {productCompactScope},
	}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := store.DefaultClient().Do(req)
	if err != nil {
		return fmt.Errorf("kroger token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("kroger token http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return fmt.Errorf("kroger token decode: %w", err)
	}
	s.token = tr.AccessToken
	s.tokenExp = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return nil
}

// ensureLocation picks the nearest Fred Meyer for the bound ZIP and
// caches it, unless KROGER_LOCATION_ID was set explicitly.
func (s *Store) ensureLocation(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locationID != "" {
		return nil
	}
	if s.zip == "" {
		return fmt.Errorf("kroger: neither KROGER_LOCATION_ID nor a ZIP is set")
	}
	q := url.Values{}
	q.Set("filter.zipCode.near", s.zip)
	q.Set("filter.chain", chainFRED)
	q.Set("filter.limit", "1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		locationsURL+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/json")

	resp, err := store.DefaultClient().Do(req)
	if err != nil {
		return fmt.Errorf("kroger locations: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("kroger locations http %d: %s",
			resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var lr struct {
		Data []struct {
			LocationID string `json:"locationId"`
			Name       string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return fmt.Errorf("kroger locations decode: %w", err)
	}
	if len(lr.Data) == 0 {
		return fmt.Errorf("kroger: no Fred Meyer found near ZIP %s", s.zip)
	}
	s.locationID = lr.Data[0].LocationID
	return nil
}

// productsResp mirrors the subset of the Products API we consume.
type productsResp struct {
	Data []struct {
		ProductID   string   `json:"productId"`
		UPC         string   `json:"upc"`
		Brand       string   `json:"brand"`
		Description string   `json:"description"`
		Categories  []string `json:"categories"`
		Items       []struct {
			ItemID string `json:"itemId"`
			Size   string `json:"size"`
			Price  struct {
				Regular float64 `json:"regular"`
				Promo   float64 `json:"promo"`
			} `json:"price"`
			SoldBy string `json:"soldBy"`
		} `json:"items"`
	} `json:"data"`
}

func (s *Store) searchProducts(ctx context.Context, term string) ([]store.Item, error) {
	q := url.Values{}
	q.Set("filter.term", term)
	q.Set("filter.locationId", s.locationID)
	q.Set("filter.limit", "50")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		productsURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/json")

	resp, err := store.DefaultClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("kroger products %q: %w", term, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("kroger products %q http %d: %s",
			term, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var pr productsResp
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("kroger products decode: %w", err)
	}

	var out []store.Item
	for _, p := range pr.Data {
		if len(p.Items) == 0 {
			continue
		}
		it := p.Items[0]
		if it.Price.Regular == 0 {
			continue
		}
		sale := it.Price.Promo
		if sale == 0 || sale >= it.Price.Regular {
			continue
		}
		savings := it.Price.Regular - sale
		attrs := make([]string, 0, 1+len(p.Categories))
		seen := map[string]bool{}
		add := func(s string) {
			s = strings.TrimSpace(s)
			if s == "" || seen[strings.ToLower(s)] {
				return
			}
			seen[strings.ToLower(s)] = true
			attrs = append(attrs, s)
		}
		add(p.Brand)
		for _, c := range p.Categories {
			add(c)
		}
		out = append(out, store.Item{
			Store:        name,
			Name:         p.Description,
			Size:         it.Size,
			SalePrice:    fmt.Sprintf("%.2f", sale),
			RegularPrice: fmt.Sprintf("%.2f", it.Price.Regular),
			Savings:      fmt.Sprintf("$%.2f", savings),
			Attributes:   attrs,
			URL:          "https://www.fredmeyer.com/p/-/" + p.ProductID,
		})
	}
	return out, nil
}
