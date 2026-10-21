#!/bin/sh
set -eu

# Debian bookworm's eMudhra distrust update. Keep the floor shared by both
# stages; a successful apt install alone can silently retain an old package.
minimum='20250419~deb12u2'
installed=$(dpkg-query -W -f='${Version}' ca-certificates)
if ! dpkg --compare-versions "$installed" ge "$minimum"; then
    printf 'ca-certificates %s is below required %s\n' "$installed" "$minimum" >&2
    exit 1
fi
printf 'ca-certificates %s meets minimum %s\n' "$installed" "$minimum"
