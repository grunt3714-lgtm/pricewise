<div align="center">

# pricewise

**Weekly-specials data from grocery stores near any US/Canada ZIP — in one CLI.**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Backends](https://img.shields.io/badge/backends-Flipp%20%7C%20Kroger%20%7C%20direct-orange)](#backends-and---backend-flag)
[![AI-ready](https://img.shields.io/badge/AI--ready-SKILL.md-7c3aed?logo=anthropic&logoColor=white)](./SKILL.md)
[![License](https://img.shields.io/badge/license-MIT-informational)](./LICENSE)
[![Status](https://img.shields.io/badge/status-maintained-brightgreen)](#)

```
pricewise specials --zip 97401 --search eggs --min-savings 1 --sort savings
```

</div>

---

One aggregator (`pricewise`) plus per-store binaries. Each adapter
implements a common `store.Store` interface and returns a normalized
`store.Item`. With a ZIP, the aggregator pulls every active flyer in
the area — typically a few hundred to a couple thousand items in a few
seconds.

## Send your AI agent shopping 🛒

Read [SKILL.md](./SKILL.md) and follow the instructions so your agent
knows how to install the CLI and query weekly specials for any ZIP.

## Supported stores

National and regional chains are covered via [Flipp](https://flipp.com/)
wherever their flyers are available. Fred Meyer has an opt-in Kroger-API
backend for the full catalog. Two regional Oregon chains are included as
examples of direct-site adapters and only activate for Oregon ZIPs.

| Store              | ID              | Direct backend              | Flipp | Region          |
|--------------------|-----------------|-----------------------------|:-----:|-----------------|
| Fred Meyer         | `fredmeyer`     | Kroger API (opt-in, OAuth)  |   ✓   | anywhere        |
| Safeway            | `safeway`       | –                           |   ✓   | anywhere        |
| Albertsons         | `albertsons`    | –                           |   ✓   | anywhere        |
| Grocery Outlet     | `groceryoutlet` | –                           |   ✓   | anywhere        |
| Costco             | `costco`        | –                           |   ✓   | anywhere        |
| Walmart            | `walmart`       | –                           |   ✓   | anywhere        |
| ALDI               | `aldi`          | –                           |   ✓   | anywhere        |
| Meijer             | `meijer`        | –                           |   ✓   | Midwest         |
| Jewel-Osco         | `jewelosco`     | –                           |   ✓   | IL/IA/IN        |
| Mariano's          | `marianos`      | –                           |   ✓   | Chicago metro   |
| Food 4 Less        | `food4less`     | –                           |   ✓   | anywhere        |
| Fresh Thyme Market | `freshthyme`    | –                           |   ✓   | Midwest         |
| Pete's Fresh Market| `petesfresh`    | –                           |   ✓   | Chicago metro   |
| Tony's Fresh Market| `tonysfresh`    | –                           |   ✓   | Chicago metro   |
| Market of Choice   | `moc`           | WordPress REST API          |   –   | 97xxx (OR)      |
| Capella Market     | `capella`       | `flyertext.html`            |   –   | 974xx (OR)      |

### Data quality per backend

| Adapter           | Name | Brand | Size | Sale | Reg | Savings | Category |
|-------------------|:----:|:-----:|:----:|:----:|:---:|:-------:|:--------:|
| `kroger` (direct) |  ✓   |   ✓   |  ✓   |  ✓   |  ✓  |    ✓    |    ✓     |
| `moc` (direct)    |  ✓   |   –   |  ✓   |  ✓   |  ✓  |    ✓    |    –     |
| `capella` (direct)|  ✓   |   ✓   |  ✓   |  ✓   |  –  |    –    |    ✓     |
| `flipp`           |  ✓   |   ✓   |  –   |  ✓   |  –  |    –    |    ✓     |

Kroger direct is the richest. Flipp is the zero-setup fallback and
covers six chains at once.

## Install

```bash
# All CLIs at once. GOBIN controls where they land.
GOBIN=~/.local/bin go install github.com/grunt3714-lgtm/pricewise/cmd/...@latest

# Or just the aggregator.
GOBIN=~/.local/bin go install github.com/grunt3714-lgtm/pricewise/cmd/pricewise@latest
```

Set `PRICEWISE_ZIP` in your shell profile so you don't have to pass
`--zip` every time:

```bash
export PRICEWISE_ZIP=97401
```

## Usage

<details>
<summary><strong>Example output</strong></summary>

```
$ pricewise specials --search milk --min-savings 1 --sort savings
STORE      ITEM                                                   SIZE      SALE   REG    SAVE   ATTR
fredmeyer  Simple Truth Organic Vitamin D Whole Milk              1/2 gal   3.99   4.99   $1.00  Simple Truth Organic, Dairy, Natural & Organic
fredmeyer  Nancy's Plain Organic Probiotic Whole Milk Yogurt Tub  32 oz     4.99   5.99   $1.00  Nancy's, Natural & Organic, Dairy
```

</details>

```bash
# Aggregate across every store active for your ZIP.
pricewise specials --zip 97401

# Restrict to two stores.
pricewise specials --stores safeway,fredmeyer

# Substring match + minimum savings.
pricewise specials --search eggs --min-savings 1

# Cap the price instead.
pricewise specials --search bread --max-price 4

# Filter by attribute.
pricewise specials --attribute organic

# Sort by price instead of savings.
pricewise specials --search coffee --sort price

# JSON for piping.
pricewise specials --stores safeway --json | jq '.[0]'

# Force Flipp even where a direct backend exists.
pricewise specials --stores fredmeyer --backend flipp

# Force direct; error rather than silently falling back.
pricewise specials --stores fredmeyer --backend direct --no-fallback

# Which stores are active for a ZIP?
pricewise stores --zip 97401

# Per-store binary (same flags, no --stores).
safeway --search eggs --json
fredmeyer --search pepsi
```

## Backends and `--backend` flag

Each store may have multiple backends. The `--backend` flag picks which
one to try first:

| Flag                  | Behavior                                                        |
|-----------------------|-----------------------------------------------------------------|
| `--backend auto`      | default — each store's primary, fall back on error             |
| `--backend direct`    | prefer the store's own API/site; fall back to Flipp on error   |
| `--backend flipp`     | prefer Flipp; fall back to direct if Flipp has no record        |
| `--no-fallback`       | disable the fallback chain — clean error instead of substitute |

### Enabling Kroger direct for Fred Meyer

Register a free app at https://developer.kroger.com/, then export:

```bash
export KROGER_CLIENT_ID=...
export KROGER_CLIENT_SECRET=...
# Optional: pin a specific Fred Meyer location rather than letting the
# adapter pick the nearest one to $PRICEWISE_ZIP.
export KROGER_LOCATION_ID=...
```

Without these env vars, `fredmeyer` silently uses Flipp. With them, the
adapter auto-resolves the nearest Fred Meyer to your ZIP via the Kroger
Locations API and pulls the full promo catalog.

## Architecture

```
store/           Store interface, MultiStore, ZIPGated, shared helpers
store/flipp/     Flipp client (backflipp.wishabi.com), parameterized by merchant+ZIP
store/kroger/    Kroger OAuth client; auto-resolves ZIP → Fred Meyer location
store/moc/       Market of Choice WP REST client (Oregon-only example adapter)
store/capella/   Capella flyertext.html parser (Lane County, OR example adapter)
internal/cli/    Shared flag parsing, filtering, tabular + JSON output
cmd/pricewise/   Aggregator CLI
cmd/<store>/     Per-store binary (10-line wrapper)
```

Each adapter is responsible for two things:

1. Fetching upstream data.
2. Returning a slice of normalized `store.Item` records.

`store.MultiStore` composes multiple `BackendStore` implementations
under one stable store ID and routes `Fetch` calls based on the caller's
`--backend` / `--no-fallback` preferences. Adapters signal opt-in
backends (e.g. Kroger without credentials) by returning
`store.ErrBackendUnavailable`, which the fallback chain treats as a
normal miss.

`store.ZIPGated` lets regional adapters (MOC, Capella) limit themselves
to their service area; the aggregator filters them out of the registry
for ZIPs they don't serve.

### Adding a new store

1. Implement `store.Store` and `store.BackendStore` in `store/<name>/`.
2. For region-locked stores, also implement `store.ZIPGated`.
3. Add a `cmd/<name>/main.go` (one-liner around `cli.RunSingle`).
4. Register in `cmd/pricewise/main.go`'s `registry()`. Use
   `store.NewMulti(id, primary, fallbacks...)` for multi-backend stores.

The aggregator, filters, flags, and JSON output all work automatically
once the adapter returns items.

## Notes

- Adapters spoof a desktop Chrome `User-Agent`. Plain `curl` gets 403s
  or 503s from most upstreams. See `store.UA`.
- The aggregator fetches stores concurrently. A per-store failure is
  reported to stderr but doesn't fail the whole run.
- Data freshness: Flipp flyers carry explicit `valid_from` / `valid_to`
  timestamps, surfaced as a `thru YYYY-MM-DD` attribute. Kroger is live
  per-request. MOC rotates weekly (Fridays). Capella runs two-week
  cycles.
- Safeway and Albertsons share a parent company and return identical
  Flipp item lists. Dedupe in post if that matters.
- Unofficial. Not affiliated with any retailer. Respect ToS;
  don't hammer the endpoints.

## License

[MIT](./LICENSE)
