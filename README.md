# OpenSky Go API

A REST API that fetches live flight data from the [OpenSky Network](https://opensky-network.org/) with caching, rate limiting, and airport search.

Built with Go and [Gin](https://github.com/gin-gonic/gin).

## Features

- **Real-time flight tracking** - Current positions, altitude, velocity worldwide
- **Airport database** - 86k airports from OurAirports with search/autocomplete
- **In-memory caching** - 15s TTL for live flight data
- **Rate limiting** - Token bucket algorithm, 10 req/s per IP
- **Cloudflare Worker proxy** - Routes requests through Cloudflare to avoid OpenSky rate limits
- **CI/CD** - GitHub Actions runs tests and linting before deploying to Railway

## Endpoints

### Flights (Real-time)

| Method | Path                     | Description                                              |
| ------ | ------------------------ | -------------------------------------------------------- |
| GET    | `/flights/area`          | Flights in bounding box (`?lamin=&lamax=&lomin=&lomax=`) |
| GET    | `/flights/airport/:icao` | Flights near an airport (`?radius=0.3`)                  |

### Airports

| Method | Path              | Description                             |
| ------ | ----------------- | --------------------------------------- |
| GET    | `/airports`       | Search airports (`?q=toronto&limit=10`) |
| GET    | `/airports/:icao` | Airport details by ICAO code            |

### System

| Method | Path              | Description                                         |
| ------ | ----------------- | --------------------------------------------------- |
| GET    | `/ping`           | Health check                                        |
| GET    | `/cache/stats`    | Cache statistics                                    |
| GET    | `/debug/cors`     | CORS debug info (non-production only)               |
| GET    | `/debug/upstream` | Probe upstream latency/status (non-production only) |

`/cache/stats` now also includes upstream failover diagnostics (`upstream`) with counters and last-event details, including:

- `lastRequestId` to correlate an API error with a failover event
- `lastPrimaryMs` and `lastFallbackMs` to identify where latency is spent
- `servedStale`, `lastServedStale`, and `lastStaleAgeMs` to confirm stale-on-error behavior

## Run Locally

```bash
# Install dependencies
go mod tidy

# Run the server
go run .

# Test endpoints
curl http://localhost:8080/ping
curl "http://localhost:8080/airports?q=JFK"
curl "http://localhost:8080/flights/area?lamin=40&lamax=42&lomin=-75&lomax=-73"
```

### Environment Variables (Optional)

Create a `.env` file to override defaults:

```env
# Primary upstream (recommended)
OPENSKY_BASE_URL=https://opensky-network.org

# Optional fallback (for example, Cloudflare Worker)
OPENSKY_FALLBACK_BASE_URL=https://your-worker.workers.dev

# Optional direct OpenSky credentials (recommended in production)
# Applied as HTTP Basic Auth to non-worker upstream requests
OPENSKY_CLIENT_ID=your_opensky_client_id
OPENSKY_CLIENT_SECRET=your_opensky_client_secret

# API key for proxy authentication
OPENSKY_API_KEY=your_key

# Upstream request tuning (per attempt)
OPENSKY_TIMEOUT_SECONDS=10
OPENSKY_MAX_ATTEMPTS=2

# Serve expired cache up to this age (seconds) when both upstreams fail
# Set 0 to disable stale-on-error behavior
OPENSKY_STALE_MAX_AGE_SECONDS=300

# Optional probe controls for /debug/upstream
OPENSKY_PROBE_TIMEOUT_SECONDS=5
OPENSKY_PROBE_PATH=/api/states/all?lamin=37.90&lamax=38.00&lomin=23.80&lomax=23.90

# Comma-separated list of allowed frontend origins for browser requests
# Example: http://localhost:5173,https://your-app.pages.dev
CORS_ALLOWED_ORIGINS=http://localhost:5173

# Set to "production" in Railway to disable debug endpoints
APP_ENV=development
```

### Cloudflare Worker Notes

- `GET /` and `GET /health` on the worker return local health JSON.
- API calls are proxied to OpenSky (for example, `/api/states/all?...`).
- If your worker is configured with `API_KEY`, set matching `OPENSKY_API_KEY` in Railway.
- Use worker URL without trailing slash in Railway when setting fallback:
  - `OPENSKY_FALLBACK_BASE_URL=https://your-worker.workers.dev`

### Troubleshoot 522 Errors

`522` means the worker is reachable but its upstream request to OpenSky timed out.

```bash
# Worker health (should be 200)
curl -i "https://your-worker.workers.dev/health"

# Worker root (should also be 200)
curl -i "https://your-worker.workers.dev/"

# Proxied OpenSky request (this is where 522 may appear)
curl -i "https://your-worker.workers.dev/api/states/all?lamin=43&lamax=44&lomin=-80&lomax=-78"
```

If health is `200` but proxy calls are `522`:

- Prefer direct OpenSky as primary and worker as fallback.
- Lower timeout/attempts to fail fast and avoid long hangs.
- Retry with a smaller area query to reduce upstream load.

### Upstream Probe Endpoint

Use this non-production endpoint to benchmark both configured upstreams from the same runtime:

```bash
curl -s "<API_URL>/debug/upstream" | jq .
```

Look at:

- `primary.success`, `primary.durationMs`, `primary.statusCode`, `primary.error`
- `fallback.success`, `fallback.durationMs`, `fallback.statusCode`, `fallback.error`
- `primary.authMode` and `fallback.authMode` to verify whether credentials are being used (`basic_auth`, `proxy_key`, or `none`)
- `probePath` and `timeoutMs` used for the probe

### Verify CORS Manually

Replace `<API_URL>` and `<ORIGIN>` with your deployed API and frontend origin.

```bash
# Simple CORS request (should include access-control-allow-origin)
curl -i "<API_URL>/ping" \
	-H "Origin: <ORIGIN>"

# Preflight request (should return 204 and allow headers/methods)
curl -i -X OPTIONS "<API_URL>/ping" \
	-H "Origin: <ORIGIN>" \
	-H "Access-Control-Request-Method: GET" \
	-H "Access-Control-Request-Headers: Content-Type"
```

Expected:

- `Access-Control-Allow-Origin: <ORIGIN>` for allowed origins
- `204 No Content` for successful preflight
- `403 Forbidden` when origin is not in `CORS_ALLOWED_ORIGINS`

## Testing

```bash
# Run all tests (~62% coverage)
go test ./...

# Verbose output
go test -v ./...

# With coverage report
go test -cover ./...

# Skip integration tests (no CSV file required, faster for CI)
go test -short ./...
```

The test suite has three layers:

- **Unit tests** — cache, rate limiter, helpers, parsing (`*_test.go`)
- **Handler tests** — HTTP handlers via `httptest.NewRecorder()` (`handlers_test.go`)
- **Mock server tests** — OpenSky API client via `httptest.NewServer()` (`mock_test.go`)
- **Integration tests** — real `airports.csv` data, skipped with `-short` (`integration_test.go`)

## CI/CD

GitHub Actions runs on every push to `main` and all pull requests:

1. **Test** — `go vet` + `go test -race`
2. **Lint** — `golangci-lint` (errcheck, staticcheck, unused, etc.)
3. **Build** — compiles the binary
4. **Deploy** — triggers Railway deployment (main branch only, after all checks pass)

Pull requests cannot be merged if any check fails.

## Project Structure

```
├── main.go              # Entry point, router setup
├── handlers.go          # HTTP handlers
├── opensky.go           # OpenSky API client with caching
├── airports.go          # Airport loading, indexing, search
├── cache.go             # In-memory cache with TTL
├── ratelimit.go         # Token bucket rate limiter
├── models.go            # Data structures
├── helpers.go           # Type conversion utilities
├── cloudflare-worker/
│   └── worker.js        # Cloudflare Worker proxy script
├── data/
│   └── airports.csv     # OurAirports database (~86k airports)
├── .github/
│   └── workflows/
│       └── ci.yml       # GitHub Actions CI/CD pipeline
└── *_test.go            # Test files
```

## Key Go Concepts Demonstrated

- **Structs with JSON tags** - Type-safe data modeling with serialization
- **Multiple return values** - `(result, error)` pattern instead of exceptions
- **Pointers for nullable fields** - `*string` vs `string`
- **Goroutines** - Background cache cleanup routines
- **sync.RWMutex** - Thread-safe concurrent access
- **Interfaces** - `interface{}` for flexible JSON parsing
- **Table-driven tests** - Idiomatic Go testing patterns
- **httptest** - Testing HTTP handlers and mock servers

## Tech Stack

- **Go 1.26** - Language
- **Gin** - HTTP framework
- **OpenSky Network API** - Flight data source
- **OurAirports** - Airport database (CSV)
- **Cloudflare Workers** - Proxy layer
- **Railway** - Hosting
