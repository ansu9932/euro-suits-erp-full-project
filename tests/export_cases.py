# Real HTTP checks, only executed within company_setup.py's disposable database.
import copy
export_start_checks=checks
export_grants=['export_view','export_edit','export_files','export_download']
def export_user(username,role='Finance',permissions=None,scope='company',**assignments):
    grants=permissions if permissions is not None else export_grants
    designation_id=admin.call('/access-designations',{'name':'Export '+username,'department':'Export QA','role':role,'features':[],'permissions':grants})['id']
    body={'username':username,'name':username.replace('qa.export.','').replace('.',' ').title(),'password':password,'designation_id':designation_id,'inherit_designation':True,'scope':scope,'companies':[opscompany],'active':True,**assignments}
    identifier=admin.call('/setup-users',body)['id']
    return identifier,Client(username)

ex_writer_id,ex_writer=export_user('qa.export.writer')
ex_finance_id,ex_finance=export_user('qa.export.head','Finance Head',export_grants+['export_finance_check','export_discount_first'])
ex_final_id,ex_final=export_user('qa.export.md','MD',['export_view','export_download','export_discount_final'])
ex_reader_id,ex_reader=export_user('qa.export.reader',permissions=['export_view'])
ex_auditor_id,ex_auditor=export_user('qa.export.auditor',permissions=['export_view','export_audit_check'])
ex_team_a_id,ex_team_a=export_user('qa.export.team.a',scope='team',teams=[team_a])
ex_team_b_id,ex_team_b=export_user('qa.export.team.b',scope='team',teams=[team_b])
ex_line_id,ex_line=export_user('qa.export.line',scope='line',lines=[line_a])
ex_buyer_id,ex_buyer=export_user('qa.export.buyer',restrict_buyers=True,buyer_ids=[buyer_b])

ex_options=ex_finance.call('/exports/options?company_id='+str(opscompany))
check(any(c['id']==opscompany for c in ex_options['companies']),'Standalone Finance Head sees assigned export company')
check('view_orders' not in ex_finance.call('/session')['user']['grants'],'Export permissions do not imply order visibility')
ex_finance.call('/data',expect=403)
Client().call('/exports',expect=401)
merch_b.call('/exports/options',expect=403)
check(not ex_writer.call('/exports/options?company_id='+str(opscompany))['permissions']['audit_check'],'Audit review is not enabled by Finance default')
check(ex_auditor.call('/exports/options?company_id='+str(opscompany))['permissions']['audit_check'],'Explicit Finance audit grant is effective')

ex_body={'company_id':opscompany,'party_type':'buyer','party_id':buyer_a,'order_id':po_a,'invoice_number':'EX/2026-27/0001','currency':'USD','invoice_amount':'1000.00','profile':{'export_reference':'2026-27/0001','export_date':'2026-10-08','shipping_bill_number':'SB-0001','shipping_bill_date':'2026-10-08','shipping_mode':'Sea','transport_document_number':'BL-001','advance_license_number':'ADV-001','payment_details':'One invoice, multiple receipts','adjustment_note':'Keep the original shortfall visible','drawback_claim_amount':'125.50','drawback_claim_currency':'INR','rosctl_amount':'22.25','rosctl_currency':'INR','eway_bill_number':'EW-001','customs_house_agent':'QA CHA','freight_forwarder':'QA Freight','clearing_invoice':'CLEAR-001','port_code':'INMAA1','linked_bank':'QA Bank','scrip_reference':'SCR-001','scrip_sale_percentage':'2.50','scrip_sale_amount':'25.00','scrip_sale_currency':'INR','scrip_bank_credit':'CR-001','scrip_bank_credit_amount':'25.00','scrip_bank_credit_currency':'INR','scrip_tally_reference':'TALLY-SCRIP','statuses':{'drawback_claim_amount':'NIL pending verification'}},'receipts':[{'reference':'R-1','receipt_date':'2026-10-08','currency':'USD','payment_received':'400.00','gross_payment_received':'405.00','net_payment_received':'400.00','foreign_bank_charges':'5.00','foreign_bank_charges_currency':'USD','bank_charges':'5.00','bank_charges_currency':'USD','receipt_bank':'Receiving bank','receipt_advice':'ADVICE-1','receipt_tally_reference':'TALLY-R1','converted_amount':'33000.00','converted_currency':'INR','exchange_rate':'82.50','conversion_amount':'32950.00','conversion_currency':'INR','credit_bank_017':'017-REF','credit_bank_017_amount':'32950.00','credit_bank_017_currency':'INR','packing_credit':'PC-REF','packing_credit_amount':'200.00','packing_credit_currency':'INR','bank_advice_reference':'BANK-ADVICE-1','bank_advice_date':'2026-10-08','receiving_bank':'000017','note':'Synthetic receipt','statuses':{'credit_bank_017':'Manual allocation'}}],'ebrcs':[{'export_reference':'EBRC-REF','ebrc_amount':'400.00','currency':'USD','shipping_bill_number':'SB-0001','shipping_bill_date':'2026-10-08','ebrc_file_number':'FILE-1','ebrc_number':'EBRC-1','ebrc_balance_amount':'600.00','ebrc_note':'PRINT PENDING','statuses':{'ebrc_note':'Pending'}}]}
ex_id=ex_writer.call('/exports',ex_body)['id'];ex_path='/exports/'+str(ex_id)
def ex_detail(actor=ex_writer):return actor.call(ex_path)
def ex_record(actor=ex_writer):return ex_detail(actor)['record']
def ex_update(body=None,actor=ex_writer,expect=200,version=None):
    current=ex_record(actor)
    return actor.call(ex_path,{**(body or ex_body),'version':current['version'] if version is None else version},expect)
