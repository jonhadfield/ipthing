# AGENTS.md

IPThing shows clients their connection details (IP, headers, TLS). See `CLAUDE.md` for build/test commands and architecture.

## Deployment: no reverse proxy

Run this service **directly on the public interface** (or with TLS terminated on the process itself via AutoTLS). Do **not** put nginx, Caddy, a cloud LB, or similar in front.

A proxy breaks the product: `RemoteAddr` becomes the proxy, and trusting `X-Forwarded-For` / `X-Real-IP` / `Cf-Connecting-Ip` invites spoofing unless the edge strips client-supplied values. The intended source of truth for the client IP is the TCP peer (`RemoteAddr`), with forwarding headers shown as request metadata only.

When changing IP extraction, middleware, or Echo `IPExtractor`, preserve direct-connection semantics (`echo.ExtractIPDirect()` or equivalent). Do not reintroduce header-first IP trust for “convenience.”
