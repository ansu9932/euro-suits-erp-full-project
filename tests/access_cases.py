# Executed inside company_setup.py's isolated database/server lifecycle.
meta=admin.call('/setup-users')
check(len(meta['designations'])>=57,'Workbook designation profiles remain available')
check(not any(k in json.dumps(meta) for k in ['password_hash','argon2id$']),'Access metadata contains no password hashes')
opscompany=int(sql(f'SELECT id FROM companies WHERE setup_id={cid}',name))
team_a=admin.call('/access-resources',{'kind':'teams','company_id':opscompany,'name':'QA Team A'})['id']
team_b=admin.call('/access-resources',{'kind':'teams','company_id':opscompany,'name':'QA Team B'})['id']
line_a=admin.call('/access-resources',{'kind':'lines','company_id':opscompany,'name':'QA Line 1'})['id']
line_b=admin.call('/access-resources',{'kind':'lines','company_id':opscompany,'name':'QA Line 2'})['id']
finance.call('/access-resources',{},403);md.call('/access-designations',{},403)
buyer_a=admin.call('/records/buyers',{'company_id':opscompany,'team_id':team_a,'name':'QA Buyer A'})['id']
buyer_b=admin.call('/records/buyers',{'company_id':opscompany,'team_id':team_b,'name':'QA Buyer B'})['id']
def designation(label):return next(d for d in admin.call('/setup-users')['designations'] if d['name']==label)
def ops_user(username,label,scope='company',teams=None,lines=None,orders=None,units=None,inherit=True,permissions=None):
    d=designation(label)
    body={'username':username,'name':'Employee '+username,'password':password,'designation_id':d['id'],'inherit_designation':inherit,'features':[],'permissions':permissions or [],'scope':scope,'companies':[opscompany],'teams':teams or [],'lines':lines or [],'orders':orders or [],'finishing_units':units or [],'active':True}
    return admin.call('/setup-users',body)['id']
