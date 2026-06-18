# go-user-management

User management microservice for the landslide early warning system (Bai et al. 2020), rewritten in Go as part of a software engineering thesis.

## Responsibilities

- User registration, authentication (JWT HS256), and profile management
- Alert subscription management per project (min level, quiet hours, notification channels)
- FCM device token registration with automatic ownership transfer
- gRPC server for the early-warning service to query alert recipients at dispatch time

## Architecture

```
Nginx :80
  └── HTTP REST  →  :8080  (external API)
                        ├── POST   /api/v1/users/register
                        ├── POST   /api/v1/users/login
                        ├── GET    /api/v1/users/{id}
                        ├── PUT    /api/v1/users/{id}
                        ├── POST   /api/v1/users/{id}/subscriptions
                        ├── GET    /api/v1/users/{id}/subscriptions
                        ├── POST   /api/v1/users/{id}/devices
                        ├── GET    /api/v1/users/{id}/devices
                        └── DELETE /api/v1/users/{id}/devices/{deviceId}

early-warning-service
  └── gRPC  →  :9090  (internal, bypasses Nginx)
                   └── AlertRecipientService/GetRecipients
```

Consul is used for service discovery and health checking. Prometheus metrics are exposed at `/metrics`.

## SE Techniques

| Technique | Where |
|---|---|
| **gRPC** | `AlertRecipientService` on `:9090` — queried by early-warning at alert dispatch |
| **Docker** | Multi-stage Dockerfile, published to GHCR via CD |
| **CI/CD** | GitHub Actions: build → vet → unit → integration → lint → image push |
| **Circuit Breaker** | Outbound call to project-management when creating subscriptions (sony/gobreaker) |
| **CDCT** | pact-go consumer contract for `GetRecipients` gRPC interface |

## Project structure

```
cmd/server/          main entry point
internal/
  config/            env loading, DB connection, goose migrations
  domain/            domain types (User, Subscription, Device, RecipientRow)
  repository/        sqlx queries (user, subscription, device)
  service/           business logic (auth, user, subscription, device)
  handler/           HTTP handlers (auth, user, subscription, device)
  middleware/        JWT middleware + context helpers
  client/            HTTP client for project-management with circuit breaker
  grpc/              gRPC server implementation + generated pb files
    alertrecipientpb/  generated protobuf Go code
  observability/     structured slog logger + Prometheus metrics middleware
  discovery/         Consul register/deregister
migrations/          goose SQL migrations (embedded)
proto/               .proto source definitions
test/
  integration/       Testcontainers integration tests (requires Docker)
  cdct/              pact-go consumer-driven contract tests
```

## Running locally

### Prerequisites

- Go 1.25+
- Docker (for integration tests and infra)
- The `go-tesis-infra` stack running (`make infra-up`)

### Setup

```bash
cp .env.example .env
# fill in DB_PASS, DB_ROOT_PASS, JWT_SECRET
```

### Run

```bash
go run ./cmd/server
```

The service will:
1. Run goose migrations via the root DSN
2. Connect to MySQL with the least-privilege runtime user
3. Start HTTP on `:8080` and gRPC on `:9090`
4. Register itself in Consul

### With Docker

```bash
docker build -t go-user-management .
docker run --env-file .env --network landslide-net go-user-management
```

## API reference

All protected endpoints require `Authorization: Bearer <token>`.

Non-admins can only access their own resources (`{id}` must match the JWT `sub`). Admins can access any user.

### Auth

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/users/register` | — | Register a new user |
| POST | `/api/v1/users/login` | — | Login, returns JWT |

**Register body:**
```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "password123",
  "phone_e164": "+1234567890",
  "role": "viewer"
}
```

**Login body:**
```json
{ "username": "alice", "password": "password123" }
```

**Login response:**
```json
{ "token": "<jwt>", "user": { ... } }
```

### Users

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/v1/users/{id}` | JWT | Get user profile |
| PUT | `/api/v1/users/{id}` | JWT | Update profile (email, phone, password, role*, status*) |

*`role` and `status` are admin-only fields.

