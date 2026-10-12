#!/usr/bin/env python3
"""Real HTTP regression suite. Creates and removes its own PostgreSQL DB and Redis container.
Run after building backend/erp-server, with local Compose PostgreSQL running.
Never changes the normal ERP database or its users.
"""
from pathlib import Path
import os, json, subprocess, secrets, socket, tempfile, time, urllib.request, urllib.error, http.cookiejar, concurrent.futures, hashlib
ROOT=Path(__file__).resolve().parents[1]
config=dict(line.split('=',1) for line in (ROOT/'.env').read_text().splitlines() if '=' in line and not line.startswith('#'))
name='erp_test_'+secrets.token_hex(5); password=secrets.token_urlsafe(20); checks=0; report=[]
def run(*args,**kw): return subprocess.run(args,check=True,capture_output=True,text=True,**kw).stdout.strip()
def freeport():
    with socket.socket() as s: s.bind(('127.0.0.1',0)); return s.getsockname()[1]
def sql(statement,db='postgres'): return run('docker','compose','exec','-T','postgres','psql','-U','euroerp','-d',db,'-v','ON_ERROR_STOP=1','-Atc',statement,cwd=ROOT)
def check(condition,label):
    global checks
    if not condition: raise AssertionError(label)
    checks+=1; report.append(label)
class Client:
    def __init__(self,user=None):
        self.user=user; self.csrf=''; self.opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        if user: self.call('/login',{'username':user,'password':password})
    def call(self,path,body=None,expect=200,headers=None):
        h={'Origin':os.getenv('COMPANY_TEST_ORIGIN',base),'X-CSRF-Token':self.csrf}
        if isinstance(body,dict) and (path=='/setup-users' or path.endswith('/replace')) and body.get('username'):body={'email':body['username']+'@example.test',**body}
        if body is not None and not isinstance(body,bytes): body=json.dumps(body).encode();h['Content-Type']='application/json'
        if headers: h.update(headers)
        req=urllib.request.Request(base+'/api'+path,data=body,headers=h)
        try: response=self.opener.open(req,timeout=15)
        except urllib.error.HTTPError as e: response=e
        raw=response.read(); content=response.headers.get('Content-Type','')
        data=json.loads(raw) if 'application/json' in content else raw
        check(response.status==expect,f'{self.user or "anonymous"} {path}: expected {expect}, got {response.status}: {str(data)[:160] if response.status!=expect else "ok"}')
        if isinstance(data,dict) and data.get('csrf'): self.csrf=data['csrf']
        return data
    def order(self,oid): return self.call('/orders/'+str(oid))
    def action(self,oid,action,expect=200,**kw): return self.call('/orders/'+str(oid),{'version':self.order(oid)['version'],'action':action,**kw},expect)
def new_order(c,po='TEST-PO',buyer=1,company=1):
    return c.call('/orders',{'company_id':company,'buyer_id':buyer,'po':po,'style':'Regression suit','product':'Suits','quantity':10,'due_date':'2026-12-01','unit_price':'42.50','currency':'USD'})['id']

