# OpenSky Go API

A simple REST API that fetches live flight data from the [OpenSky Network](https://opensky-network.org/) and returns clean JSON.

Built with Go and [Gin](https://github.com/gin-gonic/gin) as a learning project.

## Endpoints

| Method | Path             | Description                                |
| ------ | ---------------- | ------------------------------------------ |
| GET    | `/ping`          | Health check                               |
| GET    | `/flights`       | Get all current flights worldwide          |
| GET    | `/flights/:icao` | Get a specific flight by ICAO24 identifier |

## Run Locally

```bash
# Install dependencies
go mod tidy

# Run the server
go run main.go

# Test endpoints
curl http://localhost:8080/ping
curl http://localhost:8080/flights | head -c 500
```

## Key Go Concepts Demonstrated

- **Structs with JSON tags** - Type-safe data modeling with serialization hints
- **Multiple return values** - `(result, error)` pattern instead of exceptions
- **Pointers for nullable fields** - `*string` vs `string`
- **Type assertions** - Safe conversion from `interface{}` to concrete types
- **defer** - Resource cleanup (like `finally` in JavaScript)
- **Environment variables** - `os.Getenv()` for configuration

## Tech Stack

- **Go 1.22+** - Language
- **Gin** - HTTP framework (like Express.js)
- **OpenSky Network API** - Data source (free, no auth required)
