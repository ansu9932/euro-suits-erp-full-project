#!/usr/bin/env python3
import base64, getpass, os, re, secrets
from pathlib import Path
os.umask(0o077)
path = Path('.env.production')
if path.exists():
    raise SystemExit('Configuration already exists. Keep it: do not regenerate encryption/database keys.')
host = input('ERP hostname (for example erp.example.com): ').strip().lower()
if not re.fullmatch(r'[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?', host) or '.' not in host:
    raise SystemExit('Enter a hostname without https:// or a path.')
site = input('Real Turnstile site key: ').strip()
secret = getpass.getpass('Real Turnstile secret key: ').strip()
if not site or not secret or site.startswith(('1x000','2x000','3x000')):
    raise SystemExit('Real Turnstile keys are required.')
settings = {
 'APP_ENV':'production', 'APP_ORIGIN':'https://'+host, 'PUBLIC_HOST':host,
 'POSTGRES_PASSWORD':secrets.token_hex(32), 'APP_DB_PASSWORD':secrets.token_hex(32),
 'REDIS_PASSWORD':secrets.token_hex(32),
 'KYC_ENCRYPTION_KEY':base64.b64encode(secrets.token_bytes(32)).decode(),
 'DEMO_SEED':'false', 'OWNER_USERNAME':'admin', 'OWNER_NAME':'Super Admin',
 'OWNER_PASSWORD':'', 'TURNSTILE_SITE_KEY':site, 'TURNSTILE_SECRET_KEY':secret,
 'SMTP_HOST':'', 'SMTP_PORT':'587', 'SMTP_FROM':'', 'SMTP_USERNAME':'',
 'SMTP_PASSWORD':'', 'SMTP_AUTH_MODE':'',
}
if input('Configure outgoing SMTP email now? [y/N]: ').strip().lower() == 'y':
    settings['SMTP_HOST'] = input('SMTP host (Brevo: smtp-relay.brevo.com): ').strip()
    settings['SMTP_FROM'] = input('Verified sender email: ').strip()
    settings['SMTP_USERNAME'] = input('SMTP login: ').strip()
    settings['SMTP_PASSWORD'] = getpass.getpass('SMTP password: ')
for value in settings.values():
    if any(c in value for c in "\r\n'"):
        raise SystemExit('Values containing newlines or single quotes require manual configuration.')
# Keep generated database passwords unquoted for provision_database.py.
plain = {'POSTGRES_PASSWORD','APP_DB_PASSWORD','REDIS_PASSWORD'}
path.write_text(''.join(k+'='+ (v if k in plain else "'"+v+"'")+'\n' for k,v in settings.items()))
print('Saved private .env.production. Back it up securely; its encryption key is required to recover documents.')
