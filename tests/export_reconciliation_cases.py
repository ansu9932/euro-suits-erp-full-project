# Real HTTP/RLS checks run only inside company_setup.py's disposable database.
# Documentary clearance does not post a receipt, write-off or approval.
import uuid
assert name.startswith('erp_test_') and runtime_role == name + '_api'
check(not env.get('SMTP_HOST'), 'Reconciliation tests cannot send bank or employee email')
exr_start = checks
exr_writer_id, exr_writer = export_user('qa.export.reconciliation.writer', companies=[])
exr_files_id, exr_files = export_user('qa.export.reconciliation.files', companies=[])
exr_md_id, exr_md = export_user('qa.export.reconciliation.md', 'MD', ['export_view', 'export_download', 'export_approve'], companies=[])
exr_sa_id, exr_sa = export_user('qa.export.reconciliation.sa', 'Super Admin', export_grants + ['export_approve'], companies=[])
exr_reader_id, exr_reader = export_user('qa.export.reconciliation.reader', permissions=['export_view'], companies=[])
exr_keys = [str(uuid.uuid4()), str(uuid.uuid4())]
exr_body = {
    **copy.deepcopy(exw_body), 'invoice_number': 'EX-RECONCILIATION-001',
    'invoice_amount': '1000.00',
    'profile': {'export_date': '2026-10-10', 'invoice_date': '2026-10-08',
                'shipping_bill_date': '2026-10-09', 'shipping_bill_number': 'SB-RECON-001',
                'transport_document_number': 'AWB-001', 'transport_document_date': '2026-10-09',
                'port_of_loading': 'Bengaluru', 'port_of_departure': 'Mumbai', 'port_code': 'INBLR4'},
    'receipts': [
        {'receipt_key': exr_keys[0], 'inward_reference': 'OUR-001', 'inward_due_date': '2026-10-20',
         'gross_payment_received': '125.50', 'currency': 'USD', 'payment_received': '100.00',
         'disposal_credit_account': '000017', 'disposal_debit_account': '000344',
         'credit_advice_bill_number': 'BOB-BILL-001', 'reference': 'FOREIGN-001',
         'bank_advice_amount': '120.50', 'inr_conversion_rate': '82.123456',
         'inr_received_amount': '9895.00', 'inr_bank_charges': '18.00', 'inr_currency': 'INR',
         'credit_advice_credit_account': '000017', 'credit_advice_debit_account': '000344',
         'receipt_note': 'Preserve legacy receipt notes'},
        {'receipt_key': exr_keys[1], 'inward_reference': 'OUR-002',
         'gross_payment_received': '40.00', 'currency': 'GBP', 'note': 'Second remittance'},
    ],
    'ebrcs': [{'ebrc_number': 'EBRC-RECON-001', 'status': 'Received', 'currency': 'USD',
               'ebrc_amount': '100.00', 'ebrc_note': 'Document evidence only'}],
}


def exr_new(label, body=None, actor=exr_writer):
    values = copy.deepcopy(exr_body if body is None else body)
    values['invoice_number'] = 'EX-RECON-' + label
    rid = actor.call('/exports', values)['id']
    return '/exports/' + str(rid), rid


def exr_detail(path, actor=exr_writer):
    return actor.call(path)


def exr_record(path):
    return exr_detail(path)['record']


def exr_version(path):
    return exr_record(path)['version']


def exr_current_body(path):
    record = exr_record(path)
    return {key: record[key] for key in ('record_kind', 'company_id', 'party_type', 'party_id',
             'order_id', 'party_name', 'invoice_number', 'currency', 'invoice_amount',
             'profile', 'receipts', 'ebrcs') if key in record}


def exr_save(path, values=None, expect=200, version=None, actor=exr_writer):
    return actor.call(path, {**(exr_current_body(path) if values is None else values),
        'version': exr_version(path) if version is None else version}, expect)


def exr_upload(path, kind='ebrc', *, receipt_key=None, field_key=None, expect=200,
               version=None, actor=exr_files, request_key=None, extra_query=None):
    values = {'kind': kind, 'version': exr_version(path) if version is None else version}
    if receipt_key is not None: values['receipt_key'] = receipt_key
    if field_key is not None: values['field_key'] = field_key
    if extra_query: values.update(extra_query)
    data, headers = exd_multipart('reconciliation-proof.pdf', payload)
    if request_key: headers['Idempotency-Key'] = request_key
    return actor.call(path + '/files?' + urllib.parse.urlencode(values), data, expect, headers)


def exr_clear(path, file_id, *, number='EBRC-RECON-001', actor=exr_writer,
              expect=200, version=None, extra=None):
    body = {'version': exr_version(path) if version is None else version,
            'ebrc_number': number, 'file_id': file_id, **(extra or {})}
    return actor.call(path + '/reconcile', body, expect)


