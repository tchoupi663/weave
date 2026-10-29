# Target App Contract

The Target App is a dummy e-commerce application. v1 is healthy-only: valid requests succeed. Internal hops are HTTP; the two external edges are HTTPS.

## Nodes

Frontend (NGINX), API Gateway, Auth, Catalog, Orders, Payment, External Payment Provider, Postgres.

## Frontend

- Serves the built SPA over HTTPS.
- Static assets live under `/assets/`.
- History fallback: any page route returns `index.html`.
- Proxies `/api/*` and `/admin/reset` to the API Gateway over HTTP.
- `GET /health` → `200 {"status":"ok"}`.
- The Simulator talks only to this service.

## API Gateway

| Path | Upstream | Auth |
|---|---|---|
| `/api/auth/*` | Auth | no |
| `/api/products*` | Catalog | no |
| `/api/orders*` | Orders | yes |
| `/api/payments*` | Payment | yes |
| `/admin/reset` | reset handler | no |

For protected routes the Gateway calls `POST /api/auth/validate` with the `Authorization` header:

- `200`: forward `X-User: <username>` to the upstream and overwrite any incoming `X-User`.
- `401` or missing token: return `401`.

Errors: `400` malformed, `401` unauthorized, `404` unknown route, `502`/`504` upstream failure. Healthy Runs should not see `5xx`.

## Auth

In-memory; no Postgres.

`POST /api/auth/login`

```json
{"username": "user0001", "password": "password"}
```

- `200`: `{"token": "<opaque>"}`
- `401`: `{"error": "invalid credentials"}`

`POST /api/auth/validate`

Header: `Authorization: Bearer <token>`

- `200`: `{"valid": true, "username": "user0001"}`
- `401`: `{"error": "invalid token"}`

Seed: `user0001` through `user2000`, password `password`. Tokens are opaque and stored in memory. Reset clears tokens and reseeds the accounts.

## Catalog

In-memory; no Postgres.

`GET /api/products`

- `200`: twenty products, for example `[{"id":"p001","name":"...","price":19.99}, ...]`

`GET /api/products/{id}`

- `200`: product
- `404`: `{"error": "not found"}`

Reset restores the twenty products.

## Orders

Postgres-backed.

`POST /api/orders`

Headers: `Authorization: Bearer <token>`; the Gateway adds `X-User`.

```json
{"items": [{"productId": "p001", "quantity": 1}], "total": 19.99}
```

- `201`: `{"orderId": "<uuid>", "status": "created", "total": 19.99, "items": [...]}`

`GET /api/orders/{id}`

Headers: Bearer; the Gateway adds `X-User`.

- `200`: order
- `404`: `{"error": "not found"}`

Orders trusts the submitted `total` and does not call Catalog. It stores `user_id` from `X-User`. It does not call Payment and does not change status after payment; status stays `created`.

Postgres schema:

```text
orders(id, user_id, total, status, created_at)
order_items(order_id, product_id, quantity)
```

## Payment

`POST /api/payments`

Headers: Bearer; the Gateway adds `X-User`.

```json
{"orderId": "<uuid>", "amount": 19.99, "card": "4242424242424242"}
```

- `200`: `{"paymentId": "<uuid>", "status": "succeeded"}`

Payment calls the External Payment Provider `POST /charge` over HTTPS. It trusts the order and amount and does not call Orders. It is stateless.

## External Payment Provider

`POST /charge`

```json
{"orderId": "<uuid>", "amount": 19.99, "card": "4242424242424242"}
```

- `200`: `{"chargeId": "<uuid>", "status": "succeeded"}`

Dummy success for every valid charge. HTTPS with a self-signed certificate.

## Reset

`POST /admin/reset` through the Frontend; NGINX proxies to the Gateway.

- No auth; idempotent; setup only.
- `200`: `{"status": "reset"}`

Effects:

- Truncate `orders` and `order_items`.
- Clear Auth tokens and reseed the 2,000 accounts.
- Restore the 20 Catalog products.
- Payment and External are stateless.

## Protocol summary

TLS terminates at the Frontend. The internal `Frontend → Gateway` hop is HTTP even though the original diagram labels it HTTPS; TLS is only on the external edges (`Source IP → Frontend` and `Payment → External Payment Provider`).

| Edge | Protocol |
|---|---|
| Source IP → Frontend | HTTPS |
| Frontend → Gateway | HTTP |
| Gateway → Auth | HTTP |
| Gateway → Catalog | HTTP |
| Gateway → Orders | HTTP |
| Gateway → Payment | HTTP |
| Payment → External Payment Provider | HTTPS |
| Orders → Postgres | TCP |

## Health and preflight

- `GET /health` on the Frontend returns 200.
- `GET /api/products` returning 200 verifies Frontend → Gateway → Catalog.
- The Simulator waits for both before calling `/admin/reset`.
