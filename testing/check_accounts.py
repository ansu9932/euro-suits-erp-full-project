#!/usr/bin/env python3
from client import *
checks=[]
for a in state['accounts']:
 c=Client(a['username'],a['password']);u=c.call('/session')['user'];assert u['id']==a['id'];assert '/'+u['portal']==a['portal']
 if u.get('renewal_blocks'):c.call('/data',expect=423)
 elif 'view_orders' in u['grants']:c.call('/data')
 if 'view' in u['setup_grants']:c.call('/company-setup')
 path=a['portal']
 assert 'view_orders' in u['grants'] or 'view' in u['setup_grants'], a['username']+' has no assigned dashboard'
 r=c.o.open(BASE+path);assert r.status==200
 c.call('/logout',{});checks.append({'username':a['username'],'portal':a['portal'],'dashboard_path':path,'ok':True,'renewal_blocked':bool(u.get('renewal_blocks'))})
(PRIVATE/'account-checks.json').write_text(json.dumps({'accounts':checks,'checked_at':datetime.datetime.now().isoformat()},indent=2))
print('PASS: '+str(len(checks))+' account logins, sessions, allowed dashboard APIs and page routes on final build.')
