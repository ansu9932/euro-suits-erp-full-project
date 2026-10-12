#!/usr/bin/env python3
"""Populate the dedicated UAT deployment through the same APIs the UI uses.
Never points at port 8080 or uses the live application's .env.
Private manifest preserves generated passwords across interrupted runs.
"""
from pathlib import Path
import json,os,secrets,datetime,http.cookiejar,urllib.request,urllib.error,time,re
ROOT=Path(__file__).resolve().parent;PRIVATE=ROOT/'private';os.umask(0o077)
BASE='http://localhost:8082';env=dict(x.split('=',1) for x in (PRIVATE/'environment.env').read_text().splitlines() if '=' in x)
statefile=PRIVATE/'accounts.json';state=json.loads(statefile.read_text()) if statefile.exists() else {'accounts':[],'resources':{},'orders':{}}
def save():statefile.write_text(json.dumps(state,indent=2))
class Client:
 def __init__(self,username=None,password=None):
  self.o=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()));self.csrf=''
  if username:self.call('/login',{'username':username,'password':password})
 def call(self,path,body=None,expect=200,headers=None):
  h={'Origin':BASE,'X-CSRF-Token':self.csrf,**(headers or {})}
  if isinstance(body,dict):body=json.dumps(body).encode();h['Content-Type']='application/json'
  req=urllib.request.Request(BASE+'/api'+path,data=body,headers=h)
  try:r=self.o.open(req,timeout=40)
  except urllib.error.HTTPError as e:r=e
  raw=r.read();d=json.loads(raw) if 'application/json' in r.headers.get('Content-Type','') else raw
  if r.status!=expect:raise RuntimeError(f'{path}: HTTP {r.status}: '+str(d)[:220])
  if isinstance(d,dict) and d.get('csrf'):self.csrf=d['csrf']
  return d
 def action(self,oid,action,**kw):return self.call('/orders/'+str(oid),{'action':action,'version':self.call('/orders/'+str(oid))['version'],**kw})
admin=Client('admin',env['BOOTSTRAP_PASSWORD']);finance=Client('finance',env['BOOTSTRAP_PASSWORD']);md=Client('md',env['BOOTSTRAP_PASSWORD'])
meta=admin.call('/setup-users')
def byname(label):return next(d for d in meta['designations'] if d['name']==label)
def resource(key,kind,name,**kw):
 if key not in state['resources']:
  state['resources'][key]=admin.call('/access-resources',{'kind':kind,'company_id':company,'name':name,**kw})['id'];save()
 return state['resources'][key]
# Finance -> submit -> MD approval, with clearly labelled synthetic evidence.
if 'company' not in state:
 header={'legal_name':'UAT Euro Suits Demo Company','category':'Internal','company_type':'Self','country':'IN','city':'Test Bengaluru','business_mode':'Domestic'}
 setup=finance.call('/company-setup',header)['id'];state['company']={'setup_id':setup,'header':header};save()
setup=state['company']['setup_id'];header=state['company']['header'];path='/company-setup/'+str(setup)
detail=finance.call(path)
if detail['status']=='Draft':
 sea=next((p['id'] for p in finance.call('/company-setup')['ports'] if p['code']=='UATSEA'),None)
 if sea is None:sea=finance.call('/company-ports',{'name':'UAT Test Sea Port','code':'UATSEA','country':'IN','mode':'Sea'})['id']
 air=next((p['id'] for p in finance.call('/company-setup')['ports'] if p['code']=='UATAIR'),None)
 if air is None:air=finance.call('/company-ports',{'name':'UAT Test Airport','code':'UATAIR','country':'IN','mode':'Air'})['id']
 boundary='uat-proof';pdf=b'%PDF-1.4\n% SYNTHETIC UAT SAMPLE - NOT A REAL KYC DOCUMENT\n%%EOF\n'
 multipart=(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="UAT-SAMPLE-ONLY.pdf"\r\nContent-Type: application/pdf\r\n\r\n'.encode()+pdf+f'\r\n--{boundary}--\r\n'.encode())
 fid=finance.call(path+'/files',multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})['id']
 profile={'address':'Synthetic registered address - test use only','owner':'UAT Company Owner','email':'uat@example.test','phone':'0000000000','reminder_email':'uat@example.test','banks':[{'id':'uat-bank','name':'UAT Test Bank','holder':'UAT Demo Company','account':'UAT-NOT-REAL-ACCOUNT','routing':'UAT-ONLY','currency':'INR','documents':[fid]}],'ports':[{'id':sea,'default':True,'registered':False},{'id':air,'default':True,'registered':False}],'fields':[{'id':f['id'],'not_applicable':True,'reason':'Synthetic UAT company. No real legal registration.'} for f in finance.call('/company-setup')['fields'] if f['section']=='KYC' and f['country'] in ['*',header['country']] and header['category'] in f['categories'] and (not f.get('business_modes') or header['business_mode'] in f['business_modes'])]}
 finance.call(path,{'action':'save','version':detail['version'],**header,'profile':profile})
 finance.call(path,{'action':'submit','version':finance.call(path)['version']})
