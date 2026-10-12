# JV checks run only after export fixtures in the disposable HTTP/RLS harness.
assert name.startswith('erp_test_') and runtime_role == name + '_api'
check(not env.get('SMTP_HOST'), 'JV tests cannot send real employee messages')
exj_start = checks
exj_writer_id, exj_writer = export_user('qa.export.jv.writer', companies=[])
exj_md_id, exj_md = export_user('qa.export.jv.md', 'MD', ['export_view', 'export_download', 'export_approve'], companies=[])
exj_body = {'record_kind': 'jv', 'party_type': 'manual', 'invoice_number': 'JV-ISOLATED-001',
            'currency': 'USD', 'invoice_amount': '1234.56',
            'profile': {'export_date': '2026-10-10', 'jv_by_date': 'Finance / 10 Oct 2026',
                        'scrip_tally_reference': '000017'}}
exj_export_before = exj_writer.call('/exports')
exj_options_before = exj_writer.call('/exports/options')['standalone_parties']
exj_created = exj_writer.call('/exports', exj_body)
exj_id = exj_created['id']
exj_path = '/exports/' + str(exj_id)


def exj_detail():
    return exj_writer.call(exj_path)


def exj_version():
    return exj_detail()['record']['version']


def exj_action(action, actor=exj_writer, expect=200, **extra):
    return actor.call(exj_path + '/workflow', {'version': exj_version(), 'action': action, **extra}, expect)


def exj_upload(kind='jv_supporting', expect=200, actor=exj_writer):
    return actor.call(exj_path + '/files?kind=' + kind + '&version=' + str(exj_version()), multipart,
                      expect, {'Content-Type': 'multipart/form-data; boundary=' + boundary})


exj_initial = exj_detail()
check(exj_initial['record']['party_name'] == 'EURO SUITS MFG' and
      exj_initial['record']['profile']['shipping_mode'] == 'HANDO', 'JV creation applies only requested company and mode defaults')
check(exj_initial['record']['profile']['jv_by_date'] == 'Finance / 10 Oct 2026', 'JV By / date preserves combined text without treating it as an ISO date')
check(all(exj_initial['record'].get(exj_key) is None for exj_key in ('company_id', 'party_id', 'buyer_id', 'party_setup_id', 'order_id')),
      'JV has no operational company, buyer or PO link')
check('balance' not in exj_initial['record'] and 'bill_amount' not in exj_initial['record'], 'JV has no export settlement balance')
check(not any(exj_initial['permissions'][exj_key] for exj_key in ('finance_check', 'audit_check', 'discount_first', 'discount_final')),
      'JV capabilities hide unrelated financial actions')
exj_options = exj_writer.call('/exports/options?record_kind=jv')
check(exj_options['document_kinds'] == [{'key': 'jv_supporting', 'label': 'Supporting JV document'}],
      'JV offers only supporting JV documents')
check(len(exj_writer.call('/exports/options')['document_kinds']) == 12 and
      all(exj_kind['key'] != 'jv_supporting' for exj_kind in exj_writer.call('/exports/options')['document_kinds']),
      'Export checklist keeps its original twelve categories')
check(exj_writer.call('/exports')['totals'] == exj_export_before['totals'] and
      exj_writer.call('/exports')['records'] == exj_export_before['records'], 'Creating a JV leaves the Export register and financial totals unchanged')
exj_rows = exj_writer.call('/exports?record_kind=jv')
check(exj_rows['totals'] == [] and [exj_row['id'] for exj_row in exj_rows['records']] == [exj_id], 'JV register is separate and contains no export totals')
for exj_query in ('record_kind=unknown', 'record_kind=JV'):
    exj_writer.call('/exports?' + exj_query, expect=400)
    exj_writer.call('/exports/options?' + exj_query, expect=400)
exj_writer.call('/exports/options?record_kind=jv&company_id=' + str(opscompany), expect=400)
exj_writer.call('/exports/statement?record_kind=jv', expect=400)

# Existing feature/role boundaries apply to new JV rows, even if UI is bypassed.
for exj_actor in (exw_noview, exw_unsupported, merch_b):
    exj_actor.call('/exports?record_kind=jv', expect=403)
    exj_actor.call(exj_path, expect=403)
for exj_actor in (ex_reader, exj_md, exw_noview, exw_unsupported, merch_b):
    exj_actor.call('/exports', {**exj_body, 'invoice_number': 'JV-DENIED'}, 403)
