# Harjod Backend — Foundation & Auth

Branch: `feature/foundation-supabase-auth`

## Decision

| Topic | Choice |
|-------|--------|
| Database | Supabase Postgres |
| Auth (Google/Apple) | Supabase Auth → Go verifies JWT |
| Auth (Line) | Custom later |
| Custom Go `issue-token` | Removed — Supabase Auth is the only JWT issuer |
| Legacy Google resolve | Kept in codebase but not registered on router |

## What this branch adds

1. **Foundation**
   - Config for `SUPABASE_PROJECT_URL`, `SECRET_SUPABASE_JWT_SECRET`
   - Postgres via `DATABASE_URL` or discrete `DB_*` fields with `sslmode=require`

2. **Auth**
   - `GET /api/v1/auth/me` (Bearer Supabase access token required)
   - Package `app/auth/supabaseauth` verifies access tokens:
     - **ES256** via project JWKS (`/auth/v1/.well-known/jwks.json`) — current Supabase signing keys
     - **HS256** fallback with `SECRET_SUPABASE_JWT_SECRET` for legacy tokens

## Local setup

1. Copy `.env.template` → `.env`
2. Fill (required for auth):
   - `SUPABASE_PROJECT_URL`
   - `SECRET_SUPABASE_JWT_SECRET`
3. Optional until places/quotes APIs:
   - `DATABASE_URL` (Supabase **pooler** URI; prefer IPv4 host `*.pooler.supabase.com`)
4. Run with Docker (team default — Windows / macOS / Linux):

```bash
# Start Docker Desktop first, then:
make up          # build + run in background
make logs        # follow logs
make down        # stop

# Or foreground:
make run
```

API: `http://localhost:8080/liveness`

- `Dockerfile` — Harjod-adapted (used by Compose)
- `Dockerfile.upstream` — original forked template (kept untouched)

Optional host run (no Docker): `make run-local`

Auth boots without a live Postgres connection. Postgres is wired when business routes are added.

## Verify auth

1. Sign in on the app via Supabase Auth (Google/Apple) and copy the access token
2. Call:

```bash
curl -H "Authorization: Bearer <supabase_access_token>" http://localhost:8080/api/v1/auth/me
```

Expected: `userId`, `email`, `role` from JWT claims.

## Next

- `places` read API (requires Postgres)
- `quotes` pricing API
- Line custom login
- Frontend: stop calling Supabase tables directly; send Bearer token to Go
- Remove or admin-gate leftover legacy Google resolve if unused
