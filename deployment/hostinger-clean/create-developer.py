#!/usr/bin/env python3
"""Run privately on the VPS; never print or persist the chosen password."""
import getpass, os, subprocess
os.chdir('/opt/euro-suits-erp')
email = os.environ.get('DEVELOPER_EMAIL', '').strip() or input('Developer email: ').strip()
if '@' not in email or any(c.isspace() for c in email):
    raise SystemExit('Enter a valid developer email.')
password = getpass.getpass('Choose developer password (12–128 characters): ')
if not 12 <= len(password) <= 128:
    raise SystemExit('Use a password between 12 and 128 characters.')
if password != getpass.getpass('Confirm developer password: '):
    raise SystemExit('Passwords do not match.')
subprocess.run(['./dc', 'run', '--rm', '--no-deps', '-e', 'DEVELOPER_EMAIL', '-e', 'DEVELOPER_PASSWORD', 'migrate', 'create-developer'], env={**os.environ, 'DEVELOPER_EMAIL':email, 'DEVELOPER_PASSWORD':password}, check=True)
print('Developer account created. Username: developer. Login: https://erp.example.com/developer')
print('Email verification is mandatory. Existing accounts were not changed.')
