#!/usr/bin/env python3
"""Run interactively on the prepared VPS; secrets stay in the terminal and private config."""
import getpass, os, re, shutil, subprocess
from pathlib import Path
os.umask(0o077)
os.chdir('/opt/euro-suits-erp')
config=Path('.env.production')
def run(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)
def set_values(values):
    for value in values.values():
        if any(c in value for c in "\n\r'"):
            raise SystemExit('Please enter single-line values without single quotes.')
    lines=config.read_text().splitlines()
    lines=[line for line in lines if line.split('=',1)[0] not in values]
    lines.extend(k+"='"+v+"'" for k,v in values.items())
    config.write_text('\n'.join(lines)+'\n')
    config.chmod(0o600)
print('Private setup for https://erp.example.com — no demo data will be loaded.')
existing = dict(line.split('=',1) for line in config.read_text().splitlines() if '=' in line and not line.startswith('#'))
if not existing.get('TURNSTILE_SITE_KEY', '').strip("'\"") or not existing.get('TURNSTILE_SECRET_KEY', '').strip("'\""):
    site=input('Turnstile site key for erp.example.com: ').strip()
    secret=getpass.getpass('Turnstile secret key (hidden): ').strip()
    if not site or not secret or site.startswith(('1x000','2x000','3x000')) or 'AAAAAAAAAA' in secret:
        raise SystemExit('Enter real production Turnstile keys.')
    set_values({'TURNSTILE_SITE_KEY':site,'TURNSTILE_SECRET_KEY':secret})
else:
    print('Turnstile keys already configured; no re-entry needed.')
if input('Configure outgoing SMTP email now? [y/N]: ').strip().lower()=='y':
    smtp_host=input('SMTP server [smtp-relay.brevo.com]: ').strip() or 'smtp-relay.brevo.com'
    sender=input('Brevo-verified sender email: ').strip()
    username=input('SMTP login: ').strip()
    password=getpass.getpass('SMTP password (hidden): ')
    if not sender or not username or not password:
        raise SystemExit('SMTP sender, login and password are required when configuring mail.')
    set_values({'SMTP_HOST':smtp_host,'SMTP_PORT':'587','SMTP_FROM':sender,'SMTP_USERNAME':username,'SMTP_PASSWORD':password,'SMTP_AUTH_MODE':''})
cert=Path('/etc/letsencrypt/live/erp.example.com')
if not (cert/'fullchain.pem').exists():
    print('Certificate setup: enter your contact email and review the terms when asked.')
    run('certbot','certonly','--standalone','--cert-name','erp.example.com','-d','erp.example.com')
Path('certs').mkdir(exist_ok=True)
for filename,mode in [('fullchain.pem',0o644),('privkey.pem',0o600)]:
    shutil.copyfile(cert/filename,Path('certs')/filename)
    (Path('certs')/filename).chmod(mode)
count=run('./dc','exec','-T','postgres','psql','-U','euroerp','-d','euroerp','-Atc','SELECT count(*) FROM users;',capture_output=True,text=True).stdout.strip()
if count=='0':
    run('python3','create-admin.py')
else:
    print('Account already exists; no account or password will be changed.')
run('./dc','up','-d')
run('./install-certificate-hooks.sh')
run('./backup-production.sh')
cron=Path('/etc/cron.d/euro-suits-backup')
cron.write_text('15 2 * * * root /opt/euro-suits-erp/backup-production.sh >> /var/log/euro-suits-backup.log 2>&1\n')
cron.chmod(0o644)
print('Services started. Tell the assistant setup is complete so it can verify HTTPS and the empty database.')
print('Login: https://erp.example.com/admin — username admin, password you entered.')
