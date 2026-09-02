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

| Method | Path           | Description      |
| ------ | -------------- | ---------------- |
| GET    | `/ping`        | Health check     |
| GET    | `/cache/stats` | Cache statistics |

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
# Override the OpenSky API base URL (e.g., point to a Cloudflare Worker proxy)
OPENSKY_BASE_URL=https://your-worker.workers.dev

# API key for proxy authentication
OPENSKY_API_KEY=your_key
```

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
