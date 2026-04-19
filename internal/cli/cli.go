// Package cli holds the shared flag-parsing, filtering, and output logic
// used by both the aggregator and each per-store binary.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/grunt3714-lgtm/pricewise/store"
)

// Filters carries the common CLI knobs.
type Filters struct {
	Search     string
	Attribute  string
	Category   string
	MinSavings float64
	MaxPrice   float64
	Store      string        // substring of store name; mostly for aggregator
	SortBy     string        // "savings" (default), "price", "name", "store"
	JSON       bool
	Backend    store.Backend // auto | direct | flipp
	NoFallback bool          // if true, fail rather than trying other backends
	ZIP        string        // postal code; required for most adapters
}

// ResolveZIP returns the ZIP to use: flag takes precedence, then the
// PRICEWISE_ZIP env var. Returns "" if neither is set; callers should
// treat that as an error.
func (f *Filters) ResolveZIP() string {
	if f.ZIP != "" {
		return f.ZIP
	}
	return strings.TrimSpace(os.Getenv("PRICEWISE_ZIP"))
}

// Apply is the shared filter+sort implementation.
func (f Filters) Apply(items []store.Item) []store.Item {
	out := items[:0:0]
	search := strings.ToLower(f.Search)
	attr := strings.ToLower(f.Attribute)
	cat := strings.ToLower(f.Category)
	storeFilter := strings.ToLower(f.Store)
	for _, it := range items {
		if search != "" && !strings.Contains(strings.ToLower(it.Name), search) {
			continue
		}
		if attr != "" && !containsFold(it.Attributes, attr) {
			continue
		}
		if cat != "" && !strings.Contains(strings.ToLower(it.Category), cat) {
			continue
		}
		if storeFilter != "" && !strings.Contains(strings.ToLower(it.Store), storeFilter) {
			continue
		}
		if it.SavingsFloat() < f.MinSavings {
			continue
		}
		if f.MaxPrice > 0 {
			if p := it.SalePriceFloat(); p > 0 && p > f.MaxPrice {
				continue
			}
		}
		out = append(out, it)
	}
	sortItems(out, f.SortBy)
	return out
}

func containsFold(hay []string, needle string) bool {
	for _, h := range hay {
		if strings.Contains(strings.ToLower(h), needle) {
			return true
		}
	}
	return false
}

func sortItems(items []store.Item, by string) {
	switch by {
	case "price":
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].SalePriceFloat() < items[j].SalePriceFloat()
		})
	case "name":
		sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	case "store":
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Store != items[j].Store {
				return items[i].Store < items[j].Store
			}
			return items[j].SavingsFloat() < items[i].SavingsFloat()
		})
	default: // "savings"
		sort.SliceStable(items, func(i, j int) bool {
			return items[j].SavingsFloat() < items[i].SavingsFloat()
		})
	}
}

// ParseFilters binds the standard flag set.
func ParseFilters(fs *flag.FlagSet, includeStore bool) *Filters {
	f := &Filters{Backend: store.BackendAuto}
	fs.StringVar(&f.Search, "search", "", "substring match on item name")
	fs.StringVar(&f.Search, "s", "", "shorthand for --search")
	fs.StringVar(&f.Attribute, "attribute", "", "filter by attribute (Organic, Vegan, G-Free, ...)")
	fs.StringVar(&f.Attribute, "a", "", "shorthand for --attribute")
	fs.StringVar(&f.Category, "category", "", "filter by category (aisle/department)")
	fs.Float64Var(&f.MinSavings, "min-savings", 0, "minimum savings in dollars")
	fs.Float64Var(&f.MinSavings, "m", 0, "shorthand for --min-savings")
	fs.Float64Var(&f.MaxPrice, "max-price", 0, "maximum sale price in dollars (0 = no cap)")
	fs.StringVar(&f.SortBy, "sort", "savings", "sort by savings|price|name|store")
	fs.BoolVar(&f.JSON, "json", false, "output JSON")
	fs.Func("backend", "preferred backend: auto|direct|flipp (default: auto)", func(v string) error {
		switch store.Backend(v) {
		case store.BackendAuto, store.BackendDirect, store.BackendFlipp:
			f.Backend = store.Backend(v)
			return nil
		}
		return fmt.Errorf("unknown backend %q (want auto|direct|flipp)", v)
	})
	fs.BoolVar(&f.NoFallback, "no-fallback", false, "fail rather than trying a different backend on error")
	fs.StringVar(&f.ZIP, "zip", "", "postal code (default: $PRICEWISE_ZIP)")
	if includeStore {
		fs.StringVar(&f.Store, "stores", "", "comma-separated list of stores to include (default: all)")
	}
	return f
}