def ex_action(actor,suffix,expect=200,**body):
    return actor.call(ex_path+suffix,{'version':ex_record(actor)['version'],**body},expect)
check(ex_record()['balance']['outstanding']=='590.00','Receipts and both distinct charges settle once; gross/net do not double count')
check(ex_record()['receipts'][0]['receiving_bank']=='000017','Bank account-like references preserve leading zeroes')
check(ex_record()['profile']['scrip_sale_amount']=='25.00','Scrip fields persist independently from settlement')
check(ex_record()['ebrcs'][0]['ebrc_note']=='PRINT PENDING','eBRC notes stay separate from numeric amounts')
ex_writer.call('/exports',ex_body,409)
ex_writer.call('/exports',{**ex_body,'invoice_number':' ex/2026-27/0001 '},409)
ex_reader.call('/exports',ex_body,403)
ex_reader.call(ex_path,{**ex_body,'version':ex_record()['version']},403)
ex_team_a.call(ex_path);ex_line.call(ex_path)
for actor in [ex_team_b,ex_buyer]:
    actor.call(ex_path,expect=404)
    check(all(r['id']!=ex_id for r in actor.call('/exports?company_id='+str(opscompany))['records']),'Restricted export register hides other buyers')
    actor.call('/exports',{**ex_body,'invoice_number':'DENIED'},404)
ex_writer.call(ex_path,{**ex_body,'version':ex_record()['version']},403,{'X-CSRF-Token':''})
for invalid in ['-1.00','NaN','1e3','1.001','FOC']:
    ex_writer.call('/exports',{**ex_body,'invoice_number':'INVALID-'+invalid,'invoice_amount':invalid},400)
ex_writer.call('/exports',{**ex_body,'invoice_number':'INVALID-NUMBER','invoice_amount':1000},400)
for changes in [{'unknown_field':'secret'},{'export_date':'2026-02-30'},{'shipping_mode':'Teleport'},{'scrip_sale_percentage':'101'},{'drawback_claim_amount':'10.00','drawback_claim_currency':''}]:
    ex_writer.call('/exports',{**ex_body,'invoice_number':'INVALID-PROFILE','profile':{**ex_body['profile'],**changes}},400)
ex_writer.call('/exports',{**ex_body,'invoice_number':'INVALID-FX','receipts':[{'currency':'INR','payment_received':'100.00'}]},400)
ex_writer.call('/exports',{**ex_body,'invoice_number':'INVALID-CHARGE-FX','receipts':[{'foreign_bank_charges':'100.00','foreign_bank_charges_currency':'INR'}]},400)

# All writes require a current version. One of concurrent edits wins.
ex_old_version=ex_record()['version'];ex_update()
ex_writer.call(ex_path,{**ex_body,'version':ex_old_version},409)
ex_edit_version=ex_record()['version']
def ex_race(_):
    client=Client('qa.export.writer');req=urllib.request.Request(base+'/api'+ex_path,data=json.dumps({**ex_body,'version':ex_edit_version}).encode(),headers={'Origin':os.getenv('COMPANY_TEST_ORIGIN',base),'X-CSRF-Token':client.csrf,'Content-Type':'application/json'})
    try:return client.opener.open(req).status
    except urllib.error.HTTPError as e:return e.code
