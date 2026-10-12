#!/usr/bin/env python3
"""Provision the private API login without printing credentials. Idempotent.
Run with local PostgreSQL up, before `docker compose up --build`.
For production pass --env-file .env.production and the production project name
through COMPOSE_PROJECT_NAME. Does not change the owner password or business data.
"""
import argparse, os, secrets, subprocess
from pathlib import Path

root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--env-file', default='.env')
args = parser.parse_args()
path = root / args.env_file
content = path.read_text()
config = dict(line.split('=', 1) for line in content.splitlines() if '=' in line and not line.startswith('#'))
password = config.get('APP_DB_PASSWORD') or secrets.token_hex(32)
if any(c not in 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-' for c in password):
    raise SystemExit('APP_DB_PASSWORD must use letters, digits, underscore or hyphen.')
# Password is passed only on stdin; never as a process argument or tool output.
statement = """DO $$ BEGIN
IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='euroerp_runtime') THEN
CREATE ROLE euroerp_runtime NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
END IF;
IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='euroerp_app') THEN
CREATE ROLE euroerp_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
END IF;
END $$;
ALTER ROLE euroerp_app WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS PASSWORD '%s';
GRANT euroerp_runtime TO euroerp_app;
""" % password
result = subprocess.run(['docker','compose','--env-file',str(path),'exec','-T','postgres','psql','-U','euroerp','-d','euroerp','-v','ON_ERROR_STOP=1'], input=statement, text=True, capture_output=True, cwd=root, env={**os.environ, "APP_DB_PASSWORD": password, "REDIS_PASSWORD": config.get('REDIS_PASSWORD','provision-only')})
if result.returncode:
    raise SystemExit('Database provisioning failed. Check that the PostgreSQL container is running.')
if not config.get('APP_DB_PASSWORD'):
    with path.open('a') as f: f.write('\nAPP_DB_PASSWORD='+password+'\n')
if not config.get('REDIS_PASSWORD'):
    with path.open('a') as f: f.write('REDIS_PASSWORD='+secrets.token_hex(32)+'\n')
os.chmod(path, 0o600)
print('Private API database login provisioned; credentials saved only in '+path.name)