def exr_clearance(record):
    return record.get('documentation_cleared') or {}


def exr_financial_snapshot(record):
    return {key: record.get(key) for key in ('invoice_amount', 'currency', 'receipts', 'ebrcs',
             'balance', 'workflow_status', 'submitted_revision', 'approved_revision',
             'approved_by', 'submitted_by', 'review_note', 'content_revision')}


exr_path, exr_id = exr_new('MAIN')
exr_initial = exr_record(exr_path)
check(not exr_clearance(exr_initial), 'New export starts without documentation clearance')
check(exr_initial['balance']['outstanding'] == '900.00',
      'Documentary bank advice and INR charges do not invent settlement receipts')
for exr_key, exr_value in exr_body['profile'].items():
    check(exr_initial['profile'][exr_key] == exr_value, 'Reconciliation profile field round-trips: ' + exr_key)
for exr_key, exr_value in exr_body['receipts'][0].items():
    check(exr_initial['receipts'][0][exr_key] == exr_value, 'Remittance field round-trips: ' + exr_key)
check(exr_initial['profile']['invoice_date'] != exr_initial['profile']['export_date'],
      'Invoice date remains distinct from the original export date')
check(exr_initial['receipts'][0]['inward_reference'] != exr_initial['receipts'][0]['reference'],
      'Our inward reference is independent from the foreign bank reference')

# Strict input validation and canonical stable receipt identities.
exr_parent_hash = sql(f'SELECT md5(to_jsonb(e)::text) FROM export_invoices e WHERE id={exr_id}', name)
for exr_patch in ({'receipt_key':'not-a-uuid'}, {'inward_due_date':'2026-02-30'},
                  {'disposal_amount':'125.50'}, {'disposal_currency':'USD'},
                  {'inward_reference':'A\nB'}, {'credit_advice_bill_number':'A'*161}):
    exr_bad = copy.deepcopy(exr_body)
    exr_bad['invoice_number'] = exr_initial['invoice_number']
    exr_bad['receipts'][0].update(exr_patch)
    exr_save(exr_path, exr_bad, 400)
exr_duplicate = exr_current_body(exr_path)
exr_duplicate['receipts'][1]['receipt_key'] = exr_duplicate['receipts'][0]['receipt_key']
exr_save(exr_path, exr_duplicate, 400)
for exr_profile_patch in ({'invoice_date':'2026-02-30'}, {'transport_document_date':'10/10/2026'},
                          {'port_of_loading':'A'*2001}):
    exr_bad = exr_current_body(exr_path)
    exr_bad['profile'].update(exr_profile_patch)
    exr_save(exr_path, exr_bad, 400)
check(sql(f'SELECT md5(to_jsonb(e)::text) FROM export_invoices e WHERE id={exr_id}', name) == exr_parent_hash,
      'Invalid receipt identities and documentary fields leave the stored invoice unchanged')

# eBRC evidence is mandatory, parent-bound, active and cannot be fabricated in an action.
exr_clear(exr_path, 0, expect=400)
exr_no_file = exr_version(exr_path)
exr_clear(exr_path, 99999999, expect=400)
exr_wrong_kind = exr_upload(exr_path, 'invoice')['id']
exr_clear(exr_path, exr_wrong_kind, expect=400)
exr_other_path, exr_other_id = exr_new('OTHER')
exr_other_file = exr_upload(exr_other_path)['id']
exr_clear(exr_path, exr_other_file, expect=400)
exr_ebrc = exr_upload(exr_path)['id']
for exr_number in ('', 'UNSAVED-CERTIFICATE'):
    exr_clear(exr_path, exr_ebrc, number=exr_number, expect=400)
exr_clear(exr_path, exr_ebrc, extra={'workflow_status':'Approved'}, expect=400)
for exr_actor in (exr_reader, exr_md, exw_noview, exw_unsupported, merch_b):
    exr_clear(exr_path, exr_ebrc, actor=exr_actor, expect=403)
Client().call(exr_path + '/reconcile', {'version':exr_version(exr_path),'ebrc_number':'EBRC-RECON-001','file_id':exr_ebrc}, 401)
exr_writer.call(exr_path + '/reconcile', {'version':exr_version(exr_path),'ebrc_number':'EBRC-RECON-001','file_id':exr_ebrc}, 403, {'X-CSRF-Token':''})
exr_clear(exr_path, exr_ebrc, version=exr_no_file, expect=409)

