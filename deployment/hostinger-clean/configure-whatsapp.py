#!/usr/bin/env python3
"""Run on the ERP server. Reads credentials privately; never writes them to console."""
from pathlib import Path
import getpass,os,re,secrets,subprocess
root=Path('/opt/euro-suits-erp')
path=root/'.env.whatsapp'
existing={}
if path.exists():
 existing=dict(line.split('=',1) for line in path.read_text().splitlines() if '=' in line)
values={}
for key,label,default in [('WHATSAPP_WABA_ID','WhatsApp Business account ID',''),('WHATSAPP_PHONE_ID','Connected phone number ID','')]:
 value=input(label+' ['+existing.get(key,default)+']: ').strip() or existing.get(key,default)
 if not re.fullmatch(r'[0-9]{5,30}',value):raise SystemExit('Invalid ID; no files changed.')
 values[key]=value
for key,label in [('WHATSAPP_ACCESS_TOKEN','Production system-user access token'),('WHATSAPP_APP_SECRET','Meta app secret')]:
 value=getpass.getpass(label+' (hidden; leave empty to keep existing): ').strip() or existing.get(key,'')
 if not re.fullmatch(r'[A-Za-z0-9_\-]{16,4096}',value):raise SystemExit('Invalid credential; no files changed.')
 values[key]=value
values['WHATSAPP_VERIFY_TOKEN']=existing.get('WHATSAPP_VERIFY_TOKEN') or secrets.token_urlsafe(32)
values['WHATSAPP_API_VERSION']='v25.0'
os.umask(0o077)
tmp=path.with_suffix('.tmp');tmp.write_text(''.join(k+'='+v+'\n' for k,v in values.items()));os.replace(tmp,path);path.chmod(0o600)
subprocess.run([str(root/'dc'),'run','--rm','migrate'],check=True,cwd=root)
subprocess.run([str(root/'dc'),'up','-d','--no-deps','api'],check=True,cwd=root)
print('Credentials saved privately. Automatic sending remains controlled from Developer → WhatsApp.')
print('Webhook URL: https://erp.example.com/api/whatsapp/webhook')
print('The verify token is stored in /opt/euro-suits-erp/.env.whatsapp; enter it privately in Meta.')