exj_writer.call(exj_path, {**exj_body, 'version': exj_version()}, 403, {'X-CSRF-Token': ''})
for exj_patch in (
    {'company_id': opscompany}, {'party_id': buyer_a}, {'order_id': po_a},
    {'currency': ''}, {'invoice_amount': 10}, {'invoice_amount': '10.001'},
    {'receipts': [{'payment_received': '1.00', 'currency': 'USD'}]},
    {'ebrcs': [{'ebrc_number': 'NO-JV-EBRC'}]},
    {'profile': {'shipping_bill_number': 'NO-JV-SHIPPING'}},
    {'profile': {'export_date': '2026-02-30'}},
    {'profile': {'statuses': {'shipping_mode': 'Pending'}}},
):
    exj_writer.call('/exports', {**exj_body, 'invoice_number': 'JV-INVALID', **exj_patch}, 400)
exj_writer.call('/exports', {exj_key: exj_value for exj_key, exj_value in exj_body.items() if exj_key != 'currency'}, 400)
exj_writer.call(exj_path, {**exj_body, 'record_kind': 'export', 'version': exj_version()}, 400)
ex_writer.call(ex_path, {**ex_body, 'record_kind': 'jv', 'version': ex_record()['version']}, 400)
exj_writer.call(exj_path + '/discount', {'version': exj_version(), 'action': 'request', 'amount': '1.00', 'reason': 'No JV discount'}, 400)
ex_finance.call(exj_path + '/check', {'version': exj_version(), 'kind': 'finance', 'status': 'Checked', 'note': 'No JV financial check'}, 400)
ex_auditor.call(exj_path + '/check', {'version': exj_version(), 'kind': 'audit', 'status': 'Checked', 'note': 'No JV audit check'}, 400)

# A supporting JV file is necessary both initially and after correction/removal.
exj_action('submit', expect=400)
for exj_wrong_kind in ('invoice', 'other', 'packing_list', 'bank_advice'):
    exj_upload(exj_wrong_kind, expect=400)
exd_upload(exd_path, 'jv_supporting', expect=400)
exj_upload(actor=ex_reader, expect=403)
exj_file = exj_upload()['id']
check(exj_writer.call('/export-files/' + str(exj_file)) == payload, 'Supporting JV document decrypts to its exact original contents')
check(payload not in bytes.fromhex(sql(f"SELECT encode(ciphertext,'hex') FROM export_files WHERE id={exj_file}", name)),
      'Supporting JV document remains encrypted at rest')
exj_file_hash = sql(f'SELECT md5(to_jsonb(f)::text) FROM export_files f WHERE id={exj_file}', name)
exj_writer.call(exj_path + '/files/' + str(exj_file) + '/remove', {'version': exj_version()})
exj_action('submit', expect=400)
exj_writer.call('/export-files/' + str(exj_file), expect=404)
check(sql(f'SELECT md5(to_jsonb(f)::text) FROM export_files f WHERE id={exj_file}', name) == exj_file_hash,
      'JV removal retains original encrypted evidence without physical deletion')
exj_replacement = exj_upload()['id']
exj_before_submit = exj_version()
exj_action('submit')
exj_writer.call(exj_path + '/workflow', {'version': exj_before_submit, 'action': 'submit'}, 409)
exj_upload(expect=400)
exj_writer.call(exj_path + '/files/' + str(exj_replacement) + '/remove', {'version': exj_version()}, 400)
exj_action('approve', expect=403)
exj_action('request_changes', exj_md, note='Please correct the Tally reference')
exj_writer.call(exj_path + '/files/' + str(exj_replacement) + '/remove', {'version': exj_version()})
exj_action('submit', expect=400)
exj_upload()
exj_action('submit')
exj_action('approve', exj_md)
check(exj_detail()['record']['workflow_status'] == 'Approved' and
      exj_detail()['record']['reviewed_by'] == exj_md_id and
      any(exj_comment['note'] == 'Please correct the Tally reference' for exj_comment in exj_detail()['comments']),
      'Independent MD review, correction comments and resubmission approve the JV')
check(exj_writer.call('/exports/options')['standalone_parties'] == exj_options_before,
      'Approved JV company names do not populate the invoice customer dropdown')
exj_writer.call(exj_path, {**exj_body, 'version': exj_version()}, 400)
exj_action('reopen', note='Correct the approved JV reference')
check(exj_detail()['record']['approved_revision'] is None, 'Reopening JV revokes approval of the prior content')
exj_saved_version = exj_version()
exj_edit = {**exj_body, 'party_name': 'JV-only revised company', 'profile': {**exj_body['profile'], 'shipping_mode': 'Courier'}}
exj_writer.call(exj_path, {**exj_edit, 'version': exj_saved_version})
exj_writer.call(exj_path, {**exj_body, 'version': exj_saved_version}, 409)
check(exj_detail()['record']['party_name'] == 'JV-only revised company' and
      exj_detail()['record']['profile']['shipping_mode'] == 'Courier', 'JV edit preserves explicit company and mode without reapplying defaults')

# Concurrent JV saves serialize on the same version lock as Export invoices.
exj_race_version = exj_version()

