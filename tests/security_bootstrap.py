#!/usr/bin/env python3
"""Fresh production bootstrap and startup guards in a disposable database."""
import base64, json, os, secrets, subprocess, tempfile
from pathlib import Path
root=Path(__file__).resolve().parents[1]
config=dict(line.split('=',1) for line in (root/'.env').read_text().splitlines() if '=' in line and not line.startswith('#'))
database='erp_bootstrap_'+secrets.token_hex(5)
checks=[]
def sql(query):
    return subprocess.check_output(['docker','compose','exec','-T','postgres','psql','-U','euroerp','-d','postgres','-v','ON_ERROR_STOP=1','-Atc',query],cwd=root,text=True).strip()
sql('CREATE DATABASE '+database)
try:
 with tempfile.TemporaryDirectory() as temp:
  env={**os.environ,'DATABASE_URL':f'postgres://euroerp:{config["POSTGRES_PASSWORD"]}@127.0.0.1:54329/{database}?sslmode=disable','REDIS_URL':f'redis://:{config["REDIS_PASSWORD"]}@127.0.0.1:63799/0','APP_ENV':'production','APP_ORIGIN':'https://erp.example.test','DEMO_SEED':'false','FILE_ROOT':temp,'KYC_ENCRYPTION_KEY':base64.b64encode(secrets.token_bytes(32)).decode(),'OWNER_USERNAME':'first-admin','OWNER_NAME':'Bootstrap test','OWNER_PASSWORD':'1234','TURNSTILE_SITE_KEY':'','TURNSTILE_SECRET_KEY':''}
  def command(args,success,label,changes=None):
   result=subprocess.run([str(root/'backend/erp-server'),*args],env={**env,**(changes or {})},capture_output=True,text=True,timeout=10)
   assert (result.returncode==0)==success,label
   checks.append(label)
  command(['create-owner'],False,'Production refuses a weak bootstrap password')
  env['OWNER_PASSWORD']=secrets.token_urlsafe(24)
  command(['create-owner'],True,'Production creates the initial Super Admin with a strong password')
  result=subprocess.check_output(['docker','compose','exec','-T','postgres','psql','-U','euroerp','-d',database,'-Atc',"SELECT role FROM users; SELECT count(*) FROM companies; SELECT count(*) FROM access_history;"],cwd=root,text=True).splitlines()
  assert result==['Super Admin','0','1'],result
  checks.append('Bootstrap creates no pre-approved companies and retains audit history')
  command(['create-owner'],False,'Bootstrap refuses an existing workspace')
  command([],False,'Production refuses missing bot-verification keys')
  command([],False,'Production refuses public Turnstile test keys',{'TURNSTILE_SITE_KEY':'1x00000000000000000000AA','TURNSTILE_SECRET_KEY':'1x0000000000000000000000000000000AA'})
  command([],False,'Production refuses an HTTP origin',{'APP_ORIGIN':'http://erp.example.test'})
  command([],False,'API refuses an owner or superuser database login',{'APP_ENV':'local','APP_ORIGIN':'http://localhost:8080'})
  command([],False,'Missing encryption key stops startup',{'APP_ENV':'local','KYC_ENCRYPTION_KEY':''})
 (root/'docs/security-results/bootstrap.json').write_text(json.dumps({'status':'PASS','checks':checks},indent=2))
 print('PASS: '+str(len(checks))+' bootstrap and production guards')
finally:sql('DROP DATABASE '+database+' WITH (FORCE)')
