# OpenSky Go API

A REST API that fetches live flight data from the [OpenSky Network](https://opensky-network.org/) with caching, rate limiting, and airport search.

Built with Go and [Gin](https://github.com/gin-gonic/gin).

## Features

- **Real-time flight tracking** - Current positions, altitude, velocity worldwide
- **Airport database** - 86k airports from OurAirports with search/autocomplete
- **Historical data** - Arrivals and departures (requires OAuth2 credentials)
- **In-memory caching** - 15s TTL for flights, 5min for historical data
- **Rate limiting** - Token bucket algorithm, 10 req/s per IP

## Endpoints

### Flights (Real-time)

| Method | Path                     | Description                                              |
| ------ | ------------------------ | -------------------------------------------------------- |
| GET    | `/flights`               | All current flights (`?country=US&on_ground=true`)       |
| GET    | `/flights/:icao`         | Single aircraft by ICAO24 identifier                     |
| GET    | `/flights/area`          | Flights in bounding box (`?lamin=&lamax=&lomin=&lomax=`) |
| GET    | `/flights/airport/:icao` | Flights near an airport (`?radius=0.3`)                  |

### Airports

| Method | Path                         | Description                             |
| ------ | ---------------------------- | --------------------------------------- |
| GET    | `/airports`                  | Search airports (`?q=toronto&limit=10`) |
| GET    | `/airports/:icao`            | Airport details by ICAO code            |
| GET    | `/airports/:icao/arrivals`   | Historical arrivals (OAuth2 required)   |
| GET    | `/airports/:icao/departures` | Historical departures (OAuth2 required) |

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
curl http://localhost:8080/flights | jq '.count'
curl http://localhost:8080/airports?q=JFK
curl "http://localhost:8080/flights/area?lamin=40&lamax=42&lomin=-75&lomax=-73"
```

### OAuth2 Setup (Optional)

For arrivals/departures endpoints, create a `.env` file:

```env
OPENSKY_CLIENT_ID=your_client_id
OPENSKY_CLIENT_SECRET=your_client_secret
```

Get credentials at [OpenSky Network](https://opensky-network.org/).

## Testing

```bash
# Run all tests (60 tests, ~62% coverage)
go test ./...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...

# Skip integration tests (faster, for CI)
go test -short ./...
```

## Project Structure

```
├── main.go           # Entry point, router setup
├── handlers.go       # HTTP handlers
├── opensky.go        # OpenSky API client with caching
├── airports.go       # Airport loading, indexing, search
├── cache.go          # In-memory cache with TTL
├── ratelimit.go      # Token bucket rate limiter
├── models.go         # Data structures
├── helpers.go        # Type conversion utilities
├── data/
│   └── airports.csv  # OurAirports database (~86k airports)
└── *_test.go         # Test files
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

- **Go 1.22+** - Language
- **Gin** - HTTP framework
- **OpenSky Network API** - Flight data source
- **OurAirports** - Airport database (CSV)
