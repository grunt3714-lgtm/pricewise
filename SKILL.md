---
name: pricewise
description: Query weekly-specials data from grocery stores near any US/Canada ZIP. Use when the user asks what's on sale, wants to compare prices across local stores, plan grocery shopping, or find deals on specific items. Covers Fred Meyer, Safeway, Albertsons, Grocery Outlet, Costco, Walmart via Flipp; Fred Meyer via Kroger API if credentials are configured; Market of Choice and Capella in Oregon as regional examples.
---

# pricewise skill

A Go CLI that aggregates weekly-specials data from grocery stores near any US/Canada ZIP. Use this when the user asks about local grocery prices, deals, or shopping.

## When to trigger

Phrases that should route here:

- "what's on sale at [Fred Meyer|Safeway|Albertsons|Costco|Walmart|Grocery Outlet]"
- "grocery deals near me"
- "price of X at [store]"
- "compare [item] prices across stores"
- "what should I buy this week"
- "is [item] on sale anywhere"
- "plan a grocery list"

Don't trigger for: general recipe questions, real-time inventory (this is weekly-ad data), or stores outside pricewise's coverage (Trader Joe's, Whole Foods, WinCo).

## Install check

Before calling the CLI, confirm it exists:

```bash
command -v pricewise >/dev/null && echo "ready" || echo "missing"
```

If missing, install:

```bash
GOBIN=~/.local/bin go install github.com/grunt3714-lgtm/pricewise/cmd/...@latest
```

## ZIP is required

Every command needs a ZIP. Either:

- Pass `--zip 97401` on the command.
- Or ensure `PRICEWISE_ZIP` is set in the environment.

If neither is set, the CLI exits with `error: --zip is required`. Ask the user for their ZIP if you don't know it.

## Commands

### `pricewise specials` — aggregate query across all stores

```bash
pricewise specials [--zip ZIP] [--stores LIST] [--search STR] [--attribute STR]
                   [--category STR] [--min-savings N] [--max-price N]
                   [--sort KEY] [--backend KEY] [--no-fallback] [--json]
```

Flags:
- `--zip`: postal code (or `$PRICEWISE_ZIP`). Required.
- `--stores`: comma-separated store IDs, default all active for the ZIP.
- `--search` / `-s`: substring match on item name
- `--attribute` / `-a`: filter by tag like `Organic`, `Vegan`, `Dairy`
- `--category`: filter by aisle or flyer section
- `--min-savings` / `-m`: minimum dollar savings (Kroger, MOC only)
- `--max-price`: cap the sale price
- `--sort`: `savings` (default) | `price` | `name` | `store`
- `--backend`: `auto` (default) | `direct` | `flipp`
- `--no-fallback`: fail rather than trying another backend on error
- `--json`: structured output

### `pricewise stores --zip ZIP` — list active store IDs for a ZIP

```bash
pricewise stores --zip 97401
```

Useful to probe which adapters activate for the user's area — regional stores (like Oregon's Market of Choice) drop out when the ZIP isn't in range.

### Per-store binaries

Same flags as `pricewise specials` but scoped to one store, no `--stores`:

```bash
safeway --zip 97401 --search bananas
fredmeyer --search pepsi --json
```

## Output format

### Table (default)

```
STORE      ITEM                               SIZE      SALE    REG    SAVE   ATTR
fredmeyer  Simple Truth Organic Whole Milk    1/2 gal   3.99    4.99   $1.00  Simple Truth Organic, Dairy
safeway    Horizon Organic UHT Milk                     5.99                  Horizon, thru 2026-05-03
```

### JSON (`--json`)

```json
[
  {
    "store": "fredmeyer",
    "name": "Simple Truth Organic Whole Milk",
    "size": "1/2 gal",
    "sale_price": "3.99",
    "regular_price": "4.99",
    "savings": "$1.00",
    "attributes": ["Simple Truth Organic", "Dairy"],
    "category": "",
    "url": "https://www.fredmeyer.com/p/-/..."
  }
]
```

