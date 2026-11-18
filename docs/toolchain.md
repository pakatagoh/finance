# Mise toolchain environments

## Configuration ownership

- `mise.toml`: shared Go, templ and Tailwind versions; generation, CSS and unit-test tasks.
- `mise.development.toml`: Air, Goose, development-only environment values and database/dev/integration tasks.
- `mise.build.toml`: Supercronic, which is a build-only addition.
- `mise.local.toml` and `mise.*.local.toml`: optional personal overrides, excluded from Git and Docker context.

Mise always loads the base file. `-E build` adds the build overlay; it does not disable base environment settings. Therefore no database URLs or development API tokens belong in the shared base. Do not put production credentials in these files or pass them as Docker build arguments.

## Verified stable pins

Upstream release check: 2026-11-17. Keep concrete versions, not floating `latest` selectors.

- Go: 1.27.4 (patch update from 1.27.3; intentionally remain on the 1.27 line).
- templ: v0.3.1118 (from v0.3.960).
- Tailwind: v4.3.5 (from v4.3.3).
- Goose CLI: v3.30.1 (from v3.26.0).
- Air: v1.67.4 (from v1.62.0).
- Supercronic: v0.2.51 (unchanged from the proposed scheduler).
- Vendored daisyUI: 5.8.1 (from 5.7.27), required to preserve join focus/hover rules with the new Tailwind optimizer; see `docs/ASSETS.md` for release checksums and smoke-test limits.

The templ runtime and embedded Goose dependency in `go.mod` match their CLI versions. Goose's dependency requirements also update pgx and some transitive modules; these are tested with the application, not treated as CLI-only changes.

Upstream sources:

- https://go.dev/dl/
- https://github.com/a-h/templ/releases/tag/v0.3.1118
- https://github.com/tailwindlabs/tailwindcss/releases/tag/v4.3.5
- https://github.com/pressly/goose/releases/tag/v3.30.1
- https://github.com/air-verse/air/releases/tag/v1.67.4
- https://github.com/aptible/supercronic/releases/tag/v0.2.51

## Development

```sh
mise -E development trust
mise -E development install --locked
mise -E development run generate
mise -E development run css
mise -E development run test-unit
```

Development database and server commands also require `-E development`. Shared tasks remain usable without an overlay. Nested Mise tasks inherit the selected `MISE_ENV`. Do not enable development by default in committed configuration: builds must not accidentally load local database settings.

## Build environment

```sh
mise -E build trust
CGO_ENABLED=0 mise -E build install --locked
mise -E build run generate
mise -E build run css
mise -E build run test-unit
```

Set `CGO_ENABLED=0` when installing the scheduler if copying it into a minimal runtime. It affects compilation of Go-installed tools; changing it after installation does not rebuild an already installed tool. Check the installed executable's `go version -m` metadata when validating an image. Go-installed Supercronic prints `<unset>` for `-version` without upstream release ldflags; its module metadata records the installed version.

Keep Mise and build tools out of the final application/worker images. The existing Dockerfile is still the previous two-stage implementation, with matching updated tool pins; conversion to the planned Mise-based worker targets is separate work. Existing CI generation pins are synchronized and guarded by a regression check until that conversion.

## Lockfiles and updates

The initial supported platform is `linux-x64`, matching homeserver. Generate all environment locks deliberately:

```sh
mise -E development lock --platform linux-x64
mise -E build lock --platform linux-x64
```

Review and retain `mise.lock`, `mise.development.lock` and `mise.build.lock`. Copy the base and selected-environment configs and lockfiles into the eventual Docker builder. Add any other platform explicitly and verify it before claiming support.

In the validated Mise 2026.10.4 release, Go source backends skip per-platform artifact entries. The Go archive and Tailwind asset have platform URLs/checksums; Go-installed programs are compiled from pinned module versions verified through Go's module system. Do not interpret `--locked` as a checksum of every compiled executable.

After updating pins, regenerate templ output and CSS, test, and review the generated diff. Check configuration isolation and legacy pipeline pin consistency with:

```sh
python3 -m unittest discover -s build -p 'test_*.py'
```

Reference: https://mise.jdx.dev/configuration/environments.html
