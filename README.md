# finances

A small household finance app: import bank statements, categorize the transactions, and see where the joint budget actually went each month.

## Prerequisites

- Node.js with pnpm
- Go 1.25+
- Supabase Postgres database

## Quick start

```bash
pnpm install
cp api/.env.example api/.env   # then fill in DATABASE_URL
```

Check database connection:

```bash
cd api && go run ./cmd/pgcheck
```

Then run both services, in two terminals:

```bash
pnpm api    # Go API on :5001
pnpm dev    # SvelteKit UI on :5173
```

Vite proxies `/api/*` to the Go server, so both need to be up. Open http://localhost:5173.

### Seeding demo data

To seed demo data, turn on demo mode and run the `dummyseed` command to generate a set of transactions:

```bash
cd api && DEMO_MODE=true go run ./cmd/dummyseed
```

That writes twelve months of invented transactions for a fake household and refuses to run unless `DEMO_MODE` is on. Set `DEMO_MODE=true` for the API process too and it'll serve only those rows. One thing it doesn't seed: you need two rows in `budget_contributors` first, since `owner` is a foreign key to it.

## Usage

### Import a statement

Upload it through the UI at `/upload`, or hit the endpoint directly. Statement format is automatically detected by its contents.

```bash
curl -F file=@transactions/dbs/statement.csv \
  http://localhost:5001/api/transactions/import
```

This parses and suggests categories and owners but writes nothing. You review the rows, then commit them:

```bash
curl -X POST http://localhost:5001/api/transactions \
  -H 'Content-Type: application/json' \
  -d '[{"date":"2026-08-01","account":"DBS Multiplier","category":"Groceries","reference":"NTUC FAIRPRICE","amount":-42.30,"currency":"SGD"}]'
```

Amounts are signed: negative is spend, positive is income. There's deliberately no dedupe guard, since identical transactions can legitimately repeat.

There are CLI converters too, if you'd rather skip the review step and write straight to the database:

```bash
cd api
go run ./cmd/dbsaccconv  path/to/account.csv
go run ./cmd/dbscardconv path/to/card.csv
go run ./cmd/uobcardconv path/to/card.xls
```

### Query it

```bash
# the month's dashboard (defaults to the latest month with data)
curl 'localhost:5001/api/dashboard?month=2026-07'

# transactions, filtered and paged
curl 'localhost:5001/api/transactions?from=2026-07-01&to=2026-07-31&category=Groceries&limit=50'

# spend by category, and the month-over-month totals
curl 'localhost:5001/api/transactions/summary?from=2026-01-01'
curl 'localhost:5001/api/transactions/monthly'

# just one partner's personal budget, or just the shared spending
curl 'localhost:5001/api/transactions?owner=alice'
curl 'localhost:5001/api/transactions?owner=joint'
```

`owner=joint` is the reserved value for the shared rows, which are stored as `NULL` and otherwise unreachable through a query param.

### Assign transaction owners

Each transaction can be tied to a specific owner and takes it out of the joint budget. The `owner_rules` table matches a keyword against the transaction reference, optionally pinned to a category. If match, that owner rule is applied to the transaction.

If you edit the `owner_rules` table, replay the rules across all transactions:

```bash
cd api && go run ./cmd/ownerfill
```

The replay skips any transaction ownership set by hand, so your one-off corrections survive. To make one of those corrections:

```bash
curl -X PATCH localhost:5001/api/transactions/1234 \
  -H 'Content-Type: application/json' \
  -d '{"owner":"alice"}'   # or {"owner":null} for joint
```

### After changing the schema

If you've edited anything in the Supabase editor, run:

```bash
cd api && go run ./cmd/schemacheck
```

It reports where the live schema and the queries have drifted apart: a column the code needs that's gone, a type that changed under `txns.normalize`, a `NOT NULL` where the code writes `NULL`, etc. It writes nothing and only exits non-zero on a breaking difference.

## Layout

```
api/                Go HTTP server, no framework. All handlers in main.go.
  internal/txns/    DB access, the Transaction type, categorization, owner rules
  internal/imports/ Statement parsers (pure: stream in, transactions out)
  cmd/              CLI tools: pgcheck, schemacheck, ownerfill, dummyseed, converters
ui/                 SvelteKit app, Svelte 5 runes mode
```

The UI styling comes from `bleed`, a separate design-system repo, copied into `ui/src/lib/assets/ds` by a prebuild script. That directory is generated and should not be committed. If you have `bleed` checked out next door you can symlink it instead (`ln -s ../../../../../bleed/dist ui/src/lib/assets/ds`) and the `sync-ds` script will leave your symlink alone.

Other commands worth knowing:

```bash
pnpm check    # svelte-check
pnpm format   # prettier write
cd api && go test ./...
```
