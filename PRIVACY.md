# Privacy

IPThing shows you details of the HTTP connection you made to this service (IP address, headers, TLS, and related metadata). That is the product.

## What we store

When a database is configured, each inspected request may be recorded for operations, abuse investigation, and aggregate analysis. Typical fields include:

- Client IP, IP family (IPv4/IPv6), optional reverse-DNS (PTR), and geolocation cache derived from the IP
- Method, path, query parameters, User-Agent, Referer, Host, protocol, and similar request metadata
- Declared content length and HTTP status code for the response
- TLS details when present (version, cipher, SNI, ALPN, session resume, key exchange, client certificate subject if mTLS)
- TLS ClientHello fingerprints (JA3, JA4) derived from the handshake — not a unique device ID
- Cookie **names** only (values never stored); presence of cookies as a boolean
- A copy of request headers with secrets removed (see below)
- Timing and response format

**Request bodies are not stored** (oversized bodies are rejected; only size/status metadata may be recorded).

Sensitive header values are redacted before persistence, including `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, and `X-Auth-Token` (stored as `[redacted]`).

Forwarding headers such as `X-Forwarded-For` and `Cf-Connecting-Ip` may be shown and stored as request metadata but are **never** trusted as the client IP.

## Retention

Request and related records are retained indefinitely unless the operator deletes them manually.

## Public aggregate stats

A separate project publishes **aggregate-only** charts and narrative from this data at [stats.ipthing.net](https://stats.ipthing.net/). Source code: [jonhadfield/ipthing-analysis](https://github.com/jonhadfield/ipthing-analysis). That report does not publish raw headers, full User-Agents tied to individuals, PTR hostnames for single clients, or other row-level identifiers.

## Legal basis / purpose

Processing is for providing the inspection service you requested, securing and operating the service, and understanding usage patterns. If you are in the EEA/UK and have questions about personal data held about your visits, contact the service operator.