import base64,datetime,io,zipfile
if os.getenv('COMPANY_TEST_PASSWORD'):password=os.environ['COMPANY_TEST_PASSWORD']
proc=None;redis_created=False;db_created=False;role_created=False
try:
 sql('CREATE DATABASE '+name);db_created=True
 rp=freeport();ap=int(os.getenv('COMPANY_TEST_PORT','0')) or freeport();base=f'http://127.0.0.1:{ap}'
 run('docker','run','-d','--name',name,'-p',f'127.0.0.1:{rp}:6379','redis:8-alpine','redis-server','--save','','--appendonly','no');redis_created=True
 with tempfile.TemporaryDirectory(prefix=name) as temp:
  env={**os.environ,'DATABASE_URL':f'postgres://euroerp:{config["POSTGRES_PASSWORD"]}@127.0.0.1:54329/{name}?sslmode=disable','REDIS_URL':f'redis://127.0.0.1:{rp}/0','HTTP_ADDR':f'127.0.0.1:{ap}','APP_ORIGIN':os.getenv('COMPANY_TEST_ORIGIN',base),'APP_ENV':'local','DEMO_SEED':'false','OWNER_PASSWORD':password,'FILE_ROOT':temp+'/files','KYC_ENCRYPTION_KEY':base64.b64encode(secrets.token_bytes(32)).decode(),'RESET_COMPANY_WORKSPACE':'REMOVE_LOCAL_TEST_DATA','SMTP_HOST':''}
  run(str(ROOT/'backend/erp-server'),'initialize-company-workspace',env=env)
  try:
   run(str(ROOT/'backend/erp-server'),'initialize-company-workspace',env=env);raise AssertionError('Initialization overwrote existing workspace')
  except subprocess.CalledProcessError:check(True,'Initialization refuses existing workspace without resetting data')
  check(sql('SELECT count(*) FROM users',name)=='3','Fresh initialization creates only three Company Setup accounts')
  runtime_role=name+'_api';runtime_password=secrets.token_hex(32)
  sql("CREATE ROLE "+runtime_role+" LOGIN PASSWORD '"+runtime_password+"'; GRANT euroerp_runtime TO "+runtime_role,name);role_created=True
  env['DATABASE_URL']=f'postgres://{runtime_role}:{runtime_password}@127.0.0.1:54329/{name}?sslmode=disable'
  log=open(temp+'/server.log','w+');proc=subprocess.Popen([str(ROOT/'backend/erp-server')],env=env,stdout=log,stderr=log)
  for _ in range(100):
   try:urllib.request.urlopen(base+'/api/health',timeout=1);break
   except Exception:
    if proc.poll() is not None:log.seek(0);raise RuntimeError(log.read())
    time.sleep(.1)
  admin,finance,md=Client('admin'),Client('finance'),Client('md')
  blank=finance.call('/company-setup');check(blank['companies']==[],'Company register starts clean')
  check(sql('SELECT count(*) FROM orders',name)=='0','No demo orders remain')
  for c in [admin,md]:c.call('/company-setup',{},403)
  admin.call('/records/companies',{'name':'Bypass','kind':'Self'},400)
  finance.call('/company-fields',{},403);md.call('/company-fields',{},403)
  extra=admin.call('/company-fields',{'label':'Local registration','section':'Additional','country':'IN','categories':['Internal'],'input_type':'text','required':True,'document_required':True,'expiry_enabled':True})['id']
  finance.call('/company-types',{'category':'Supplier','label':'Custom testing service'},403);md.call('/company-types',{},403)
  admin.call('/company-types',{'category':'Supplier','label':'Custom testing service'})
  check('Custom testing service' in finance.call('/company-setup')['types']['Supplier'],'New supplier type is available to Finance')
  admin.call('/company-types',{'category':'Supplier','label':'Fabrics'},400)
  admin.call('/company-types',{'category':'Unknown','label':'Invalid'},400)
  header={'legal_name':'REGRESSION INTERNAL','category':'Internal','company_type':'Self','country':'IN','city':'Bengaluru'}
  cid=finance.call('/company-setup',header)['id'];path='/company-setup/'+str(cid)
  def detail(c=finance):return c.call(path)
  def act(c,action,expect=200,**kw):return c.call(path,{'action':action,'version':detail(c)['version'],**kw},expect)
  check('profile' not in detail(md),'MD cannot read draft KYC')
  check('profile' in detail(admin),'Super Admin can inspect full KYC')
  act(md,'approve',400);act(finance,'approve',403);act(finance,'submit',400)
  act(admin,'save',403,**header,profile={})
  sea=finance.call('/company-ports',{'name':'Regression Harbour','code':'TEST','country':'IN','mode':'Sea'})['id']
  air=finance.call('/company-ports',{'name':'Regression Airport','code':'TST','country':'IN','mode':'Air'})['id']
  boundary='company-regression';payload=b'%PDF-1.4\nCompany proof - private\n'
  multipart=(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="proof.pdf"\r\nContent-Type: application/pdf\r\n\r\n'.encode()+payload+f'\r\n--{boundary}--\r\n'.encode())
  fid=finance.call(path+'/files',multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})['id']
  finance.call('/company-files/'+str(fid));md.call('/company-files/'+str(fid),expect=403)
  encrypted=list(Path(temp+'/files').iterdir())[0].read_bytes();check(payload not in encrypted,'Uploaded originals are encrypted on disk')
  check(admin.call('/company-files/'+str(fid))==payload,'Super Admin decrypts original attachment')
  expires=(datetime.date.today()+datetime.timedelta(days=30)).isoformat()
  p={'address':'Regression registered address','owner':'Regression owner','email':'finance@example.test','phone':'0000000000','reminder_email':'renewal@example.test','contacts':[{'role':'CEO','name':'Regression CEO','email':'ceo@example.test','phone':''}],'banks':[{'id':'bank-1','name':'Regression Bank','holder':'Regression holder','account':'SECRET-BANK-123','routing':'DEMO','currency':'INR','documents':[fid]}],'ports':[{'id':sea,'default':True,'registered':True,'registration':'SECRET-PORT-456','bank_id':'bank-1','expiry':True,'valid_from':datetime.date.today().isoformat(),'expires_on':expires,'documents':[fid]},{'id':air,'default':True,'registered':False}],'fields':[{'id':extra,'value':'SECRET-KYC-789','expiry':True,'valid_from':datetime.date.today().isoformat(),'expires_on':expires,'documents':[fid]}]}
  p['fields'] += [{'id':d['id'],'not_applicable':True,'reason':'Not applicable to this synthetic test company'} for d in blank['fields'] if d['section']=='KYC']
  act(finance,'save',**header,profile=p);check(detail()['summary']['ready'],'Complete draft is ready for approval')
  raw=sql(f"SELECT encode(private_data,'hex') FROM company_setups WHERE id={cid}",name);check('SECRET' not in bytes.fromhex(raw).decode(errors='ignore'),'KYC profile is encrypted in PostgreSQL')
  stale=detail()['version'];act(finance,'save',**header,profile=p);finance.call(path,{'action':'save','version':stale,**header,'profile':p},409)
  badp={**p,'ports':[{**p['ports'][0],'bank_id':'missing'},p['ports'][1]]};act(finance,'save',**header,profile=badp);act(finance,'submit',400)
  badp={**p,'fields':[{**p['fields'][0],'documents':[99999]}]};act(finance,'save',400,**header,profile=badp)
  badp={**p,'fields':[{**p['fields'][0],'expires_on':'2020-01-01'}]};act(finance,'save',400,**header,profile=badp)
  act(finance,'save',**header,profile=p);act(finance,'submit')
  for c in [finance,md]:
   d=detail(c);check('profile' not in d and 'files' not in d,'Submitted KYC hidden from '+c.user)
   check('SECRET-' not in json.dumps(d),'No KYC values leaked in approval summary for '+c.user)
   c.call('/company-files/'+str(fid),expect=403)
   out=c.call(path+'/export');z=zipfile.ZipFile(io.BytesIO(out));check(b'SECRET-' not in z.read('xl/worksheets/sheet1.xml'),'Excel summary preserves privacy for '+c.user)
  out=admin.call(path+'/export');z=zipfile.ZipFile(io.BytesIO(out));check(b'SECRET-KYC-789' in z.read('xl/worksheets/sheet1.xml'),'Super Admin Excel includes full KYC')
  act(finance,'save',403,**header,profile=p);finance.call(path+'/files',multipart,403,{'Content-Type':'multipart/form-data; boundary='+boundary})
  act(md,'request_changes',note='Please verify supporting copies.');check('profile' in detail(),'Finance can edit returned draft')
  act(finance,'submit');act(md,'approve');check(detail(md)['status']=='Approved','MD approves company')
  reminders=md.call('/company-setup')['reminders'];check(len(reminders)==2,'Field and port expiry reminders become visible two months ahead')
  check(sql("SELECT bool_and(remind_on=expires_on-interval '2 months') FROM company_reminders",name)=='t','Calendar-month reminder scheduling')
  act(finance,'amend');check(detail()['revision']==2,'Amendment creates a new revision requiring approval')
  check(len(md.call('/company-setup')['reminders'])==2,'Last approved renewals stay active until amendment approval')
  act(finance,'submit');act(admin,'approve');check(detail(admin)['status']=='Approved','Super Admin also approves company')
  act(md,'approve',400)
  check('SECRET-' not in json.dumps(md.call('/company-setup')),'List and reminders reveal no KYC values')
  # Buyer/supplier variants and country-specific required field isolation.
  buyer=finance.call('/company-setup',{'legal_name':'REGRESSION BUYER','category':'Buyer','company_type':'Retailer','country':'GB','city':'London'})['id']
  check(buyer!=cid,'Buyer has an independent record')
  buyerpath='/company-setup/'+str(buyer)
  buyerprofile={'address':'Test address','owner':'Test buyer','email':'buyer@example.test','phone':'000','reminder_email':'reminder@example.test','sample_address':'Test sample address','brands':'Test brand','departments':['Menswear']}
  buyerprofile['fields']=[{'id':d['id'],'not_applicable':True,'reason':'Not applicable to synthetic buyer'} for d in blank['fields'] if d['section']=='KYC' and d['country']=='*']
  bh={'legal_name':'REGRESSION BUYER','category':'Buyer','company_type':'Retailer','country':'GB','city':'London'}
  finance.call(buyerpath,{'action':'save','version':1,**bh,'profile':buyerprofile});bv=finance.call(buyerpath)['version'];finance.call(buyerpath,{'action':'submit','version':bv});md.call(buyerpath,{'action':'approve','version':finance.call(buyerpath)['version']})
  supplier=finance.call('/company-setup',{'legal_name':'REGRESSION SUPPLIER','category':'Supplier','company_type':'Custom testing service','country':'IN','city':'Mumbai'})['id'];sp='/company-setup/'+str(supplier)
  sh={'legal_name':'REGRESSION SUPPLIER','category':'Supplier','company_type':'Custom testing service','country':'IN','city':'Mumbai'}
  supplierprofile={**buyerprofile,'relationship':'Nominated'}
  finance.call(sp,{'action':'save','version':1,**sh,'profile':supplierprofile});finance.call(sp,{'action':'submit','version':finance.call(sp)['version']},400)
  # Authentication, CSRF and content protection.
  Client().call('/company-setup',expect=401);finance.call('/company-setup',{},403,{'X-CSRF-Token':''})
  try:sql("UPDATE company_setup_history SET note='tamper'",name);raise AssertionError('History mutable')
  except subprocess.CalledProcessError:check(True,'Company approval history is append-only')
  # Admin-managed accounts, portal routing and per-user feature boundaries.
  md.call('/setup-users',expect=403);finance.call('/setup-users',expect=403)
  md.call('/users',{},403);md.call('/users/2',{'active':False},403)
  md.call('/setup-users',{},403);finance.call('/setup-users',{},403)
  check('password' not in json.dumps(admin.call('/setup-users')),'User register never returns password data')
  def account(username,role,features,active=True):
   return admin.call('/setup-users',{'username':username,'name':'Regression '+username,'password':password,'role':role,'features':features,'active':active})['id']
  def permissions(uid,features,role='Finance Head',active=True,expect=200):
   record=next(u for u in admin.call('/setup-users')['users'] if u['id']==uid)
   return admin.call('/setup-users/'+str(uid),{'name':record['name'],'role':role,'features':features,'active':active,'version':record['version']},expect)
  limited_id=account('limited.finance','Finance Head',['view'])
  limited=Client('limited.finance')
  check(limited.call('/session')['user']['portal']=='finance','New Finance account automatically uses /finance')
  check(limited.call('/company-setup')['companies']==[],'New Finance account cannot see another Finance account records')
  limited.call(path,expect=404);limited.call('/company-files/'+str(fid),expect=403)
  limited.call('/company-setup',header,403);limited.call('/company-ports',{},403)
  limited.call('/company-fields',{},403);limited.call('/setup-users',{},403)
  limited.call('/company-setup/1/export',expect=403)
  check(limited.call('/company-setup')['reminders']==[],'Reminders excluded when feature is disabled')
  limited.call('/login',{'username':'limited.finance','password':password,'portal':'md'},403)
  permissions(limited_id,['view','approve'],expect=400)
  permissions(limited_id,['create'],expect=400)
  permissions(limited_id,['view','submit'],expect=400)
  features=['view','create','edit','export','documents','ports','reminders','submit','amend']
  permissions(limited_id,features)
  limited.call('/company-setup',expect=401)
  limited=Client('limited.finance')
  own=limited.call('/company-setup',{'legal_name':'RESTRICTED OWN COMPANY','category':'Buyer','company_type':'Retailer','country':'GB','city':'London'})['id']
  ownpath='/company-setup/'+str(own)
  check(limited.call(ownpath)['can_edit'],'Granting draft editing takes effect after fresh sign-in')
  finance.call(ownpath,expect=404)
  permissions(limited_id,['view'])
  limited.call(ownpath,expect=401);limited=Client('limited.finance')
  readonly=limited.call(ownpath);check(not readonly['can_edit'],'Revoking editing changes the server capability')
  limited.call(ownpath,{'action':'save','version':readonly['version']},403)
  limited.call(ownpath,{'action':'submit','version':readonly['version']},403)
  limited.call(ownpath+'/files',multipart,403,{'Content-Type':'multipart/form-data; boundary='+boundary})
  limited.call(ownpath+'/export',expect=403)
  check(sql(f"SELECT count(*) FROM company_setup_files WHERE company_id={own}",name)=='0','Denied document upload creates no files')
  approver_id=account('limited.md','MD',['view'])
  approver=Client('limited.md');check(approver.call('/session')['user']['portal']=='md','MD accounts use /md')
  approver.call(path,{'action':'approve','version':detail(admin)['version']},403)
  approver.call(path,{'action':'request_changes','version':detail(admin)['version'],'note':'No authority'},403)
  approver.call('/company-setup',header,403)
  permissions(limited_id,[],active=False)
  limited.call('/company-setup',expect=401)
  Client().call('/login',{'username':'limited.finance','password':password,'portal':'finance'},401)
  record=next(u for u in admin.call('/setup-users')['users'] if u['id']==approver_id)
  permissions(approver_id,['view','approve'],'MD')
  admin.call('/setup-users/'+str(approver_id),{'name':'Old tab','role':'MD','features':['view'],'active':True,'version':record['version']},409)
  admin.call('/setup-users/'+str(admin.call('/session')['user']['id']),{'name':'Self','role':'MD','features':['view'],'active':True},400)
  approver.call('/company-setup',expect=401)
  noaccess_id=account('no.features','Finance Head',[]);noaccess=Client('no.features')
  check(noaccess.call('/session')['user']['setup_grants']==[],'Empty checkbox selection means no features')
  noaccess.call('/company-setup',expect=403)
  # Extra fields can be added to each company section and validated server-side.
  custom=[]
  for section,kind in [('Overview','textarea'),('Contacts','email'),('Bank accounts','select'),('Ports','checkbox'),('Documents','tel')]:
   custom.append((admin.call('/company-fields',{'label':'Custom '+section,'section':section,'country':'GB','categories':['Buyer'],'input_type':kind,'options':['Choice A','Choice B'] if kind=='select' else []})['id'],kind))
  admin.call('/company-fields',{'label':'Bad dropdown','section':'Overview','country':'GB','categories':['Buyer'],'input_type':'select','options':['Same','Same']},400)
  permissions(limited_id,features);limited=Client('limited.finance')
  fields=[{'id':identifier,'value':{'textarea':'Long office note','email':'custom@example.test','select':'Choice A','checkbox':'false','tel':'123456'}[kind]} for identifier,kind in custom]
  oh={'legal_name':'RESTRICTED OWN COMPANY','category':'Buyer','company_type':'Retailer','country':'GB','city':'London'}
  def ownsave(p,expect=200):return limited.call(ownpath,{'action':'save','version':limited.call(ownpath)['version'],**oh,'profile':p},expect)
  ownsave({'fields':fields});check(len(limited.call(ownpath)['profile']['fields'])==5,'Custom fields persist across all form sections')
  for identifier,kind in custom:
   if kind in ['email','select','checkbox']:ownsave({'fields':[{'id':identifier,'value':'INVALID'}]},400)
  out=limited.call(ownpath+'/export');z=zipfile.ZipFile(io.BytesIO(out));check(b'Choice A' in z.read('xl/worksheets/sheet1.xml'),'New custom field values are included in Excel')
  check(len(admin.call('/setup-users')['history'])>=8,'Account and permission changes are audited')
  try:sql("DELETE FROM setup_access_history",name);raise AssertionError('Access history mutable')
  except subprocess.CalledProcessError:check(True,'Access history is append-only')
  admin.call('/setup-users',{'username':'default.finance','name':'Local default password','role':'Finance Head','features':['view'],'active':True})
  defaultuser=Client();defaultuser.call('/login',{'username':'default.finance','password':'1234','portal':'finance'})
  check(defaultuser.call('/session')['user']['portal']=='finance','New local accounts default to password 1234 and their role portal')
  exec((ROOT/'tests/password_confirmation_cases.py').read_text(),globals(),globals())
  exec((ROOT/'tests/user_credential_cases.py').read_text(),globals(),globals())
  exec((ROOT/'tests/business_flow_cases.py').read_text(),globals(),globals())
  exec((ROOT/'tests/company_address_cases.py').read_text(),globals(),globals())
  exec((ROOT/'tests/company_document_cases.py').read_text(),globals(),globals())
  exec((ROOT/'tests/company_additional_field_cases.py').read_text(),globals(),globals())
  exec((ROOT/'tests/access_cases.py').read_text(),globals(),globals())
  # Company deletion requires Super Admin and an exact name; test only disposable fixtures.
  deletepath=path+'/delete';deletebody={'version':detail(admin)['version'],'confirmation_name':'REGRESSION INTERNAL','password':password}
  finance.call(deletepath,deletebody,403);md.call(deletepath,deletebody,403)
  for wrong in ['', 'REGRESSION', 'regression internal', 'REGRESSION INTERNAL ']:
   admin.call(deletepath,{**deletebody,'confirmation_name':wrong},400)
  admin.call(deletepath,{**deletebody,'version':0},409)
  check(detail(admin)['status']=='Approved','Rejected deletion leaves company unchanged')
  admin.call(deletepath,deletebody)
  for actor in [admin,finance,md]:
   actor.call(path,expect=404);actor.call(path+'/export',expect=404)
   check(all(c['id']!=cid for c in actor.call('/company-setup')['companies']),'Deleted company excluded from register')
  admin.call('/company-files/'+str(fid),expect=404)
  admin.call(deletepath,deletebody,404)
  check(sql(f"SELECT count(*) FROM company_reminders WHERE company_id={cid} AND active",name)=='0','Deletion deactivates renewal reminders')
  check(sql(f"SELECT count(*) FROM company_setup_history WHERE company_id={cid} AND action='Deleted'",name)=='1','Deletion keeps an audit record')
  result={'status':'PASS','checks':checks,'tested_at':time.strftime('%Y-%m-%d %H:%M:%S'),'scope':'Company Setup, user access, RLS, full order flow, uploads, authentication and renewal locks','checks_detail':report}
  (ROOT/'docs/company-test-results.json').write_text(json.dumps(result,indent=2));print(f'PASS: {checks} Company Setup assertions',flush=True)
  if os.getenv('KEEP_COMPANY_TEST_SERVER'):
   print('QA server ready at '+base,flush=True)
   while True:time.sleep(5)
finally:
 if proc:proc.terminate();proc.wait(timeout=15)
 if redis_created:run('docker','rm','-f',name)
 if db_created:sql('DROP DATABASE '+name+' WITH (FORCE)')

 if role_created: sql('DROP ROLE '+runtime_role)
