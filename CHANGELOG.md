# Change Log

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

### Added

- Harjod foundation: Supabase Auth JWT verify (`GET /api/v1/auth/me`)
- Places map list API: `GET /api/v1/places` (public read, nested parking/hours/rate)
- Postgres pool required at boot for places routes
- Docker Compose API service as team default (`make up` / `make run`)
- `Dockerfile.upstream` keeps the original forked Dockerfile untouched
- Docs: `docs/FOUNDATION_AUTH.md`

### Removed

- Legacy `IssueToken` handler (`POST /auth/issue-token`) — unsigned minting of JWTs for arbitrary `userId` (Supabase Auth is the sole issuer)
- Legacy Google `ResolveIdentify` handler + `app/auth/access` (never registered; wiring would nil-panic)
- Legacy config/env for custom JWT mint, Google tokeninfo client, AESGCM, hash pepper
- Dockerfile `GIT_USERNAME` / `GIT_PASSWORD` build-args (password leaked via layer metadata); Harjod modules are public via `proxy.golang.org`

### Changed

- Supabase Auth verifier now validates **ES256** tokens via project JWKS (`/auth/v1/.well-known/jwks.json`), with HS256 + `SECRET_SUPABASE_JWT_SECRET` as legacy fallback
- Default auth path is Supabase JWT verify only
- `Dockerfile` adapted for Harjod/GitHub modules; original saved as `Dockerfile.upstream`
- `make run` / `make up` use Docker Compose (Windows/macOS friendly, no `--network host`)

### Fixed

- `ListenAndServe` failure now exits with code 1 so orchestrators restart the process (was `return` → exit 0)
- `.golangci.yaml`: drop stale `unused` exclude for missing `openapi.go`; point `rowserrcheck` at `jackc/pgx/v5` instead of `jmoiron/sqlx`
- `gitlabci.yml`: bump `GO_VERSION` from 1.17.3 → 1.25.0 to match `go.mod`
- AccessControl typo fixing
- [8317ae4] add NewHTTPClientWithCA in httpclient
- [c5bddc1] fix warn error in RefIDMiddleware
- [d35168b] add openapi for interpermit
- [4931a0e] add `make upgrade` for dev tools upgrading

## [1.0.0] - 2024-10-29

### Added

- Document (README.md, Go doc)
- Simple strucure [README.md](./README.md)
- Fundamental Set
- Makefile
- Middleware
  - securityHeaders
  - RefIDMiddleware, keep ref-id
  - TraceContextTraceIDMiddleware, accept traceparent header and forward trace-id
  - AutoLoggingMiddleware, automated log when error has found
  - handlerTimeoutMiddleware, handler timeout
  - accessColtrol, e.g. CORS, allow-headers
- Utility packages
  - **s**error
  - looger
    - replacer, e.g. GCPKeyReplacer, CensorReplacer
  - httpclient
    - options, e.g. ForwardRefIDOption
- GOMAXPROCS, GOMEMLIMIT settings
- .env for environment variable configuration
- /liveness, /readiness, /metrics

<!-- ### Added
### Changed
### Deprecated
### Removed
### Fixed
### Security -->
