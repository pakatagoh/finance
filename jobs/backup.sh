#!/bin/sh
# Run through the backup crontab's timeout wrapper; credentials are runtime PG* env.
set -eu
umask 077

: "${PGHOST:?Set PGHOST at runtime}"
: "${PGUSER:?Set PGUSER at runtime}"
: "${PGDATABASE:?Set PGDATABASE at runtime}"
backup_dir=${FINANCE_BACKUP_DIR:-/backups}
retention_days=${FINANCE_BACKUP_RETENTION_DAYS:-30}
case "$backup_dir" in
  /) echo "FINANCE_BACKUP_DIR must not be the filesystem root" >&2; exit 2 ;;
  /*) ;;
  *) echo "FINANCE_BACKUP_DIR must be an absolute path" >&2; exit 2 ;;
esac
case "$retention_days" in
  ''|*[!0-9]*) echo "FINANCE_BACKUP_RETENTION_DAYS must be a positive integer" >&2; exit 2 ;;
esac
if ! [ "$retention_days" -ge 1 ] 2>/dev/null; then
  echo "FINANCE_BACKUP_RETENTION_DAYS must be a positive integer" >&2
  exit 2
fi
export PGCONNECT_TIMEOUT="${PGCONNECT_TIMEOUT:-10}"

mkdir -p -- "$backup_dir"
# A shared directory lock protects cooperating workers/manual calls of this script.
# Never delete the lock file: removing it would allow callers to lock different inodes.
exec 9>"$backup_dir/.finance-backup.lock"
if ! flock -n 9; then
  echo "another Finance backup is already running" >&2
  exit 75
fi

tmp=
cleanup() {
  if [ -n "$tmp" ]; then
    rm -f -- "$tmp"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

timestamp=$(date -u +%Y%m%dT%H%M%SZ)
# Unique, private, same-directory staging avoids both collisions and cross-FS rename.
tmp=$(mktemp --suffix=.dump.tmp "$backup_dir/.finance-${timestamp}.XXXXXX")
filename=${tmp##*/}
filename=${filename#.}
final="$backup_dir/${filename%.tmp}"
pg_dump --format=custom --no-password --file="$tmp"
# Read the TOC and every archive entry before publishing. A real restore drill is
# still required; archive readability alone cannot prove successful restoration.
pg_restore --list "$tmp" >/dev/null
pg_restore --file=/dev/null "$tmp"
mv -- "$tmp" "$final"
tmp=

# Preserve the existing k3s GNU-find policy: +30 uses rounded whole-day ages.
# Prune only matching regular archives, and only after a new archive was published.
find "$backup_dir" -maxdepth 1 -type f \
  -name 'finance-*.dump' -mtime "+$retention_days" -delete
printf 'backup created: %s\n' "$final"
