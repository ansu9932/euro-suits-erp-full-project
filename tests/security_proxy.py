#!/usr/bin/env python3
"""Check production Nginx using a disposable proxy and explicitly trusted test
certificate. Does not install a certificate or change the running ERP proxy.
"""
import json, secrets, socket, subprocess, tempfile, time
from pathlib import Path
root=Path(__file__).resolve().parents[1]
def run(*args,**kw):return subprocess.run(args,check=True,text=True,capture_output=True,**kw).stdout
def freeport():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
name='erp-tls-check-'+secrets.token_hex(4);http_port,tls_port=freeport(),freeport();started=False
checks=[]
try:
 with tempfile.TemporaryDirectory(prefix='erp-tls-') as temp:
  folder=Path(temp)
  run('openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(folder/'privkey.pem'),'-out',str(folder/'fullchain.pem'),'-days','1','-subj','/CN=qa.erp.test','-addext','subjectAltName=DNS:qa.erp.test')
  (folder/'nginx.conf').write_text((root/'deploy/nginx.production.conf').read_text().replace('${PUBLIC_HOST}','qa.erp.test'))
  networks=json.loads(run('docker','inspect','euro-suits-erp-nginx-1'))[0]['NetworkSettings']['Networks']
  network=next(n for n in networks if n.endswith('_default'))
  run('docker','run','-d','--name',name,'--network',network,'-p',f'127.0.0.1:{http_port}:80','-p',f'127.0.0.1:{tls_port}:443','-v',str(folder/'nginx.conf')+':/etc/nginx/conf.d/default.conf:ro','-v',temp+':/etc/nginx/certs:ro','nginx:stable-alpine');started=True
  for _ in range(50):
   try:
    headers=run('curl','--silent','--show-error','--max-time','2','--head','-H','Host: qa.erp.test',f'http://127.0.0.1:{http_port}/admin');break
   except subprocess.CalledProcessError:time.sleep(.1)
  assert '308' in headers and 'Location: https://qa.erp.test/admin' in headers,headers
  checks.append('HTTP redirects to the configured HTTPS hostname')
  tls=run('curl','--silent','--show-error','--max-time','10','--head','--resolve',f'qa.erp.test:{tls_port}:127.0.0.1','--cacert',str(folder/'fullchain.pem'),f'https://qa.erp.test:{tls_port}/admin')
  assert '200 OK' in tls,tls
  checks.append('HTTPS page loads with certificate verification enabled')
  for key in ['strict-transport-security:','content-security-policy:','x-content-type-options:','x-frame-options:','permissions-policy:']:
   assert key in tls.lower(),key
   checks.append('HTTPS includes '+key[:-1])
  assert "'nonce-" in tls and "'unsafe-eval'" not in tls and "script-src 'self'" in tls
  checks.append('SvelteKit supplies a nonce for its bootstrap script without unsafe-eval')
  status=run('curl','--silent','--show-error','--max-time','5','--output','/dev/null','--write-out','%{http_code}','--resolve',f'qa.erp.test:{tls_port}:127.0.0.1','--cacert',str(folder/'fullchain.pem'),f'https://qa.erp.test:{tls_port}/.env')
  assert status=='404',status;checks.append('Proxy blocks hidden configuration paths')
  bad=subprocess.run(['curl','--silent','--max-time','3','--head','-H','Host: untrusted.example',f'http://127.0.0.1:{http_port}/admin'],capture_output=True,text=True)
  assert bad.returncode!=0 and '308' not in bad.stdout
  checks.append('Unrecognized Host headers cannot become redirect destinations')
 (root/'docs/security-results/proxy.json').write_text(json.dumps({'status':'PASS','checks':checks},indent=2))
 print('PASS: '+str(len(checks))+' production proxy checks')
finally:
 if started:run('docker','rm','-f',name)
