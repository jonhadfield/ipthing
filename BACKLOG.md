# Session backlog

Living list from mix what’s-next passes. Not a commitment; reorder or drop freely.

## Done recently

- DuckDuckGo snippet: drop homepage `FAQPage` JSON-LD; `data-nosnippet` on FAQ (visible FAQ unchanged).
- Bump `github.com/go-sql-driver/mysql` to 1.11.0.
- Cover `altSvcMiddleware` (QUIC Alt-Svc header wiring).
- Ship `main` to production (`X-IPThing-Version: v1.3.0-a6973c9`, 2026-11-28).
- OWASP ZAP in CI: optional local baseline only; do not full-scan production (investigated 2026-11-28).

## Open

1. **Refresh `make setup` tool install** — still uses `go get -u` for some tools.