// Configure applies backend/fallback flags to a slice of Stores. Stores
// that implement *store.MultiStore are reconfigured; single-backend
// adapters are returned unchanged. Returns the potentially-rewrapped
// slice.
func Configure(stores []store.Store, f *Filters) []store.Store {
	out := make([]store.Store, 0, len(stores))
	for _, s := range stores {
		if m, ok := s.(*store.MultiStore); ok {
			m = m.WithBackend(f.Backend).WithFallback(!f.NoFallback)
			out = append(out, m)
		} else {
			out = append(out, s)
		}
	}
	return out
}

// Print renders items as either JSON or a tabular text report.
func Print(items []store.Item, asJSON bool, showStore bool) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if items == nil {
			items = []store.Item{}
		}
		_ = enc.Encode(items)
		return
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "(no items)")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if showStore {
		fmt.Fprintln(w, "STORE\tITEM\tSIZE\tSALE\tREG\tSAVE\tATTR")
	} else {
		fmt.Fprintln(w, "ITEM\tSIZE\tSALE\tREG\tSAVE\tATTR")
	}
	for _, i := range items {
		attrs := strings.Join(i.Attributes, ", ")
		name := truncate(i.Name, 60)
		if showStore {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				i.Store, name, i.Size, i.SalePrice, i.RegularPrice, i.Savings, attrs)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				name, i.Size, i.SalePrice, i.RegularPrice, i.Savings, attrs)
		}
	}
	_ = w.Flush()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// Factory builds a Store given the resolved ZIP. Each per-store binary
// passes one of these so its adapter can be constructed after flag
// parsing rather than before.
type Factory func(zip string) store.Store

// RunSingle is the entry point for a per-store CLI binary. It parses
// flags, resolves the ZIP, constructs the store via factory, and runs
// one Fetch through the shared filter/print stack.
func RunSingle(name string, factory Factory, args []string) int {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	f := ParseFilters(fs, false)
	_ = fs.Parse(args)

	zip := f.ResolveZIP()
	if zip == "" {
		fmt.Fprintln(os.Stderr, "error: --zip is required (or set $PRICEWISE_ZIP)")
		return 2
	}

	s := factory(zip)
	if !store.ServesZIP(s, zip) {
		fmt.Fprintf(os.Stderr, "error: %s does not serve ZIP %s\n", s.Name(), zip)
		return 1
	}
	s = Configure([]store.Store{s}, f)[0]

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	items, err := s.Fetch(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	Print(f.Apply(items), f.JSON, false)
	return 0
}

// FetchAll runs every store concurrently and returns a merged slice. Per-store
// errors don't fail the whole run; they're returned as a map for reporting.
func FetchAll(ctx context.Context, stores []store.Store) ([]store.Item, map[string]error) {
	var (
		mu    sync.Mutex
		all   []store.Item
		errs  = map[string]error{}
		wg    sync.WaitGroup
	)
	for _, s := range stores {
		s := s
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := s.Fetch(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[s.Name()] = err
				return
			}
			all = append(all, items...)
		}()
	}
	wg.Wait()
	return all, errs
}
