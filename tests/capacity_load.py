#!/usr/bin/env python3
"""Synthetic capacity test. ONLY the disposable VPS test cluster is accepted.
500 real password logins, 2,000 orders, then 50 requests/second for 20 seconds.
No production credentials, email, WhatsApp or business records are used.
"""
from pathlib import Path
import base64,concurrent.futures,datetime,hashlib,http.cookiejar,json,os,secrets,socket,subprocess,tempfile,time,urllib.error,urllib.request
ROOT=Path(__file__).resolve().parents[1]
if str(ROOT)!='/opt/euro-suits-capacity-20261003' or 'euro-capacity-isolated' not in (ROOT/'compose.yaml').read_text():
 raise SystemExit('Refusing: run only in the dedicated capacity test directory/cluster')
cfg=dict(x.split('=',1) for x in (ROOT/'.env').read_text().splitlines() if '=' in x)
name='erp_capacity_'+secrets.token_hex(4);password=secrets.token_urlsafe(24);proc=None;role=False;redis=False;db=False
report={'users':500,'orders':2000,'target_requests_per_second':50,'duration_seconds':20,'isolated_postgres_cpu_limit':2.5,'api_gomaxprocs':2,'production_data_used':False,'external_notifications_sent':False}
def run(*args,**kw):return subprocess.run(args,check=True,capture_output=True,text=True,**kw).stdout.strip()
def sql(q,dbname=None):return run('docker','compose','exec','-T','postgres','psql','-U','euroerp','-d',dbname or name,'-v','ON_ERROR_STOP=1','-At',input=q,cwd=ROOT)
def port():
 with socket.socket() as s:s.bind(('127.0.0.1',0));return s.getsockname()[1]
class Client:
 def __init__(self,i):self.i=i;self.csrf='';self.o=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
 def call(self,path,payload=None):
  h={'Origin':base,'X-CSRF-Token':self.csrf,'Content-Type':'application/json'}
  start=time.monotonic()
  try:r=self.o.open(urllib.request.Request(base+'/api'+path,data=None if payload is None else json.dumps(payload).encode(),headers=h),timeout=30)
  except urllib.error.HTTPError as e:r=e
  out=json.load(r)
  if isinstance(out,dict) and out.get('csrf'):self.csrf=out['csrf']
  return r.status,(time.monotonic()-start)*1000,out
