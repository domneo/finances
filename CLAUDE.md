# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Dev commands

Run from the repo root (pnpm workspace):

```bash
pnpm api          # start Go API on :5001
pnpm dev          # start SvelteKit dev server on :5173
pnpm check        # svelte-check type check (ui)
pnpm lint         # prettier check (ui)
pnpm format       # prettier write (ui)
```

The Vite dev server proxies `/api/*` to `http://localhost:5001`, so both services must be running for the UI to work.

## Architecture

Two independent services sharing a Supabase Postgres database:

**`api/`** — Go HTTP server (`net/http`, no framework). `main.go` registers routes and contains all handlers. The shared database code lives in `internal/txns`: it opens the Postgres connection (`github.com/jackc/pgx/v5/stdlib` behind `database/sql`) into the package-level `txns.DB`, defines the `Transaction` type, and provides `QueryRows`/`QueryOne` (returning `[]map[string]any`) plus `Exec` and `Insert` (writes a `[]Transaction` in one transaction, batched into multi-row statements since the database is now across a network).

**Connecting** — the connection string comes from `DATABASE_URL`, falling back to `api/.env` (see `api/.env.example`, which explains why the URL points at the Supavisor pooler and carries `default_query_exec_mode=exec`). `go run ./cmd/pgcheck` prints the server version and table counts to verify it, and `go run ./cmd/schemacheck` goes further and reports where the live schema and the queries have drifted apart — a table or column the code needs and the database no longer has, a `date` or `numeric` column that changed type out from under `txns.normalize`, a `NOT NULL` added where the code writes `NULL`, and (as a note rather than a failure) any column no query names yet. Run it after changing anything in Supabase; it writes nothing and exits non-zero only on a breaking difference. Because the pooler runs in transaction mode, nothing in this package may create a named prepared statement — that is why `Insert` and `ApplyOwnerRules` build statements inline rather than calling `Prepare`. A `DATABASE_URL` on port 6543 must carry `default_query_exec_mode=exec`; without it a fresh backend answers fine and only a reused one raises `SQLSTATE 42P05`, so the mistake surfaces as intermittent 500s rather than as a connection that plainly does not work.

**Dialect** — queries in this package are written with `?` placeholders and rewritten to `$1, $2 …` by `txns.rebind`, because they are assembled from fragments (a date filter, an owner filter, a `LIMIT`) and hand-numbering them as they concatenate invites off-by-one bugs. Grouping a month uses `to_char(date, 'YYYY-MM')`. `QueryRows` normalises two column types on the way out so the JSON contract is unchanged: `DATE` arrives as a `time.Time` and is formatted back to `YYYY-MM-DD`, and `NUMERIC` arrives as a decimal string and is parsed to a float.

**Personal ownership** — most transactions are joint, but a few kinds belong to one partner's personal budget (their own insurance, salary, allowance to their parents). `transactions.owner` names that partner; `NULL` means joint. Assignment is rule-driven, not manual: `internal/txns/owners.go` reads the `owner_rules` table (`keyword`, optional `category`, `owner`) and matches a rule's keyword against the reference. A rule pinned to a category is tried before an unpinned one, since the same counterparty can run both ways — an insurer's premiums are one partner's own insurance while its payouts are a joint loan. `AssignOwners` tags freshly parsed rows during import (after `CategorizeRows`, which pinned rules depend on); `ApplyOwnerRules` replays the rules over the whole table and is exposed as `go run ./cmd/ownerfill` — run it after editing `owner_rules`.

An owner can also be set by hand for a one-off, so `transactions.owner_source` records who decided: `rule`, `manual`, or `NULL` when nothing claimed the row. `ResolveOwnerSources` derives it on the way into `POST /api/transactions` by comparing the submitted owner against what the rules would say — agreement (including agreeing the row is joint) is `rule`, any divergence is `manual`, and whatever the client sent in `ownerSource` is ignored rather than trusted. `ApplyOwnerRules` then skips `manual` rows, so a replay still corrects everything the rules assigned without undoing a deliberate override. `PATCH /api/transactions/{id}` with `{"owner": "<partner>"}` (or `null` for joint) reassigns a single transaction through `SetOwner`, which judges the change the same way; it is what the owner dropdown on the transactions table calls. The `/api/dashboard` joint budget excludes owned transactions entirely; they belong to a partner's own budget, not the household's.