exr_before_clear = exr_record(exr_path)
exr_cleared = exr_clear(exr_path, exr_ebrc)
exr_after_clear = exr_record(exr_path)
check(exr_clearance(exr_after_clear)['file_id'] == exr_ebrc and
      exr_clearance(exr_after_clear)['ebrc_number'] == 'EBRC-RECON-001' and
      exr_clearance(exr_after_clear)['by'] == exr_writer_id and exr_clearance(exr_after_clear)['at'] and
      exr_clearance(exr_after_clear)['content_revision'] == exr_after_clear['content_revision'],
      'Finance clears documentation with the exact saved certificate, active file, actor, timestamp and revision')
check(exr_financial_snapshot(exr_before_clear) == exr_financial_snapshot(exr_after_clear) and
      exr_after_clear['workflow_status'] == 'Draft' and exr_after_clear['balance']['outstanding'] == '900.00',
      'Documentation can clear a Draft with an unpaid balance without changing approval or financial state')
check(exr_after_clear['version'] == exr_before_clear['version'] + 1,
      'Documentation clearance advances optimistic concurrency version only')
check(any('documentation cleared' in str(row).lower() for row in exr_detail(exr_path)['history']),
      'Documentation clearance is visible in immutable invoice history')
exr_clear(exr_path, exr_ebrc, version=exr_before_clear['version'], expect=409)

# An unrelated comment is non-content history; it keeps the precise evidence.
exr_writer.call(exr_path + '/comments', {'version':exr_version(exr_path),'note':'Certificate checked; payment remains outstanding.'})
check(exr_clearance(exr_record(exr_path)) == exr_clearance(exr_after_clear),
      'An unrelated discussion comment preserves documentary clearance evidence')
exr_history_snapshot = exr_writer.call(exr_path + '/history/' + str(exr_detail(exr_path)['history'][0]['id']))['snapshot']
check(exr_history_snapshot['documentation_cleared']['file_id'] == exr_ebrc,
      'Audit snapshot retains the documentary clearance evidence')

# Content save, attachment changes and an explicit reopen invalidate clearance.
exr_save(exr_path)
check(not exr_clearance(exr_record(exr_path)), 'Saving a new invoice content revision invalidates old documentation clearance')
exr_clear(exr_path, exr_ebrc)
exr_writer.call(exr_path + '/files/' + str(exr_ebrc) + '/remove', {'version':exr_version(exr_path)})
check(not exr_clearance(exr_record(exr_path)), 'Removing the selected certificate invalidates documentation clearance')
exr_clear(exr_path, exr_ebrc, expect=400)
exr_ebrc = exr_upload(exr_path)['id']
exr_clear(exr_path, exr_ebrc)
exr_upload(exr_path, 'other')
check(not exr_clearance(exr_record(exr_path)), 'Adding an attachment invalidates clearance for the previous content revision')
exr_writer.call(exr_path + '/workflow', {'version':exr_version(exr_path),'action':'submit'})
exr_before_submitted_clear = exr_record(exr_path)
exr_clear(exr_path, exr_ebrc, actor=exr_sa)
check(exr_financial_snapshot(exr_record(exr_path)) == exr_financial_snapshot(exr_before_submitted_clear),
      'Super Admin may clear Submitted documentation without performing the independent invoice approval')
exr_md.call(exr_path + '/workflow', {'version':exr_version(exr_path),'action':'approve'})
exr_before_approved_clear = exr_record(exr_path)
exr_clear(exr_path, exr_ebrc)
check(exr_financial_snapshot(exr_record(exr_path)) == exr_financial_snapshot(exr_before_approved_clear),
      'Finance may clear Approved documentation without altering its approval or unpaid balance')
exr_reopen_content = exr_record(exr_path)['content_revision']
exr_writer.call(exr_path + '/workflow', {'version':exr_version(exr_path),'action':'reopen','note':'Recheck final certificate'})
check(not exr_clearance(exr_record(exr_path)) and exr_record(exr_path)['content_revision'] == exr_reopen_content,
      'Explicit approved-invoice reopening invalidates clearance even before content revision changes')

# Two equal-version clear requests cannot both create an audit event.
exr_race_version = exr_version(exr_path)
exr_history_before_race = int(sql(f'SELECT count(*) FROM export_history WHERE invoice_id={exr_id}', name))

def exr_race_clear(_):
    actor = Client('qa.export.reconciliation.writer')
    request = urllib.request.Request(base + '/api' + exr_path + '/reconcile',
        data=json.dumps({'version':exr_race_version,'ebrc_number':'EBRC-RECON-001','file_id':exr_ebrc}).encode(),
        headers={'Origin':os.getenv('COMPANY_TEST_ORIGIN',base),'X-CSRF-Token':actor.csrf,'Content-Type':'application/json'})
    try: response = actor.opener.open(request, timeout=15)
    except urllib.error.HTTPError as error: response = error
    response.read()
    return response.status

