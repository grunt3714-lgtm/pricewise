// Command pricewise aggregates weekly-specials data from every
// supported grocery store near a given ZIP and prints a merged report.
//
// Usage:
//
//	pricewise specials [--zip ZIP] [--stores LIST] [--search STR] ...
//	pricewise stores   [--zip ZIP]    list stores active for a ZIP
//	pricewise help
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/grunt3714-lgtm/pricewise/internal/cli"
	"github.com/grunt3714-lgtm/pricewise/store"
	"github.com/grunt3714-lgtm/pricewise/store/capella"
	"github.com/grunt3714-lgtm/pricewise/store/flipp"
	"github.com/grunt3714-lgtm/pricewise/store/kroger"
	"github.com/grunt3714-lgtm/pricewise/store/moc"
)

// registry returns every supported adapter constructed for the given
// ZIP. Region-locked adapters (those implementing store.ZIPGated) are
// filtered out here if they don't serve the ZIP; that keeps the --stores
// listing honest and avoids useless Fetch calls.
//
// Alphabetical by ID for stable output.
func registry(zip string) []store.Store {
	all := []store.Store{
		flipp.New("albertsons", "Albertsons", zip),
		flipp.New("aldi", "ALDI", zip),
		capella.New(),
		flipp.New("costco", "Costco", zip),
		flipp.New("food4less", "Food 4 Less", zip),
		// Fred Meyer: prefer Kroger direct (full catalog, needs
		// KROGER_CLIENT_ID/SECRET); fall back to Flipp (weekly ad
		// only, zero setup) when credentials aren't present.
		store.NewMulti("fredmeyer",
			kroger.New(zip),
			flipp.New("fredmeyer", "Fred Meyer", zip)),
		flipp.New("freshthyme", "Fresh Thyme Market", zip),
		flipp.New("groceryoutlet", "Grocery Outlet", zip),
		flipp.New("jewelosco", "Jewel-Osco", zip),
		flipp.New("marianos", "Mariano's", zip),
		flipp.New("meijer", "Meijer", zip),
		moc.New(),
		flipp.New("petesfresh", "Pete's Fresh Market", zip),
		flipp.New("safeway", "Safeway", zip),
		flipp.New("tonysfresh", "Tony's Fresh Market", zip),
		flipp.New("walmart", "Walmart", zip),
	}
	out := make([]store.Store, 0, len(all))
	for _, s := range all {
		if store.ServesZIP(s, zip) {
			out = append(out, s)
		}
	}
	return out
}

func usage() {
	fmt.Fprintln(os.Stderr, `pricewise — aggregated weekly-specials CLI for grocery stores near any US/Canada ZIP

usage:
  pricewise specials [flags]
  pricewise stores [--zip ZIP]     list stores active for a ZIP
  pricewise help

flags (for 'specials'):
  --zip ZIP             postal code (default: $PRICEWISE_ZIP; required)
  --stores LIST         comma-separated store IDs to include (default: all)
  -s, --search STR      substring match on item name
  -a, --attribute STR   filter by attribute
  --category STR        filter by category / aisle / flyer name
  -m, --min-savings N   minimum savings in dollars
  --max-price N         max sale price in dollars (0 = no cap)
  --sort KEY            savings | price | name | store  (default: savings)
  --backend KEY         auto | direct | flipp    (default: auto)
  --no-fallback         don't try other backends on error
  --json                output JSON

Set $PRICEWISE_ZIP to avoid passing --zip on every invocation.
For Fred Meyer direct (richer data), set KROGER_CLIENT_ID and
KROGER_CLIENT_SECRET from https://developer.kroger.com/.`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "specials":
		os.Exit(cmdSpecials(os.Args[2:]))
	case "stores":
		os.Exit(cmdStores(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func cmdStores(args []string) int {
	fs := flag.NewFlagSet("stores", flag.ExitOnError)
	f := cli.ParseFilters(fs, true)
	_ = fs.Parse(args)
	zip := f.ResolveZIP()
	if zip == "" {
		fmt.Fprintln(os.Stderr, "error: --zip is required (or set $PRICEWISE_ZIP)")
		return 2
	}
	for _, s := range registry(zip) {
		fmt.Println(s.Name())
	}
	return 0
}

func cmdSpecials(args []string) int {
	fs := flag.NewFlagSet("specials", flag.ExitOnError)
	f := cli.ParseFilters(fs, true)
	_ = fs.Parse(args)

	zip := f.ResolveZIP()
	if zip == "" {
		fmt.Fprintln(os.Stderr, "error: --zip is required (or set $PRICEWISE_ZIP)")
		return 2
	}

	all := registry(zip)
	selected := selectStores(all, f.Store)
	if len(selected) == 0 {
		fmt.Fprintf(os.Stderr, "no matching stores for ZIP %s\n", zip)
		return 1
	}
	selected = cli.Configure(selected, f)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	items, errs := cli.FetchAll(ctx, selected)

	// Clear the store filter before Apply — we already restricted the
	// registry above.
	f.Store = ""
	items = f.Apply(items)

	cli.Print(items, f.JSON, true)

	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "errors:")
		ids := make([]string, 0, len(errs))
		for k := range errs {
			ids = append(ids, k)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", id, errs[id])
		}
	}
	return 0
}

// selectStores filters the registry by a comma-separated allowlist.
// Empty filter → all stores. Unknown IDs are ignored silently (common
// when users script this).
func selectStores(all []store.Store, csv string) []store.Store {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return all
	}
	want := map[string]bool{}
	for _, p := range strings.Split(csv, ",") {
		if id := strings.ToLower(strings.TrimSpace(p)); id != "" {
			want[id] = true
		}
	}
	var out []store.Store
	for _, s := range all {
		if want[strings.ToLower(s.Name())] {
			out = append(out, s)
		}
	}
	return out
}