Missing fields are omitted or empty.

## Recipes

### "What's on sale for [item]?"

```bash
pricewise specials --search "ground beef" --sort price --json
```

Pipe to `jq` for post-processing. Prefer `--json` when you need to reason over the data; use table output when piping to the user as-is.

### "Best deals this week"

```bash
pricewise specials --min-savings 2 --sort savings
```

Caveat: only Kroger direct and MOC publish explicit savings amounts. Flipp stores will have empty savings and rank last.

### "Organic items under $5"

```bash
pricewise specials --attribute organic --max-price 5
```

### "Generate a shopping plan"

```bash
pricewise specials --json > /tmp/all.json
# Reason over /tmp/all.json locally for each desired item.
```

Do one bulk fetch and filter locally rather than calling the CLI per item.

## Backends and fallback

- `--backend auto` (default): each store's primary; fall back on error.
- `--backend direct`: prefer the store's own site/API. Falls back to Flipp unless `--no-fallback`.
- `--backend flipp`: prefer Flipp. Falls back to direct for stores without a Flipp record.

Only Fred Meyer has multiple backends today (Kroger direct + Flipp fallback). Enabling Kroger direct needs a free developer app at https://developer.kroger.com/ and:

```bash
export KROGER_CLIENT_ID=...
export KROGER_CLIENT_SECRET=...
```

Without those, `fredmeyer` silently uses Flipp (weekly-ad items only). With them, you get the full catalog with store-level promo pricing for the nearest Fred Meyer to `$PRICEWISE_ZIP`.

## Data quality

| Adapter           | Name | Brand | Size | Sale | Reg | Savings | Category |
|-------------------|:----:|:-----:|:----:|:----:|:---:|:-------:|:--------:|
| `kroger` (direct) |  ✓   |   ✓   |  ✓   |  ✓   |  ✓  |    ✓    |    ✓     |
| `moc` (direct)    |  ✓   |   –   |  ✓   |  ✓   |  ✓  |    ✓    |    –     |
| `capella` (direct)|  ✓   |   ✓   |  ✓   |  ✓   |  –  |    –    |    ✓     |
| `flipp`           |  ✓   |   ✓   |  –   |  ✓   |  –  |    –    |    ✓     |

- **Kroger direct** has the richest data, but only for Fred Meyer and only with credentials.
- **Flipp-backed stores** return weekly-ad items only; no sizes or savings. When the user asks "where is X cheapest," use `--sort price`, not `--sort savings`.

## Regional adapters

`moc` (Market of Choice) and `capella` (Capella Market) are Oregon-only direct-site adapters. They automatically drop out of the store list for ZIPs outside their service area:

- `moc` serves Oregon — any ZIP starting with 97.
- `capella` serves Lane County — any ZIP starting with 974.

For ZIPs outside Oregon, these adapters are invisible. Don't mention them unless the user's ZIP is in range.

## Common pitfalls

1. **ZIP is required.** Ask the user for one if not provided; don't guess.
2. **Substring matching is loose.** `--search egg` matches "veggie straws". Use `jq` with word-boundary regex after `--json` if precision matters.
3. **Staleness.** Flipp flyers have explicit valid-through dates surfaced as a `thru YYYY-MM-DD` attribute; call out the window when answering.
4. **This is weekly-ad / promo data, not full catalog.** If a search returns nothing, it means nothing on sale this week, not that no store carries it. (Kroger direct does return the promo subset of the full catalog.)
5. **Safeway and Albertsons mirror each other.** Same parent company, same flyer; expect duplicate rows.

## Reporting back to the user

- Cite prices from CLI output directly rather than paraphrasing.
- Always include the `thru YYYY-MM-DD` date when giving a price so the user knows the window.
- If the aggregator reports stderr errors for some stores, mention which stores are missing from the answer.
