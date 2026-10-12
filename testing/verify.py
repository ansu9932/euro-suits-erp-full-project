#!/usr/bin/env python3
"""Read-only account/scope checks and an idempotent purchase receipt test in UAT."""
from client import *
import socket,subprocess
checks=[]
def check(ok,label):
 if not ok:raise AssertionError(label)
 checks.append(label)
def c(username):
 a=next(a for a in state['accounts'] if a['username']==username);return Client(a['username'],a['password'])
md=c('md');data=md.call('/data');check(len(data['orders'])==15,'MD sees 15 sample orders')
purchases=data['purchases'];check(len(purchases)>=2,'MD sees supplier purchase approvals')
check(all(p['supplier_name'] and p['item_name'] for p in purchases),'Purchase summaries contain supplier and material names')
check(all('unit_cost_minor' not in p for p in purchases),'MD approval-only access does not leak purchase unit costs')
merchant=c('test.junior.merchant');team_b=c('test.merchant.team.b');alpha=c('test.merchant.alpha')
check(all(o['team_id']==state['resources']['team_a'] for o in merchant.call('/data')['orders']),'Team A order isolation')
check([o['po'] for o in team_b.call('/data')['orders']]==['UAT-TEAM-B'],'Team B order isolation')
check(all(o['buyer_id']==state['resources']['buyer_a'] for o in alpha.call('/data')['orders']),'Brand Alpha restriction')
team_b.call('/orders/'+str(state['orders']['UAT-SEWING']),expect=404);checks.append('Team B cannot open Team A order directly')
for name,field,key in [('test.production.line.02','line_id','line_b'),('test.cutting.table.2','cutting_table',None),('test.finishing.line.b','sub_line',None)]:
 orders=c(name).call('/data')['orders'];check(len(orders)==1 and orders[0]['po']=='UAT-TEAM-B',name+' assigned-area isolation')
merchant.call('/setup-users',expect=403);checks.append('Merchant cannot administer accounts')
check('profile' not in md.call('/company-setup/'+str(state['company']['setup_id'])),'MD KYC summary boundary')
check(md.call('/orders/'+str(state['orders']['UAT-DISPATCHED']))['stage']=='Dispatched','Full workflow reaches dispatch')
# Receive one purchase and leave the second waiting for the reviewer.
p=next(p for p in purchases if p['reference']=='UAT-FAB-001-PURCHASE')
if p['status']=='Pending':md.call('/records/purchases/'+str(p['id']),{'action':'approve'})
stores=c('test.fabric.store.manager');p=next(x for x in stores.call('/data')['purchases'] if x['id']==p['id'])
if p['status']=='Approved':stores.call('/records/purchases/'+str(p['id']),{'action':'receive'})
check(next(x for x in stores.call('/data')['purchases'] if x['id']==p['id'])['status']=='Received','Purchase approval to stock receipt')
# Public requests validate HTTPS normally, using a public DNS answer only if local DNS fails.
import client
url=env['PUBLIC_ORIGIN'];host=url.split('//')[1];dns='system'
try:socket.getaddrinfo(host,443)
except socket.gaierror:
 ips=subprocess.run(['dig','+short',host,'@1.1.1.1'],capture_output=True,text=True,check=True).stdout.splitlines();ip=next(x for x in ips if re.fullmatch(r'[0-9.]+',x));old=socket.getaddrinfo
 socket.getaddrinfo=lambda h,*a,**kw:old(ip if h==host else h,*a,**kw);dns='public DNS fallback; system DNS failed'
client.BASE=url
anonymous=Client();anonymous.call('/data',expect=401);checks.append('ERP data still requires account authentication')
r=anonymous.o.open(url+'/md');check(r.status==200 and not r.headers.get('WWW-Authenticate'),'Login page opens without preview password prompt')
pub=Client();a=next(a for a in state['accounts'] if a['username']=='md')
pub.call('/login',{'username':a['username'],'password':a['password']});check(pub.call('/session')['user']['id']==a['id'],'Public HTTPS MD login and secure session')
check(len(pub.call('/data')['orders'])==15,'Public MD dashboard API')
check(pub.o.open(url+'/md').status==200,'Public MD page responds');pub.call('/logout',{});checks.append('Public CSRF-protected logout')
(PRIVATE/'acceptance.json').write_text(json.dumps({'checks':checks,'public_origin':url,'dns':dns,'verified_at':datetime.datetime.now().isoformat()},indent=2))
print('PASS '+str(len(checks))+' acceptance checks; HTTPS preview '+url+'; DNS: '+dns)
