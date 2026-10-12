#!/usr/bin/env python3
import getpass, os, subprocess
password = getpass.getpass('Choose admin password (12–128 characters): ')
if not 12 <= len(password) <= 128:
    raise SystemExit('Use a password between 12 and 128 characters.')
if password != getpass.getpass('Confirm admin password: '):
    raise SystemExit('Passwords do not match.')
subprocess.run(['./dc','run','--rm','--no-deps','-e','OWNER_PASSWORD','migrate','create-owner'],
               env={**os.environ,'OWNER_PASSWORD':password}, check=True)
print('Super Admin created. Username: admin. The password was not written to the config file.')
