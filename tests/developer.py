#!/usr/bin/env python3
"""Isolated real HTTP + STARTTLS email tests. Never sends mail to an external service.
Creates its own database, Redis, trusted temporary test CA and mail receiver.
Set DEVELOPER_TEST_HOLD=1 to retain this isolated workspace for browser QA until
the stop file printed by the script is created. Cleanup runs afterwards.
"""
from pathlib import Path
from email import policy
from email.parser import Parser
import base64,concurrent.futures,http.cookiejar,http.server,json,os,re,secrets,socket,socketserver,ssl,subprocess,tempfile,threading,time,urllib.request,urllib.error
ROOT=Path(__file__).resolve().parents[1]
config=dict(x.split('=',1) for x in (ROOT/'.env').read_text().splitlines() if '=' in x and not x.startswith('#'))
name='erp_dev_test_'+secrets.token_hex(4);report=[];proc=None;redis=False;db=False;role=False
def run(*args,**kw):
 p=subprocess.run(args,capture_output=True,text=True,**kw)
 if p.returncode:raise subprocess.CalledProcessError(p.returncode,args,output=p.stdout,stderr=p.stderr)
 return p.stdout.strip()
def sql(q):return run('docker','compose','exec','-T','postgres','psql','-U','euroerp','-d',name,'-v','ON_ERROR_STOP=1','-Atc',q,cwd=ROOT)
def port():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
def check(ok,label):
 if not ok:raise AssertionError(label)
 report.append(label)
class Client:
 def __init__(self):self.cookies=http.cookiejar.CookieJar();self.o=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.cookies));self.csrf=''
 def call(self,path,body=None,status=200,key=None):
  headers={'Origin':base,'Content-Type':'application/json','X-CSRF-Token':self.csrf}
  if key:headers['Idempotency-Key']=key
  req=urllib.request.Request(base+'/api'+path,data=None if body is None else json.dumps(body).encode(),headers=headers)
  try:r=self.o.open(req,timeout=20)
  except urllib.error.HTTPError as e:r=e
  data=json.load(r);check(r.status==status,f'{path}: HTTP {status} (actual {r.status}; {data if r.status!=status else "ok"})')
  if data.get('csrf'):self.csrf=data['csrf']
  return data
 def login(self,username,password='1234',**kw):return self.call('/login',{'username':username,'password':password,**kw})
messages=[]
mail_test_mode="accept" # Local bank-email tests override this only after OTP checks.
class MailHandler(socketserver.StreamRequestHandler):
 def handle(self):
  def send(text):self.wfile.write(text.encode()+b'\r\n');self.wfile.flush()
  send('220 localhost test SMTP')
  while line:=self.rfile.readline():
   command=line.decode().strip().upper()
   if command.startswith('EHLO'):send('250-localhost\r\n250-STARTTLS\r\n250 AUTH PLAIN')
   elif command=='STARTTLS':
    send('220 Ready');self.connection=mailtls.wrap_socket(self.connection,server_side=True);self.rfile=self.connection.makefile('rb');self.wfile=self.connection.makefile('wb')
   elif command.startswith('RCPT TO') and mail_test_mode=='reject_before_data':send('550 Synthetic recipient rejection before DATA')
   elif command=='DATA':
    current_mail_mode=mail_test_mode
    send('354 Send message');parts=[]
    while (part:=self.rfile.readline()) not in (b'.\r\n',b''):parts.append(part)
    message=b''.join(parts).decode();messages.append(message)
    (temp/'mail-count.txt').write_text(str(len(messages)))
    match=re.search(r'code is: (\d{6})',message)
    if match:(temp/'latest-test-otp.txt').write_text(match[1])
    if current_mail_mode=='drop_after_data':return # Simulate lost acceptance after receiving DATA.
    if current_mail_mode=='slow_accept':time.sleep(.25)
    send('250 Accepted')
   elif command=='QUIT':send('221 Goodbye');break
   else:send('250 OK')
class SMTPServer(socketserver.ThreadingTCPServer):allow_reuse_address=True;daemon_threads=True
def code():
 message=Parser(policy=policy.default).parsestr(messages[-1])
 plain=message.get_body(preferencelist=('plain',));html=message.get_body(preferencelist=('html',))
 assert plain is not None and html is not None,'OTP needs both plain text and branded HTML'
 assert 'cid:euro-suits-logo' in html.get_content(),'OTP is missing its inline logo reference'
 assert any(p.get('Content-ID')=='<euro-suits-logo>' and p.get_content_type()=='image/png' for p in message.walk()),'OTP logo attachment missing'
 return re.search(r'code is: (\d{6})',plain.get_content())[1]