with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:ex_race_codes=sorted(pool.map(ex_race,[1,2]))
check(ex_race_codes==[200,409],'Concurrent export edits cannot overwrite each other')

# Cross-currency settlement must use separately supplied invoice equivalents.
ex_body['receipts'].append({'reference':'R-2','currency':'INR','payment_received':'48000.00','invoice_currency_amount':'580.00','bank_charges':'100.00','bank_charges_currency':'INR','bank_charges_invoice_amount':'1.00'})
ex_update();check(ex_record()['balance']['outstanding']=='9.00','Explicit invoice currency equivalents settle cross-currency receipts and charges')

# Encrypted documents use the same scoped parent and version discipline.
ex_upload=ex_writer.call(ex_path+'/files?kind=invoice&version='+str(ex_record()['version']),multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})
ex_file_id=ex_upload['id'];ex_file_path='/export-files/'+str(ex_file_id)
check(ex_writer.call(ex_file_path)==payload,'Export file decrypts to original bytes')
check(payload not in bytes.fromhex(sql(f"SELECT encode(ciphertext,'hex') FROM export_files WHERE id={ex_file_id}",name)),'Export attachment is encrypted in PostgreSQL')
for actor in [ex_team_b,ex_buyer]:actor.call(ex_file_path,expect=404)
ex_reader.call(ex_file_path,expect=403)
ex_reader.call(ex_path+'/files?kind=invoice&version='+str(ex_record()['version']),multipart,403,{'Content-Type':'multipart/form-data; boundary='+boundary})
ex_writer.call(ex_path+'/files?kind=unknown&version='+str(ex_record()['version']),multipart,400,{'Content-Type':'multipart/form-data; boundary='+boundary})
ex_writer.call(ex_path+'/files?kind=invoice&version=1',multipart,409,{'Content-Type':'multipart/form-data; boundary='+boundary})

ex_action(ex_writer,'/check',403,kind='finance',status='Checked',note='No permission')
ex_action(ex_finance,'/check',kind='finance',status='Checked',note='Checked receipt and invoice')
ex_action(ex_auditor,'/check',kind='audit',status='Checked',note='Independent audit')
check(ex_record()['finance_check']['status']=='Checked' and ex_record()['audit_check']['status']=='Checked','Finance and explicit audit checks record separately')
ex_update();check(ex_record()['finance_check'].get('status')!='Checked' and ex_record()['audit_check'].get('status')!='Checked','Editing export facts resets prior review checks')

# A changed invoice invalidates a pending commercial decision.
ex_action(ex_writer,'/discount',action='request',amount='8.00',reason='First proposal before correction')
ex_pending=ex_detail()['discounts'][0]['id']
ex_action(ex_finance,'/discount',action='first_approve',discount_id=ex_pending,reason='Initial review')
ex_update()
check(next(d for d in ex_detail()['discounts'] if d['id']==ex_pending)['status']=='Rejected','Editing invoice invalidates its pending discount approval')
ex_action(ex_final,'/discount',400,action='final_approve',discount_id=ex_pending,reason='Stale proposal cannot settle')

# The balance stays open during both review stages; final approval alone applies discount.
ex_action(ex_writer,'/discount',400,action='request',amount='10.00',reason='More than outstanding')
ex_action(ex_writer,'/discount',400,action='request',amount='9.00',reason='')
ex_action(ex_writer,'/discount',action='request',amount='9.00',reason='Agreed small collection shortfall')
ex_discount=next(d['id'] for d in ex_detail()['discounts'] if d['status']=='Pending finance')
check(ex_record()['balance']['outstanding']=='9.00','Discount request does not settle outstanding')
ex_action(ex_final,'/discount',400,action='final_approve',discount_id=ex_discount,reason='Cannot skip Finance Head')
ex_action(admin,'/discount',403,action='first_approve',discount_id=ex_discount,reason='Super Admin is not first approver')
ex_action(ex_finance,'/discount',action='first_approve',discount_id=ex_discount,reason='Finance Head checked the shortfall')
check(ex_record()['balance']['outstanding']=='9.00','Finance approval alone does not settle outstanding')
ex_action(ex_finance,'/discount',403,action='final_approve',discount_id=ex_discount,reason='Cannot approve both stages')
ex_action(ex_final,'/discount',action='final_approve',discount_id=ex_discount,reason='Approved commercial discount')
check(ex_record()['balance']['outstanding']=='0.00' and ex_record()['balance']['approved_discount']=='9.00','Final approval applies the discount exactly once')
ex_action(ex_final,'/discount',400,action='final_approve',discount_id=ex_discount,reason='Cannot apply twice')

