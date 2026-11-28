# Session backlog

Living list from mix what’s-next passes. Not a commitment; reorder or drop freely.

## Done recently

- DuckDuckGo snippet: drop homepage `FAQPage` JSON-LD; `data-nosnippet` on FAQ (visible FAQ unchanged).
- OWASP ZAP in CI: optional local baseline only; do not full-scan production (investigated 2026-11-28).

## Open

1. **Ship current `main` to production** — live was still `v1.3.0-e8d0d63` while `main` moved on.
2. **Bump `github.com/go-sql-driver/mysql` 1.10.2 → 1.11.0**
3. **Cover `altSvcMiddleware`** — still 0% in `server.go`.
4. **Refresh `make setup` tool install** — still uses `go get -u` for some tools.
