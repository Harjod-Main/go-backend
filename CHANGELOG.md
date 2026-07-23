# Change Log

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

### Added

- Harjod foundation: Supabase Postgres config (`DATABASE_URL` / `DB_*` + SSL)
- Supabase Auth JWT verification (`app/auth/supabaseauth`, HS256)
- Protected endpoint `GET /api/v1/auth/me`
- Docs: `docs/FOUNDATION_AUTH.md`

### Changed

- Default auth path is Supabase JWT verify (legacy Google resolve / issue-token no longer registered)

### Fixed

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
