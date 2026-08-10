# Harjod Backend — Foundation & Auth

Branch: `feature/foundation-supabase-auth`

## Decision

| Topic | Choice |
|-------|--------|
| Database | Supabase Postgres |
| Auth (Google/Apple) | Supabase Auth → Go verifies JWT |
| Auth (Line web) | Supabase Auth `custom:line` browser OAuth |
| Auth (Line iOS/Android) | LINE Login SDK → Edge Function `line-login` → Supabase session → Go verifies JWT |
| Custom Go `issue-token` | Removed — Supabase Auth is the only JWT issuer |
| Legacy Google resolve | Removed — dead code (nil deps if wired) |

## What this branch adds

1. **Foundation**
   - Config for `SUPABASE_PROJECT_URL`
   - Postgres via `DATABASE_URL` or discrete `DB_*` fields with `sslmode=require`

2. **Auth**
   - `GET /api/v1/auth/me` (Bearer Supabase access token required)
   - Package `app/auth/supabaseauth` verifies access tokens:
     - **ES256 only** via project JWKS (`/auth/v1/.well-known/jwks.json`)
     - Legacy **HS256** shared-secret verification removed (forge risk if secret leaks)

## Local setup

1. Copy `.env.template` → `.env`
2. Fill (required for auth):
   - `SUPABASE_PROJECT_URL`
3. CORS (`ACCESS_CONTROL_ALLOW_ORIGIN`):
   - Local template defaults to `*` (Bearer auth, not cookies — acceptable for LOCAL)
   - **PROD refuses `*`** — set the Vercel origin only, e.g. `https://frontend-sigma-pearl-96.vercel.app` (do not add `localhost`)
   - Local Expo web should call `http://localhost:8080`, not the Render URL, so prod CORS stays locked down
4. Optional until places/quotes APIs:
   - `DATABASE_URL` (Supabase **pooler** URI; prefer IPv4 host `*.pooler.supabase.com`)
5. Run the API locally:

**Day-to-day coding (fast — no Docker rebuild):** stop the API container if it holds port 8080, then:

```bash
docker stop go-backend-api   # if still running
make run-local               # go run . with .env — restart after code changes
```

Docker images do **not** auto-reload when you edit Go files. The container runs a baked binary from `docker build`.

**Docker (closer to deploy / share with team):**

```bash
make up          # rebuild image + restart API in background
make logs        # follow logs
make down        # stop
```

API: `http://localhost:8080/liveness`

- `Dockerfile` — Harjod-adapted (used by Compose)
- `Dockerfile.upstream` — original forked template (kept untouched)

Postgres for places: use Supabase `DATABASE_URL` in `.env` (pooler). Local Compose `postgres` is optional.

## Verify auth

1. Sign in on the app via Supabase Auth (Google/Apple) and copy the access token
2. Call:

```bash
curl -H "Authorization: Bearer <supabase_access_token>" http://localhost:8080/api/v1/auth/me
```

Expected: `userId`, `email`, `role` from JWT claims.

## Next

- ~~`places` read API (requires Postgres)~~ → `GET /api/v1/places` (public map list)
- ~~Place rate sheet~~ → `GET /api/v1/places/:placeId/rate`
- ~~Place privileges~~ → `GET /api/v1/places/:placeId/privileges` + `GET /api/v1/privileges/:kind/:id`
- ~~`quotes` pricing API~~ → `GET /api/v1/places/:placeId/quote?hours=` + `POST /api/v1/quotes`
- ~~Line custom login~~ → native LINE SDK + Supabase Edge Function `line-login` (web keeps `custom:line` OAuth)
- Frontend: call Go places/rate/privileges instead of Supabase table reads
- ~~Remove leftover legacy Google resolve~~ → deleted (`ResolveIdentify` + `app/auth/access`)

## Places read API

`GET /api/v1/places` — public (no JWT). Returns non-blacklisted places nested like the current frontend PostgREST select (`place_id`, `parking_area`, `hours`, `rate`, `rate_tier`).

Requires `DATABASE_URL` (or discrete `DB_*`) at boot — the server opens a Postgres pool when places routes are registered.

```bash
curl http://localhost:8080/api/v1/places
```

Expected: `{ "data": [ { "place_id", "name_th", "name_en", "parking_area": [...] } ], "code", "message" }`.

## Place rate API

`GET /api/v1/places/:placeId/rate` — public read. Returns the first parking area rate sheet (`free_minutes`, tiers with `from_hour`/`to_hour`, night rate, etc.) or `data: null` when none.

```bash
curl http://localhost:8080/api/v1/places/<place-uuid>/rate
```

## Place privileges API

`GET /api/v1/places/:placeId/privileges` — public read. Nested validation / reserved / EV charger rows (PostgREST-compatible).

`GET /api/v1/privileges/:kind/:id` — public read. `kind` is `stamp`, `reserve`, or `ev`.

```bash
curl http://localhost:8080/api/v1/places/<place-uuid>/privileges
curl http://localhost:8080/api/v1/privileges/stamp/<validation-uuid>
```

## Quotes pricing API

`GET /api/v1/places/:placeId/quote?hours=3` — public. Calculates stay price from the place rate sheet.

`POST /api/v1/quotes` — public batch:

```json
{ "hours": 3, "placeIds": ["<uuid>", "<uuid>"] }
```

Pricing rules: subtract `free_minutes`, round remaining up to whole hours, apply hourly/flat tiers, then cap at `daily_max`. Fully free rates use `free_minutes = -1`.

```bash
curl "http://localhost:8080/api/v1/places/<place-uuid>/quote?hours=3"
curl -X POST http://localhost:8080/api/v1/quotes \
  -H "Content-Type: application/json" \
  -d '{"hours":3,"placeIds":["<uuid>"]}'
```