with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
    exr_race_codes = sorted(pool.map(exr_race_clear, (1, 2)))
check(exr_race_codes == [200,409] and
      int(sql(f'SELECT count(*) FROM export_history WHERE invoice_id={exr_id}', name)) == exr_history_before_race + 1,
      'Concurrent documentation submissions commit exactly one version and audit event')

# Existing linked-record scope restrictions apply to the new action as well.
for exr_actor in (ex_team_b, ex_buyer):
    exr_actor.call(ex_path + '/reconcile', {'version':ex_record()['version'],'ebrc_number':'EBRC-1','file_id':exr_ebrc}, 404)
exr_writer.call(exj_path + '/reconcile', {'version':exj_version(),'ebrc_number':'EBRC-RECON-001','file_id':exr_ebrc}, 400)

# Receipt attachments belong to stable UUID + field category, never array index.
exr_scope_path, exr_scope_id = exr_new('RECEIPT-SCOPE')
exr_scope_file = exr_upload(exr_scope_path, 'payment_details', receipt_key=exr_keys[0], field_key='inward_remittance')['id']
exr_disposal_file = exr_upload(exr_scope_path, 'payment_details', receipt_key=exr_keys[0], field_key='disposal_instruction')['id']
exr_credit_file = exr_upload(exr_scope_path, 'bank_advice', receipt_key=exr_keys[1], field_key='credit_advice')['id']
exr_legacy_file = exr_upload(exr_scope_path, 'payment_details')['id']
exr_scope_metadata = {row['id']:row for row in exr_detail(exr_scope_path)['files']}
check(exr_scope_metadata[exr_scope_file]['receipt_key'] == exr_keys[0] and
      exr_scope_metadata[exr_scope_file]['field_key'] == 'inward_remittance' and
      exr_scope_metadata[exr_disposal_file]['field_key'] == 'disposal_instruction' and
      exr_scope_metadata[exr_credit_file]['receipt_key'] == exr_keys[1] and
      not exr_scope_metadata[exr_legacy_file].get('receipt_key') and not exr_scope_metadata[exr_legacy_file].get('field_key'),
      'Receipt-specific files retain exact stage and receipt identity while historical unscoped files stay unscoped')
exr_reordered = exr_current_body(exr_scope_path)
exr_reordered['receipts'].reverse()
exr_save(exr_scope_path, exr_reordered)
check({row['id']:(row.get('receipt_key'),row.get('field_key')) for row in exr_detail(exr_scope_path)['files']} ==
      {key:(row.get('receipt_key'),row.get('field_key')) for key,row in exr_scope_metadata.items()},
      'Reordering remittances cannot move documents to another receipt')
exr_drop_receipt = exr_current_body(exr_scope_path)
exr_drop_receipt['receipts'] = [row for row in exr_drop_receipt['receipts'] if row.get('receipt_key') != exr_keys[0]]
exr_save(exr_scope_path, exr_drop_receipt, 400)
exr_change_key = exr_current_body(exr_scope_path)
exr_change_key['receipts'][0]['receipt_key'] = str(uuid.uuid4())
exr_save(exr_scope_path, exr_change_key, 400)

# Isolate failed upload validation on a different actor to respect real upload limits.
exr_bad_files_id, exr_bad_files = export_user('qa.export.reconciliation.bad.files', companies=[])
for exr_kind, exr_receipt_key, exr_field in (
    ('payment_details', exr_keys[0], None), ('payment_details', None, 'inward_remittance'),
    ('payment_details', 'not-a-uuid', 'inward_remittance'),
    ('payment_details', str(uuid.uuid4()), 'inward_remittance'),
    ('invoice', exr_keys[0], 'inward_remittance'),
    ('payment_details', exr_keys[0], 'credit_advice'),
    ('bank_advice', exr_keys[0], 'disposal_instruction'),
    ('payment_details', exr_keys[0], 'unknown_stage'),
):
    exr_upload(exr_scope_path, exr_kind, receipt_key=exr_receipt_key, field_key=exr_field,
               actor=exr_bad_files, expect=400)
exr_upload(exj_path, 'jv_supporting', receipt_key=exr_keys[0], field_key='inward_remittance', actor=exr_bad_files, expect=400)

# Same action key and same captured revision cannot create an extra scoped file.
exr_upload_key = secrets.token_hex(16)
exr_upload_version = exr_version(exr_scope_path)
exr_retry_file = exr_upload(exr_scope_path, 'bank_advice', receipt_key=exr_keys[0], field_key='credit_advice',
                             version=exr_upload_version, request_key=exr_upload_key)['id']
