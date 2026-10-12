"""Historical reference regression in the isolated UAT database only."""
from client import Client,state
from copy import deepcopy
import time
account=next(x for x in state['accounts'] if x['username']=='test.fabric.sourcing.manager')
c=Client(account['username'],account['password'])
d=c.call('/materials')
s=next(x for x in d['samples'] if x['kind']=='fabric')
p=deepcopy(s['profile']);p['old_etf']='ETF/UAT-HISTORY/'+str(time.time_ns());p['legacy_reference']='Separate archival reference';p['article']='UAT old ETF regression'
body={k:s[k] for k in ['company_id','supplier_setup_id','kind','active']};body.update(origin='Dubai',profile=p)
r=c.call('/materials',body);assert r['code']==p['old_etf']
c.call('/materials',body,expect=409)
saved=c.call('/materials/'+str(r['id']));assert saved['profile']['legacy_reference']=='Separate archival reference'
body['version']=saved['version'];body['profile']['old_etf']='OTHER-REFERENCE';c.call('/materials/'+str(r['id']),body,expect=400)
body['profile']['old_etf']=r['code'];body['active']=False
c.call('/materials/'+str(r['id']),body)
assert c.call('/materials/'+str(r['id']))['code']==r['code']
body.pop('version');body['profile']['old_etf']='';body['active']=False
new=c.call('/materials',body);assert new['code'].startswith('ETF-')
print('PASS: historical reference, duplicate rejection, separate legacy field, immutable reference, generated fallback. Test records archived.')