### Subscriptions

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/users/{id}/subscriptions` | JWT | Subscribe to a project's alerts |
| GET | `/api/v1/users/{id}/subscriptions` | JWT | List subscriptions |

**Create subscription body:**
```json
{
  "project_id": "proj-001",
  "min_alert_level": "orange",
  "sms_enabled": true,
  "push_enabled": true,
  "email_enabled": false,
  "quiet_hours_start": "22:00",
  "quiet_hours_end": "06:00"
}
```

Creating a subscription calls project-management to verify the project exists. If project-management is down, the circuit breaker opens and returns `503`.

### Devices

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/api/v1/users/{id}/devices` | JWT | Register FCM token |
| GET | `/api/v1/users/{id}/devices` | JWT | List active devices |
| DELETE | `/api/v1/users/{id}/devices/{deviceId}` | JWT | Revoke a device |

If a token is already registered to another user, it is silently transferred (ownership transfer on upsert).

### Internal gRPC

`AlertRecipientService.GetRecipients` — called by the early-warning service at alert dispatch time.

```protobuf
rpc GetRecipients(GetRecipientsRequest) returns (GetRecipientsResponse);

message GetRecipientsRequest {
  string project_id  = 1;
  string alert_level = 2; // "blue" | "orange" | "red"
}
```

**Alert level rules:**
- Subscription with `min_alert_level=orange` is not notified on a `blue` alert
- Red alerts bypass quiet hours entirely
- Quiet hours support midnight-crossing windows (e.g. `22:00–06:00`)

## Configuration

| Variable | Default | Description |
|---|---|---|
| `SERVICE_NAME` | `user-management-service` | |
| `SERVICE_VERSION` | `0.1.0` | |
| `ENV` | `dev` | |
| `HTTP_PORT` | `8080` | |
| `GRPC_ADDR` | `:9090` | |
| `ADVERTISE_HOST` | `host.docker.internal` | Address reported to Consul |
| `DB_HOST` | `127.0.0.1` | |
| `DB_PORT` | `3306` | |
| `DB_USER` | `svc_user_mgmt` | Runtime user (least-privilege) |
| `DB_PASS` | — | **Required** |
| `DB_NAME` | `user_management` | |
| `DB_ROOT_USER` | `root` | Migration-only user |
| `DB_ROOT_PASS` | — | **Required** |
| `JWT_SECRET` | — | **Required**, HS256 signing key |
| `JWT_EXPIRY_HOURS` | `24` | |
| `PROJECT_MANAGEMENT_URL` | `http://project-management-1:8080` | |
| `CONSUL_ENABLED` | `true` | |
| `CONSUL_HOST` | `localhost` | |
| `CONSUL_PORT` | `8500` | |
| `CONSUL_HEALTH_INTERVAL` | `10s` | |

## Testing

```bash
# Unit + routing tests (fast, no Docker)
make test

# Integration tests (requires Docker)
make test-integration

# Consumer-driven contract tests (requires pact-go native lib)
make test-contract
```

### Test pyramid

| Layer | What | Command |
|---|---|---|
| Unit | `isQuietHour` logic, alert level rank | `make test` |
| Routing | JWT middleware (6 cases: missing, wrong scheme, malformed, expired, wrong secret, valid) | `make test` |
| Integration | Register → login → get profile, duplicate, wrong password, cross-user 403 | `make test-integration` |
| CDCT | pact-go consumer contract for `GetRecipients` gRPC | `make test-contract` |

## CI/CD

| Workflow | Trigger | Steps |
|---|---|---|
| `ci.yml` | push/PR to `main` | tidy check → build → vet → unit → integration → lint |
| `cd.yml` | CI success on `main` | build & push `ghcr.io/tifacusil/user-management:latest` |

## Circuit Breaker

The outbound call to project-management uses `sony/gobreaker`:

- Opens after **5 consecutive failures**
- Stays open for **30 seconds**
- Probes with **3 requests** in half-open state before closing

When open, `POST /subscriptions` returns `503 Service Unavailable`.

## Database migrations

Managed by [goose](https://github.com/pressly/goose) with embedded SQL files. Migrations run automatically at startup using the root DSN (DDL permissions). The runtime user has only `SELECT/INSERT/UPDATE/DELETE`.
