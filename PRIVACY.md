# Privacy

IPThing shows you details of the HTTP connection you made to this service (IP address, headers, TLS, and related metadata). That is the product.

## What we store

When a database is configured, each inspected request may be recorded for operations, abuse investigation, and aggregate analysis. Typical fields include:

- Client IP and geolocation cache derived from it
- Method, path, query parameters, User-Agent, Referer, Host, protocol, and similar request metadata
- Declared content length and HTTP status code for the response
- TLS details when present
- A copy of request headers with secrets removed (see below)
- Timing and response format

**Request bodies are not stored** (oversized bodies are rejected; only size/status metadata may be recorded).

Sensitive header values are redacted before persistence, including `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key`, and `X-Auth-Token` (stored as `[redacted]`).

## Retention

Request and related records are retained indefinitely unless the operator deletes them manually.

## Legal basis / purpose

Processing is for providing the inspection service you requested, securing and operating the service, and understanding usage patterns. If you are in the EEA/UK and have questions about personal data held about your visits, contact the service operator.
