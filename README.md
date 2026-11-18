# Finance application

A server-rendered finance application for reviewing and enriching imported
transactions.

- Go, templ, Tailwind CSS, daisyUI, HTMX, and PostgreSQL
- Server-rendered UI with no client-side state framework
- Authenticated ingestion API for importing source transactions
- Category and notes editing through the browser UI

## Getting started

Local development uses [mise](https://mise.jdx.dev/) for the pinned Go,
templ, Goose, Tailwind, and Air toolchain, and Docker Compose for PostgreSQL.

From the repository root:

```sh
mise -E development trust
mise -E development install --locked
mise -E development run db-up
mise -E development run db-migrate
mise -E development run seed-categories
mise -E development run generate
mise -E development run css
mise -E development run dev
```

Shared tool versions live in `mise.toml`. Development-only tools, local
environment values and database tasks live in `mise.development.toml`;
`mise.build.toml` adds the container scheduler without loading development
settings. Select development explicitly with `-E development` (or set
`MISE_ENV=development` in your local shell). See [`docs/toolchain.md`](docs/toolchain.md)
for pins, lockfiles and build-environment commands.

The development server runs at <http://localhost:8080>. `db-up` starts the
PostgreSQL container, while the migration and seed tasks initialize the local
database. The database is intentionally empty until those tasks are run.

### Styling

The UI uses Tailwind CSS v4 with daisyUI components. Add utility classes and
daisyUI component classes directly to the server-rendered templates; do not
edit `static/css/app.css` by hand.

The source stylesheet is `assets/css/input.css`. It imports Tailwind and loads
the vendored daisyUI plugins from `assets/css/daisyui.mjs` and
`assets/css/daisyui-theme.mjs`. Rebuild the minified browser stylesheet with:

```sh
mise run css
```

This writes the generated output to `static/css/app.css`. Run it after changing
CSS inputs or template classes. The same build step runs in CI and in the Docker
image build. See [`docs/ASSETS.md`](docs/ASSETS.md) for the asset provenance and
rebuild details.

See [`docs/container-images.md`](docs/container-images.md) for the Mise-based web,
categorizer and backup Dockerfile targets. Workers require externally mounted
job files; building them does not enable production schedules.

See the documentation under `docs/` for the local database, browser security,
and asset build workflows.
