# AGENTS.md

IPThing shows clients their connection details (IP, headers, TLS). See `CLAUDE.md` for build/test commands and architecture.

## Deployment: no reverse proxy

Run this service **directly on the public interface** (or with TLS terminated on the process itself via AutoTLS). Do **not** put nginx, Caddy, a cloud LB, or similar in front.

A proxy breaks the product: `RemoteAddr` becomes the proxy, and trusting `X-Forwarded-For` / `X-Real-IP` / `Cf-Connecting-Ip` invites spoofing unless the edge strips client-supplied values. The intended source of truth for the client IP is the TCP peer (`RemoteAddr`), with forwarding headers shown as request metadata only.

When changing IP extraction, middleware, or Echo `IPExtractor`, preserve direct-connection semantics (`echo.ExtractIPDirect()` or equivalent). Do not reintroduce header-first IP trust for “convenience.”

## Protocols

HTTPS serves HTTP/1.1 and HTTP/2 on TCP 443. HTTP/3 (QUIC) listens on **UDP 443** and is advertised via `Alt-Svc`. Allow UDP/443 on both the host firewall and any DigitalOcean Cloud Firewall (or equivalent) in front of the droplet — host rules alone are not enough if the cloud firewall drops UDP.

## Hardening (do not weaken casually)

These limits exist to keep a public inspector from being an easy resource sink:

- Rate limit: **5 req/s per IP**, burst **10** (favicon paths skipped)
- Body size: **1 MiB** max; request bodies are inspected for size only and **never stored**
- Server timeouts: **5s** read headers, **10s** read, **15s** write, **60s** idle (+ HTTP/3 idle 60s)
- Handler bound: Echo `ContextTimeout` **8s**; prefer this over Echo’s discouraged `Timeout` middleware
- ipinfo.io: at most **8** concurrent API fetches; DB geo cache TTL **24h** (stale cache returned on fetch failure)
- XSS: responses use `html/template` escaping plus a tight CSP (inline CSS only, no scripts)

Crawl aids: `/robots.txt`, `/sitemap.xml`, and HTML meta/canonical on `/` and `/privacy`. Register the site in Google Search Console after deploy. Responses include `X-IPThing-Version: <tag>-<sha>` (from build ldflags).

Privacy wording lives in `PRIVACY.md` and `/privacy`. Retention is indefinite by design; do not add automatic purge unless the operator asks.

## Related project

Aggregate traffic charts live at [stats.ipthing.net](https://stats.ipthing.net/), built from the read-only analysis repo [jonhadfield/ipthing-analysis](https://github.com/jonhadfield/ipthing-analysis). That project is separate from this Go service; keep DB credentials for it read-only.