merch_a_id=ops_user('qa.merch.a','Junior Merchant','team',teams=[team_a]);ops_user('qa.merch.b','Junior Merchant','team',teams=[team_b])
merch_a,merch_b=Client('qa.merch.a'),Client('qa.merch.b')
check(merch_a.call('/session')['user']['portal']=='merchandising','Merchants receive /merchandising portal')
merch_a.call('/login',{'username':'qa.merch.a','password':password,'portal':'sewing'},403)
merch_a.call('/setup-users',expect=403);merch_a.call('/company-setup',expect=403)
po_a=new_order(merch_a,'QA-A',buyer_a,opscompany);po_b=new_order(merch_b,'QA-B',buyer_b,opscompany)
check([o['id'] for o in merch_a.call('/data')['orders']]==[po_a],'Team A cannot see Team B orders')
merch_a.call('/orders/'+str(po_b),expect=404)
merch_b.call('/orders/'+str(po_a),expect=404)
merch_a.call('/orders',{'company_id':opscompany,'buyer_id':buyer_b,'po':'BYPASS','style':'X','product':'Suits','quantity':10,'due_date':'2027-01-01','unit_price':'1'},403)
ops_user('qa.approver','MD');approver_ops=Client('qa.approver')
for oid in [po_a,po_b]:approver_ops.action(oid,'approve')
ops_user('qa.planner','PPD Manager');planner=Client('qa.planner')
planner.action(po_a,'allocate',line_id=line_a,cutting_table='1',marker='A',sub_line='A')
planner.action(po_b,'allocate',line_id=line_b,cutting_table='2',marker='B',sub_line='B')
planner.action(po_a,'advance')
ops_user('qa.cad','Pattern CAD Manager');cad=Client('qa.cad');cad_order=cad.order(po_a)
check(cad_order['stage']=='Development','Planning handoff reaches CAD')
check(any(w['order_id']==po_a for w in cad.call('/data')['work_items']),'CAD work records are created at planning handoff')
line_user_id=ops_user('qa.sewing','Production Manager','line',lines=[line_a]);line_user=Client('qa.sewing')
check([o['id'] for o in line_user.call('/data')['orders']]==[po_a],'Sewing user sees only assigned line orders')
line_user.call('/orders/'+str(po_b),expect=404);line_user.action(po_a,'allocate',403,line_id=line_b)
line_user.call('/company-setup',expect=403)
exec((ROOT/'tests/security_access.py').read_text())
exec((ROOT/'tests/security_flow.py').read_text())
exec((ROOT/'tests/phase2_access_cases.py').read_text())
exec((ROOT/'tests/sample_planning_cases.py').read_text())
exec((ROOT/'tests/enquiry_cases.py').read_text())
exec((ROOT/'tests/export_cases.py').read_text())
# Per-user feature removal and template removal are server enforced; stale sessions are revoked.
viewer_id=ops_user('qa.viewer','Junior Merchant','team',teams=[team_a],inherit=False,permissions=['view_orders']);viewer=Client('qa.viewer')
viewer.call('/orders',{},403);viewer.call('/orders/'+str(po_a)+'/documents',multipart,403,{'Content-Type':'multipart/form-data; boundary='+boundary})
check('unit_price_minor' not in viewer.order(po_a),'Order visibility does not imply financial access')
custom_id=admin.call('/access-designations',{'name':'QA Restricted Merch','department':'QA','role':'Marketing','features':[],'permissions':['view_orders','orders']})['id']
custom_user_id=admin.call('/setup-users',{'username':'qa.template','name':'Template employee','password':password,'designation_id':custom_id,'inherit_designation':True,'active':True,'companies':[opscompany],'scope':'team','teams':[team_a]})['id']
template_user=Client('qa.template');check('orders' in template_user.call('/session')['user']['grants'],'New account inherits designation actions')
custom=next(d for d in admin.call('/setup-users')['designations'] if d['id']==custom_id)
admin.call('/access-designations/'+str(custom_id),{**custom,'permissions':['view_orders']})
template_user.call('/data',expect=401);template_user=Client('qa.template');template_user.call('/orders',{},403)
admin.call('/access-designations/'+str(custom_id),{**custom,'permissions':['view_orders','orders']},409)
admin.call('/setup-users',{'username':'qa.escalate','name':'Invalid role cap','password':password,'designation_id':custom_id,'inherit_designation':False,'permissions':['view_orders','orders'],'features':[],'active':True},400)
# Completed orders are read only; automatic replacement keeps original actors and saves snapshots.
shipped=new_order(merch_a,'QA-SHIPPED',buyer_a,opscompany)
sql(f"UPDATE orders SET stage='Dispatched',approval='Approved' WHERE id={shipped}",name)
source=next(u for u in admin.call('/setup-users')['users'] if u['id']==merch_a_id)
merch_a.call('/setup-users/'+str(merch_a_id)+'/replace',{},403)
replacement=admin.call('/setup-users/'+str(merch_a_id)+'/replace',{'name':'New merchandising employee','username':'qa.replacement','password':password,'version':source['version']})
check(replacement['orders_transferred']==1,'Replacement automatically transfers only unfinished work in source scope')
merch_a.call('/data',expect=401);Client().call('/login',{'username':'qa.merch.a','password':password},401)
new_employee=Client('qa.replacement');check(new_employee.order(po_a)['stage']=='Development','Replacement continues current order stage')
check(new_employee.order(shipped)['stage']=='Dispatched','Replacement can read but cannot modify shipped history')
new_employee.action(shipped,'add_milestone',400,title='Must not change',due_date='2027-01-01')
new_employee.call('/orders/'+str(shipped)+'/documents',multipart,400,{'Content-Type':'multipart/form-data; boundary='+boundary})
check(sql(f'SELECT created_by FROM orders WHERE id={po_a}',name)==str(merch_a_id),'Order authorship remains with the original employee')
history=admin.call('/setup-users')['history'];handover=next(h for h in history if h['action'].startswith('Employee replaced') and h['subject_id']==merch_a_id)
check(handover['detail']['orders'][0]['work_items']!=[],'Handover stores immutable work and milestone snapshots')
try:sql('DELETE FROM access_history',name);raise AssertionError('Access history mutable')
except subprocess.CalledProcessError:check(True,'Designation and handover history are append-only')
# Responsibility lock at 30 overdue days; a separate corrective path stays open.
admin.call(path+'/responsibility',{'version':detail(admin)['version'],'user_id':line_user_id})
sql(f"UPDATE company_reminders SET expires_on=CURRENT_DATE-29,remind_on=CURRENT_DATE-90 WHERE company_id={cid}",name)
check(line_user.call('/session')['user']['renewal_blocks']==[],'29-day overdue document does not lock dashboard')
sql(f"UPDATE company_reminders SET expires_on=CURRENT_DATE-30 WHERE company_id={cid}",name)
check(len(line_user.call('/session')['user']['renewal_blocks'])==2,'Lock begins at exactly 30 overdue days')
line_user.call('/data',expect=423);line_user.call('/orders/'+str(po_a),expect=423)
check(admin.call('/session')['user']['renewal_blocks']==[],'Super Admin stays available to resolve overdue renewals')
admin.call(path+'/responsibility',{'version':detail(admin)['version'],'user_id':finance.call('/session')['user']['id']})
finance.call('/company-setup',header,423)
check(len(finance.call('/session')['user']['renewal_blocks'])==2,'Responsible Finance account sees renewal lock')
act(finance,'amend');check(len(finance.call('/session')['user']['renewal_blocks'])==2,'Starting amendment cannot bypass renewal lock')
# Include the new multi-recipient and trade registration fields in the approved amendment.
p['reminder_emails']=['logistics@example.test','accounts@example.test']
p['contacts']=[{'role':'Sales and Export Head','name':'Contact person','email':'contact@example.test','phone':'000'}]
p['trade_registrations']=[{'id':'qa-trade','portal':'DGFT','registration':'REG','licence':'ADVANCE-TEST','bank_id':'bank-1','port_ids':[sea],'expiry':True,'valid_from':datetime.date.today().isoformat(),'expires_on':expires,'documents':[fid]}]
act(finance,'save',**header,profile=p)
check(len(finance.call('/session')['user']['renewal_blocks'])==2,'Saving new dates cannot bypass approval lock')
act(finance,'submit');act(admin,'approve')
check(finance.call('/session')['user']['renewal_blocks']==[],'Approved renewal clears the responsible user lock')
check(len(md.call('/company-setup')['reminders'])==3,'Trade registrations also generate renewal reminders')
check('logistics@example.test' not in json.dumps(detail(md)),'Additional recipients remain private in MD summary')
check(len(detail(admin)['profile']['reminder_emails'])==2,'Multiple reminder recipients persist')
# Both company name and current password are required for deletion.
admin.call(path+'/delete',{'version':detail(admin)['version'],'confirmation_name':'REGRESSION INTERNAL'},403)
admin.call(path+'/delete',{'version':detail(admin)['version'],'confirmation_name':'REGRESSION INTERNAL','password':'wrong'},403)
run('docker','exec',name,'redis-cli','DEL','company-delete:'+str(admin.call('/session')['user']['id']))

