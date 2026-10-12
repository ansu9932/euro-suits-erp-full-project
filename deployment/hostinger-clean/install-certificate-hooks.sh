#!/bin/sh
set -eu
mkdir -p /etc/letsencrypt/renewal-hooks/pre /etc/letsencrypt/renewal-hooks/post /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/pre/euro-suits <<'EOF'
#!/bin/sh
set -eu
/opt/euro-suits-erp/dc stop nginx
EOF
cat > /etc/letsencrypt/renewal-hooks/deploy/euro-suits <<'EOF'
#!/bin/sh
set -eu
expected=$(sed -n "s/^PUBLIC_HOST='\(.*\)'$/\1/p" /opt/euro-suits-erp/.env.production)
[ "$(basename "$RENEWED_LINEAGE")" = "$expected" ] || exit 0
install -m 644 "$RENEWED_LINEAGE/fullchain.pem" /opt/euro-suits-erp/certs/fullchain.pem
install -m 600 "$RENEWED_LINEAGE/privkey.pem" /opt/euro-suits-erp/certs/privkey.pem
EOF
cat > /etc/letsencrypt/renewal-hooks/post/euro-suits <<'EOF'
#!/bin/sh
set -eu
/opt/euro-suits-erp/dc up -d --no-deps nginx
EOF
chmod 700 /etc/letsencrypt/renewal-hooks/pre/euro-suits /etc/letsencrypt/renewal-hooks/post/euro-suits /etc/letsencrypt/renewal-hooks/deploy/euro-suits
systemctl enable --now certbot.timer