def exj_race(_):
    exj_client = Client('qa.export.jv.writer')
    exj_request = urllib.request.Request(base + '/api' + exj_path,
        data=json.dumps({**exj_edit, 'version': exj_race_version}).encode(),
        headers={'Origin': os.getenv('COMPANY_TEST_ORIGIN', base), 'X-CSRF-Token': exj_client.csrf,
                 'Content-Type': 'application/json'})
    try:
        return exj_client.opener.open(exj_request, timeout=15).status
    except urllib.error.HTTPError as exj_error:
        return exj_error.code

with concurrent.futures.ThreadPoolExecutor(max_workers=2) as exj_pool:
    exj_race_codes = sorted(exj_pool.map(exj_race, [1, 2]))
check(exj_race_codes == [200, 409], 'Concurrent JV saves commit exactly one current revision')

# Super Admin cannot self-approve either a created or submitted JV.
exj_self = exw_sa.call('/exports', {**exj_body, 'invoice_number': 'JV-SELF-REVIEW'})['id']
exj_self_path = '/exports/' + str(exj_self)
exw_upload_invoice(exj_self_path, actor=exw_sa, kind='jv_supporting')
exw_sa.call(exj_self_path + '/workflow', {'version': exw_sa.call(exj_self_path)['record']['version'], 'action': 'submit'})
exw_sa.call(exj_self_path + '/workflow', {'version': exw_sa.call(exj_self_path)['record']['version'], 'action': 'approve'}, 400)
exj_md.call(exj_self_path + '/workflow', {'version': exj_md.call(exj_self_path)['record']['version'], 'action': 'approve'})

# Exactly eight highlighted workbook columns; original invoice exports stay separate.
exj_literals = exs_workbook_literals(exj_writer.call(exj_path + '/export'))
check(all(exj_label in exj_literals for exj_label in ('JV REF', 'DATE', 'PARTY NAME', 'CURR', 'AMOUNT', 'MODE', 'BY / DATE', 'TALLY REF')) and
      'JV-ISOLATED-001' in exj_literals and 'Finance / 10 Oct 2026' in exj_literals and '000017' in exj_literals,
      'JV workbook contains all eight highlighted fields with exact references and text')
check('JV-ISOLATED-001' not in exs_workbook_literals(exj_writer.call('/exports/export')) and
      'JV-ISOLATED-001' in exs_workbook_literals(exj_writer.call('/exports/export?record_kind=jv')),
      'Register workbook downloads do not mix Export and JV entries')
check('JV entry' in ' '.join(exj_item['action'] for exj_item in exj_detail()['history']), 'JV actions use a distinct audit label')

# Direct database writes cannot reclassify records or inject unrelated data.
for exj_sql in (
    f"UPDATE export_invoices SET record_kind='export' WHERE id={exj_id}",
    f"UPDATE export_invoices SET record_kind='jv' WHERE id={ex_id}",
    f"UPDATE export_invoices SET receipts='[{{\"payment_received\":\"1.00\",\"currency\":\"USD\"}}]' WHERE id={exj_id}",
    f"UPDATE export_invoices SET profile=profile||'{{\"shipping_bill_number\":\"injected\"}}'::jsonb WHERE id={exj_id}",
    f"UPDATE export_invoices SET finance_check='{{\"status\":\"Checked\"}}' WHERE id={exj_id}",
    f"UPDATE export_files SET kind='invoice' WHERE id={exj_file}",
    f"UPDATE export_files SET kind='jv_supporting' WHERE id={ex_file_id}",
):
    try:
        sql('BEGIN; ' + exj_sql + '; ROLLBACK;', name)
        raise AssertionError('JV database shape/category guard bypassed')
    except subprocess.CalledProcessError as exj_error:
        check('check constraint' in exj_error.stderr.lower() or 'classification' in exj_error.stderr.lower() or
              'document category' in exj_error.stderr.lower(), 'Database independently rejects JV reclassification and unrelated fields/documents')
for exj_identity in (exw_identity(exports=False), exw_identity(role='Marketing')):
    exj_count_output = exw_runtime_sql(f'SELECT json_build_array(count(*)) FROM export_invoices WHERE id={exj_id}', exj_identity)
    check(json.loads(next(exj_line for exj_line in exj_count_output.splitlines() if exj_line.startswith('['))) == [0],
          'RLS hides JV rows from unsupported or export-disabled users')
try:
    exw_runtime_sql(f"INSERT INTO export_discounts(invoice_id,amount,currency,reason,requested_by) VALUES({exj_id},1,'USD','No JV discount',{exj_writer_id})",
                    exw_identity(uid=exj_writer_id, exports_edit=True))
    raise AssertionError('RLS allowed JV discount insertion')
except subprocess.CalledProcessError as exj_error:
    check('row-level security' in exj_error.stderr.lower(), 'Restrictive RLS blocks direct JV discount insertion')
print('JV HTTP/RLS checks passed: ' + str(checks - exj_start), flush=True)