# Every department portal maps to the correct designation; no real staff are imported.
for label,portal_name in [('CEO','management'),('Accounts Executive','accounts'),('Fabric Sourcing Manager','sourcing'),('Fabric Store Keeper','stores'),('Sampling Manager','sampling'),('Cutting Manager','cutting'),('Finishing Manager','finishing'),('Finishing QA','quality'),('Warehouse Manager','warehouse')]:
    username='qa.portal.'+portal_name
    ops_user(username,label,units=['Suits'] if portal_name=='finishing' else [])
    user=Client(username)
    check(user.call('/session')['user']['portal']==portal_name,'Designation routes to /'+portal_name)
    user.call('/data')

# Domestic buyer KYC excludes IEC by business configuration, with country rules still applied.
domestic=finance.call('/company-setup',{'legal_name':'QA DOMESTIC BUYER','category':'Buyer','company_type':'Retailer','country':'IN','city':'Delhi','business_mode':'Domestic'})['id']
domestic_detail=finance.call('/company-setup/'+str(domestic))
check(not any(f['label']=='IEC' for f in domestic_detail['summary']['checklist']),'Domestic company flow automatically excludes IEC field')
check(any(f['label']=='GST registration' for f in domestic_detail['summary']['checklist']),'Domestic India company retains GST field')
finance.call('/company-setup',{'legal_name':'INVALID MODE','category':'Buyer','company_type':'Retailer','country':'IN','business_mode':'invalid'},400)

# Finance replacement receives preparation responsibility; the original creator stays recorded.
finance_source=next(u for u in admin.call('/setup-users')['users'] if u['id']==limited_id)
finance_successor=admin.call('/setup-users/'+str(limited_id)+'/replace',{'version':finance_source['version'],'username':'qa.finance.successor','name':'New Finance employee','password':password})
finance_new=Client('qa.finance.successor')
check(finance_new.call(ownpath)['can_edit'],'Finance successor can continue the transferred company draft')
check(finance_new.call(ownpath)['created_by']==limited_id,'Company draft retains its original creator')
limited.call(ownpath,expect=401)
