# API Structure & Conventions

This document defines the package structure, file naming, and conventions to follow when adding new domains or extending existing ones.

## Table of Contents

- [Project Layout](#project-layout)
- [Domain Package Structure](#domain-package-structure)
  - [The Four Files](#the-four-files)
  - [File Responsibilities](#file-responsibilities)
- [Shared Packages](#shared-packages)
- [Wiring a New Domain](#wiring-a-new-domain)
- [Naming Conventions](#naming-conventions)
  - [Packages](#packages)
  - [Files](#files)
  - [Functions](#functions)
  - [Types](#types)
  - [Metrics & Spans](#metrics--spans)
  - [Routes](#routes)
- [Dependency Rules](#dependency-rules)
- [Complete Walkthrough: Adding a New Domain](#complete-walkthrough-adding-a-new-domain)

---

## Project Layout

```
go-chi-observability/
├── cmd/
│   └── api/
│       ├── main.go              # Entrypoint — init order, server start, graceful shutdown
│       └── init.go              # Composition root — wires domain metric initialisers
├── docs/
│   ├── observability.md         # Observability internals & instrumentation guide
│   └── api-structure.md         # This file
├── internal/
│   ├── calculator/              # Domain package (example)
│   │   ├── types.go             # Request/response structs
│   │   ├── metrics.go           # OTel metric instruments + InitMetrics()
│   │   ├── handlers.go          # HTTP handler functions + tracer + helpers
│   │   └── routes.go            # RegisterRoutes(r chi.Router)
│   ├── handlers/                # Shared handler utilities
│   │   ├── health.go            # GET /health
│   │   └── response.go          # WriteError() — shared JSON error response
│   ├── observability/           # Generic observability infrastructure
│   │   ├── errors.go            # RecordError() — shared span+metric+log+response
│   │   ├── logger.go            # Zap logger + trace correlation
│   │   ├── metrics.go           # OTel MeterProvider + Prometheus handler
│   │   ├── middleware.go        # RequestID, Tracing, Logging middlewares
│   │   ├── request_id.go        # UUID request ID + context helpers
│   │   └── tracing.go           # OTel TracerProvider
│   └── server/
│       └── router.go            # Chi router — middleware + route composition
├── go.mod
└── go.sum
```

### Key principles

1. **`cmd/api/`** is the composition root. It initialises systems and wires things together. It imports domain packages but contains no business logic.
2. **`internal/<domain>/`** packages are self-contained. Each owns its types, metrics, handlers, and routes.
3. **`internal/observability/`** is generic infrastructure. It never imports domain packages.
4. **`internal/handlers/`** holds shared handler utilities (error responses, health check) — things too small or generic to warrant their own domain package.
5. **`internal/server/`** composes route groups. It imports domain packages to call their `RegisterRoutes()`.

---

## Domain Package Structure

Every API domain (calculator, users, orders, etc.) follows the same four-file pattern inside `internal/<domain>/`.

### The Four Files

```
internal/<domain>/
  types.go       # Data structures
  metrics.go     # Metric instruments
  handlers.go    # HTTP handlers
  routes.go      # Route registration
```

Each file has a single, focused responsibility. This makes it trivial to find where something lives and keeps files small as the domain grows.

### File Responsibilities

#### `types.go`

Contains all request/response structs and domain-specific data types. No imports beyond the standard library (primarily for JSON tags). No logic.

```go
package mydomain

type CreateRequest struct {
    Name string `json:"name"`
}

type CreateResponse struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    RequestID string `json:"request_id"`
}
```

**Rules:**
- One file for all types in the domain
- Only struct definitions and constants
- No methods, no logic, no external imports
- All JSON-facing structs get `json` tags

#### `metrics.go`

Declares metric instrument variables (package-level) and an `InitMetrics()` function that registers them with the OTel meter.

```go
package mydomain

var (
    opsCounter   metric.Int64Counter
    errorCounter metric.Int64Counter
)

func InitMetrics() error {
    meter := otel.Meter("mydomain")
    // ... register instruments ...
    return nil
}
```

**Rules:**
- Meter name matches the package/domain name
- Metric variables are unexported (package-private) — only handlers in this package use them
- `InitMetrics()` is exported — called from `cmd/api/init.go`
- Always return errors, never panic

#### `handlers.go`

Contains all HTTP handler functions, the package-level tracer, and any domain-specific helper functions.

```go
package mydomain

var tracer = otel.Tracer("mydomain")

func Create(w http.ResponseWriter, r *http.Request) { ... }
func Get(w http.ResponseWriter, r *http.Request) { ... }
func List(w http.ResponseWriter, r *http.Request) { ... }
```

**Rules:**
- Tracer name matches the package/domain name (same as the meter name)
- Handlers are exported functions with the standard `http.HandlerFunc` signature
- Each handler gets a trace-correlated logger and request ID from context at the top
- Handlers create custom child spans for business logic
- Handlers use `observability.RecordError()` for error paths
- Domain-specific helpers (unexported) live here too

#### `routes.go`

A single exported function that mounts all routes onto a chi router under the domain prefix.

```go
package mydomain

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router) {
    r.Route("/mydomain", func(r chi.Router) {
        r.Post("/", Create)
        r.Get("/{id}", Get)
        r.Get("/", List)
    })
}
```

**Rules:**
- One function: `RegisterRoutes(r chi.Router)`
- Uses `r.Route("/prefix", ...)` to group under the domain prefix
- Route prefix matches the domain name (e.g. `/calculator`, `/users`, `/orders`)
- Only references handlers from the same package — never cross-domain

---

## Shared Packages

### `internal/handlers/`

For utilities that are too small for their own domain but are used across domains:

| File | Contents |
|---|---|
| `health.go` | `GET /health` handler — simple liveness check |
| `response.go` | `WriteError()` — standardised JSON error response |

Add new shared response helpers here (e.g. `WriteJSON()`, `WritePaginated()`). Do not put domain-specific handlers here.

### `internal/observability/`

Generic infrastructure. See [docs/observability.md](observability.md) for details. Domain packages import from here — never add domain-specific code to this package.

### `internal/server/`

Just `router.go`. It composes the middleware stack and calls each domain's `RegisterRoutes()`. This file grows by one line per domain.

---

## Wiring a New Domain

Adding a new domain requires changes in exactly **three places**:

### 1. Create the domain package

```
mkdir internal/newdomain
```

Create the four files: `types.go`, `metrics.go`, `handlers.go`, `routes.go`.

### 2. Register routes in `internal/server/router.go`

```go
import "go-chi-observability/internal/newdomain"

// Inside NewRouter():
newdomain.RegisterRoutes(r)
```

### 3. Register metrics in `cmd/api/init.go`

```go
import "go-chi-observability/internal/newdomain"

// Inside initMetrics():
if err := newdomain.InitMetrics(); err != nil {
    return nil, err
}
```

That's it. `main.go` never changes.

---

## Naming Conventions

### Packages

- Lowercase, single word, noun: `calculator`, `users`, `orders`, `inventory`
- Match the URL path prefix: package `users` -> routes under `/users`
- No `_` or camelCase in package names

### Files

| File | Purpose | Naming |
|---|---|---|
| `types.go` | Data structures | Always `types.go` |
| `metrics.go` | Metric instruments | Always `metrics.go` |
| `handlers.go` | HTTP handlers | Always `handlers.go` |
| `routes.go` | Route registration | Always `routes.go` |

If a domain grows large enough that `handlers.go` becomes unwieldy, split by sub-concern but keep the prefix clear:

```
internal/orders/
  handlers.go           # Main CRUD handlers
  handlers_search.go    # Complex search/filter handlers
  handlers_export.go    # Export-related handlers
  ...
```

### Functions

- **Handlers:** verb or noun matching the HTTP action — `Create`, `Get`, `List`, `Delete`, `Add`, `Chain`
- **Route registration:** always `RegisterRoutes`
- **Metric init:** always `InitMetrics`
- **Unexported helpers:** descriptive, camelCase — `handleBinaryOp`, `validateInput`

### Types

- **Requests:** `<Action>Request` — `CreateRequest`, `CalcRequest`, `ChainRequest`
- **Responses:** `<Action>Response` — `CreateResponse`, `CalcResponse`, `ChainResponse`
- **Internal models:** plain descriptive nouns — `ChainStep`, `ChainResult`

### Metrics & Spans

- **Tracer name:** domain name — `"calculator"`, `"users"`
- **Meter name:** same as tracer name
- **Span names:** `domain.action` — `"calculator.add"`, `"users.create"`
- **Metric names:** `domain.noun.suffix` — `"calculator.operations.total"`, `"users.signups.total"`
- **Attribute keys:** `domain.noun` — `"calculator.operand.a"`, `"users.email"`

### Routes

- **Prefix:** `/<domain>` — `/calculator`, `/users`, `/orders`
- **Resources:** nouns, plural — `/users`, `/orders`
- **Actions on resources:** HTTP verbs, not URL verbs — `POST /users` not `POST /users/create`
- **Path parameters:** `/{id}` — use chi's URL parameter syntax

---

## Dependency Rules

```
cmd/api/          -> internal/observability, internal/server, internal/<domain>
internal/server/  -> internal/observability, internal/handlers, internal/<domain>
internal/<domain> -> internal/observability, internal/handlers
internal/handlers -> (standard library only)
internal/observability -> (external libs only, no internal imports)
```

**Prohibited:**
- `internal/observability` must never import any `internal/` package (prevents circular deps)
- `internal/handlers` must never import domain packages
- Domain packages must never import other domain packages (if cross-domain calls are needed, refactor shared logic into a new shared package)

---

## Complete Walkthrough: Adding a New Domain

Let's say you're adding a `users` domain with `POST /users` and `GET /users/{id}`.

### 1. `internal/users/types.go`

```go
package users

type CreateRequest struct {
    Email string `json:"email"`
    Name  string `json:"name"`
}

type UserResponse struct {
    ID        string `json:"id"`
    Email     string `json:"email"`
    Name      string `json:"name"`
    RequestID string `json:"request_id"`
}
```

### 2. `internal/users/metrics.go`

```go
package users

import (
    "fmt"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/metric"
)

var (
    opsCounter   metric.Int64Counter
    errorCounter metric.Int64Counter
)

func InitMetrics() error {
    meter := otel.Meter("users")
    var err error

    opsCounter, err = meter.Int64Counter("users.operations.total",
        metric.WithDescription("Total user operations"),
        metric.WithUnit("{operation}"),
    )
    if err != nil {
        return fmt.Errorf("creating ops counter: %w", err)
    }

    errorCounter, err = meter.Int64Counter("users.errors.total",
        metric.WithDescription("Total user errors"),
        metric.WithUnit("{error}"),
    )
    if err != nil {
        return fmt.Errorf("creating error counter: %w", err)
    }

    return nil
}
```

### 3. `internal/users/handlers.go`

```go
package users

import (
    "encoding/json"
    "net/http"

    "go-chi-observability/internal/observability"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/metric"
    "go.opentelemetry.io/otel/trace"
    "go.uber.org/zap"
)

var tracer = otel.Tracer("users")

func Create(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    logger := observability.LoggerWithTrace(ctx)
    requestID := observability.RequestIDFromContext(ctx)

    ctx, span := tracer.Start(ctx, "users.create",
        trace.WithAttributes(attribute.String("request.id", requestID)),
    )
    defer span.End()

    var req CreateRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        observability.RecordError(ctx, span, logger, errorCounter, "create", "invalid request body", err, http.StatusBadRequest, w)
        return
    }

    // ... business logic ...

    opsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "create")))
    span.SetStatus(codes.Ok, "")

    logger.Info("user created",
        zap.String("operation", "create"),
        zap.String("request_id", requestID),
    )

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(UserResponse{
        ID:        "generated-id",
        Email:     req.Email,
        Name:      req.Name,
        RequestID: requestID,
    })
}
```

### 4. `internal/users/routes.go`

```go
package users

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router) {
    r.Route("/users", func(r chi.Router) {
        r.Post("/", Create)
    })
}
```

### 5. Wire it up

**`internal/server/router.go`** — add one line:

```go
users.RegisterRoutes(r)
```

**`cmd/api/init.go`** — add one line:

```go
if err := users.InitMetrics(); err != nil {
    return nil, err
}
```

Done. The new domain has full observability: automatic request tracing via middleware, custom spans, custom metrics, trace-correlated logging, and standardised error handling.
