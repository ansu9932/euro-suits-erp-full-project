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
        h={'Origin':base,'X-CSRF-Token':self.csrf}
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
proc=None;redis_created=False;db_created=False
try:
    sql('CREATE DATABASE '+name);db_created=True
    rp=freeport();ap=freeport();base=f'http://127.0.0.1:{ap}'
    run('docker','run','-d','--name',name,'-p',f'127.0.0.1:{rp}:6379','redis:8-alpine','redis-server','--save','','--appendonly','no');redis_created=True
    with tempfile.TemporaryDirectory(prefix=name) as temp:
        env={**os.environ,'DATABASE_URL':f'postgres://euroerp:{config["POSTGRES_PASSWORD"]}@127.0.0.1:54329/{name}?sslmode=disable','REDIS_URL':f'redis://127.0.0.1:{rp}/0','HTTP_ADDR':f'127.0.0.1:{ap}','APP_ORIGIN':base,'APP_ENV':'local','DEMO_SEED':'true','OWNER_PASSWORD':password,'FILE_ROOT':temp+'/files'}
        log=open(temp+'/server.log','w+')
        proc=subprocess.Popen([str(ROOT/'backend/erp-server')],env=env,stdout=log,stderr=log)
        for _ in range(100):
            try:
                urllib.request.urlopen(base+'/api/health',timeout=1);break
            except Exception:
                if proc.poll() is not None: log.seek(0);raise RuntimeError(log.read())
                time.sleep(.1)
        users=['admin','ceo','finance','merchant.a','merchant.b','planning','cad','sampling','sourcing','stores','cutting','pm.lines1-6','pm.lines7-20','finishing','finishing.suits','quality','warehouse']
        clients={u:Client(u) for u in users};c=clients;a=c['admin'];m=c['merchant.a'];p=c['planning'];cad=c['cad'];samp=c['sampling'];qa=c['quality']
        snapshots={u:cl.call('/data') for u,cl in c.items()}
        check(len(snapshots['admin']['users'])==17,'All 17 local test accounts exist')
        for u,d in snapshots.items():
            profile=d['user'];allowed={o['id'] for o in d['orders']}
            for o in d['orders']:
                check(o['company_id'] in profile['companies'],u+' company boundary')
                if profile['scope']=='team': check(o['team_id'] in profile['teams'],u+' team boundary')
                if profile['scope']=='line': check(o['line_id'] in profile['lines'],u+' line boundary')
                if profile['scope']=='po': check(o['id'] in profile['orders'],u+' PO boundary')
                if profile['role']=='Finishing': check(o['finishing_unit'] in profile['finishing_units'],u+' finishing unit boundary')
                if not {'orders','finance'}.intersection(profile['grants']): check('unit_price_minor' not in o,u+' financial fields hidden')
            for key in ['entries','quality','work_items','reminders']: check(all(x['order_id'] in allowed for x in d[key]),u+' nested '+key+' scope')
            cl=c[u];cl.call('/orders/export')
        Client().call('/data',expect=401)
        a.call('/orders',{},403,{'X-CSRF-Token':''});a.call('/orders',{},403,{'Origin':'https://other.invalid'})
        oid=new_order(m);check(m.order(oid)['stage']=='Planning','New orders start in Planning')
        c['merchant.b'].call('/orders/'+str(oid),expect=404);c['pm.lines1-6'].call('/orders/'+str(oid),expect=404)
        m.call('/orders',{'company_id':1,'buyer_id':2,'po':'FORBIDDEN','style':'x','product':'Suits','quantity':10,'due_date':'2026-12-01'},403)
        c['merchant.b'].call('/orders/13',expect=404)
        m.action(oid,'approve',403);p.action(oid,'advance',400);a.action(oid,'approve');p.action(oid,'advance',400)
        p.action(oid,'allocate',line_id=1,cutting_table='Table 1',marker='A',sub_line='Suits A')
        stale=p.order(oid)['version'];p.action(oid,'advance');check(cad.order(oid)['stage']=='Development','Planning handoff reaches CAD')
        p.call('/orders/'+str(oid),{'action':'advance','version':stale},409)
        c['pm.lines1-6'].order(oid);c['pm.lines7-20'].call('/orders/'+str(oid),expect=404)
        c['finishing.suits'].call('/orders/'+str(oid),expect=404)
        c['pm.lines1-6'].action(oid,'advance',403)
        work=[w for w in cad.call('/data')['work_items'] if w['order_id']==oid]
        check({w['kind'] for w in work}=={'Style development','CAD / pattern'},'CAD receives both required records automatically')
        cad.action(oid,'advance',400)
        for w in work: cad.call('/records/work_items/'+str(w['id']),{'action':'update','status':'Approved','notes':'Checked'})
        cad.action(oid,'advance');check(samp.order(oid)['stage']=='Sampling','CAD handoff reaches Sampling')
        cad.call('/records/work_items/'+str(work[0]['id']),{'status':'Changes needed'},400)
        samp.action(oid,'advance',400)
        samples=[w for w in samp.call('/data')['work_items'] if w['order_id']==oid and w['kind']=='Pre-production sample']
        check(len(samples)==1,'Sampling receives PP sample automatically')
        samp.call('/records/work_items/'+str(samples[0]['id']),{'action':'update','status':'Approved','notes':'Fit approved'})
        samp.action(oid,'advance')
        for stage,user in [('Cutting','cutting'),('Sewing','pm.lines1-6'),('Finishing','finishing'),('Packing','finishing')]:
            worker=c[user];check(worker.order(oid)['stage']==stage,'Reached '+stage)
            worker.action(oid,'advance',400)
            worker.action(oid,'output',400,good=11,rework=0)
            worker.action(oid,'output',400,good=-1,rework=0)
            qa.action(oid,'quality',inspected=10,defects=0,result='Pass')
            worker.action(oid,'output',good=10,rework=0)
            worker.action(oid,'advance',400)
            qa.action(oid,'quality',inspected=10,defects=1,result='Hold')
            worker.action(oid,'advance',400)
            qa.action(oid,'quality',inspected=10,defects=0,result='Pass')
            worker.action(oid,'advance')
        check(c['warehouse'].order(oid)['stage']=='Warehouse','Packed PO reaches warehouse')
        c['warehouse'].action(oid,'advance',403,invoice_number='INV-1');c['finance'].action(oid,'advance',400)
        c['finance'].action(oid,'advance',invoice_number='INV-TEST');check(c['warehouse'].order(oid)['stage']=='Invoiced','Invoice queue complete')
        c['warehouse'].action(oid,'advance',400);c['warehouse'].action(oid,'advance',dispatch_reference='SHIP-TEST');check(a.order(oid)['stage']=='Dispatched','Full role-based journey ends at Dispatched')
        a.action(oid,'advance',400)
        # Documents are never public or readable outside an order scope.
        boundary='erp-test-boundary';payload=b'CAD test attachment\n'
        multipart=(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="pattern.txt"\r\nContent-Type: text/plain\r\n\r\n'.encode()+payload+f'\r\n--{boundary}--\r\n'.encode())
        doc=m.call(f'/orders/{oid}/documents',multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})
        did=m.order(oid)['documents'][0]['id'];check(m.call('/documents/'+str(did))==payload,'Private document round trip')
        c['merchant.b'].call('/documents/'+str(did),expect=404);Client().call('/documents/'+str(did),expect=401)
        # Purchase approval/receipt, exact quantities, duplicate and concurrent issue protection.
        item=c['stores'].call('/records/inventory',{'company_id':1,'name':'QA fabric','sku':'QA-FAB','store':'Fabric','unit':'metres','reorder_level':'5'})['id']
        supplier=c['sourcing'].call('/records/suppliers',{'company_id':1,'name':'QA nominated mill','category':'Fabrics','designation':'Nominated','buyer_id':1})['id']
        purchase=c['sourcing'].call('/records/purchases',{'company_id':1,'reference':'BUY-TEST','supplier_id':supplier,'item_id':item,'order_id':oid,'quantity':'10.25','unit_cost':'3.50','due_date':'2026-12-01'})['id']
        route='/records/purchases/'+str(purchase)
        c['stores'].call(route,{'action':'receive'},400);c['sourcing'].call(route,{'action':'approve'},403);c['finance'].call(route,{'action':'approve'});c['stores'].call(route,{'action':'receive'});c['stores'].call(route,{'action':'receive'},400)
        hist=c['stores'].call('/inventory/'+str(item));check(sum(x['quantity_hundredths'] for x in hist)==1025,'Receipt entered once with exact decimals')
        stockbody={'kind':'Issue','order_id':oid,'quantity':'6','reference':'Concurrency test'}
        def issue():
            try: c['stores'].call('/inventory/'+str(item),stockbody);return 200
            except AssertionError as e:
                if 'got 400:' in str(e):return 400
                raise
        with concurrent.futures.ThreadPoolExecutor(2) as pool: statuses=list(pool.map(lambda _:issue(),range(2)))
        check(sorted(statuses)==[200,400],'Concurrent stock issues cannot overdraw stock')
        check(sum(x['quantity_hundredths'] for x in c['stores'].call('/inventory/'+str(item)))==425,'Stock balance remains correct')
        # Permission checkboxes constrain actions and updating access revokes old sessions.
        profile={'username':'qa.readonly','name':'Read only QA','password':password,'role':'Marketing','scope':'team','companies':[1],'teams':[1],'permissions':[]}
        uid=a.call('/users',profile)['id'];readonly=Client('qa.readonly');check(readonly.call('/data')['user']['grants']==[],'Unchecked actions give no write grants')
        readonly.call('/orders',{},403)
        a.call('/users/'+str(uid),{**profile,'permissions':['orders']});readonly.call('/data',expect=401)
        readonly=Client('qa.readonly');check(readonly.call('/data')['user']['grants']==['orders'],'Checked action is granted after signing back in')
        a.call('/users/'+str(uid),{**profile,'permissions':['users']},400)
        a.call('/users/'+str(uid),{**profile,'companies':[1],'teams':[3]},400)
        a.call('/users/'+str(uid),{'active':False});readonly.call('/data',expect=401)
        # Every non-admin role is denied user management.
        for user in users:
            if user!='admin': c[user].call('/users',{},403)
        check(sql('SELECT count(*) FROM audit',name)!='0','Actions recorded in audit trail')
        try: sql("UPDATE audit SET detail='tamper'",name);raise AssertionError('Audit mutable')
        except subprocess.CalledProcessError: check(True,'Audit update rejected by database')
        result={'status':'PASS','checks':checks,'tested_at':time.strftime('%Y-%m-%d %H:%M:%S'),'roles':users,'checks_detail':report}
        (ROOT/'docs/test-results.json').write_text(json.dumps(result,indent=2))
        print(f'PASS: {checks} assertions, 17 roles, complete workflow, isolated data.',flush=True)
finally:
    if proc: proc.terminate();proc.wait(timeout=15)
    if redis_created: run('docker','rm','-f',name)
    if db_created: sql('DROP DATABASE '+name+' WITH (FORCE)')
