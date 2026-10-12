# Real HTTP/RLS tests, executed only inside company_setup.py's disposable database.
enq_team=admin.call('/access-resources',{'kind':'teams','company_id':opscompany,'name':'QA Enquiry Team'})['id']
enq_buyer=admin.call('/records/buyers',{'company_id':opscompany,'team_id':enq_team,'name':'QA Enquiry Buyer'})['id']
ops_user('qa.enquiry.merchant','Junior Merchant','team',teams=[enq_team]);em=Client('qa.enquiry.merchant')
ops_user('qa.enquiry.reader','Junior Merchant','team',teams=[enq_team],inherit=False,permissions=['view_orders','enquiries','enquiry_costing','documents']);er=Client('qa.enquiry.reader')
check(em.call('/enquiries')['can_create'],'Merchant can create enquiry in assigned team')
check(not er.call('/enquiries')['can_create'] and not er.call('/enquiries')['can_edit_cost'],'Read access does not grant costing mutations')
er.call('/enquiries',{},403);line_user.call('/enquiries',expect=403)
for kind,label in [('brand','QA Enquiry Brand'),('season','Monsoon'),('currency','jpy')]:em.call('/enquiry-options',{'company_id':opscompany,'kind':kind,'label':label})
em.call('/enquiry-options',{'company_id':opscompany,'kind':'currency','label':'JPY'},409)
em.call('/enquiry-options',{'company_id':opscompany,'kind':'currency','label':'ZZ12'},400)
ep={'enquiry_type':'Booking enquiry','sale_type':'Export','brand':'QA Enquiry Brand','product':'Suits','buyer_style':'QA-BOOKING-STYLE','consumption':'1.35','garment_currency':'USD','incoterm':'FOB','garment_delivery_date':'2027-01-15','final_price':'6','et_reference':'LEGACY-ET-42','description':'Navy wool fabric'}
enqbody={'company_id':opscompany,'buyer_id':enq_buyer,'material_id':fabric,'profile':ep,'colours':[{'colour':'Navy','buyer_colour_code':'001','quantity':2000},{'colour':'Black','buyer_colour_code':'002','quantity':2000}]}
merch_b.call('/enquiries',enqbody,404)
for colours in [[],[{'colour':'Navy','quantity':-1}],[{'colour':'Navy','quantity':1.5}],[{'colour':'Navy','quantity':1},{'colour':'Navy','quantity':1}]]:em.call('/enquiries',{**enqbody,'colours':colours},400)
eqid=em.call('/enquiries',enqbody)['id'];eqpath='/enquiries/'+str(eqid)
def eqnow(c=em):return c.call(eqpath)['record']
def eqact(c,action,expect=200,**kw):return c.call(eqpath,{'action':action,'version':eqnow(c)['version'],**kw},expect)
check(eqnow()['status']=='Draft','Enquiry starts in Draft')
merch_a.call(eqpath,expect=404);merch_b.call(eqpath,expect=404)
sourcing_worker.call(eqpath,expect=404)
check(not any(r['id']==eqid for r in merch_b.call('/enquiries')['records']),'Another buyer team cannot list enquiry')
check(not any(r['id']==eqid for r in sourcing_worker.call('/enquiries')['records']),'Sourcing sees only raised enquiries')
# Permission reduction and action validation.
er.call(eqpath,{'action':'costing','version':eqnow()['version'],'costing':{'currency':'USD'}},404)
eqact(em,'book',400);eqact(em,'approve',403);eqact(em,'submit',400)
eqact(em,'request_fabric')
er.call(eqpath,{'action':'costing','version':eqnow()['version'],'costing':{'currency':'USD'}},403)
check(eqnow(sourcing_worker)['status']=='Fabric requested','Request enters sourcing response queue')
fquote={'price':'1.75','unit':'MTR','currency':'USD','freight_metre':'.25','advance_license_metre':'.2','clearance_mode':'Advance licence','purchase_type':'Import','incoterm':'CNF','moq_colour':100,'moq_order':500,'ex_mill_days':70,'transit_days':40,'order_confirmed_date':'2026-10-01','supplier_reference':'SECRET-MILL-123','payment_terms':'SECRET-PAYMENT','remarks':'SECRET-MILL-NOTE'}
eqact(em,'fabric',403,fabric=fquote)
eqact(sourcing_worker,'fabric',400,fabric={**fquote,'duty_metre':'1'})
eqact(sourcing_worker,'fabric',400,fabric={**fquote,'sgst_percent':101})
eqact(sourcing_worker,'fabric',fabric=fquote)
check('supplier_reference' not in eqnow()['fabric'] and eqnow()['fabric']['price']=='1.75','Merchant gets quote price but no supplier identity')
check(eqnow(sourcing_worker)['fabric']['supplier_reference']=='SECRET-MILL-123','Sourcing retains its supplier details')
quoted_file=sourcing_worker.call(eqpath+'/files?category=Supplier%20quotation',multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})['id']
check(all(f['id']!=quoted_file for f in em.call(eqpath)['files']),'Supplier attachment metadata hidden from merchant')
em.call('/enquiry-files/'+str(quoted_file),expect=403)
check(sourcing_worker.call('/enquiry-files/'+str(quoted_file))==payload,'Sourcing decrypts its quotation')
check(payload not in bytes.fromhex(sql(f"SELECT encode(ciphertext,'hex') FROM enquiry_files WHERE id={quoted_file}",name)),'Enquiry document is encrypted at rest')
buyer_file=em.call(eqpath+'/files?category=Buyer%20email',b'--bad',400,{'Content-Type':'multipart/form-data; boundary=bad'})
buyer_file=em.call(eqpath+'/files?category=Specification',multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})['id']
merch_b.call('/enquiry-files/'+str(buyer_file),expect=404)
co={'currency':'USD','trims':'2','rejection_percent':'2','finance_percent':'3','overhead_percent':'5','margin_percent':'8','commission_percent':'5','components':[]}
eqact(em,'costing',400,costing={**co,'components':{'price':3}})
eqact(em,'costing',400,costing={**co,'currency':'INR'})
eqact(em,'costing',costing=co)
calc=eqnow()['calculation'];check(calc['cost_per_piece']=='6.1578' and calc['total_cost']=='24631.32','Full workflow reproduces original costing workbook')
check('costing' not in eqnow(sourcing_worker),'Fabric response permission does not expose garment costing')
export=em.call(eqpath+'/export');check(export[:2]==b'PK','Enquiry exports a real workbook')
with zipfile.ZipFile(io.BytesIO(export)) as z:check('SECRET-MILL' not in ''.join(z.read(n).decode(errors='replace') for n in z.namelist()),'Merchant workbook excludes supplier identity')
for h in em.call(eqpath)['history']:
    snapshot=em.call('/enquiry-history/'+str(h['id']))['snapshot']
    check('SECRET-MILL' not in json.dumps(snapshot),'Historical snapshots enforce supplier redaction')