exr_upload(exr_scope_path, 'bank_advice', receipt_key=exr_keys[0], field_key='credit_advice',
            version=exr_upload_version, request_key=exr_upload_key, expect=409)
check(sum(row['id'] == exr_retry_file for row in exr_detail(exr_scope_path)['files']) == 1,
      'Retrying a scoped upload never creates another attachment')
exr_scope_history = exr_writer.call(exr_scope_path + '/history/' + str(exr_detail(exr_scope_path)['history'][0]['id']))['snapshot']
check(any(row['id'] == exr_retry_file and row['receipt_key'] == exr_keys[0] and row['field_key'] == 'credit_advice'
          for row in exr_scope_history['files']), 'Immutable history includes receipt document bindings')

# Removing all active documents allows a receipt to be removed while retaining its file audit.
for exr_fid in (exr_scope_file, exr_disposal_file, exr_retry_file):
    exr_writer.call(exr_scope_path + '/files/' + str(exr_fid) + '/remove', {'version':exr_version(exr_scope_path)})
exr_drop_receipt = exr_current_body(exr_scope_path)
exr_drop_receipt['receipts'] = [row for row in exr_drop_receipt['receipts'] if row.get('receipt_key') != exr_keys[0]]
exr_save(exr_scope_path, exr_drop_receipt)
check(len(exr_record(exr_scope_path)['receipts']) == 1 and
      sql(f'SELECT count(*) FROM export_files WHERE invoice_id={exr_scope_id}', name) == '5',
      'Receipt deletion after explicit document removal preserves all encrypted attachment records')

# Historical receipts can stay without a UUID until explicitly edited/bound.
exr_legacy_path, exr_legacy_id = exr_new('LEGACY', {**copy.deepcopy(exw_body), 'receipts':[{'currency':'USD','payment_received':'12.00'}],'ebrcs':[]})
check(not exr_record(exr_legacy_path)['receipts'][0].get('receipt_key'),
      'Saving a legacy receipt does not silently assign or backfill receipt identities')
exr_legacy_body = exr_current_body(exr_legacy_path)
exr_legacy_body['receipts'][0]['receipt_key'] = exr_keys[0].upper()
exr_save(exr_legacy_path, exr_legacy_body)
check(exr_record(exr_legacy_path)['receipts'][0]['receipt_key'] == exr_keys[0],
      'A supplied valid receipt UUID is saved canonically without affecting its settlement')

exr_xlsx = exr_writer.call('/exports/export?search=EX-RECON-MAIN')
with zipfile.ZipFile(io.BytesIO(exr_xlsx)) as exr_archive:
    exr_workbook_text = ''.join(exr_archive.read(member).decode('utf-8') for member in exr_archive.namelist() if member.endswith('.xml'))
check(all(value in exr_workbook_text for value in ('invoice date','transport document date','port of loading','OUR-001','BOB-BILL-001','EBRC-RECON-001')),
      'Excel includes distinct invoice/transport dates, ports, inward references and credit advice fields')
check('Documentation cleared' in exr_workbook_text,
      'Excel distinguishes documentary clearance from financial settlement and invoice approval')
check(exr_record(exr_path)['balance']['outstanding'] == '900.00',
      'All reconciliation and file actions leave the actual unpaid balance intact')
# Dedicated post-invoice evidence updates preserve the approved invoice revision.
exre_start = checks
exre_writer_id, exre_writer = export_user('qa.export.reconciliation.evidence', 'Finance Head',
    export_grants + ['export_finance_check', 'export_audit_check'], companies=[])
exre_no_files_id, exre_no_files = export_user('qa.export.reconciliation.no.files',
    permissions=['export_view', 'export_edit'], companies=[])
exre_bad_files_id, exre_bad_files = export_user('qa.export.reconciliation.evidence.bad', companies=[])


def exre_preserved(record):
    return {key:value for key,value in record.items()
            if key not in ('version','updated_at','ebrcs','documentation_cleared')}


def exre_number(path, number, *, previous=None, actor=exre_writer, expect=200, version=None, extra=None):
    body = {'version':exr_version(path) if version is None else version, 'ebrc_number':number}
    if previous is not None: body['previous_ebrc_number'] = previous
    body.update(extra or {})
    return actor.call(path + '/reconciliation/ebrc', body, expect)


def exre_upload(path, *, actor=exre_writer, kind='ebrc', expect=200, version=None, extra=None):
    values = {'version':exr_version(path) if version is None else version,'kind':kind,**(extra or {})}
    data, headers = exd_multipart('post-invoice-ebrc.pdf', payload)
    return actor.call(path + '/reconciliation/files?' + urllib.parse.urlencode(values), data, expect, headers)


