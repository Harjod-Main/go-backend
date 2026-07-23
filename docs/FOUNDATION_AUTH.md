# Harjod Backend — Foundation & Auth

Branch: `feature/foundation-supabase-auth`

## Decision

| Topic | Choice |
|-------|--------|
| Database | Supabase Postgres |
| Auth (Google/Apple) | Supabase Auth → Go verifies JWT |
| Auth (Line) | Custom later |
| Custom Go `issue-token` / Google resolve | Not primary path |

## What this branch adds

1. **Foundation**
   - Config for `SUPABASE_PROJECT_URL`, `SECRET_SUPABASE_JWT_SECRET`
   - Postgres via `DATABASE_URL` or discrete `DB_*` fields with `sslmode=require`

2. **Auth**
   - `GET /api/v1/auth/me` (Bearer Supabase access token required)
   - Package `app/auth/supabaseauth` verifies HS256 JWT with project JWT secret

## Local setup

1. Copy `.env.template` → `.env`
2. Fill:
   - `SUPABASE_PROJECT_URL`
   - `SECRET_SUPABASE_JWT_SECRET`
   - `DATABASE_URL` (or DB host/user/password)
3. Run:

```bash
export ENV=LOCAL
# load .env however you prefer (direnv / manually)
make run
# or: go run .
```

## Verify auth

1. Sign in on the app via Supabase Auth (Google/Apple) and copy the access token
2. Call:

```bash
curl -H "Authorization: Bearer <supabase_access_token>" http://localhost:8080/api/v1/auth/me
```

Expected: `userId`, `email`, `role` from JWT claims.

## Next

- `places` read API
- `quotes` pricing API
- Line custom login
- Frontend: stop calling Supabase tables directly; send Bearer token to Go
