#!/usr/bin/env python3
"""Generate local configuration once; never reset an existing workspace."""
import base64
import os
from pathlib import Path
import secrets

root = Path(__file__).resolve().parent
path = root / '.env'
if path.exists():
    print('Existing .env retained. No accounts, passwords or data changed.')
    raise SystemExit(0)
password = secrets.token_urlsafe(24)
values = {
    'APP_ENV': 'local', 'APP_ORIGIN': 'http://localhost:8080', 'DEMO_SEED': 'false',
    'APP_DB_PASSWORD': secrets.token_hex(32), 'REDIS_PASSWORD': secrets.token_hex(32), 'POSTGRES_PASSWORD': secrets.token_hex(24), 'OWNER_PASSWORD': password,
    'KYC_ENCRYPTION_KEY': base64.b64encode(secrets.token_bytes(32)).decode(),
    'SMTP_HOST': '', 'SMTP_PORT': '587', 'SMTP_FROM': '', 'SMTP_USERNAME': '', 'SMTP_PASSWORD': '',
}
with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as f:
    f.write(''.join(f'{key}={value}\n' for key, value in values.items()))
access = root / 'LOCAL-ACCESS.txt'
with os.fdopen(os.open(access, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), 'w') as f:
    f.write('LOCAL COMPANY SETUP ACCESS\n\nOpen: http://localhost:8080\n'
            'finance — Finance Head: create and prepare company drafts\n'
            'md — Managing Director: review summaries and approve\n'
            'admin — Super Admin: full KYC, configuration and approval\n\n'
            'Initial password for these three local accounts: ' + password + '\n\n'
            'Initialize the empty workspace using README instructions.\n'
            'Keep this file and .env private. These credentials are not for production.\n')
print('Private local configuration created. Follow README to initialize the empty workspace.')