def exre_remove(path, file_id, *, actor=exre_writer, expect=200, version=None):
    return actor.call(path + '/reconciliation/files/' + str(file_id) + '/remove',
        {'version':exr_version(path) if version is None else version}, expect)


exre_mail_count = sql('SELECT count(*) FROM export_bank_emails', name)
exre_paths = {}
for exre_status in ('Draft','Changes requested','Submitted','Approved'):
    exre_path, exre_id = exr_new('EVIDENCE-' + exre_status.replace(' ','-'), {**copy.deepcopy(exr_body), 'ebrcs':[]}, actor=exre_writer)
    exre_invoice_file = exr_upload(exre_path, 'invoice', actor=exre_writer)['id']
    if exre_status != 'Draft':
        exre_writer.call(exre_path + '/workflow', {'version':exr_version(exre_path),'action':'submit'})
        if exre_status in ('Approved','Changes requested'):
            exr_md.call(exre_path + '/workflow', {'version':exr_version(exre_path),
                'action':'approve' if exre_status=='Approved' else 'request_changes', 'note':'Review documentary evidence'})
    for exre_check in ('finance','audit'):
        exre_writer.call(exre_path + '/check', {'version':exr_version(exre_path),'kind':exre_check,'status':'Checked','note':'Preserve review marker'})
    exre_before = exr_record(exre_path)
    check(exre_before['workflow_status'] == exre_status and exre_before['balance']['outstanding']=='900.00',
          'Post-invoice evidence fixture has intended workflow and unpaid balance: '+exre_status)
    exre_original = exre_preserved(exre_before)
    exre_first_version = exre_before['version']
    exre_number(exre_path, 'EBRC-POST-001')
    exre_added = exr_record(exre_path)
    check(exre_preserved(exre_added)==exre_original and exre_added['version']==exre_first_version+1 and
          any(row['ebrc_number']=='EBRC-POST-001' for row in exre_added['ebrcs']),
          'Dedicated eBRC number save preserves every invoice/approval/financial field: '+exre_status)
    exre_file = exre_upload(exre_path)['id']
    exre_file_hash = exd_file_hash(exre_file)
    check(exre_preserved(exr_record(exre_path))==exre_original,
          'Dedicated eBRC attachment preserves approval, content revision and financial checks: '+exre_status)
    exr_clear(exre_path, exre_file, number='EBRC-POST-001', actor=exre_writer)
    exre_number(exre_path, 'EBRC-POST-CORRECTED', previous=' ebrc-post-001 ')
    exre_corrected = exr_record(exre_path)
    check(exre_preserved(exre_corrected)==exre_original and not exr_clearance(exre_corrected) and
          len(exre_corrected['ebrcs'])==1 and exre_corrected['ebrcs'][0]['ebrc_number']=='EBRC-POST-CORRECTED',
          'Correcting saved eBRC invalidates clearance only and preserves the invoice review: '+exre_status)
    exr_clear(exre_path, exre_file, number='EBRC-POST-CORRECTED', actor=exre_writer)
    exre_remove(exre_path, exre_file)
    exre_removed = exr_record(exre_path)
    check(exre_preserved(exre_removed)==exre_original and not exr_clearance(exre_removed) and
          exd_file_hash(exre_file)==exre_file_hash and
          not any(row['id']==exre_file for row in exr_detail(exre_path)['files']),
          'Dedicated eBRC removal retains encrypted audit bytes and invalidates only clearance: '+exre_status)
    exr_clear(exre_path, exre_file, number='EBRC-POST-CORRECTED', actor=exre_writer, expect=400)
    exre_number(exre_path, 'EBRC-SECOND', actor=exre_no_files)
    check(exre_preserved(exr_record(exre_path))==exre_original and len(exr_record(exre_path)['ebrcs'])==2,
          'Editor without file permission can record another certificate without replacing earlier evidence: '+exre_status)
    if exre_status in ('Submitted','Approved'):
        exr_save(exre_path, actor=exre_writer, expect=400)
        exr_upload(exre_path, 'ebrc', actor=exre_writer, expect=400)
        exre_writer.call(exre_path + '/files/' + str(exre_invoice_file) + '/remove', {'version':exr_version(exre_path)}, 400)
        check(exre_preserved(exr_record(exre_path))==exre_original,
              'Normal invoice save/upload/remove endpoints remain frozen while dedicated eBRC operations succeed: '+exre_status)
    exre_paths[exre_status] = (exre_path,exre_id,exre_original,exre_invoice_file)

