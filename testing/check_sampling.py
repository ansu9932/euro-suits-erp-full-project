"""UAT-only sample approval, sequence, stock and role-boundary regression."""
from client import Client,state
import datetime,json,uuid
from pathlib import Path

def client(name):
 a=next(x for x in state['accounts'] if x['username']==name)
 return Client(a['username'],a['password'])
m=client('test.junior.merchant');md=client('md');sam=client('test.sampling.manager');qa=client('test.sampling.qa');cad=client('test.pattern.cad.manager');other=client('test.merchant.team.b')
l=m.call('/sample-requests');buyer=l['buyers'][0];fabric=next(x for x in l['materials'] if x['company_id']==buyer['company_id'])
p={'company_id':buyer['company_id'],'buyer_id':buyer['id'],'profile':{'buyer_style':'UAT-SAMPLE-'+datetime.datetime.now().strftime('%H%M%S'),'sample_type':'Proto','est':'9999','plan_date':'2026-10-10','buyer_required_date':'2026-10-15','material_id':str(fabric['id'])},'variants':[{'size':'M','colour':'Navy','quantity':2},{'size':'XL','colour':'Navy','quantity':1}],'bom':[{'group':'Sewing trims','component':'Thread','reference':'UAT-THREAD','quantity':'1'}],'action':'save'}
r=m.call('/sample-requests',p);rid=r['id'];url='/sample-requests/'+str(rid)
assert not m.call(url)['pieces']
# Scoped buyer must be rejected from another team.
assert rid not in [x['id'] for x in other.call('/sample-requests')['records']]
other.call(url,expect=404)
# Attach an encrypted document while still draft.
boundary='test'+uuid.uuid4().hex
body=(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="sample-note.txt"\r\nContent-Type: text/plain\r\n\r\nUAT sample note\r\n--{boundary}--\r\n').encode()
f=m.call(url+'/files?category=Buyer%20email',body,headers={'Content-Type':'multipart/form-data; boundary='+boundary})
assert m.call('/sample-files/'+str(f['id']))==b'UAT sample note'
other.call('/sample-files/'+str(f['id']),expect=404)
p.update(action='submit',version=r['version']);r=m.call(url,p)
assert rid not in [x['id'] for x in sam.call('/sample-requests')['records']] # only approved goes to sampling
sam.call(url,expect=403)
m.call(url,{'version':r['version'],'action':'approve'},expect=403)
md.call(url,{'version':r['version'],'action':'approve'})
md.call(url,{'version':r['version'],'action':'approve'},expect=409)
d=sam.call(url);pieces=d['pieces'];assert len(pieces)==3
assert [x['id'] for x in pieces]==list(range(pieces[0]['id'],pieces[0]['id']+3))
assert [x['size'] for x in pieces]==['M','M','XL']
piece=pieces[0];iid=piece['id']
def step(c,action,expect=200,**kw):
 piece=next(x for x in c.call(url)['pieces'] if x['id']==iid)
 return c.call('/sample-indents/'+str(iid),{'version':piece['version'],'action':action,**kw},expect=expect)
step(m,'pattern',403)
step(sam,'handover',400)
step(cad,'pattern')
step(sam,'cut',400)
step(sam,'plan',production={'fabric_update_date':'2026-09-29','trims_update_date':'2026-09-29','style_update_date':'2026-09-29','pattern_by':'UAT CAD','sample_maker':'UAT tailor','handover_dc':'UAT-HANDOVER'})
stock={'company_id':buyer['company_id'],'material_id':fabric['id'],'metres':4,'reference':'UAT-RECEIPT'}
sam.call('/sample-stock',stock)
issue={**stock,'indent_id':iid,'metres':2};move=sam.call('/sample-stock',issue);assert move['dc'].startswith('SDC-')
sam.call('/sample-stock',{**issue,'metres':100000},expect=400)
step(sam,'cut');step(sam,'sew');step(sam,'audit',403,result='Approved')
step(qa,'audit',result='Rework',note='UAT stitch correction');step(sam,'sew');step(qa,'audit',result='Approved',note='UAT passed');step(sam,'handover')
assert next(x for x in sam.call(url)['pieces'] if x['id']==iid)['status']=='Handed over'
step(sam,'plan',403,production={})
Path('testing/private/sampling-test-result.json').write_text(json.dumps({'request_id':rid,'indents':[x['id'] for x in pieces],'passed':True},indent=2))
print('PASS: three piece numbers; MD-only approval; pre-approval queue hidden; cross-team request/file denial; encrypted upload/download; stock issue and overdraft rejection; CAD, cutting, sewing, QA rework, approval, handover; closed record protection.')
