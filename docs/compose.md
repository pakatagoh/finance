# Compose development environment

The repository's `compose.yaml` provides a PostgreSQL-only development
database. It does not start application containers or seed data. The database
starts empty so migrations and application initialization can be tested
explicitly.

## Contract

| Item | Value |
| --- | --- |
| Service name | `postgres` |
| Image | A pinned PostgreSQL image declared in `compose.yaml` |
| Container port | `5432` |
| Credentials | Deterministic development-only values declared in `compose.yaml`; never reuse them outside local development |
| Connection URL | Use the development URL declared by the project configuration |
| Storage | A named Compose volume mounted at the PostgreSQL data directory |
| Healthcheck | Bounded `pg_isready` check declared in `compose.yaml` |

The image reference is kept in the Compose file. When changing it, verify the
image and update the file's pin according to the project's dependency policy.
Do not copy local development credentials into production configuration.

## Usage

```sh
docker compose up -d
docker compose ps                 # wait until the database is healthy
docker compose down -v            # stop and remove local data
```

Treat a healthy Compose status, or an equivalent `pg_isready` result, as the
`db-ready` signal before running migrations or the application.

## Recreate an empty database

The supported way to obtain a fresh database is to remove the named volume and
let PostgreSQL initialize again:

```sh
docker compose down -v
docker compose up -d
```

There is deliberately no truncate script and no automatic seed data. Any
future reset task should wrap these commands rather than silently deleting
unrelated databases.

## No automatic seeding

- No initialization SQL is mounted into the PostgreSQL container.
- No application or migration container runs automatically on `up`.
- Local startup leaves the database empty until a developer explicitly runs
  migrations or other initialization commands.