exre_path, exre_id, exre_original, exre_invoice_file = exre_paths['Approved']
exre_file = exre_upload(exre_path)['id']
# Evidence-only handlers reject broad edits, wrong categories and hidden records.
for exre_actor in (exr_reader, exr_md, exw_noview, exw_unsupported, merch_b):
    exre_number(exre_path, 'DENIED', actor=exre_actor, expect=403)
for exre_actor in (exre_no_files, exr_reader, exr_md):
    exre_upload(exre_path, actor=exre_actor, expect=403)
    exre_remove(exre_path, exre_file, actor=exre_actor, expect=403)
for exre_patch in ({'invoice_amount':'0.00'}, {'workflow_status':'Approved'}, {'receipts':[]},
                   {'ebrcs':[]}, {'documentation_cleared':{}}, {'ebrc_amount':'900.00'}):
    exre_number(exre_path, 'DENIED-FIELDS', extra=exre_patch, expect=400)
for exre_number_value in ('', '   ', 'A'*2001):
    exre_number(exre_path, exre_number_value, expect=400)
exre_number(exre_path, 'UNSAVED-PREVIOUS', previous='MISSING', expect=400)
exre_duplicate_before = exr_record(exre_path)
exre_number(exre_path, 'ebrc-second')
check(exr_record(exre_path)==exre_duplicate_before, 'Repeated equivalent eBRC number save is a no-op without duplicate history or clearing evidence')
exre_number(exre_path, 'EBRC-SECOND', previous='EBRC-POST-CORRECTED', expect=400)
for exre_kind in ('invoice','bank_advice','payment_details','jv_supporting'):
    exre_upload(exre_path, actor=exre_bad_files, kind=exre_kind, expect=400)
exre_upload(exre_path, actor=exre_bad_files,
    extra={'receipt_key':exr_keys[0],'field_key':'inward_remittance'}, expect=400)
exre_remove(exre_path, exre_invoice_file, actor=exre_bad_files, expect=400)
for exre_actor in (ex_team_b,ex_buyer):
    exre_number(ex_path, 'DENIED-SCOPE', actor=exre_actor, expect=404, version=ex_record()['version'])
    exre_upload(ex_path, actor=exre_actor, expect=404, version=ex_record()['version'])
    exre_remove(ex_path, ex_file_id, actor=exre_actor, expect=404, version=ex_record()['version'])
exre_number(exj_path, 'NO-JV-EVIDENCE', expect=400, version=exj_version())
exre_upload(exj_path, actor=exre_bad_files, expect=400, version=exj_version())
exre_remove(exj_path, exj_file, actor=exre_bad_files, expect=400, version=exj_version())
Client().call(exre_path+'/reconciliation/ebrc', {'version':exr_version(exre_path),'ebrc_number':'NO-SESSION'}, 401)
exre_writer.call(exre_path+'/reconciliation/ebrc', {'version':exr_version(exre_path),'ebrc_number':'NO-CSRF'}, 403, {'X-CSRF-Token':''})
exre_old_version = exr_version(exre_path)
exre_number(exre_path, 'EBRC-FINAL')
exre_number(exre_path, 'STALE', version=exre_old_version, expect=409)
exre_upload(exre_path, actor=exre_bad_files, version=exre_old_version, expect=409)
exre_remove(exre_path, exre_file, version=exre_old_version, expect=409)
check(exre_preserved(exr_record(exre_path))==exre_original,
      'Invalid, hidden, unauthorized and stale evidence requests never alter approved invoice or finance checks')

# Number correction retains all other fields on the selected stored certificate.
exre_legacy_path, exre_legacy_id = exr_new('EVIDENCE-CORRECTION', actor=exre_writer)
exre_before_rows = copy.deepcopy(exr_record(exre_legacy_path)['ebrcs'])
exre_number(exre_legacy_path, 'EBRC-CORRECTED-LEGACY', previous='EBRC-RECON-001')
exre_after_rows = exr_record(exre_legacy_path)['ebrcs']
check({k:v for k,v in exre_before_rows[0].items() if k!='ebrc_number'} ==
      {k:v for k,v in exre_after_rows[0].items() if k!='ebrc_number'} and len(exre_after_rows)==len(exre_before_rows),
      'Correcting a certificate number preserves its original amount, status, notes and other stored fields')

# Concurrent new certificate writes share the same version lock and preserve approval.
exre_race_version = exr_version(exre_path)
exre_before_count = len(exr_record(exre_path)['ebrcs'])

