#!/bin/sh
# Back up platform data before a release and rotate old backups and releases.
# Usage: backup-data.sh <version being deployed>
# Keeps the newest $KEEP data backups and release directories; every action is
# appended to backups/backup.log. The release that `current` points to is never removed.
set -eu
VERSION=${1:?version required}
ROOT=${DEVICETRUST_ROOT:-/opt/devicetrust}
DATA=${DEVICETRUST_DATA:-/var/lib/devicetrust}
BACKUPS=$ROOT/backups
LOG=$BACKUPS/backup.log
KEEP=10
now=$(date -u +%Y%m%dT%H%M%SZ)
file=$BACKUPS/data-before-$VERSION-$now.tar.gz

mkdir -p "$BACKUPS"
tar -czf "$file" -C "$DATA" platform
tar -tzf "$file" >/dev/null # unreadable archive fails the deploy here
echo "$now backup $(basename "$file") $(stat -c %s "$file") bytes sha256 $(sha256sum "$file" | cut -d' ' -f1)" >>"$LOG"

ls -1t "$BACKUPS"/data-before-*.tar.gz | tail -n +$((KEEP + 1)) | while read -r old; do
	rm -f "$old"
	echo "$now pruned $(basename "$old")" >>"$LOG"
done

current=$(readlink -f "$ROOT/current")
# Releases are ordered by version, newest first: cp -a carries the old mtime.
ls -1d "$ROOT"/releases/*/ | sed 's:/$::' | sort -rV | tail -n +$((KEEP + 1)) | while read -r old; do
	[ "$(readlink -f "$old")" = "$current" ] && continue
	rm -rf "$old"
	echo "$now pruned release $(basename "$old")" >>"$LOG"
done
echo "$file"
