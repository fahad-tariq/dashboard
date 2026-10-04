#!/usr/bin/env bash
#
# Back up all dashboard data: every file under the data and users
# directories plus a consistent snapshot of the SQLite database, taken with
# SQLite's online backup API so it is safe while the server is running (a
# plain copy of a WAL database is not). Old archives are pruned.
#
# Env:
#   DATA_DIR        bind-mounted /data/db directory (default ./data)
#   USERS_DIR       bind-mounted /data/users directory (default ./users)
#   BACKUP_DIR      where archives go (default ./backups)
#   RETENTION_DAYS  delete archives older than this (default 14)
#
# Restore: stop the container, extract the archive somewhere, copy data/ and
# users/ back, put dashboard.db at data/dashboard.db (remove any stale
# dashboard.db-wal/-shm first), then start the container.

set -euo pipefail

DATA_DIR="${DATA_DIR:-./data}"
USERS_DIR="${USERS_DIR:-./users}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
readonly RETENTION_DAYS="${RETENTION_DAYS:-14}"

log() { printf '%s %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"; }
die() { log "error: $*" >&2; exit 1; }

main() {
    [[ -d "$DATA_DIR" ]] || die "DATA_DIR $DATA_DIR does not exist"
    [[ -d "$USERS_DIR" ]] || die "USERS_DIR $USERS_DIR does not exist"
    [[ -f "$DATA_DIR/dashboard.db" ]] || die "no database at $DATA_DIR/dashboard.db"
    command -v python3 >/dev/null || die "python3 is required for the database snapshot"

    mkdir -p "$BACKUP_DIR"
    # tar's -C options are cumulative, so resolve everything to absolute paths.
    DATA_DIR="$(cd "$DATA_DIR" && pwd)"
    USERS_DIR="$(cd "$USERS_DIR" && pwd)"
    BACKUP_DIR="$(cd "$BACKUP_DIR" && pwd)"
    local stamp work archive
    stamp="$(date +%Y%m%d-%H%M%S)"
    work="$(mktemp -d)"
    # shellcheck disable=SC2064 # expand now: work is local
    trap "rm -rf '$work'" EXIT

    python3 - "$DATA_DIR/dashboard.db" "$work/dashboard.db" <<'PY'
import sqlite3, sys, urllib.parse
src = sqlite3.connect("file:" + urllib.parse.quote(sys.argv[1]) + "?mode=ro", uri=True)
dst = sqlite3.connect(sys.argv[2])
src.backup(dst)
if dst.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
    sys.exit("snapshot failed integrity_check")
dst.close()
src.close()
PY

    archive="$BACKUP_DIR/dashboard-backup-$stamp.tar.gz"
    local data_name
    data_name="$(basename "$DATA_DIR")"
    # Exclude the live database by path so the snapshot added below survives.
    # COPYFILE_DISABLE stops macOS tar adding ._ metadata files.
    local rc=0
    COPYFILE_DISABLE=1 tar -czf "$archive" \
        --exclude="$data_name/dashboard.db" \
        --exclude="$data_name/dashboard.db-wal" \
        --exclude="$data_name/dashboard.db-shm" \
        -C "$(dirname "$DATA_DIR")" "$data_name" \
        -C "$(dirname "$USERS_DIR")" "$(basename "$USERS_DIR")" \
        -C "$work" dashboard.db || rc=$?
    # GNU tar exits 1 when a file changed while being read (an atomic rename
    # during the backup); the archive is still complete, so warn and go on.
    if [[ $rc -eq 1 ]] && tar --version 2>/dev/null | grep -q 'GNU tar'; then
        log "warning: a file changed during the backup; archive kept"
    elif [[ $rc -ne 0 ]]; then
        rm -f "$archive"
        die "tar failed with status $rc"
    fi
    log "created $archive ($(du -h "$archive" | cut -f1))"

    find "$BACKUP_DIR" -maxdepth 1 -name 'dashboard-backup-*.tar.gz' -mtime "+$RETENTION_DAYS" -print -delete |
        while read -r old; do log "pruned $old"; done
}

main "$@"