def exre_race_number(index):
    actor = Client('qa.export.reconciliation.evidence')
    request = urllib.request.Request(base+'/api'+exre_path+'/reconciliation/ebrc',
        data=json.dumps({'version':exre_race_version,'ebrc_number':'EBRC-RACE-'+str(index)}).encode(),
        headers={'Origin':os.getenv('COMPANY_TEST_ORIGIN',base),'X-CSRF-Token':actor.csrf,'Content-Type':'application/json'})
    try: response=actor.opener.open(request,timeout=15)
    except urllib.error.HTTPError as error: response=error
    response.read()
    return response.status

with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
    exre_race_codes=sorted(pool.map(exre_race_number,(1,2)))
check(exre_race_codes==[200,409] and len(exr_record(exre_path)['ebrcs'])==exre_before_count+1 and
      exre_preserved(exr_record(exre_path))==exre_original,
      'Concurrent dedicated eBRC updates commit one evidence event without altering approval')
# Direct runtime SQL cannot use the evidence exception for mixed changes,
# another parent, a noneditor identity or non-eBRC document removal.
exre_sql_identity = exw_identity(role='Finance Head',uid=exre_writer_id,exports_edit=True,exports_files=True)
exre_marker = f"SET LOCAL erp.export_reconciliation='{exre_id}'; "
exre_wrong_marker = f"SET LOCAL erp.export_reconciliation='{exre_id+100000}'; "
exre_append_sql = f"UPDATE export_invoices SET ebrcs=ebrcs||'[{{\"ebrc_number\":\"SQL-ONLY-NUMBER\"}}]'::jsonb,version=version+1,updated_at=now() WHERE id={exre_id}"
exre_remove_sql = f"INSERT INTO export_file_removals(file_id,removed_by) VALUES({exre_file},{exre_writer_id})"
exre_insert_sql = f"INSERT INTO export_files(invoice_id,kind,filename,mime,size,sha256,ciphertext,created_by) SELECT invoice_id,'invoice',filename,mime,size,sha256,ciphertext,{exre_writer_id} FROM export_files WHERE id={exre_file}"
for exre_label,exre_statement,exre_identity in (
    ('mixed invoice amount',exre_marker+f"UPDATE export_invoices SET invoice_amount=1,version=version+1,updated_at=now() WHERE id={exre_id}",exre_sql_identity),
    ('wrong parent marker',exre_wrong_marker+exre_append_sql,exre_sql_identity),
    ('noneditor MD evidence',exre_marker+exre_append_sql,exw_identity(role='MD',uid=exr_md_id,exports_edit=False,exports_files=True)),
    ('certificate metadata overwrite',exre_marker+f"UPDATE export_invoices SET ebrcs=jsonb_set(ebrcs,'{{0,ebrc_amount}}','\"999.00\"'),version=version+1,updated_at=now() WHERE id={exre_id}",exre_sql_identity),
    ('wrong document kind',exre_marker+f"INSERT INTO export_file_removals(file_id,removed_by) VALUES({exre_invoice_file},{exre_writer_id})",exre_sql_identity),
    ('wrong uploaded document kind',exre_marker+exre_insert_sql,exre_sql_identity),
    ('wrong parent file insertion',exre_wrong_marker+exre_insert_sql.replace("'invoice'","'ebrc'"),exre_sql_identity),
    ('nonFinance evidence insertion',exre_marker+exre_insert_sql.replace("'invoice'","'ebrc'"),{**exre_sql_identity,'role':'MD'}),
    ('wrong parent file removal',exre_wrong_marker+exre_remove_sql,exre_sql_identity),
    ('no evidence marker',exre_remove_sql,exre_sql_identity),
    ('missing editor permission',exre_marker+exre_remove_sql,{**exre_sql_identity,'exports_edit':False}),
    ('missing file permission',exre_marker+exre_remove_sql,{**exre_sql_identity,'exports_files':False}),
):
    try:
        exw_runtime_sql(exre_statement,exre_identity)
        raise AssertionError('Runtime SQL bypassed dedicated evidence guard: '+exre_label)
    except subprocess.CalledProcessError as exre_sql_error:
        check(any(text in exre_sql_error.stderr.lower() for text in ('reconciliation','ebrc','row-level security')),
              'Database independently blocks '+exre_label+' in reconciliation')
check(exre_preserved(exr_record(exre_path))==exre_original and
      sql(f'SELECT count(*) FROM export_file_removals WHERE file_id={exre_file}',name)=='0',
      'Denied direct SQL evidence attempts leave approved invoice and active document intact')

check(sql('SELECT count(*) FROM export_bank_emails',name)==exre_mail_count,
      'No reconciliation action prepares or sends bank email automatically')
print('Post-invoice eBRC evidence HTTP checks passed: '+str(checks-exre_start),flush=True)

print('Export reconciliation HTTP checks passed: ' + str(checks-exr_start), flush=True)