**Joint budget** — the pooled spending account is fed by a fixed monthly contribution from each partner, paid in by standing instruction. `budget_contributors` (`name`, `reference_match`, `expected`) drives the dashboard's budget panel: a contribution is a credit into the `Joint Budgeting` account whose reference contains `reference_match`, which is the payer's name as their bank writes it. It is a table rather than a constant because a legal name and a salary are personal data and do not belong in the source tree — the same reason `owner_rules` and `category_keywords` live in the database. Edit the table to change who contributes or how much.

**Statement import** — `internal/imports` parses raw bank statements into `[]txns.Transaction`. Each parser is pure (stream in, transactions out, no filesystem or DB): `ParseDBSAccount`/`ParseDBSCard` (CSV) and `ParseUOBCard` (legacy BIFF8 `.xls` via `github.com/extrame/xls`). `imports.Detect` sniffs the format from the file's contents (OLE2 magic bytes for UOB; marker strings for the two DBS layouts) and `imports.Parse` dispatches. Two entry points feed it: the CLI converters in `cmd/{dbsaccconv,dbscardconv,uobcardconv}` (open a file, insert directly into the DB), and the `POST /api/transactions/import` endpoint (multipart `file` upload, auto-detected). Parsers apply `txns.ShortName` to map a statement's account label to the canonical short name; the plain `POST /api/transactions` JSON path stores `account` verbatim. There is intentionally no dedupe guard — identical transactions can legitimately repeat.

**`ui/`** — SvelteKit app in **Svelte 5 runes mode** (enforced project-wide via `svelte.config.js`). Each route has a `+page.ts` universal load function that fetches from the API and a `+page.svelte` for rendering. Filter state lives in URL search params — `goto()` with `replaceState: true` is used to update params, which re-triggers the load function. Local `$state` variables mirror the URL params and are kept in sync via `$effect`.

**Design system** — the styling comes from `bleed`, a separate repo, whose built CSS, fonts and icon sprite have to sit at `ui/static/ds` for `app.html` and the sprite import in `+layout.svelte` to find them. That directory is generated, not committed: `ui/scripts/sync-ds.js` runs ahead of `dev`, `build` and `check` and copies `dist` out of the `bleed` dependency (a git dependency pinned in `pnpm-lock.yaml`), unless the directory is already a working symlink, which it leaves alone. So a deploy gets the pinned commit, and a checkout next to a local `bleed` clone can keep `ln -s ../../../bleed/dist ui/static/ds` and see design changes live without reinstalling. It was previously that symlink and nothing else, committed, which built here and failed anywhere the sibling repo did not exist. Run `pnpm --filter ui sync:bleed` to refresh the copy by hand, and `pnpm update bleed` in `ui/` to move to a newer design system.

## Data conventions

- Amounts are signed floats in the API: **negative = spend, positive = credit/income**. The column is `numeric`, so Postgres sums it exactly; `QueryRows` converts it to a float on the way out
- Dates are stored as `date` and returned as `YYYY-MM-DD` strings — `QueryRows` formats them, so handlers and the UI still see the same strings they always did
- The `transactions` table columns: `id`, `date`, `account`, `category`, `reference`, `amount`, `currency`, `owner`, `owner_source`
- `category` is a foreign key to the `categories` table; can be null
- `owner` is the partner whose personal budget the transaction falls in; null means joint. API endpoints take `?owner=` to filter, where the reserved value `joint` selects the null rows. It is a foreign key to `budget_contributors.name` (as is `owner_rules.owner`), so a partner has to be a contributor before anything can be assigned to them, and `budget_contributors` is therefore what `txns.Owners` lists rather than the names some row happens to use. The key is `ON UPDATE CASCADE`, so renaming a contributor carries through the history by itself. A write naming anyone else comes back as `txns.ErrUnknownOwner`, which `writeError` answers with 400
- `owner_source` is `rule`, `manual`, or null — it says whether `owner_rules` or a person put the owner there, and only the former may be replayed over

## UI style

Keep generated UI simple and minimal — plain system fonts, neutral colours, no component libraries, no animations. Prefer plain HTML elements over wrapper components.

## ui/CLAUDE.md

The `ui/` directory has its own CLAUDE.md with Svelte MCP tool instructions — use those tools when writing Svelte code.