def unlock(uid):run('docker','exec',name,'redis-cli','DEL',f'otp:cooldown:{uid}',f'otp:send:{uid}')
def mfa(c,username='developer',password='1234'):
 result=c.login(username,password);check(result.get('otp_required') and 'user' not in result and 'csrf' not in result,'Password alone gives no authenticated session');c.call('/login/otp',{'code':code()});return c
try:
 run('docker','compose','exec','-T','postgres','createdb','-U','euroerp',name,cwd=ROOT);db=True
 with tempfile.TemporaryDirectory(prefix=name) as folder:
  temp=Path(folder);os.chmod(temp,0o700)
  run('openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(temp/'key.pem'),'-out',str(temp/'cert.pem'),'-days','1','-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost')
  mailtls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);mailtls.load_cert_chain(temp/'cert.pem',temp/'key.pem')
  mailserver=SMTPServer(('127.0.0.1',0),MailHandler);threading.Thread(target=mailserver.serve_forever,daemon=True).start()
  rp,ap,proxyport=port(),port(),port();base=f'http://127.0.0.1:{proxyport}'
  run('docker','run','-d','--name',name,'-p',f'127.0.0.1:{rp}:6379','redis:8-alpine','redis-server','--save','','--appendonly','no');redis=True
  env={**os.environ,'DATABASE_URL':f'postgres://euroerp:{config["POSTGRES_PASSWORD"]}@127.0.0.1:54329/{name}?sslmode=disable','REDIS_URL':f'redis://127.0.0.1:{rp}/0','HTTP_ADDR':f'127.0.0.1:{ap}','APP_ORIGIN':base,'APP_ENV':'local','FILE_ROOT':str(temp/'files'),'KYC_ENCRYPTION_KEY':base64.b64encode(secrets.token_bytes(32)).decode(),'OWNER_PASSWORD':'1234','DEVELOPER_PASSWORD':'1234','DEVELOPER_EMAIL':'owner@example.test','SMTP_HOST':'localhost','SMTP_PORT':str(mailserver.server_address[1]),'SMTP_FROM':'sender@example.test','SMTP_AUTH_MODE':'','SMTP_USERNAME':'','SMTP_PASSWORD':'','SSL_CERT_FILE':str(temp/'cert.pem'),'SSL_CERT_DIR':str(temp),'DEMO_SEED':'false'}
  binary=str(ROOT/'backend/erp-server')
  run(binary,'initialize-company-workspace',env=env);run(binary,'create-developer',env=env)
  try:run(binary,'create-developer',env=env);raise AssertionError('bootstrap overwrote developer')
  except subprocess.CalledProcessError:check(True,'Developer bootstrap refuses an existing account')
  runtime=name+'_api';runtimepw=secrets.token_hex(24);sql("CREATE ROLE "+runtime+" LOGIN PASSWORD '"+runtimepw+"'; GRANT euroerp_runtime TO "+runtime);role=True
  env['DATABASE_URL']=f'postgres://{runtime}:{runtimepw}@127.0.0.1:54329/{name}?sslmode=disable'
  log=open(temp/'server.log','w+');proc=subprocess.Popen([binary],env=env,stdout=log,stderr=log)
  class Proxy(http.server.BaseHTTPRequestHandler):
   def do_GET(self):self.proxy()
   def do_POST(self):self.proxy()
   def log_message(self,*args):pass
   def proxy(self):
    target=(f'http://127.0.0.1:{ap}' if self.path.startswith('/api/') else 'http://localhost:8080')+self.path
    headers={k:v for k,v in self.headers.items() if k.lower() not in ['host','connection','accept-encoding']}
    body=self.rfile.read(int(self.headers.get('Content-Length',0))) if self.command=='POST' else None
    try:r=urllib.request.urlopen(urllib.request.Request(target,data=body,headers=headers,method=self.command),timeout=25)
    except urllib.error.HTTPError as e:r=e
    content=r.read();self.send_response(r.status)
    for k,v in r.headers.items():
     if k.lower() not in ['connection','transfer-encoding','content-length']:self.send_header(k,v)
    self.send_header('Content-Length',str(len(content)));self.end_headers();self.wfile.write(content)
  proxy=http.server.ThreadingHTTPServer(('127.0.0.1',proxyport),Proxy);threading.Thread(target=proxy.serve_forever,daemon=True).start()
  def wait_server():
   for _ in range(200):
    try:urllib.request.urlopen(f'http://127.0.0.1:{ap}/api/health',timeout=1);return
    except Exception:
     if proc.poll() is not None:log.seek(0);raise RuntimeError(log.read())
     time.sleep(.1)
   raise RuntimeError('Test server did not become healthy')
  wait_server()
  admin=Client();admin.login('admin');finance=Client();finance.login('finance');anonymous=Client();dev=Client()
  for c in [anonymous,admin,finance]:c.call('/developer',status=401 if c is anonymous else 403)
  for c in [admin,finance]:c.call('/password',{'current_password':'1234','new_password':'A-new-password-2026'},403)
  did=int(sql("SELECT id FROM users WHERE is_developer"));fid=int(sql("SELECT id FROM users WHERE username='finance'"))
  dev.login('developer',portal='developer');first=code();check('owner@example.test' in messages[-1],'OTP delivered only to the configured owner mailbox')
  check(not any(c.name=='erp_session' for c in dev.cookies),'No session cookie before OTP verification')
  dev.call('/developer',status=401);dev.call('/login/otp',{'code':'000000' if first!='000000' else '999999'},401);dev.call('/login/otp',{'code':first});dev.call('/login/otp',{'code':first},401)
  data=dev.call('/developer');check(data['user']['portal']=='developer' and data['user']['is_developer'],'Owner lands in the separate developer portal')
  anonymous.call('/developer/performance',status=401)
  admin.call('/developer/performance',status=403)
  perf=dev.call('/developer/performance')
  check(perf['database']['max']==24 and perf['limits']['office_logins_per_15_minutes']==2000,'Developer performance endpoint reports bounded pool and office limits')
  check(set(perf)=={'observed_at','uptime_seconds','traffic','database','process','diagnostics','worker','limits'},'Performance response has no user, credential or business-record fields')
  check(data['health']['email_configured'],'Local test SMTP is configured using verified STARTTLS')
  anonymous.call('/developer/whatsapp',status=401)
  admin.call('/developer/whatsapp',status=403)
  wa=dev.call('/developer/whatsapp')
  check(len(wa['templates'])==5 and not wa['settings']['enabled'],'WhatsApp starts paused with five templates')
  dev.call('/developer/whatsapp',{'action':'settings','enabled':True,'reminder_hour':9,'daily_cap':1000,'version':1,'password':'1234'},400)
  dev.call('/developer/whatsapp',{'action':'settings','enabled':False,'reminder_hour':9,'daily_cap':200,'version':1,'password':'1234'})
  dev.call('/developer/whatsapp',{'action':'settings','enabled':False,'reminder_hour':9,'daily_cap':200,'version':1,'password':'1234'},409)
  template=wa['templates'][0]
  dev.call('/developer/whatsapp',{'action':'template','kind':template['kind'],'name':template['name'],'language':'en','body':'bad','version':template['version'],'password':'1234'},400)
  dev.call('/developer/whatsapp',{'action':'template','kind':template['kind'],'name':template['name'],'language':'en','body':template['body'],'version':template['version'],'password':'1234'})
  check('WHATSAPP_ACCESS_TOKEN' not in json.dumps(wa),'WhatsApp API does not expose credentials')

  def raw_post(path,payload,key,client=None):
   headers={'Origin':base,'Content-Type':'application/json','Idempotency-Key':key}
   if client:headers.update({'Cookie':'; '.join(c.name+'='+c.value for c in client.cookies),'X-CSRF-Token':client.csrf})
   req=urllib.request.Request(base+'/api'+path,data=json.dumps(payload).encode(),headers=headers)
   try:r=urllib.request.urlopen(req,timeout=20)
   except urllib.error.HTTPError as e:r=e
   return r.status
  # Duplicate submissions must not send two emails, even across two HTTP clients.
  unlock(did);before_mail=len(messages);login_key=secrets.token_hex(16)
  with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
   statuses=list(pool.map(lambda _:raw_post('/login',{'username':'developer','password':'1234'},login_key),range(2)))
  check(sorted(statuses)==[200,409] and len(messages)==before_mail+1,'Same login action concurrently sends exactly one OTP')
  check(raw_post('/login',{'username':'developer','password':'1234'},login_key)==409 and len(messages)==before_mail+1,'Completed login action cannot be replayed to resend mail')
  unlock(did);before_mail=len(messages)
  with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
   statuses=list(pool.map(lambda _:raw_post('/login',{'username':'developer','password':'1234'},secrets.token_hex(16)),range(2)))
  check(sorted(statuses)==[200,429] and len(messages)==before_mail+1,'Different login keys still obey the atomic per-account OTP cooldown')
  create_key=secrets.token_hex(16);type_payload={'category':'Supplier','label':'Single-submit test service'}
  with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
   statuses=list(pool.map(lambda _:raw_post('/company-types',type_payload,create_key,admin),range(3)))
  check(sorted(statuses)==[200,409,409],'Three concurrent create requests execute one mutation')
  check(sql("SELECT count(*) FROM company_custom_types WHERE label='Single-submit test service'")=='1','Duplicate create attempts produce exactly one database row')
  admin.call('/company-types',{**type_payload,'label':'Replay must not create'},409,key=create_key)
  check(sql("SELECT count(*) FROM company_custom_types WHERE label='Replay must not create'")=='0','Reusing a completed key with changed content cannot create another row')
  retry_key=secrets.token_hex(16)
  admin.call('/company-types',{'category':'Unknown','label':'Corrected request'},400,key=retry_key)
  admin.call('/company-types',{'category':'Supplier','label':'Corrected request'},key=retry_key)
  admin.call('/company-types',{'category':'Supplier','label':'Invalid key'},400,key='bad')
  dev.call('/company-types',{'category':'Supplier','label':'Independent owner action'},key=create_key)
  check(sql("SELECT count(*) FROM company_custom_types WHERE label='Independent owner action'")=='1','Action keys are isolated between authenticated users')
  check(int(run('docker','exec',name,'redis-cli','TTL',next(k for k in run('docker','exec',name,'redis-cli','KEYS','action:*').splitlines())))>86000,'Successful action reservation survives request completion for 24 hours')
  meta=admin.call('/setup-users');owner=next(u for u in meta['users'] if u['id']==did)
  admin.call('/setup-users/'+str(did),{**owner,'features':owner['setup_features'],'permissions':owner['effective_permissions'],'active':False},403)
  admin.call('/setup-users/'+str(did)+'/replace',{'name':'Intruder','username':'intruder','email':'intruder@example.test','password':'1234','version':owner['version']},403)
  def account(uid):return next(u for u in dev.call('/developer')['users'] if u['id']==uid)
  def security(uid,**changes):
   u=account(uid);return {'name':u['name'],'email':u['email'],'phone':u['phone'],'mfa_enabled':u['mfa_enabled'],'version':u['version'],'password':'1234',**changes}
  admin.call('/developer/users/'+str(fid),{},403)
  dev.call('/developer/users/'+str(fid),security(fid,email='finance@example.test',mfa_enabled=True,password='wrong'),403)
  body=security(fid,email='finance@example.test',mfa_enabled=True,phone='+919876543210');dev.call('/developer/users/'+str(fid),body);dev.call('/developer/users/'+str(fid),body,409)
  finance.call('/company-setup',status=401)
  second=Client();second.login('finance');old=code()
  for _ in range(5):second.call('/login/otp',{'code':'000000' if old!='000000' else '999999'},401)
  second.call('/login/otp',{'code':old},401)
  unlock(fid);second.login('finance');expired=code();run('docker','exec',name,'redis-cli','FLUSHDB')
  second.call('/login/otp',{'code':expired},401)
  # Recreate the owner session after clearing only the isolated test Redis.
  dev=Client();mfa(dev);unlock(fid);third=Client();third.login('finance');pending=code()
  dev.call('/developer/users/'+str(fid),security(fid,new_password='Different-password-2026'))
  third.call('/login/otp',{'code':pending},401)
  unlock(fid);finance=Client();mfa(finance,'finance','Different-password-2026');finance.call('/developer',status=403)
  dev.call('/developer/users/'+str(did),security(did,mfa_enabled=False),400)
  dev.call('/developer/users/'+str(did),security(did,email='attacker@example.test'),400)
  dev.call('/developer/users/'+str(did)+'/delete',{'password':'1234','confirmation_username':'developer','version':account(did)['version']},400)
  settings=dev.call('/developer')['notification_settings'];dev.call('/developer/notifications',{'recipients':['reminders@example.test'],'version':settings['version']})
  dev.call('/developer/notifications',{'recipients':['bad\r\naddress'],'version':settings['version']+1},400)
  check(dev.call('/developer')['notification_settings']['recipients']==['reminders@example.test'],'Reminder recipient changes persist')
  finance.call('/client-issues',{'kind':'upload','page':'/finance'});finance.call('/client-issues',{'kind':'upload','page':'/finance'})
  time.sleep(2.2) # Issue persistence is batched by the diagnostics worker.
  issues=dev.call('/developer')['issues'];issue=next(i for i in issues if i['category']=='Browser');check(issue['occurrences']==2,'Repeated browser issues are grouped')
  dev.call('/developer/issues/'+str(issue['id']),{'status':'Resolved','resolution':'Verified upload configuration','version':issue['version']})
  finance.call('/client-issues',{'kind':'upload','page':'/finance'})
  time.sleep(2.2)
  issue=next(i for i in dev.call('/developer')['issues'] if i['id']==issue['id']);check(issue['status']=='Open' and issue['occurrences']==3,'New occurrence reopens a resolved issue')
  check('000000' not in json.dumps(issues) and first not in json.dumps(issues),'Issue reports contain no OTP or password payload')
  check(sql("BEGIN; SET LOCAL ROLE euroerp_runtime; SELECT count(*) FROM system_issues; ROLLBACK;").splitlines()[-2]=='0','Issue rows hidden without developer context')
  run('docker','exec',name,'redis-cli','DEL',f'developer:confirm:{did}')
  template=next(d for d in dev.call('/setup-users')['designations'] if d['name']=='Finishing Manager')
  uid=dev.call('/setup-users',{'name':'Disposable worker','username':'delete.me','email':'worker@example.test','password':'1234','designation_id':template['id'],'inherit_designation':True,'active':True})['id']
  worker=Client();worker.login('delete.me');worker.call('/password',{'current_password':'1234','new_password':'Attempt-password-2026'},403)
  u=account(uid);payload={'version':u['version'],'password':'1234','confirmation_username':'wrong'};dev.call('/developer/users/'+str(uid)+'/delete',payload,400)
  payload['confirmation_username']='delete.me';dev.call('/developer/users/'+str(uid)+'/delete',payload)
  worker.call('/data',status=401);Client().call('/login',{'username':'delete.me','password':'1234'},401)
  check(sql(f'SELECT deleted_at IS NOT NULL AND NOT active FROM users WHERE id={uid}')=='t','User deletion preserves an inactive historical identity')
  check(sql(f"SELECT count(*) FROM access_history WHERE subject_id={uid} AND action='User deleted; history retained'")=='1','User deletion recorded in audit history')
  # Concurrent use of the same valid OTP must produce exactly one session.
  unlock(fid);contender=Client();contender.login('finance','Different-password-2026');otp=code();cookie='; '.join(c.name+'='+c.value for c in contender.cookies)
  def verify_once(_):
   request=urllib.request.Request(base+'/api/login/otp',data=json.dumps({'code':otp}).encode(),headers={'Origin':base,'Content-Type':'application/json','Cookie':cookie})
   try:return urllib.request.urlopen(request,timeout=15).status
   except urllib.error.HTTPError as e:return e.code
  with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:statuses=list(pool.map(verify_once,[0,1]))
  check(sorted(statuses)==[200,401],'Concurrent OTP reuse yields exactly one authenticated session')
  # Missing sender authentication blocks a mandatory-OTP login; no bypass.
  proc.terminate();proc.wait(timeout=10);env['SMTP_HOST']='';proc=subprocess.Popen([binary],env=env,stdout=log,stderr=log)
  wait_server();Client().call('/login',{'username':'developer','password':'1234'},503)
  proc.terminate();proc.wait(timeout=10);env['SMTP_HOST']='localhost';proc=subprocess.Popen([binary],env=env,stdout=log,stderr=log)
  wait_server()
  exec((ROOT/'tests/export_bank_email_cases.py').read_text(),globals(),globals())
  result={'status':'PASS','passed':len(report),'checks':report,'real_external_email_sent':False}
  (ROOT/'docs/developer-test-results.json').write_text(json.dumps(result,indent=2));print(f'PASS: {len(report)} developer/OTP assertions',flush=True)
  if os.getenv('DEVELOPER_TEST_HOLD')=='1':
   unlock(did);info={'url':base+'/developer','folder':str(temp),'stop_file':str(temp/'stop'),'otp_file':str(temp/'latest-test-otp.txt'),'test_username':'developer','test_password':'1234'}
   (ROOT/'../../work/developer-test-session.json').resolve().write_text(json.dumps(info));print(json.dumps(info),flush=True)
   while not (temp/'stop').exists():time.sleep(1)
  proxy.shutdown();mailserver.shutdown()
finally:
 if proc:proc.terminate();proc.wait(timeout=10)
 if db:run('docker','compose','exec','-T','postgres','dropdb','-U','euroerp','--force',name,cwd=ROOT)
 if role:run('docker','compose','exec','-T','postgres','psql','-U','euroerp','-d','postgres','-c','DROP ROLE '+runtime,cwd=ROOT)
 if redis:run('docker','rm','-f',name)