if md.call(path)['status']=='Pending':md.call(path,{'action':'approve','version':md.call(path)['version']})
company=next(c['id'] for c in admin.call('/setup-users')['companies'] if c['setup_id']==setup)
state['company']['id']=company;save()
ta=resource('team_a','teams','UAT Team A');tb=resource('team_b','teams','UAT Team B')
ba=resource('buyer_a','buyers','UAT Brand Alpha',team_id=ta);ba2=resource('buyer_a2','buyers','UAT Brand Beta',team_id=ta);bb=resource('buyer_b','buyers','UAT Brand Gamma',team_id=tb)
la=resource('line_a','lines','UAT Line 01');lb=resource('line_b','lines','UAT Line 02')
ca=resource('table_a','cutting_table','UAT Table 1');cb=resource('table_b','cutting_table','UAT Table 2')
fa=resource('finish_a','finishing_line','UAT Finishing A');fb=resource('finish_b','finishing_line','UAT Finishing B')
# Exactly one primary account for every designation, plus explicit comparison users.
existing={u['username']:u for u in admin.call('/setup-users')['users']}
for index,d in enumerate(meta['designations']):
 username={'Super Admin':'admin','Finance Head':'finance','MD':'md'}.get(d['name'],'test.'+re.sub('[^a-z0-9]+','.',d['name'].lower()).strip('.'))
 entry=next((a for a in state['accounts'] if a['username']==username),None)
 if entry is None:
  entry={'username':username,'password':env['BOOTSTRAP_PASSWORD'] if username in ['admin','finance','md'] else secrets.token_urlsafe(14),'name':'TEST '+d['name'],'designation':d['name'],'designation_id':d['id'],'department':d['department'],'role':d['role']};state['accounts'].append(entry);save()
 body={'username':username,'name':entry['name'],'password':entry['password'],'designation_id':d['id'],'inherit_designation':True,'active':True,'features':[],'permissions':[],'companies':[company],'teams':[],'lines':[],'orders':[],'scope':'company','finishing_units':[]}
 if d['role']=='Marketing':body.update(scope='team',teams=[ta])
 if d['role']=='Production':body.update(scope='line',lines=[la])
 if d['role']=='Cutting':body.update(scope='cutting_table',production_areas=[ca])
 if d['role']=='Finishing':body.update(scope='finishing_line',production_areas=[fa],finishing_units=['Suits','Trousers','Shirts','General'])
 if username in existing:
  entry['id']=existing[username]['id']
  if username!='admin' and not entry.get('configured'):admin.call('/setup-users/'+str(entry['id']),{**body,'version':existing[username]['version']})
 else:entry['id']=admin.call('/setup-users',body)['id']
 entry['scope']=body['scope'];entry['configured']=True;save()
 if index%10==0:print(f'Prepared {index+1}/{len(meta["designations"])} designation accounts',flush=True)
meta_users=admin.call('/setup-users')['users']
for entry in state['accounts']:entry['portal']=next(u['portal'] for u in meta_users if u['id']==entry['id'])
save()
def entry(label):return next(a for a in state['accounts'] if a['designation']==label)
def client(label):a=entry(label);return Client(a['username'],a['password'])
# Extra users make data-isolation tests practical.
extras=[('test.merchant.team.b','Junior Merchant','Team B merchant',{'scope':'team','teams':[tb]}),('test.production.line.02','Production Manager','Line 02 manager',{'scope':'line','lines':[lb]}),('test.cutting.table.2','Cutting Manager','Table 2 cutting',{'scope':'cutting_table','production_areas':[cb]}),('test.finishing.line.b','Finishing Manager','Finishing B manager',{'scope':'finishing_line','production_areas':[fb],'finishing_units':['Suits','Trousers','Shirts','General']}),('test.merchant.alpha','Junior Merchant','Brand Alpha only',{'scope':'team','teams':[ta],'restrict_buyers':True,'buyer_ids':[ba]})]
for uname,label,title,scope in extras:
 a=next((x for x in state['accounts'] if x['username']==uname),None)
 if a is None:
  d=byname(label);a={'username':uname,'password':secrets.token_urlsafe(14),'name':'TEST '+title,'designation':label,'designation_id':d['id'],'department':d['department'],'role':d['role'],'comparison':title};state['accounts'].append(a);save()
 if not a.get('id'):
  a['id']=admin.call('/setup-users',{'username':uname,'name':a['name'],'password':a['password'],'designation_id':a['designation_id'],'inherit_designation':True,'active':True,'companies':[company],'features':[],'permissions':[],**scope})['id']
 a['scope']=scope['scope'];a['portal']=entry(label)['portal'];save()
