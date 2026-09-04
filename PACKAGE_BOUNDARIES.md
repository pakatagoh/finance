# Package boundaries

The Go module is organized into the following package boundaries. A directory
may be introduced when its feature is implemented; the boundaries keep shared
composition and parallel changes predictable.

```text
cmd/finance/          single binary and command wiring
internal/api/         authenticated ingestion API handlers and middleware
internal/config/      environment configuration and validation
internal/migrations/  embedded Goose SQL migration loader
internal/storage/     pgxpool wiring and shared storage helpers
internal/transactions/ transaction queries and HTTP handlers
internal/categories/  category queries and HTTP handlers
internal/web/         templ views, layouts, components, and middleware
```

The composition root is the application entry point under `cmd/finance/`.
Feature packages expose constructors and the binary composes them. Keep wiring
changes centralized rather than duplicating composition in feature packages.

## Rules

- `internal/web` and `internal/api` must not import each other.
- Shared domain services may be used by both HTTP layers.
- Money is represented as integer minor units with an explicit direction;
  never use floating-point values for money.
- Domain tables carry creation and update timestamps; repository writes set
  update timestamps explicitly.
- Do not commit personal financial data, production secrets, private machine
  paths, deployment manifests, or environment-specific infrastructure
  configuration to this repository.
