#!/bin/sh
set -eu
[ "$(basename "$RENEWED_LINEAGE")" = "erp-origin" ] || exit 0
install -m 644 "$RENEWED_LINEAGE/fullchain.pem" /opt/euro-suits-erp/certs/origin-fullchain.pem
install -m 600 "$RENEWED_LINEAGE/privkey.pem" /opt/euro-suits-erp/certs/origin-privkey.pem
cd /opt/euro-suits-erp
./dc exec -T nginx nginx -s reload