# Reporting tree; no silent permission inheritance.
if not state.get('reporting_ready'):
 users={u['id']:u for u in admin.call('/setup-users')['users']}
 managers={'Marketing & Merchandising':'Regional Marketing Manager','Planning & PP':'PPD Manager','Product Development':'Development Manager','Fabric Sourcing':'Fabric Sourcing Manager','Trims Sourcing':'Trims Sourcing Manager','Sampling':'Sampling Manager','Cutting':'Cutting Manager','Fabric Stores':'Fabric Store Manager','Trims Stores':'Trims Store Manager','Sewing':'Production Manager','Finishing':'Finishing Manager','Warehouse':'Warehouse Manager','Finance & Accounts':'Finance Head','Security':'Warehouse Manager'}
 for a in state['accounts']:
  if a['username'] in ['admin','md']:continue
  target=managers.get(a['department'],'MD');target='MD' if target==a['designation'] else target
  u=users[a['id']]
  admin.call('/setup-users/'+str(a['id']),{**u,'features':u['setup_features'],'permissions':u['effective_permissions'],'reports_to_id':entry(target)['id'],'territory':'UAT region' if a['department']=='Marketing & Merchandising' else ''})
 state['reporting_ready']=True;save()
# Renew clients after access saves revoked sessions.
finance=client('Finance Head');md=client('MD');merchant=client('Junior Merchant');planner=client('PPD Manager');cad=client('Pattern CAD Manager');sampling=client('Sampling Manager');cutting=client('Cutting Manager');sewing=client('Production Manager');finishing=client('Finishing Manager');quality=client('Finishing QA');accounts=client('Accounts Executive');warehouse=client('Warehouse Manager')
stages=['Planning','Development','Sampling','Cutting','Sewing','Finishing','Packing','Warehouse','Invoiced','Dispatched']
def order(name,target,product='Suits',buyer=None):
 if name not in state['orders']:
  state['orders'][name]=merchant.call('/orders',{'company_id':company,'buyer_id':buyer or ba,'po':name,'style':'UAT sample '+product,'product':product,'quantity':10,'due_date':(datetime.date.today()+datetime.timedelta(days=45)).isoformat(),'unit_price':'42.50','currency':'USD'})['id'];save()
 oid=state['orders'][name]
 if target=='Pending approval':return oid
 current=planner.call('/orders/'+str(oid))
 if current['approval']=='Pending':md.action(oid,'approve')
 while (current:=planner.call('/orders/'+str(oid)))['stage']!=target:
  stage=current['stage']
  if stages.index(stage)>stages.index(target):break
  if stage=='Planning':planner.action(oid,'allocate',line_id=la,cutting_table='UAT Table 1',marker='A',sub_line='UAT Finishing A');planner.action(oid,'advance')
  elif stage in ['Development','Sampling']:
   actor=cad if stage=='Development' else sampling
   for item in actor.call('/data')['work_items']:
    if item['order_id']==oid and item['status']!='Approved':actor.call('/records/work_items/'+str(item['id']),{'action':'update','status':'Approved','notes':'Synthetic UAT preparation'})
   actor.action(oid,'advance')
  elif stage in ['Cutting','Sewing','Finishing','Packing']:
   actor={'Cutting':cutting,'Sewing':sewing,'Finishing':finishing,'Packing':finishing}[stage]
   current_data=next(o for o in actor.call('/data')['orders'] if o['id']==oid)
   remaining=10-current_data.get('totals',{}).get(stage,0)
   if remaining:actor.action(oid,'output',good=remaining,rework=0,note='Synthetic UAT preparation')
   quality.action(oid,'quality',inspected=10,defects=0,result='Pass',notes='Synthetic UAT check')
   actor.action(oid,'advance')
  elif stage=='Warehouse':accounts.action(oid,'advance',invoice_number='UAT-INV-'+str(oid))
  elif stage=='Invoiced':warehouse.action(oid,'advance',dispatch_reference='UAT-DISPATCH-'+str(oid))
 return oid
