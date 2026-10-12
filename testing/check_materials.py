#!/usr/bin/env python3
"""Synthetic material-master acceptance checks, isolated UAT only. No email sends."""
from client import *
import io,zipfile,base64,copy

def account(name):
 a=next(x for x in state['accounts'] if x['username']==name);return Client(name,a['password'])
admin=account('admin');finance=account('finance');md=account('md');fabric=account('test.fabric.sourcing.manager');merchant=account('test.junior.merchant');other=account('test.merchant.team.b')
checks=[]
def passed(name): checks.append(name);print('PASS:',name)
company=state['company']['id'];fixtures=state.setdefault('material_examples',{});name='UAT Fabric Supplier - Practice'
header={'legal_name':name,'category':'Supplier','company_type':'Fabrics','country':'GB','city':'Test London','business_mode':'Domestic'}
register=finance.call('/company-setup');existing=next((c for c in register['companies'] if c['legal_name']==name),None)
sid=existing['id'] if existing else finance.call('/company-setup',header)['id'];fixtures['supplier_id']=sid;save();path='/company-setup/'+str(sid)
detail=finance.call(path)
if detail['status']=='Draft':
 fields=[{'id':f['id'],'not_applicable':True,'reason':'Synthetic material master practice only'} for f in register['fields'] if f['section']=='KYC' and f['country'] in ['*','GB'] and 'Supplier' in f['categories'] and (not f.get('business_modes') or 'Domestic' in f['business_modes'])]
 profile={'address':'Synthetic UAT address','owner':'UAT owner','email':'fabric@example.test','phone':'0000000000','reminder_email':'fabric@example.test','sample_address':'Synthetic sample delivery address','relationship':'Direct','supplier_code':'UAT-FAB-001','fax':'0000000000','dial_code':'+44','fields':fields,'contacts':[{'name':'UAT contact '+str(i),'email':f'contact{i}@example.test','phone':'0000000000','role':'Sales contact','website':'https://example.test','skype':'uat-'+str(i),'speciality':'Fabric sampling'} for i in range(1,4)]}
 finance.call(path,{'action':'save','version':detail['version'],**header,'profile':profile});finance.call(path,{'action':'submit','version':finance.call(path)['version']})
if md.call(path)['status']=='Pending':md.call(path,{'action':'approve','version':md.call(path)['version']})
assert sid in [s['id'] for s in fabric.call('/materials')['suppliers']]
passed('Finance supplier registration and MD approval populate material directory')
opts={}
for kind,label,parent in [('category','UAT Woven',None),('weave','UAT Twill','category'),('composition','UAT 65% Polyester 35% Viscose',None),('color','UAT Navy',None),('swatch_type','UAT Hanger',None),('product','UAT Suits',None),('season','UAT AW2027',None),('trim_type','UAT Thread',None)]:
 existing=next((o for o in fabric.call('/materials')['options'] if o['company_id']==company and o['kind']==kind and o['label']==label),None)
 opts[kind]=existing['id'] if existing else fabric.call('/material-options',{'company_id':company,'kind':kind,'label':label,'parent_id':opts.get(parent),'active':True})['id']
fixtures['options']=opts;save()
merchant.call('/material-options',{'company_id':company,'kind':'color','label':'Forbidden'},expect=403)
passed('Fabric Manager maintains category/weave masters; merchant cannot')
profile={**{k:v for k,v in opts.items() if k!='trim_type'},'received_on':datetime.date.today().isoformat(),'article':'UAT-SECRET-ARTICLE','legacy_reference':'UAT-OLD-ETF','construction':'2/2','yarn_count':'40s','density':'120x80','price':'4.75','currency':'USD','price_unit':'Metre','price_date':datetime.date.today().isoformat(),'moq_color':500,'moq_order':2000,'quantity_unit':'Metre','lead_time_days':45,'country_of_origin':'GB','sea_port':'UAT Sea Port','air_port':'UAT Airport','sample_length':2,'sample_length_unit':'Metre','length_metres':2,'labdip_reference':'UAT-LAB-01','handloom_reference':'UAT-HAND-01','location':'UAT Rack A','measurements':[{'weight':240,'weight_unit':'GSM','width':58,'width_unit':'inch','actual_weight':245,'actual_width':57.8}],'additional':[{'label':'UAT special finish','value':'Synthetic soft finish'}]}
def create(key,origin,kind='fabric'):
 if key in fixtures:
  try:return fabric.call('/materials/'+str(fixtures[key]))
  except Exception:pass
 body={'company_id':company,'supplier_setup_id':sid,'kind':kind,'origin':origin,'profile':profile}
 c=fabric if kind=='fabric' else admin
 r=c.call('/materials',body);fixtures[key]=r['id'];save();return fabric.call('/materials/'+str(r['id']))
