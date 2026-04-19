// Package store defines the common interface implemented by every grocery
// store adapter in this module.
package store

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Store is implemented by each grocery-store adapter.
type Store interface {
	// Name returns a short, stable identifier (e.g. "moc", "capella").
	Name() string
	// Fetch returns the current on-sale items for this store.
	Fetch(ctx context.Context) ([]Item, error)
}

// ZIPGated is implemented by region-locked adapters that are only
// meaningful for certain ZIP codes. The aggregator consults this before
// including the store in a run. Adapters that don't implement this
// interface are considered universal (e.g. Flipp, Kroger).
type ZIPGated interface {
	ServesZIP(zip string) bool
}

// ServesZIP reports whether s is willing to return data for the given
// ZIP. Stores without a ZIPGated implementation serve anywhere.
func ServesZIP(s Store, zip string) bool {
	if g, ok := s.(ZIPGated); ok {
		return g.ServesZIP(zip)
	}
	return true
}

// Item is the normalized shape every adapter emits.
//
// Not every store provides every field. Missing fields are zero-valued.
type Item struct {
	Store        string   `json:"store"`
	Name         string   `json:"name"`
	Size         string   `json:"size,omitempty"`
	SalePrice    string   `json:"sale_price,omitempty"`
	RegularPrice string   `json:"regular_price,omitempty"`
	Savings      string   `json:"savings,omitempty"`
	Attributes   []string `json:"attributes,omitempty"`
	Category     string   `json:"category,omitempty"`
	URL          string   `json:"url,omitempty"`
}

// SavingsFloat parses the first decimal number out of Savings. Useful for
// --min-savings filtering across stores with different formatting.
func (i Item) SavingsFloat() float64 {
	m := savingsRE.FindString(i.Savings)
	v, _ := strconv.ParseFloat(m, 64)
	return v
}

// SalePriceFloat parses the first decimal number out of SalePrice.
func (i Item) SalePriceFloat() float64 {
	m := savingsRE.FindString(i.SalePrice)
	v, _ := strconv.ParseFloat(m, 64)
	return v
}

var savingsRE = regexp.MustCompile(`[\d.]+`)

// UA is the shared User-Agent used by every adapter. Several grocery sites
// sit behind a WAF (Cloudflare, Sucuri) that 403s plain curl/Go defaults.
const UA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// DefaultClient returns an *http.Client with a sensible timeout. Adapters
// can use this directly or wrap it with per-store middleware.
func DefaultClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

// NewRequest builds a GET request with the shared UA and an Accept header.
// `accept` typically is "application/json" or "text/html,*/*".
func NewRequest(ctx context.Context, url, accept, referer string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UA)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	return req, nil
}

// ReadAll is a small helper for adapter code.
func ReadAll(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	return string(b), err
}

// DecodeEntities handles the few HTML entities that show up in real adapter
// output. Not a full decoder — just the common cases.
func DecodeEntities(s string) string {
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&#038;", "&")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&rsquo;", "'")
	s = strings.ReplaceAll(s, "&lsquo;", "'")
	s = strings.ReplaceAll(s, "&ldquo;", "\"")
	s = strings.ReplaceAll(s, "&rdquo;", "\"")
	return s
}

// StripTags removes all HTML tags.
var tagRE = regexp.MustCompile(`<[^>]+>`)

func StripTags(s string) string {
	return tagRE.ReplaceAllString(s, "")
}
