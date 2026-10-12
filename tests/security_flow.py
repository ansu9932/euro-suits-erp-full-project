# An independent order verifies every department under the runtime RLS role.
flow=new_order(merch_b,'QA-SECURE-FLOW',buyer_b,opscompany)
approver_ops.action(flow,'approve')
planner.action(flow,'allocate',line_id=line_b,cutting_table='2',marker='B',sub_line='B')
planner.action(flow,'advance')
work=[w for w in cad.call('/data')['work_items'] if w['order_id']==flow]
for item in work:cad.call('/records/work_items/'+str(item['id']),{'action':'update','status':'Approved','notes':'Security regression'})
cad.action(flow,'advance')
ops_user('qa.flow.sampling','Sampling Manager');sample_worker=Client('qa.flow.sampling')
for item in sample_worker.call('/data')['work_items']:
    if item['order_id']==flow and item['kind']=='Pre-production sample':sample_worker.call('/records/work_items/'+str(item['id']),{'action':'update','status':'Approved','notes':'Security regression'})
sample_worker.action(flow,'advance')
for username,label,scope,lines,units in [('qa.flow.cut','Cutting Manager','company',None,None),('qa.flow.sew','Production Manager','line',[line_b],None),('qa.flow.finish','Finishing Manager','company',None,['Suits']),('qa.flow.quality','Finishing QA','company',None,None),('qa.flow.accounts','Accounts Executive','company',None,None),('qa.flow.warehouse','Warehouse Manager','company',None,None)]:
    ops_user(username,label,scope,lines=lines,units=units)
flow_users={key:Client('qa.flow.'+key) for key in ['cut','sew','finish','quality','accounts','warehouse']}
for stage,key in [('Cutting','cut'),('Sewing','sew'),('Finishing','finish'),('Packing','finish')]:
    worker=flow_users[key]
    check(worker.order(flow)['stage']==stage,'RLS workflow reaches '+stage)
    worker.action(flow,'advance',400)
    worker.action(flow,'output',good=10,rework=0)
    flow_users['quality'].action(flow,'quality',inspected=10,defects=0,result='Pass')
    worker.action(flow,'advance')
flow_users['accounts'].action(flow,'advance',invoice_number='QA-SECURITY-INVOICE')
flow_users['warehouse'].action(flow,'advance',dispatch_reference='QA-SECURITY-DISPATCH')
check(flow_users['warehouse'].order(flow)['stage']=='Dispatched','Full secured order flow reaches dispatch')
flow_users['warehouse'].action(flow,'advance',400)
# Warehouse/store visibility does not reveal purchase prices.
ops_user('qa.flow.stores','Fabric Store Manager');stores_worker=Client('qa.flow.stores')
ops_user('qa.flow.sourcing','Fabric Sourcing Manager');sourcing_worker=Client('qa.flow.sourcing')
supplier=sourcing_worker.call('/records/suppliers',{'company_id':opscompany,'name':'QA secure supplier','category':'Fabrics','designation':'Direct','contact':''})['id']
item=stores_worker.call('/records/inventory',{'company_id':opscompany,'sku':'QA-SECURE-FABRIC','name':'QA fabric','store':'Fabric','unit':'metres','reorder_level':'0'})['id']
purchase=sourcing_worker.call('/records/purchases',{'company_id':opscompany,'name':'QA material purchase','reference':'QA-SECURE-PURCHASE','supplier_id':supplier,'item_id':item,'quantity':'10','unit_cost':'42.50','due_date':'2027-01-01'})['id']
check('unit_cost_minor' not in next(p for p in stores_worker.call('/data')['purchases'] if p['id']==purchase),'Stores cannot read purchase costs')
check(next(p for p in sourcing_worker.call('/data')['purchases'] if p['id']==purchase)['unit_cost_minor']==4250,'Authorized sourcing account retains purchase costs')
