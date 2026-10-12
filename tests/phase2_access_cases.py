# Runs only in company_setup.py's disposable database, using the real HTTP API and RLS role.
meta=admin.call('/setup-users')
for label in ['Regional Marketing Manager','Assistant Merchant','Trainee / Intern','Development Coordinator','Development QA','PPD Trainee','Showroom Executive','Fabric Sourcing Trainee','Fabric Technologist','Trims Sourcing Trainee','Assistant Sampling Manager','Sampling QA Manager','Assistant Cutting Manager','Industrial Engineer','IE Executive','Sewing Team Leader','Finishing Supervisor','Trimming Executive','Button Attach / Bartack Executive','Gate Security Executive']:
    check(any(d['name']==label for d in meta['designations']),'Phase 2 designation: '+label)
check(meta['access_codes']['permissions']['plan']=='OP-04','Stable permission codes are displayed separately from authorization keys')
def access_user(uid):return next(u for u in admin.call('/setup-users')['users'] if u['id']==uid)
def access_edit(uid,expect=200,**changes):
    u=access_user(uid)
    return admin.call('/setup-users/'+str(uid),{**u,'features':u['setup_features'],'permissions':u['effective_permissions'],**changes},expect)
manager_id=ops_user('qa.region','Regional Marketing Manager','team',teams=[team_a,team_b])
access_edit(manager_id,territory='Europe')
assistant_id=ops_user('qa.assistant','Assistant Merchant','team',teams=[team_a])
access_edit(assistant_id,reports_to_id=manager_id,restrict_buyers=True,buyer_ids=[buyer_a])
assistant=Client('qa.assistant');check(assistant.call('/session')['user']['reports_to_id']==manager_id,'Reporting manager persists in user identity')
check(access_user(manager_id)['territory']=='Europe','Region / territory persists')
access_edit(manager_id,400,reports_to_id=assistant_id)
access_edit(assistant_id,400,reports_to_id=assistant_id)
access_edit(assistant_id,400,reports_to_id='not-an-id')
access_edit(assistant_id,400,reports_to_id=999999)
access_edit(manager_id,400,active=False)
check(access_user(manager_id)['active'],'Cannot disable manager while active reports still depend on them')
# Brand-level visibility further narrows a team, including another brand on the SAME team.
brand_other=admin.call('/access-resources',{'kind':'buyers','company_id':opscompany,'team_id':team_a,'name':'QA Brand on Same Team'})['id']
admin.call('/access-resources',{'kind':'buyers','company_id':opscompany,'team_id':999999,'name':'Invalid team'},400)
brand_other_po=new_order(merch_a,'QA-BRAND-OTHER',brand_other,opscompany)
sql(f"UPDATE orders SET stage='Dispatched' WHERE id={brand_other_po}",name) # Exclude synthetic PO from earlier replacement expectations.
check(po_a in [o['id'] for o in assistant.call('/data')['orders']] and brand_other_po not in [o['id'] for o in assistant.call('/data')['orders']],'Brand restriction narrows orders inside the same buyer team')
assistant.call('/orders/'+str(brand_other_po),expect=404)
assistant.call('/orders/'+str(po_b),expect=404)
check([b['id'] for b in assistant.call('/data')['buyers']]==[buyer_a],'Other buyer records are absent from response metadata')
check(b'QA-BRAND-OTHER' not in assistant.call('/orders/export?company='+str(opscompany)),'CSV export respects brand restriction')
assistant.call('/orders',{'company_id':opscompany,'buyer_id':brand_other,'po':'BRAND-BYPASS','style':'X','product':'Suits','quantity':10,'due_date':'2027-01-01','unit_price':'1'},404)
access_edit(assistant_id,400,buyer_ids=[buyer_b])
access_edit(assistant_id,400,buyer_ids=[buyer_a,buyer_a])
access_edit(assistant_id,400,restrict_buyers='true')
brand_identity={**identity,'scope':'company','restrict_buyers':True,'buyer_ids':[buyer_a]}
rls=scoped_sql('SELECT id FROM orders',brand_identity)
check(str(po_a) in rls and str(brand_other_po) not in rls and str(po_b) not in rls,'Database RLS enforces buyer restriction without API filtering')
# Restrictions survive an older client which omits the new organization fields.
u=access_user(assistant_id);old_body={**u,'features':u['setup_features'],'permissions':u['effective_permissions']}
for key in ['reports_to_id','territory','restrict_buyers','buyer_ids','production_areas']:old_body.pop(key,None)
admin.call('/setup-users/'+str(assistant_id),old_body)
check(access_user(assistant_id)['buyer_ids']==[buyer_a] and access_user(assistant_id)['reports_to_id']==manager_id,'Older-client edits preserve buyer and reporting restrictions')
assistant.call('/data',expect=401);assistant=Client('qa.assistant')
access_edit(assistant_id,buyer_ids=[])
assistant.call('/data',expect=401);assistant=Client('qa.assistant')
check(assistant.call('/data')['orders']==[],'Restricted buyer list with no selection denies all orders')
access_edit(assistant_id,buyer_ids=[buyer_a]);assistant=Client('qa.assistant')
# Production areas are registered per company and mapped to planning allocation names.
table_id=admin.call('/access-resources',{'kind':'cutting_table','company_id':opscompany,'name':'1'})['id']
finish_id=admin.call('/access-resources',{'kind':'finishing_line','company_id':opscompany,'name':'A'})['id']
cut_id=ops_user('qa.table','Cutting Manager')
access_edit(cut_id,scope='cutting_table',production_areas=[table_id])
cut_user=Client('qa.table')
check(po_a in [o['id'] for o in cut_user.call('/data')['orders']],'Cutting staff can read their allocated table')
cut_user.call('/orders/'+str(po_b),expect=404)
check(cut_user.call('/data')['production_areas']==[a for a in admin.call('/setup-users')['production_areas'] if a['id']==table_id],'User receives only assigned production-area metadata')
access_edit(cut_id,400,production_areas=[finish_id])
access_edit(cut_id,400,production_areas=[])
finish_user_id=ops_user('qa.finish.line','Finishing Supervisor',units=['Suits'])
access_edit(finish_user_id,scope='finishing_line',production_areas=[finish_id])
finish_user=Client('qa.finish.line');check(finish_user.order(po_a)['id']==po_a,'Finishing sub-line and product unit both allow matching work')
finish_user.call('/orders/'+str(po_b),expect=404)
access_edit(finish_user_id,finishing_units=['Trousers'])
finish_user=Client('qa.finish.line');finish_user.call('/orders/'+str(po_a),expect=404)
area_identity={**identity,'role':'Cutting','scope':'cutting_table','production_areas':[table_id]}
rls=scoped_sql('SELECT id FROM orders',area_identity)
check(str(po_a) in rls and str(po_b) not in rls,'Database enforces cutting table visibility')
# Manager replacement reassigns direct reports and invalidates their stale sessions.
manager=access_user(manager_id)
replacement_region=admin.call('/setup-users/'+str(manager_id)+'/replace',{'name':'Regional successor','username':'qa.region.next','password':password,'version':manager['version']})
check(replacement_region['reports_transferred']==1,'Replacement transfers the reporting team')
check(access_user(assistant_id)['reports_to_id']==replacement_region['id'],'Direct reports now point to the successor')
check(access_user(replacement_region['id'])['territory']=='Europe','Replacement preserves the territory')
assistant.call('/data',expect=401)
# Trainee default is read-only; hierarchy never adds actions.
trainee_id=ops_user('qa.trainee','Trainee / Intern','team',teams=[team_a],inherit=False,permissions=['view_orders'])
access_edit(trainee_id,reports_to_id=replacement_region['id'])
trainee=Client('qa.trainee');trainee.call('/orders',{},403);trainee.call('/setup-users',{},403)
check(trainee.call('/session')['user']['grants']==['view_orders'],'Trainee receives only the explicitly granted module')

check(access_user(replacement_region['id'])['orders']==[],'Team-based replacement does not silently gain explicit PO overrides')
area_identity['orders']=[po_b]
check(str(po_b) not in scoped_sql('SELECT id FROM orders',area_identity),'Stale explicit PO IDs cannot bypass the current table scope')
# Optional browser QA runs before later tests delete the fixture company.
if os.getenv('PHASE2_BROWSER_HOLD'):
    stopfile=Path(temp)/'phase2-browser.done'
    print('Phase 2 browser fixtures ready: '+base+' ; stop file: '+str(stopfile),flush=True)
    while not stopfile.exists():time.sleep(1)