sample=create('fabric_india','India');dubai=create('fabric_dubai','Dubai');trim=create('trim','India','trim')
assert sample['code'].startswith('ESF-') and dubai['code'].startswith('ETF-') and trim['code'].startswith('ESTR-')
passed('ESF / ETF / trim references are generated and retained')
body={'company_id':company,'supplier_setup_id':sid,'kind':'fabric','origin':'India','profile':profile}
fabric.call('/materials',{**body,'supplier_setup_id':0},expect=400)
wrong=copy.deepcopy(body);wrong['profile']['weave']=opts['color'];fabric.call('/materials',wrong,expect=400)
wrong=copy.deepcopy(body);wrong['profile']['measurements'][0]['weight']=-1;fabric.call('/materials',wrong,expect=400)
merchant.call('/materials',body,expect=403)
passed('Unregistered suppliers, wrong master choices, negative measurements and merchant writes rejected')
price='4.95' if sample['profile']['price']!='4.95' else '4.75'
change={**{k:sample[k] for k in ['company_id','supplier_setup_id','kind','active','version']},'profile':{**sample['profile'],'price':price}}
fabric.call('/materials/'+str(sample['id']),change);fabric.call('/materials/'+str(sample['id']),change,expect=409)
sample=fabric.call('/materials/'+str(sample['id']));assert len(sample['history'])>=2 and sample['profile']['price']==price
passed('Price changes preserve dated history; stale update rejected')
# Valid original 1x1 PNG fixture, no real business photos.
png=(PRIVATE/'uat-fabric.png').read_bytes()
def upload(c,content,name='UAT-fabric.png',expect=200):
 boundary='uat-fabric-photo';data=f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{name}"\r\nContent-Type: image/png\r\n\r\n'.encode()+content+f'\r\n--{boundary}--\r\n'.encode()
 return c.call('/materials/'+str(sample['id'])+'/photos',data,expect=expect,headers={'Content-Type':'multipart/form-data; boundary='+boundary})
while len(sample['photos'])<2:
 upload(fabric,png,'UAT-fabric-'+str(len(sample['photos'])+1)+'.png');sample=fabric.call('/materials/'+str(sample['id']))
pid=sample['photos'][0]['id'];assert fabric.call('/material-photos/'+str(pid)).startswith(b'\x89PNG')
upload(fabric,b'<script>alert(1)</script>','fake.png',400);upload(merchant,png,expect=403);merchant.call('/material-photos/'+str(pid),expect=403)
passed('Two photos uploaded/downloaded; fake photos and merchant photo access rejected')
public=merchant.call('/materials/'+str(sample['id']))
assert set(public['profile'])=={'category','weave','composition','color','measurements'}
assert not public['photos'] and not public['history']
for forbidden in ['supplier_setup_id','UAT-SECRET-ARTICLE','actual_weight','price','UAT Fabric Supplier']:
 assert forbidden not in json.dumps(public)
assert not merchant.call('/materials')['suppliers']
for c,private in [(fabric,True),(merchant,False)]:
 raw=c.call('/materials/export?ids='+str(sample['id']));z=zipfile.ZipFile(io.BytesIO(raw));xml=z.read('xl/worksheets/sheet1.xml').decode()
 assert sample['code'] in xml and 'UAT Woven' in xml and 'UAT Twill' in xml
 assert ('UAT-SECRET-ARTICLE' in xml)==private
 if not private:assert 'Current quoted price' not in xml and 'Supplier' not in xml