# Preserve a credit instead of silently clamping an overpayment to zero.
ex_credit=ex_writer.call('/exports',{**ex_body,'invoice_number':'EX-CREDIT-001','invoice_amount':'100.00','receipts':[{'currency':'USD','payment_received':'105.00'}],'ebrcs':[]})['id']
check(ex_writer.call('/exports/'+str(ex_credit))['record']['balance']['outstanding']=='-5.00','Overpayment remains a visible negative outstanding credit')
# Idempotency protects retry after a response interruption.
ex_idem_body={**ex_body,'invoice_number':'EX-IDEMPOTENT-001','receipts':[],'ebrcs':[]}
ex_idem_key=secrets.token_hex(16)
ex_idem_first=ex_writer.call('/exports',ex_idem_body,headers={'Idempotency-Key':ex_idem_key})
ex_idem_second=ex_writer.call('/exports',ex_idem_body,409,headers={'Idempotency-Key':ex_idem_key})
check(ex_idem_second['code']=='action_already_submitted' and sql("SELECT count(*) FROM export_invoices WHERE invoice_number='EX-IDEMPOTENT-001'",name)=='1','Retry with same action key creates only one export invoice')

# Read-only workbooks and currency-separated aggregates do not expose hidden invoices.
ex_gbp=ex_writer.call('/exports',{**ex_body,'invoice_number':'EX-GBP-001','currency':'GBP','invoice_amount':'100.00','receipts':[],'ebrcs':[]})['id']
ex_totals=ex_writer.call('/exports?company_id='+str(opscompany))['totals']
check({'USD','GBP'}<=set(t['currency'] for t in ex_totals),'Register totals remain separated by currency')
for endpoint in [ex_path+'/export','/exports/export?company_id='+str(opscompany),'/exports/statement?company_id='+str(opscompany)+'&party_type=buyer&party_id='+str(buyer_a)]:
    document=ex_writer.call(endpoint)
    check(document[:2]==b'PK','Export documentation downloads a real Excel workbook')
    with zipfile.ZipFile(io.BytesIO(document)) as archive:
        check(any(n.startswith('xl/worksheets/') for n in archive.namelist()),'Export workbook contains worksheets')
    ex_reader.call(endpoint,expect=403)
for history_item in ex_detail()['history']:
    ex_writer.call(ex_path+'/history/'+str(history_item['id']))
    ex_team_b.call(ex_path+'/history/'+str(history_item['id']),expect=404)

check(sql("SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE 'dashboard_target%'",name)=='0','Paused dashboard lock tables are absent')
exec((ROOT/'tests/export_security_cases.py').read_text(),globals(),globals())
exec((ROOT/'tests/export_workflow_cases.py').read_text(),globals(),globals())
exec((ROOT/'tests/export_party_option_cases.py').read_text(),globals(),globals())
exec((ROOT/'tests/export_document_cases.py').read_text(),globals(),globals())
exec((ROOT/'tests/export_jv_cases.py').read_text(),globals(),globals())
exec((ROOT/'tests/export_banking_cases.py').read_text(),globals(),globals())
exec((ROOT/'tests/export_reconciliation_cases.py').read_text(),globals(),globals())
print('Export HTTP checks passed: '+str(checks-export_start_checks),flush=True)
if os.getenv('EXPORT_TEST_PREVIEW'):
    preview=Path(os.environ['EXPORT_TEST_PREVIEW']);preview.mkdir(parents=True,exist_ok=True)
    (preview/'preview.json').write_text(json.dumps({'base':base,'database':name,'invoice_id':ex_id,'company_id':opscompany,'writer':'qa.export.writer','finance_head':'qa.export.head','md':'qa.export.md','reader':'qa.export.reader'}))
    print('Export QA preview ready at '+base,flush=True)
    while not (preview/'resume-tests').exists():time.sleep(1)
