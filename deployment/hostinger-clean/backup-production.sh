#!/bin/sh
set -eu
umask 077
cd /opt/euro-suits-erp
out="/var/backups/euro-suits/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$out"
./dc exec -T postgres pg_dump -U euroerp -d euroerp -Fc > "$out/database.dump"
./dc exec -T api tar -czf - -C /data/files . > "$out/documents.tar.gz"
cp .env.production "$out/environment.private"
printf 'Backup saved: %s\n' "$out"
