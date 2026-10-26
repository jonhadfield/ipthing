# IPThing

HTTP request inspector. Open **[https://ipthing.net](https://ipthing.net)** in a browser to see your public IP, or `curl` it for JSON.

| | URL |
| --- | --- |
| Live service | **[https://ipthing.net](https://ipthing.net)** |
| Traffic stats | **[https://stats.ipthing.net](https://stats.ipthing.net)** |
| Privacy | [https://ipthing.net/privacy](https://ipthing.net/privacy) |
| Source | [github.com/jonhadfield/ipthing](https://github.com/jonhadfield/ipthing) |

This is the Go service behind those URLs. It is not the unrelated 2006 Windows tool also named IPThing.

## What it shows

A visit reports the connection as the server actually received it:

- Public IPv4 or IPv6 (the TCP/QUIC peer, not `X-Forwarded-For`)
- Approximate location, ASN/org, and reverse DNS when available
- HTTP method, path, protocol (HTTP/1.1, HTTP/2, HTTP/3), and headers
- TLS version, cipher, ALPN, and JA3/JA4 ClientHello fingerprints
- Cookie **names** only (values are never stored)

Browsers get HTML (no JavaScript, no ads). CLI clients get pretty-printed JSON. Plain HTTP stays available on purpose so `curl http://ipthing.net` still works when TLS is broken.

## Use the public service

```bash
# Browser HTML
open https://ipthing.net

# JSON (curl, wget, scripts)
curl https://ipthing.net

# Force address family
curl -4 https://ipthing.net
curl -6 https://ipthing.net

# HTTP without TLS
curl http://ipthing.net
```

User-Agents that look like a browser (`Mozilla`, `AppleWebKit`, `Gecko`, …) receive HTML. Everything else receives `application/json`.

JSON field names match the HTML labels (PascalCase), for example:

```json
{
  "IPAddr": "203.0.113.50",
  "IPFamily": "ipv4",
  "Country": "US",
  "City": "Dallas",
  "Org": "AS65536 Example ISP",
  "UserAgent": "curl/8.0.0",
  "Proto": "HTTP/2.0",
  "TLSVersion": "TLS 1.3",
  "JA4": "t13d…",
  "XFF": ""
}
```

`IPAddr` is always the peer address. `XFF`, `XRI`, and `Cf-Connecting-Ip` are displayed as request metadata and are never trusted as the client IP.

## Client IP

Run this process on the public interface (or terminate TLS on the process with AutoTLS). Do **not** put nginx, Caddy, or a cloud load balancer in front: `RemoteAddr` would become the proxy, and trusting forwarding headers invites spoofing.

HTTPS is HTTP/1.1 and HTTP/2 on TCP 443. HTTP/3 (QUIC) listens on **UDP 443** and is advertised with `Alt-Svc`. Allow UDP/443 on the host firewall **and** any cloud firewall, or HTTP/3 will not reach the process.

`www.ipthing.net` 301s to the apex and keeps the scheme. There is no HTTP→HTTPS redirect and no HSTS.

## Traffic stats

Who hits the public service — volume over time, IPv4 vs IPv6, countries and ASNs, TLS versions, JA3/JA4 clusters, and scanner vs named-host traffic — is charted at **[https://stats.ipthing.net](https://stats.ipthing.net)**.

That report is aggregate-only (no raw headers, full User-Agents tied to clients, or other row-level identifiers). It is built from this service’s logs by a separate, read-only project: [jonhadfield/ipthing-analysis](https://github.com/jonhadfield/ipthing-analysis).

## Privacy

When a database is configured, request metadata may be stored indefinitely for operations and aggregate analysis. Request **bodies are never stored**. Secrets in headers (`Authorization`, `Cookie`, API keys, …) are redacted before persistence.

Details: [PRIVACY.md](PRIVACY.md) and [https://ipthing.net/privacy](https://ipthing.net/privacy).

## Run locally

Requires Go 1.26.10 or later (see `go.mod`).

```bash
git clone https://github.com/jonhadfield/ipthing.git
cd ipthing
make start                 # HTTP on :8080
# or
go run .
```

Then `curl http://127.0.0.1:8080` or open that URL in a browser.

```bash
make start-background      # PID in ipthing.pid, logs in ipthing.log
make status
make logs
make stop
```

```bash
make test
make lint
make govulncheck
make build                 # .local_dist/
```

Templates, favicons, `robots.txt`, and `sitemap.xml` are embedded at compile time. There are no runtime file dependencies.

## Configuration

Loaded in this order: `IPTHING_CONFIG` (JSON string), else `config.json`, else defaults. Individual `IPTHING_*` variables override. Legacy `DATABASE_URL` is used only if no other connection string is set.

| Setting | Env | Default |
| --- | --- | --- |
| HTTP port | `IPTHING_HTTP_PORT` | `8080` |
| HTTPS port | `IPTHING_HTTPS_PORT` | `443` |
| AutoTLS hostnames | `IPTHING_HOST_WHITELIST` (comma-separated) | empty |
| Database | `IPTHING_DB_TYPE` (`sqlite`, `postgres`, `mariadb`) | none (no-op) |
| SQLite path | `IPTHING_SQLITE_PATH` | — |
| Postgres / MariaDB DSN | `IPTHING_POSTGRES_URL` | — |

Example `config.json`:

```json
{
  "listenPortHTTP": 8080,
  "listenPortHTTPS": 443,
  "hostWhitelist": ["ipthing.net", "www.ipthing.net"],
  "databaseType": "sqlite",
  "databasePath": "/var/lib/ipthing/ipthing.db"
}
```

Without a database, the inspector still works; nothing is persisted. Geolocation uses ipinfo.io and is cached in the database for 24h when one is configured.

## Docker

Image: [`ghcr.io/jonhadfield/ipthing`](https://github.com/jonhadfield/ipthing/pkgs/container/ipthing) (distroless).

```bash
make build-docker
# or
docker pull ghcr.io/jonhadfield/ipthing:latest
```

The container listens according to config (defaults HTTP 8080 / HTTPS 443). The Dockerfile `EXPOSE 1323` line is leftover and is not the listen port.

## Hardening

Public-inspector limits (do not weaken casually):

- 5 requests/s per IP, burst 10 (favicon paths skipped); one shared store across HTTP + HTTPS/HTTP/3
- 1 MiB max body; bodies inspected for size only
- Async DB writes: queue of 256, 4 workers; drops under pressure
- Timeouts: 5s headers, 10s read, 15s write, 60s idle; handler bound 8s
- At most 8 concurrent ipinfo.io fetches
- CSP (inline CSS only, no scripts), `nosniff`, `Referrer-Policy: no-referrer`
- Operational logs omit client IP, User-Agent, and query strings

CI runs tests and `govulncheck`. `make release-docker` scans with Trivy before push.

## License

[Apache License 2.0](LICENSE)
