# Finance container image targets

One Dockerfile produces three final images. The supported platform is currently
**linux/amd64**; other architectures fail explicitly until their Mise artifacts
and runtime checks are added.

## Build

```sh
# No --target remains the web default, preserving the existing k3s/CI contract.
docker build --pull -t finance:mise-dockerfile-runtime .
docker build --pull --target categorizer -t finance:mise-dockerfile-categorizer .
docker build --pull --target backup -t finance:mise-dockerfile-backup .
```

`--target runtime` explicitly selects the same web target. Intermediate `build`
and `finance-base` stages are not additional deployed services.

The builder copies the pinned static Mise binary from the official scratch
image into Debian Bookworm. This follows Mise's documented own-image method,
keeps the builder/runtime distribution family aligned, and retains the existing
Bookworm CA package-floor policy. It avoids applying that Debian-12-specific
floor to the official Mise Debian-13 image.

The builder installs from `mise.toml`, `mise.build.toml` and their generated locks.
Go, templ, Tailwind and Supercronic versions are not duplicated as Docker ARGs.
`MISE_ENV=build`, `CGO_ENABLED=0` and `GOTOOLCHAIN=local` apply before tool
installation. Configuration discovery runs from `/toolchain`, outside the source
directory; development and personal config are excluded from the Docker context.
No production credentials are required to build.

## Runtime contracts

### runtime (default)

- Non-root UID/GID `1000:1000`, work directory `/app`.
- Entrypoint `/app/finance`, default command `serve`, container port `8080`.
- Same binary supports `migrate` by overriding the command.
- Includes generated static browser assets, CA bundle and timezone data.
- No Mise, compiler, templ, Tailwind, Supercronic or PostgreSQL client tools.

The existing workflow still builds/publishes this default image; the new worker
images are local-only until an explicitly authorized CI publishing change.

### categorizer

- Inherits the same Finance executable and runtime dependencies.
- Non-root UID/GID `1000:1000`, work directory `/app`.
- Entrypoint `/usr/local/bin/supercronic`.
- Default crontab path `/etc/finance/categorize.crontab`.
- Includes no browser assets, compiler or Mise installation.

Mount `jobs/categorize.crontab` read-only and supply application credentials at
runtime. It runs `/app/finance categorize` directly in this worker, not inside
the web container, and needs no Docker socket. The file is versioned separately,
not baked into the image; adding it to Git does not activate a schedule.

### backup

- Based on `postgres:18.6-bookworm` so `pg_dump`/`pg_restore` match the database
  release. Uses the upstream `postgres` user, verified UID/GID `999:999`.
- Writable work directory `/backups`; supply a correctly owned persistent backup
  bind mount there in deployment. This is **not** a database data directory.
- Entrypoint `/usr/local/bin/supercronic`; overrides upstream's server bootstrap.
- Default crontab path `/etc/finance/backup.crontab`.
- Includes PostgreSQL client tools, CA certificates, timezone data and Supercronic.
- No Finance application, browser assets, compiler or Mise installation.

The PostgreSQL parent image also contains server binaries, declares port 5432 as
image metadata, and declares `/var/lib/postgresql` as a volume. The backup
entrypoint does not start a server, and EXPOSE does not publish any host port.
Mount `/var/lib/postgresql` as **tmpfs** for this worker to prevent Docker from
creating unused anonymous database volumes on recreations. Do not attach the
production PostgreSQL data directory to a backup worker.

The crontabs and backup script are supplied separately under `jobs/`, not baked
into these images. See [`worker-jobs.md`](worker-jobs.md) for mounts, schedules,
retention and tests. Both workers fail if their required crontab is missing;
they do not silently invent a schedule.

## Runtime settings and permissions

Set `TZ=Asia/Singapore` in the eventual worker Compose configuration. The images
supply timezone rules; the image itself does not choose production schedule times.
Set appropriate stop grace periods and mount scripts/crontabs read-only. Financial
credentials remain runtime environment values, not image layers/build arguments.

Web and categorizer inherit the CA-floor check from `finance-base`; independent
builder and backup bases enforce it too. The guard validates Debian package
version, not every issuer's distrust status or live outbound TLS.

## Validation

```sh
python3 -m unittest discover -s build -p 'test_*.py'
# Requires the three local image tags built above and a Linux Docker bridge.
python3 build/smoke_container_images.py
# Real job files, full synthetic backup/restore and retention/failure checks.
python3 build/smoke_worker_jobs.py
```

The smoke script uses unique temporary containers, an internal Docker network,
synthetic PostgreSQL credentials/data and tmpfs DB storage. It publishes no host
ports and verifies cleanup. It checks:

- Entrypoints/default commands, non-root users, CA floors and Singapore timezone.
- No build tools/configuration or development credentials in final image settings.
- Scheduler validation of valid/invalid synthetic crontabs and graceful SIGTERM.
- Categorizer command availability without contacting an AI provider.
- Application migrations, repeated migrations and backup-client schema dumping.
- Actual web serving, liveness/readiness/startup endpoints and CSS delivery.

`smoke_worker_jobs.py` additionally exercises the real backup script, restores
all synthetic application tables, compares rows/indexes/constraints/sequence state, and
checks successful retention and failure preservation. Neither script tests
production AI categorization, off-host replication, alert delivery or a live
deployment. An image build or a restore drill does not activate unattended jobs.

Reference: https://mise.jdx.dev/mise-cookbook/docker.html#copy-the-binary-into-your-own-image
