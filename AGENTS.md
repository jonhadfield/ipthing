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
- XSS: responses use `html/template` escaping plus a tight CSP (inline CSS only, no scripts)

Privacy wording lives in `PRIVACY.md` and `/privacy`. Retention is indefinite by design; do not add automatic purge unless the operator asks.