oldver=eqnow()['version'];eqact(em,'submit')
em.call(eqpath,{'action':'submit','version':oldver},409)
em.call(eqpath+'/files?category=Specification',multipart,400,{'Content-Type':'multipart/form-data; boundary='+boundary})
eqact(em,'costing',400,costing=co)
eqact(approver_ops,'review_edit',400,profile={'final_price':'6.1'},note='')
eqact(approver_ops,'review_edit',400,profile={'consumption':'0'},note='Invalid zero consumption')
eqact(approver_ops,'review_edit',profile={'final_price':'6.1','garment_delivery_date':'2027-01-20'},note='Delivery and agreed price confirmed')
check(eqnow()['profile']['final_price']=='6.1','MD correction persists with explanation')
eqact(approver_ops,'return',note='Check buyer agreement')
eqact(em,'submit');eqact(approver_ops,'approve')
eqact(em,'book')
book=eqnow();check(book['booking_number']>0,'Approved enquiry generates permanent booking number')
em.call(eqpath,{'action':'book','version':book['version']},400)
em.call(eqpath+'/orders',{'version':book['version'],'po':'QA-BOOK-TOO-MUCH','quantity':4001,'product':'Suits','due_date':'2027-01-20'},400)
child1=em.call(eqpath+'/orders',{'version':book['version'],'po':'QA-BOOK-1','quantity':1000,'product':'Suits','due_date':'2027-01-20','unit_price':'.01','buyer_id':buyer_b})['id']
em.call(eqpath+'/orders',{'version':book['version'],'po':'QA-BOOK-STALE','quantity':3000,'product':'Suits','due_date':'2027-01-20'},409)
child2=em.call(eqpath+'/orders',{'version':eqnow()['version'],'po':'QA-BOOK-2','quantity':3000,'product':'Suits','due_date':'2027-01-20'})['id']
em.call(eqpath+'/orders',{'version':eqnow()['version'],'po':'QA-BOOK-OVER','quantity':1,'product':'Suits','due_date':'2027-01-20'},400)
check(em.order(child1)['unit_price_minor']==610 and em.order(child1)['buyer_id']==enq_buyer,'Child PO uses server approved price and buyer')
check(sql(f"SELECT target_consumption FROM orders WHERE id={child1}",name)=='1.282500','PO stores exact 5 percent lower target')
check(em.order(child1)['booking_terms']['sale_type']=='Export','Export classification is preserved on production PO')
check(em.order(child1)['approval']=='Pending','Booking child enters existing PO approval flow')
eqact(approver_ops,'amend',400,note='Not allowed after child POs')
check(len(em.call(eqpath)['orders'])==2,'One booking links multiple production POs')
proof=sourcing_worker.call(eqpath+'/files?category=Purchase%20proof&order_id='+str(child1),multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})['id']
sourcing_worker.call(eqpath+'/files?category=Purchase%20proof&order_id='+str(po_a),multipart,400,{'Content-Type':'multipart/form-data; boundary='+boundary})
em.call('/enquiry-files/'+str(proof),expect=403)
check(any(f['id']==proof and f['order_id']==child1 for f in sourcing_worker.call(eqpath)['files']),'Purchase proof stays linked to a child PO')
# Row-level security remains effective even without HTTP scope clauses.
claims={'id':merch_a_id,'role':'Marketing','scope':'team','companies':[opscompany],'teams':[team_a],'enquiries':True}
encoded=json.dumps(claims).replace("'","''")
result=sql(f"BEGIN; SET LOCAL ROLE euroerp_runtime; SELECT set_config('erp.identity','{encoded}',true); SELECT count(*) FROM enquiries WHERE id={eqid}; COMMIT;",name)
check(result.splitlines()[-2]=='0','Database policy blocks cross-team enquiry access')
# Keep the role preview available before later tests deactivate the main company.
if os.getenv('ENQUIRY_TEST_PREVIEW'):
    (ROOT/'../../work/enquiry-20261006/preview.json').resolve().write_text(json.dumps({'base':base,'database':name,'enquiry_id':eqid,'merchant':'qa.enquiry.merchant','md':'qa.approver','sourcing':'qa.flow.sourcing'}))
    print('Enquiry QA preview ready at '+base,flush=True)
    while not (ROOT/'../../work/enquiry-20261006/resume-tests').resolve().exists():time.sleep(1)