passed('Merchant API and Excel expose only seven public fabric fields')
# A submitted development request for Team A; team B must not see or edit it.
data=merchant.call('/development-masters');buyer=next(b for b in data['buyers'] if b['name']=='UAT Brand Alpha')
if 'development' not in fixtures:
 r=merchant.call('/development-masters',{'company_id':company,'buyer_id':buyer['id'],'profile':{'style_name':'UAT Suit - Master Practice','development_type':'Buyer development','product':'Suit','season':'AW2027','fit':'Regular','request_email_reference':'UAT reference only; no real email sent','charge_type':'Free of charge','charge_amount':0,'currency':'USD'},'material_ids':[sample['id']],'action':'submit'});fixtures['development']=r['id'];save()
dev=next(r for r in md.call('/development-masters')['records'] if r['id']==fixtures['development'])
assert dev['id'] not in [d['id'] for d in other.call('/development-masters')['records']]
other.call('/development-masters/'+str(dev['id']),{'action':'save','version':dev['version']},expect=404)
merchant.call('/development-masters/'+str(dev['id']),{'action':'approve','version':dev['version']},expect=403)
passed('Development links material references and preserves buyer-team / approval access')
if 'blocked_review' not in fixtures:
 r=merchant.call('/development-masters',{'company_id':company,'buyer_id':buyer['id'],'profile':{'style_name':'UAT Supplier Approval Recheck','development_type':'Buyer development','charge_type':'Free of charge','charge_amount':0,'currency':'USD','request_email_reference':'Synthetic test'},'material_ids':[sample['id']],'action':'submit'});fixtures['blocked_review']=r['id'];save()
review_dev=next(r for r in md.call('/development-masters')['records'] if r['id']==fixtures['blocked_review'])
version=next(s for s in md.call('/materials')['suppliers'] if s['id']==sid)['rating_version']
rate='/material-suppliers/'+str(sid)+'/rating'
fabric.call(rate,{'rating':'Blocked','reason':'Unauthorized','version':version},expect=403)
md.call(rate,{'rating':'Blocked','reason':'Synthetic UAT restriction check','version':version})
try:
 fabric.call('/materials',body,expect=400)
 assert next(s for s in merchant.call('/materials')['samples'] if s['id']==sample['id'])['supplier_blocked']
 upload(fabric,png,'blocked-upload.png',400)
 assert merchant.call('/materials/'+str(sample['id']))['supplier_blocked']
 pending=merchant.call('/development-masters',{'company_id':company,'buyer_id':buyer['id'],'profile':{'style_name':'UAT Blocked Supplier Review','development_type':'Buyer development','charge_type':'Free of charge','charge_amount':0,'currency':'USD','request_email_reference':'Synthetic test'},'material_ids':[sample['id']],'action':'submit'},expect=400)
 if review_dev['status']=='Submitted':md.call('/development-masters/'+str(review_dev['id']),{'action':'approve','version':review_dev['version']},expect=400)
 passed('Only MD / Super Admin may block; blocked supplier rejects samples, photos, selections and approval')
finally:
 v=next(s for s in md.call('/materials')['suppliers'] if s['id']==sid)['rating_version'];md.call(rate,{'rating':'Good','reason':'UAT restriction check complete; available for practice','version':v})
if review_dev['status']=='Submitted':md.call('/development-masters/'+str(review_dev['id']),{'action':'approve','version':review_dev['version'],'note':'Synthetic restriction check complete'})
if dev['status']=='Submitted':md.call('/development-masters/'+str(dev['id']),{'action':'approve','version':dev['version'],'note':'Synthetic UAT approval'})
passed('MD approval of submitted development request')
fixtures['references']={s['kind']+'_'+s['code'].split('-')[0]:s['code'] for s in [sample,dubai,trim]};save()
(PRIVATE/'material-check.json').write_text(json.dumps({'at':datetime.datetime.now().isoformat(),'checks':checks,'references':fixtures['references']},indent=2))
print(str(len(checks))+' material acceptance checks passed')