try:
 sql('CREATE DATABASE '+name,'postgres');db=True
 rp,ap=port(),port();base=f'http://127.0.0.1:{ap}'
 run('docker','run','-d','--name',name,'--cpus','0.5','--memory','128m','-p',f'127.0.0.1:{rp}:6379','redis:8-alpine','redis-server','--save','','--appendonly','no');redis=True
 with tempfile.TemporaryDirectory(prefix=name) as folder:
  env={**os.environ,'GOMAXPROCS':'2','DATABASE_URL':f'postgres://euroerp:{cfg["POSTGRES_PASSWORD"]}@127.0.0.1:54329/{name}?sslmode=disable','REDIS_URL':f'redis://127.0.0.1:{rp}/0','HTTP_ADDR':f'127.0.0.1:{ap}','APP_ENV':'local','APP_ORIGIN':base,'DEMO_SEED':'false','OWNER_USERNAME':'capacity.admin','OWNER_NAME':'Capacity test owner','OWNER_PASSWORD':password,'KYC_ENCRYPTION_KEY':base64.b64encode(secrets.token_bytes(32)).decode(),'FILE_ROOT':folder+'/files','SMTP_HOST':'','WHATSAPP_ACCESS_TOKEN':''}
  run(str(ROOT/'backend/erp-server'),'create-owner',env=env)
  sql("""INSERT INTO companies(id,name,kind,status) VALUES(1,'Disposable capacity company','Self','Approved');
INSERT INTO teams(id,company_id,name) SELECT g,1,'Team '||g FROM generate_series(1,20)g;
INSERT INTO buyers(id,company_id,team_id,name) SELECT g,1,g,'Buyer '||g FROM generate_series(1,20)g;
INSERT INTO users(username,name,password_hash,role,scope,companies,teams,permissions,custom_permissions)
SELECT 'capacity.'||g,'Capacity user '||g,(SELECT password_hash FROM users WHERE username='capacity.admin'),'Marketing','team',ARRAY[1::bigint],ARRAY[(g-1)%20+1::bigint],ARRAY['view_orders','orders'],true FROM generate_series(1,500)g;
INSERT INTO orders(id,company_id,buyer_id,team_id,po,style,product,quantity,due_date,created_by,stage,approval)
SELECT g,1,(g-1)%20+1,(g-1)%20+1,'CAP-'||g,'Capacity suit','Suits',100,CURRENT_DATE+30,1,'Planning','Approved' FROM generate_series(1,2000)g;
SELECT setval('orders_id_seq',2000);
INSERT INTO milestones(order_id,title,due_date) SELECT o.id,'Milestone '||g,CURRENT_DATE+g FROM orders o CROSS JOIN generate_series(1,4)g;
INSERT INTO work_items(order_id,kind,title) SELECT id,'CAD / pattern','Capacity CAD' FROM orders;
ANALYZE;
""")
  runtime=name+'_app';pw=secrets.token_hex(24);sql("CREATE ROLE "+runtime+" LOGIN PASSWORD '"+pw+"'; GRANT euroerp_runtime TO "+runtime);role=True
  env['DATABASE_URL']=f'postgres://{runtime}:{pw}@127.0.0.1:54329/{name}?sslmode=disable'
  if os.getenv('CAPACITY_CANDIDATE_POLICY'):sql((ROOT/'order-policy.sql').read_text())
  if os.getenv('CAPACITY_PROFILE'):
   claims=json.dumps({'id':2,'role':'Marketing','developer':False,'scope':'team','companies':[1],'teams':[1],'lines':[],'orders':[],'units':[],'view_orders':True,'view_setup':False,'restrict_buyers':False,'buyer_ids':[],'production_areas':[]})
   queries=["SELECT o.id FROM orders o WHERE company_id=1 AND team_id=1", "SELECT o.id,(SELECT jsonb_agg(m.*) FROM milestones m WHERE m.order_id=o.id) FROM orders o WHERE company_id=1 AND team_id=1", "SELECT w.* FROM work_items w JOIN orders o ON o.id=w.order_id WHERE o.company_id=1 AND o.team_id=1"]
   for jit in ['on','off']:
    for q in queries:print(sql("BEGIN;SET LOCAL ROLE "+runtime+"; SELECT set_config('erp.identity','"+claims+"',true); SET LOCAL jit="+jit+"; EXPLAIN (ANALYZE,BUFFERS) "+q+";ROLLBACK;"),flush=True)
   raise SystemExit(0)

  with open(folder+'/api.log','w') as log:
   proc=subprocess.Popen([str(ROOT/'backend/erp-server')],env=env,stdout=log,stderr=log)
   for _ in range(100):
    try:urllib.request.urlopen(base+'/api/health',timeout=1);break
    except Exception:time.sleep(.1)
   clients=[Client(i) for i in range(1,501)]
   def login(c):
    started=time.monotonic();retries=0
    while True:
     status,ms,_=c.call('/login',{'username':'capacity.'+str(c.i),'password':password})
     if status!=429 or retries>=2:return status,(time.monotonic()-started)*1000,retries
     retries+=1;time.sleep(3)

   start=time.monotonic()
   with concurrent.futures.ThreadPoolExecutor(max_workers=6) as ex:logins=list(ex.map(login,clients))
   report['login_seconds']=round(time.monotonic()-start,2);report['login_statuses']={str(code):sum(s==code for s,_,_ in logins) for code in set(s for s,_,_ in logins)}
   assert all(s==200 for s,_,_ in logins),report['login_statuses']
   report['login_busy_retries']=sum(n for _,_,n in logins)
   print('500 actual logins succeeded from one shared IP',flush=True)
   def read(c,index):
    endpoint=['/data','/orders/'+str((c.i-1)%20+1),'/session'][index%3]
    status,ms,out=c.call(endpoint)
    if status==200 and endpoint=='/data':
     assert len(out['orders'])==100 and all(o['team_id']==(c.i-1)%20+1 for o in out['orders']),'Cross-team leakage or missing orders'
    return status,ms,endpoint
   futures=[];start=time.monotonic()
   with concurrent.futures.ThreadPoolExecutor(max_workers=50) as ex:
    for i in range(1000):
     delay=start+i/50-time.monotonic()
     if delay>0:time.sleep(delay)
     futures.append(ex.submit(read,clients[i%500],i))
    reads=[f.result() for f in futures]
   elapsed=time.monotonic()-start;lat=sorted(ms for _,ms,_ in reads)
   report['read_results']={'requests':len(reads),'elapsed_seconds':round(elapsed,2),'achieved_requests_per_second':round(len(reads)/elapsed,2),'p50_ms':round(lat[len(lat)//2],2),'p95_ms':round(lat[int(len(lat)*.95)-1],2),'p99_ms':round(lat[int(len(lat)*.99)-1],2),'max_ms':round(lat[-1],2),'statuses':{str(code):sum(s==code for s,_,_ in reads) for code in set(s for s,_,_ in reads)}}
   assert all(s==200 for s,_,_ in reads),report['read_results']
   def create(c):
    return c.call('/orders',{'company_id':1,'buyer_id':(c.i-1)%20+1,'po':'WRITE-'+str(c.i),'style':'Concurrent capacity suit','product':'Suits','quantity':10,'due_date':(datetime.date.today()+datetime.timedelta(days=40)).isoformat(),'unit_price':'42.50','currency':'INR'})
   started=time.monotonic()
   with concurrent.futures.ThreadPoolExecutor(max_workers=10) as ex:writes=list(ex.map(create,clients[:50]))
   write_times=sorted(ms for _,ms,_ in writes)
   report['write_results']={'orders_created':len(writes),'elapsed_seconds':round(time.monotonic()-started,2),'p95_ms':round(write_times[int(len(write_times)*.95)-1],2),'statuses':{str(code):sum(s==code for s,_,_ in writes) for code in set(s for s,_,_ in writes)}}
   assert all(s==200 for s,_,_ in writes),report['write_results']
   assert len({out['id'] for _,_,out in writes})==50,'Duplicate order numbers'
   assert sql("SELECT count(*) FROM audit WHERE action='Order created'")=='50','Missing transactional audit records'
   assert clients[0].call('/orders/2')[0]==404,'Cross-team order leaked'
   assert clients[0].call('/developer/performance')[0]==403,'Non-owner metrics access'
   # Revocation must take effect on the very next request; no stale cached permissions.
   sql("UPDATE users SET active=false WHERE username='capacity.1'")
   assert clients[0].call('/data')[0]==401,'Disabled account retained access'
   report['access_checks']=['Scoped lists contain exactly 100 assigned team orders','Cross-team direct lookup rejected','Performance metrics developer-only','Revocation effective immediately']
   report['status']='PASS' if report['read_results']['achieved_requests_per_second']>=45 and report['read_results']['p95_ms']<1000 else 'CAPACITY_TARGET_NOT_MET'
 (ROOT/'docs/capacity-results.json').write_text(json.dumps(report,indent=2));print(json.dumps(report,indent=2),flush=True)
finally:
 if proc:proc.terminate();proc.wait(timeout=15)
 if redis:run('docker','rm','-f',name)
 if db:sql('DROP DATABASE '+name+' WITH (FORCE)','postgres')
 if role:sql('DROP ROLE '+runtime,'postgres')
