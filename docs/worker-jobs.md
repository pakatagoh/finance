# Finance scheduled worker files

These versioned files are supplied to the worker images as read-only mounts.
Adding them to Git or running a validation command does not enable any live job.
The Dockerfile does not bake them into the image, and the current k3s jobs remain
unchanged until a separately approved single-writer cutover.

## Preserved job contract

The existing finance namespace CronJobs were inspected before authoring these
files; no production secret values or backup contents were read.

- `jobs/categorize.crontab`: hourly at minute zero, Singapore time; runs
  `/app/finance categorize` with a 3,300-second (55-minute) timeout.
- `jobs/backup.crontab`: daily at **03:15 Asia/Singapore**; runs `backup.sh`
  with a 3,600-second (60-minute) timeout.
- Both use TERM followed by KILL after another 30 seconds if needed.
- Both set `CRON_TZ=Asia/Singapore` and `TZ=Asia/Singapore`, so a container's
  default UTC does not silently shift the schedule or task timezone.
- Keep Supercronic's default non-overlapping mode and one categorizer replica.
  Do not add `-overlapping`. Disable the old k3s categorization job before
  enabling its Compose replacement; per-process serialization is not a
  cross-container or cross-platform lock.

The backup has an additional advisory lock in the shared backup directory. It
rejects a second backup with exit code 75. This protects separate processes or
containers sharing that directory, assuming the filesystem supports `flock`.
Only cooperating invocations of this script acquire that lock. The existing
k3s backup command does not, so disable the old backup CronJob before enabling
its replacement too. This is not a distributed lock across unrelated destinations.

## Required runtime mounts

Categorizer:

- `jobs/categorize.crontab` → `/etc/finance/categorize.crontab`, read-only.

Backup:

- `jobs/backup.crontab` → `/etc/finance/backup.crontab`, read-only.
- `jobs/backup.sh` → `/etc/finance/backup.sh`, read-only; the cron invokes
  `/bin/sh`, so executable mode is not required on the source file.
- A persistent backup directory → `/backups`, writable by UID/GID **999:999**.
- A tmpfs mount at `/var/lib/postgresql` to override the PostgreSQL parent
  image's unused volume declaration. Never mount live database storage here.

The current k3s backup host path is `/opt/finance/backups`; do not move, prune,
chown or replace it just by preparing these files. Select the Compose host
binding and preserve existing archives during deployment planning. The backup
worker runs as `postgres`, unlike an old root-run backup process; verify host
ownership/access before cutover. Production backup directories should be
private (for example mode 0700), not world-writable as in the isolated test.

Enable Arcane Git Sync **Sync Files** for the eventual deployment project so
these companion files arrive alongside Compose. Keep runtime copies managed by
Arcane; edit the Git source rather than hand-editing sync-managed files.

## Runtime environment

Categorizer retains its existing application configuration:

- `DATABASE_URL`
- `FINANCE_JEV_ENDPOINT`, `FINANCE_JEV_MODEL`, `FINANCE_JEV_TOKEN`
- `FINANCE_CATEGORIZE_THRESHOLD`, `FINANCE_CATEGORIZE_BATCH_LIMIT`
- Any other application configuration required by the selected release.

Backup uses PostgreSQL/libpq variables, matching the current k3s job:

- Required: `PGHOST`, `PGUSER`, `PGDATABASE`.
- Normally also supplied: `PGPORT`, `PGPASSWORD` (or an approved `PGPASSFILE`).
- `PGCONNECT_TIMEOUT` defaults to 10 seconds if unset/empty.
- `FINANCE_BACKUP_DIR` defaults to `/backups`; overrides must be absolute paths.
- `FINANCE_BACKUP_RETENTION_DAYS` defaults to `30`; must be a positive integer.

Supply real secrets only through runtime configuration (Arcane environment or
approved secret mounts). Do not write them in crontabs, Git, image layers or
Docker build arguments. `--no-password` prevents an unattended password prompt;
it does not disable credentials supplied through libpq's environment/passfile.

## Backup algorithm and retention

1. Validate required connection settings, destination and retention value.
2. Set umask 077, create the destination if needed and acquire the directory lock.
3. Create a unique hidden temporary archive in that same directory, mode 0600.
4. Run `pg_dump --format=custom --no-password`.
5. Read its TOC and all archive entries using `pg_restore`, without touching a DB.
6. Atomically rename it to `finance-<UTC timestamp>-<unique suffix>.dump`.
7. Only then prune matching regular archives with GNU `find -mtime +30` by
   default. Unrelated files, directories, symlinks and hidden temporary files
   are not retention targets.

The retention semantics deliberately match the current k3s command. GNU find
rounds age down to whole 24-hour periods: `-mtime +30` first matches at 31
whole days, **not** immediately after exactly 30×24 hours. The suffix prevents
manual reruns in the same second from overwriting a previous archive.

Dump/readability failures and handled TERM/INT remove the current temporary
archive and never run retention. SIGKILL/power loss cannot run shell traps and
may leave a hidden partial file; it is not published as a completed backup.
Do not remove the persistent `.finance-backup.lock` file while any backup might
be running, or competing processes could lock different inodes.

If retention itself fails after publication, the newly completed archive stays
in place and the job exits nonzero. An atomic rename and readable archive do
not prove a future database restore; keep independent restore drills.

## Tests (no production access)

```sh
python3 -m unittest discover -s build -p 'test_worker_jobs.py'
sh -n jobs/backup.sh
# Requires the three local worker/web image tags from docs/container-images.md.
python3 build/smoke_worker_jobs.py
```

Unit tests stub the PostgreSQL clients to inject dump/readability failures,
lock contention and SIGTERM. They also check private permissions, unique file
names, safe path quoting and retention scope.

The Docker drill validates the real crontabs with Supercronic, creates a fresh
PostgreSQL 18.6 database on an internal network with tmpfs storage, applies the
real migrations and seeds only synthetic fixtures. It runs the actual backup
script, restores into a separate empty test database, and compares every public
table's rows, index and constraint definitions, and migration sequence state. The transaction
fixtures include all six kinds plus category assignments, notes and metadata.
The categorization cron command is also executed once with no pending rows and
an unreachable fixture-only provider endpoint; no AI provider is contacted.
It verifies migrations remain repeatable after restore, successful retention,
and preservation of existing archives on a real connection/database failure.
It publishes no host ports and verifies removal of its temporary containers,
network and backup files.

## Deployment boundaries

- [CI publishes matching worker images](container-images.md#ci-and-publishing)
  after the synthetic drill passes on `main`. Publishing does not deploy a
  Compose project, sync Arcane or activate schedules.
- No off-host replication, backup encryption, failure alerts or missed-backup
  monitoring. Local-only backups cannot survive loss of the host/storage.
- Supercronic does not replay downtime schedules. Decide catch-up and alerts
  before unattended production use; log output alone is not monitoring.
- No real AI calls, bank-tracker replay or production restore rehearsal.

Reference: https://github.com/aptible/supercronic/tree/v0.2.51#timezone
