# inventory-api

Inventory management service for the **Marketly** e-commerce platform.

Tracks stock levels, handles reservations, confirms orders.

## Stack

- **Go 1.22** + net/http
- **lib/pq** for Postgres

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/products` | List all products |
| GET | `/products/{sku}` | Get a product by SKU |
| POST | `/reserve` | Reserve stock for an order |
| POST | `/confirm` | Confirm a reservation (decrement stock) |
| GET | `/health` | Liveness probe |
| GET | `/ready` | Readiness probe |

## Local development

```bash
go run ./cmd/server
```

## Tests

```bash
go test ./... -race
```