for stage in ['Pending approval']+stages:
 name='UAT-'+stage.upper().replace(' ','-');order(name,stage);print('Ready: '+name,flush=True)
order('UAT-TROUSERS-FINISHING','Finishing','Trousers');order('UAT-SHIRTS-FINISHING','Finishing','Shirts');order('UAT-BRAND-BETA','Pending approval',buyer=ba2)
if 'UAT-TEAM-B' not in state['orders']:
 a=next(a for a in state['accounts'] if a['username']=='test.merchant.team.b');m=Client(a['username'],a['password']);oid=m.call('/orders',{'company_id':company,'buyer_id':bb,'po':'UAT-TEAM-B','style':'Team B comparison suit','product':'Suits','quantity':10,'due_date':(datetime.date.today()+datetime.timedelta(days=45)).isoformat(),'unit_price':'42.50','currency':'USD'})['id'];state['orders']['UAT-TEAM-B']=oid;save();md.action(oid,'approve');planner.action(oid,'allocate',line_id=lb,cutting_table='UAT Table 2',marker='B',sub_line='UAT Finishing B')
# Open company drafts for Finance / MD to test both sides without rebuilding fixtures.
if not state.get('drafts_ready'):
 for name,submit in [('UAT Company Draft - Finance Practice',False),('UAT Company Approval - MD Practice',True)]:
  h={'legal_name':name,'category':'Buyer','company_type':'Retailer','country':'GB','city':'Test London','business_mode':'Domestic'}
  cid=next((c['id'] for c in finance.call('/company-setup')['companies'] if c['legal_name']==name),None)
  if cid is None:cid=finance.call('/company-setup',h)['id']
  d=finance.call('/company-setup/'+str(cid));profile={'sample_address':'Synthetic sample address','address':'Synthetic test address','owner':'Test Buyer Owner','email':'buyer@example.test','reminder_email':'buyer@example.test','phone':'0000000000','fields':[{'id':f['id'],'not_applicable':True,'reason':'Synthetic acceptance test only'} for f in finance.call('/company-setup')['fields'] if f['section']=='KYC' and f['country'] in ['*',h['country']] and h['category'] in f['categories'] and (not f.get('business_modes') or h['business_mode'] in f['business_modes'])]}
  finance.call('/company-setup/'+str(cid),{'action':'save','version':d['version'],**h,'profile':profile})
  if submit:finance.call('/company-setup/'+str(cid),{'action':'submit','version':finance.call('/company-setup/'+str(cid))['version']})
 state['drafts_ready']=True;save()
# Material purchasing and stock examples.
if not state.get('materials_ready'):
 sourcing=client('Fabric Sourcing Manager');stores=client('Fabric Store Manager')
 supplier=sourcing.call('/records/suppliers',{'company_id':company,'name':'UAT Fabric Supplier','category':'Fabrics','designation':'Direct','contact':'Synthetic contact'})['id']
 for sku,label,store,unit in [('UAT-FAB-001','UAT Suiting Fabric','Fabric','metres'),('UAT-TRIM-001','UAT Buttons','Trims','pieces')]:
  iid=stores.call('/records/inventory',{'company_id':company,'sku':sku,'name':label,'store':store,'unit':unit,'reorder_level':'20'})['id']
  stores.call('/inventory/'+str(iid),{'kind':'Receipt','quantity':'100','reference':'UAT opening test stock'})
  sourcing.call('/records/purchases',{'company_id':company,'reference':sku+'-PURCHASE','supplier_id':supplier,'item_id':iid,'quantity':'50','unit_cost':'2.50','due_date':(datetime.date.today()+datetime.timedelta(days=10)).isoformat()})
 state['materials_ready']=True;save()
# Login and session checks for EVERY account; no email or OTP sent.
checks=[]
for a in state['accounts']:
 c=Client(a['username'],a['password']);user=c.call('/session')['user'];assert '/'+user['portal']==a['portal'];assert user['id']==a['id']
 if user['grants'] and 'view_orders' in user['grants']:c.call('/data')
 if 'view' in user['setup_grants']:c.call('/company-setup')
 c.call('/logout',{});checks.append({'username':a['username'],'portal':a['portal'],'ok':True})
(PRIVATE/'verification.json').write_text(json.dumps({'accounts':checks,'full_flow':'A sample order passed all required approvals, quality checks, invoicing and dispatch through real APIs.','orders':state['orders'],'verified_at':datetime.datetime.now().isoformat()},indent=2))
state['complete']=True;save();print('PASS: '+str(len(checks))+' account logins/dashboards; '+str(len(state['orders']))+' workflow orders ready.',flush=True)
