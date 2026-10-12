# Loaded only by the disposable company_setup.py lifecycle after export_cases.py.
# All accounts, invoices, SQL writes and documents below are synthetic.
assert name.startswith('erp_test_') and runtime_role == name + '_api'
check(not env.get('SMTP_HOST'), 'Export approval tests cannot send real employee email')
exw_start = checks
exw_review_grants = ['export_view', 'export_download', 'export_approve']
exw_md_id, exw_md = export_user('qa.export.workflow.md', 'MD', exw_review_grants, companies=[])
exw_sa_id, exw_sa = export_user('qa.export.workflow.sa', 'Super Admin', export_grants + ['export_approve'], companies=[])
exw_unassigned_id, exw_unassigned = export_user('qa.export.unassigned', 'Finance Head', scope='team', companies=[], teams=[])
exw_noview_id, exw_noview = export_user('qa.export.no.view', permissions=[])
exw_unsupported_id, exw_unsupported = export_user('qa.export.unsupported')
# Deliberately corrupt only this isolated fixture's role. The API must still
# reject stored export permissions that exceed the employee's role family.
sql(f"UPDATE users SET role='Marketing' WHERE id={exw_unsupported_id}", name)
exw_unsupported = Client('qa.export.unsupported')

exw_body = {
    'company_id': None, 'party_type': 'manual', 'party_id': None, 'order_id': None,
    'party_name': 'Synthetic independent export customer',
    'invoice_number': 'EX-WORKFLOW-001', 'currency': 'USD', 'invoice_amount': '1000.00',
    'profile': {'export_date': '2026-10-09', 'shipping_mode': 'Sea'},
    'receipts': [{'currency': 'USD', 'payment_received': '990.00',
                  'bank_charges': '10.00', 'bank_charges_currency': 'USD'}],
    'ebrcs': [],
}
exw_id = exw_unassigned.call('/exports', exw_body)['id']
exw_path = '/exports/' + str(exw_id)


def exw_detail(actor=exw_unassigned):
    return actor.call(exw_path)


def exw_record(actor=exw_unassigned):
    return exw_detail(actor)['record']


def exw_action(actor, action, expect=200, version=None, **values):
    return actor.call(exw_path + '/workflow', {
        'version': exw_record()['version'] if version is None else version,
        'action': action, **values,
    }, expect)


def exw_save(body=None, actor=exw_unassigned, expect=200):
    return actor.call(exw_path, {**(exw_body if body is None else body),
                                'version': exw_record()['version']}, expect)


def exw_fingerprint():
    return sql(f'SELECT md5(to_jsonb(d)::text) FROM export_invoices d WHERE id={exw_id}', name)


def exw_upload_invoice(path, actor=exw_unassigned, kind='invoice'):
    version = actor.call(path)['record']['version']
    return actor.call(path + '/files?kind=' + kind + '&version=' + str(version), multipart,
                      headers={'Content-Type': 'multipart/form-data; boundary=' + boundary})


exw_initial = exw_record()
check(exw_initial['workflow_status'] == 'Draft', 'New standalone invoice starts as a draft')
check(all(exw_initial.get(key) is None for key in ('company_id', 'party_id', 'buyer_id', 'party_setup_id', 'order_id')),
      'Standalone invoice has no linked company, buyer, master or production order')
check(exw_initial['party_name'] == exw_body['party_name'], 'Manual invoice party name persists independently of Company Setup')
check(exw_initial['balance']['outstanding'] == '0.00', 'Invoice workflow preserves receipt-plus-bank-charge settlement')
check(exw_detail()['actions']['edit'] and exw_detail()['actions']['submit'], 'Finance with no company assignment can prepare and submit a draft')
for actor in (ex_team_a, ex_team_b, ex_line, ex_buyer, ex_reader, exw_md, exw_sa):
    actor.call(exw_path)
    check(any(row['id'] == exw_id for row in actor.call('/exports')['records']),
          'Standalone register is intentionally shared with export-enabled Finance and management')
for index, actor in enumerate((ex_team_a, ex_team_b, ex_line, ex_buyer, ex_writer, ex_finance, exw_sa)):
    result = actor.call('/exports', {**exw_body, 'invoice_number': 'EX-SHARED-' + str(index)})
    check(actor.call('/exports/' + str(result['id']))['record']['company_id'] is None,
          'Scoped Finance and Super Admin can create an unassigned standalone invoice')
for actor in (ex_reader, exw_md, ex_final, exw_noview, exw_unsupported, merch_b):
    actor.call('/exports', {**exw_body, 'invoice_number': 'EX-DENIED-STANDALONE'}, 403)
for actor in (exw_noview, exw_unsupported, merch_b):
    actor.call('/exports', expect=403)
    actor.call(exw_path, expect=403)
for actor in (ex_team_b, ex_buyer, exw_unassigned):
    actor.call(ex_path, expect=404)
check(exw_md.call('/exports/options')['permissions']['approve'], 'Explicit MD invoice approval capability is available')
check(not ex_final.call('/exports/options')['permissions']['approve'],
      'Existing final-discount approval does not silently grant invoice approval')
admin.call('/access-designations', {
    'name': 'Invalid Finance approval grant', 'department': 'Export QA', 'role': 'Finance Head',
    'features': [], 'permissions': export_grants + ['export_approve'],
}, 400)

# The independent workspace cannot carry company/buyer/order references.
for patch in (
    {'company_id': opscompany}, {'party_id': buyer_a}, {'order_id': po_a},
    {'buyer_id': buyer_a}, {'party_setup_id': exs_master},
    {'party_type': 'buyer', 'party_id': buyer_a}, {'party_name': ''},
    {'party_name': 'A' * 161}, {'party_name': 'Bad\nName'},
    {'workflow_status': 'Approved'}, {'approved_revision': 1},
):
    exw_unassigned.call('/exports', {**exw_body, 'invoice_number': 'EX-INVALID-MANUAL', **patch}, 400)
exw_unassigned.call('/exports', {**exw_body, 'invoice_number': ' ex-workflow-001 '}, 409)
exw_save({**exw_body, 'company_id': opscompany, 'party_type': 'buyer', 'party_id': buyer_a}, expect=400)

# Comments are versioned, immutable and do not silently submit an invoice.
for actor in (ex_reader, ex_final):
    actor.call(exw_path + '/comments', {'version': exw_record()['version'], 'note': 'Read-only cannot comment'}, 403)
exw_unassigned.call(exw_path + '/comments', {'version': exw_record()['version'], 'note': ''}, 400)
exw_comment_before = exw_record()
exw_unassigned.call(exw_path + '/comments', {
    'version': exw_comment_before['version'], 'note': 'Please check the receipt advice <literal>&.',
})
exw_comment_after = exw_record()
check(exw_comment_after['content_revision'] == exw_comment_before['content_revision'] and
      exw_comment_after['version'] > exw_comment_before['version'],
      'Discussion advances concurrency version without changing invoice content revision')
check(any(c['note'] == 'Please check the receipt advice <literal>&.' for c in exw_detail()['comments']),
      'Invoice discussion stores a visible correction comment')
exw_unassigned.call(exw_path + '/comments', {'version': exw_comment_before['version'], 'note': 'Stale comment'}, 409)

# Invalid transitions, permissions and stale actions leave all invoice state intact.
exw_action(exw_md, 'approve', 400)
for actor in (ex_reader, ex_final, exw_md):
    exw_action(actor, 'submit', 403)
exw_action(exw_unassigned, 'approve', 403)
exw_without_invoice = exw_fingerprint()
exw_missing_document = exw_action(exw_unassigned, 'submit', 400)
check('invoice document' in exw_missing_document.get('error', '').lower() and
      exw_fingerprint() == exw_without_invoice,
      'Draft can save without a file, but direct API submission requires an invoice document without changing state')
exw_unassigned.call(exw_path + '/workflow', {
    'version': exw_record()['version'], 'action': 'submit', 'invoice_file_id': ex_file_id,
}, 400)
check(exw_fingerprint() == exw_without_invoice,
      'A client-supplied file ID from another invoice cannot bypass required attachment validation')
exw_upload_invoice(exw_path, kind='packing_list')
exw_wrong_kind = exw_fingerprint()
exw_action(exw_unassigned, 'submit', 400)
check(exw_fingerprint() == exw_wrong_kind, 'Packing list alone cannot satisfy required invoice attachment')
exw_initial_invoice_file = exw_upload_invoice(exw_path)['id']
exw_before_submit = exw_record()
exw_unassigned.call(exw_path + '/workflow', {
    'version': exw_before_submit['version'], 'action': 'submit',
}, 403, {'X-CSRF-Token': ''})
exw_action(exw_unassigned, 'submit', note='Ready for review')
exw_submitted = exw_record()
check(exw_submitted['workflow_status'] == 'Submitted' and
      exw_submitted['submitted_revision'] == exw_submitted['content_revision'] and
      exw_submitted['submitted_by'] == exw_unassigned_id,
      'Submission freezes the exact content revision and submitting employee')
check(not exw_detail()['actions']['edit'] and not exw_detail()['actions']['attach'],
      'Submitted invoice content and attachment actions are blocked')
exw_save(expect=400)
exw_unassigned.call(exw_path + '/files?kind=invoice&version=' + str(exw_record()['version']),
                    multipart, 400, {'Content-Type': 'multipart/form-data; boundary=' + boundary})
exw_action(exw_unassigned, 'submit', 400)
exw_before_denied = exw_fingerprint()
exw_action(exw_md, 'request_changes', 400, note='')
exw_action(exw_md, 'approve', 409, version=exw_before_submit['version'])
exw_action(ex_finance, 'approve', 403)
check(exw_fingerprint() == exw_before_denied, 'Denied or stale approvals cannot mutate submitted invoice')
exw_action(exw_md, 'request_changes', note='Correct the shipping bill reference before approval.')
exw_correction = exw_record()
check(exw_correction['workflow_status'] == 'Changes requested' and
      exw_correction['review_note'] == 'Correct the shipping bill reference before approval.',
      'MD correction reason remains visible to Finance')
# Remove the obsolete invoice through the real API; packing-list files cannot
# replace the required active invoice attachment on correction resubmission.
exw_unassigned.call(exw_path + '/files/' + str(exw_initial_invoice_file) + '/remove',
                    {'version': exw_correction['version']})
exw_missing_resubmit = exw_fingerprint()
exw_action(exw_unassigned, 'submit', 400)
check(exw_fingerprint() == exw_missing_resubmit,
      'Correction resubmission also requires an invoice document and cannot bypass the API guard')
exw_body['profile']['shipping_bill_number'] = 'CORRECTED-SB-001'
exw_save()
check(exw_record()['workflow_status'] == 'Draft' and
      exw_record()['content_revision'] > exw_correction['content_revision'],
      'Saving a requested correction creates a fresh draft content revision')

# Attachments are encrypted, participate in revision approval and keep child RLS.
exw_file = exw_unassigned.call(exw_path + '/files?kind=invoice&version=' + str(exw_record()['version']),
                              multipart, headers={'Content-Type': 'multipart/form-data; boundary=' + boundary})['id']
check(exw_unassigned.call('/export-files/' + str(exw_file)) == payload,
      'Standalone export attachment downloads its original encrypted content')
check(payload not in bytes.fromhex(sql(f"SELECT encode(ciphertext,'hex') FROM export_files WHERE id={exw_file}", name)),
      'Standalone export attachment is not stored as plaintext')
exw_action(exw_unassigned, 'submit')
exw_before_approval = exw_record()
exw_action(exw_md, 'approve', note='Corrected invoice and attachment verified')
exw_approved = exw_record()
check(exw_approved['workflow_status'] == 'Approved' and
      exw_approved['approved_revision'] == exw_before_approval['content_revision'] and
      exw_approved['reviewed_by'] == exw_md_id,
      'Independent MD approval records the exact reviewed invoice revision')
exw_save(expect=400)
exw_action(exw_md, 'approve', 400)
exw_action(exw_unassigned, 'reopen', 400, note='')
exw_action(exw_unassigned, 'reopen', note='Additional customs reference received')
check(exw_record()['workflow_status'] == 'Draft', 'Approved invoice needs an explicit reasoned reopen before editing')

# A Super Admin cannot approve either their own invoice or their own submission.
exw_action(exw_sa, 'submit')
exw_action(exw_sa, 'approve', 400)
exw_action(exw_md, 'approve')
exw_own_id = exw_sa.call('/exports', {**exw_body, 'invoice_number': 'EX-SA-SELF-APPROVAL'})['id']
exw_own_path = '/exports/' + str(exw_own_id)
exw_own_upload = exw_upload_invoice(exw_own_path)
exw_unassigned.call(exw_own_path + '/workflow', {'version': exw_own_upload['version'], 'action': 'submit'})
exw_sa.call(exw_own_path + '/workflow', {
    'version': exw_sa.call(exw_own_path)['record']['version'], 'action': 'approve',
}, 400)
exw_md.call(exw_own_path + '/workflow', {
    'version': exw_md.call(exw_own_path)['record']['version'], 'action': 'approve',
})
exw_sa_review = exw_unassigned.call('/exports', {**exw_body, 'invoice_number': 'EX-SA-INDEPENDENT-APPROVAL'})['id']
exw_sa_review_path = '/exports/' + str(exw_sa_review)
exw_sa_review_upload = exw_upload_invoice(exw_sa_review_path)
exw_unassigned.call(exw_sa_review_path + '/workflow', {'version': exw_sa_review_upload['version'], 'action': 'submit'})
exw_sa.call(exw_sa_review_path + '/workflow', {
    'version': exw_sa.call(exw_sa_review_path)['record']['version'], 'action': 'approve',
})
check(exw_sa.call(exw_sa_review_path)['record']['workflow_status'] == 'Approved',
      'Super Admin can independently approve a Finance-submitted invoice')

# Two reviewers cannot independently overwrite the same submitted revision.
exw_race_id = exw_unassigned.call('/exports', {**exw_body, 'invoice_number': 'EX-CONCURRENT-REVIEW'})['id']
exw_race_path = '/exports/' + str(exw_race_id)
exw_race_upload = exw_upload_invoice(exw_race_path)
exw_race_version = exw_unassigned.call(exw_race_path + '/workflow', {'version': exw_race_upload['version'], 'action': 'submit'})['version']
def exw_review_race(username):
    actor = Client(username)
    request = urllib.request.Request(base + '/api' + exw_race_path + '/workflow',
        data=json.dumps({'version': exw_race_version, 'action': 'approve'}).encode(),
        headers={'Origin': os.getenv('COMPANY_TEST_ORIGIN', base), 'X-CSRF-Token': actor.csrf,
                 'Content-Type': 'application/json'})
    try:
        return actor.opener.open(request, timeout=15).status
    except urllib.error.HTTPError as error:
        return error.code
with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
    exw_race_codes = sorted(pool.map(exw_review_race, ['qa.export.workflow.md', 'qa.export.workflow.sa']))
check(exw_race_codes == [200, 409], 'Concurrent independent invoice approvals commit once and reject the stale reviewer')

# Removing the explicit new approval grant takes effect without touching discounts.
exw_md_template = next(d for d in admin.call('/setup-users')['designations']
                       if d['id'] == next(u['designation_id'] for u in admin.call('/setup-users')['users'] if u['id'] == exw_md_id))
admin.call('/setup-users', {
    'username': 'qa.export.limited.md', 'name': 'Explicitly limited MD', 'password': password,
    'designation_id': exw_md_template['id'], 'inherit_designation': False,
    'features': [], 'permissions': ['export_view', 'export_download'],
    'scope': 'company', 'companies': [], 'active': True,
})
exw_limited_md = Client('qa.export.limited.md')
exw_action(exw_limited_md, 'approve', 403)
check(not exw_limited_md.call('/exports/options')['permissions']['approve'],
      'An approver designation cannot override an employee-specific approval restriction')
admin.call('/access-designations/' + str(exw_md_template['id']), {
    **exw_md_template, 'permissions': ['export_view', 'export_download'],
})
exw_md.call('/exports', expect=401)
exw_md = Client('qa.export.workflow.md')
exw_action(exw_md, 'approve', 403)
check(not exw_md.call('/exports/options')['permissions']['approve'],
      'Explicit approval removal is enforced by the backend and returned capabilities')

# Child records, register exports and history use the same standalone scope.
exw_action(exw_unassigned, 'reopen', note='Record final finance discussion')
exw_discount_body = {**exw_body, 'invoice_amount': '1001.00'}
exw_save(exw_discount_body)
exw_unassigned.call(exw_path + '/discount', {
    'version': exw_record()['version'], 'action': 'request', 'amount': '1.00', 'reason': 'Synthetic remaining collection balance',
})
exw_discount = next(d['id'] for d in exw_detail()['discounts'] if d['status'] == 'Pending finance')
ex_finance.call(exw_path + '/discount', {'version': exw_record()['version'], 'action': 'first_approve',
                                       'discount_id': exw_discount, 'reason': 'Finance confirmed the small residual'})
check(exw_record()['balance']['outstanding'] == '1.00', 'Invoice approval does not replace first-stage discount review')
ex_final.call(exw_path + '/discount', {'version': exw_record()['version'], 'action': 'final_approve',
                                     'discount_id': exw_discount, 'reason': 'Independent commercial closure'})
check(exw_record()['balance']['outstanding'] == '0.00' and exw_record()['balance']['approved_discount'] == '1.00',
      'Standalone invoice retains independent Finance Head and MD discount approvals')
check(any(h['action'] for h in exw_detail()['history']), 'Standalone workflow retains immutable audit history')
for actor in (exw_noview, exw_unsupported):
    actor.call('/export-files/' + str(exw_file), expect=403)
    actor.call(exw_path + '/export', expect=403)
    actor.call(exw_path + '/history/' + str(exw_detail()['history'][0]['id']), expect=403)
check(exw_unassigned.call(exw_path + '/export')[:2] == b'PK', 'Standalone approved history can be exported as a real workbook')

# Typed filters and manual-party statements cannot combine unrelated customers
# or accidentally include existing linked-company invoices.
exw_open_number = 'EX-MANUAL-PARTY-OPEN'
exw_other_number = 'EX-MANUAL-OTHER-CUSTOMER'
exw_unassigned.call('/exports', {**exw_body, 'invoice_number': exw_open_number,
                                'invoice_amount': '30.00', 'receipts': []})
exw_unassigned.call('/exports', {**exw_body, 'invoice_number': exw_other_number,
                                'party_name': 'Another independent customer',
                                'invoice_amount': '45.00', 'receipts': []})
exw_standalone_rows = ex_writer.call('/exports?scope=standalone')['records']
check(exw_standalone_rows and all(row['company_id'] is None for row in exw_standalone_rows),
      'Standalone scope filter returns only NULL-company invoices')
exw_company_rows = ex_writer.call('/exports?scope=company')['records']
check(any(row['id'] == ex_id for row in exw_company_rows) and all(row['company_id'] is not None for row in exw_company_rows),
      'Company filter preserves existing linked invoices and excludes standalone records')
exw_approved_rows = ex_writer.call('/exports?scope=standalone&status=Approved')['records']
check(exw_approved_rows and all(row['workflow_status'] == 'Approved' for row in exw_approved_rows),
      'Approval status filter returns only genuinely approved invoice revisions')
exw_statement_query = urllib.parse.urlencode({'party_type': 'manual', 'party_name': '  ' + exw_body['party_name'].upper() + '  '})
exw_statement_cells = exs_workbook_literals(exw_unassigned.call('/exports/statement?' + exw_statement_query))
check(exw_open_number in exw_statement_cells and exw_other_number not in exw_statement_cells and ex_body['invoice_number'] not in exw_statement_cells,
      'Manual customer statement matches trimmed names case-insensitively and excludes other customers/company rows')
for exw_query in (
    'scope=unknown', 'status=Pending', 'party_type=unknown',
    'party_type=manual&party_id=' + str(buyer_a), 'party_type=buyer&party_name=Invalid',
    'company_id=-1', 'party_id=invalid', 'currency=INVALID', 'fy=2026-99',
    'party_type=manual&party_name=Bad%0AName',
):
    exw_unassigned.call('/exports?' + exw_query, expect=400)
exw_unassigned.call('/exports/statement?party_type=manual', expect=400)
ex_reader.call('/exports/statement?' + exw_statement_query, expect=403)


def exw_identity(role='Finance', uid=ex_team_b_id, exports=True, **overrides):
    return {'id': uid, 'role': role, 'scope': 'team', 'companies': [], 'teams': [],
            'lines': [], 'orders': [], 'units': [], 'buyer_ids': [], 'production_areas': [],
            'restrict_buyers': True, 'view_orders': False, 'view_setup': False,
            'exports': exports, **overrides}


def exw_runtime_sql(statement, identity):
    raw = json.dumps(identity).replace("'", "''")
    return sql('BEGIN; SET LOCAL ROLE euroerp_runtime; '
               "SET LOCAL erp.identity = '" + raw + "'; SET LOCAL erp.service = ''; " +
               statement + '; ROLLBACK;', name)


def exw_rls_counts(identity):
    output = exw_runtime_sql('SELECT json_build_array(' + ','.join([
        f'(SELECT count(*) FROM export_invoices WHERE id={exw_id})',
        *[f'(SELECT count(*) FROM {table} WHERE invoice_id={exw_id})'
          for table in ('export_history', 'export_files', 'export_discounts', 'export_comments')],
        f'(SELECT count(*) FROM export_invoices WHERE id={ex_id})',
    ]) + ')', identity)
    return json.loads(next(line for line in output.splitlines() if line.startswith('[')))


for exw_role in ('Finance', 'Finance Head', 'MD', 'Super Admin'):
    exw_counts = exw_rls_counts(exw_identity(role=exw_role))
    # Preserve the existing database-level Super Admin company bypass; all
    # non-admin identities below have no company assignment and stay isolated.
    exw_linked_expected = 1 if exw_role == 'Super Admin' else 0
    check(exw_counts[0] == 1 and all(n > 0 for n in exw_counts[1:5]) and exw_counts[-1] == exw_linked_expected,
          'Runtime RLS shares standalone parent and children while retaining existing linked-company rules for ' + exw_role)
for exw_role in ('Marketing', 'Production', 'Quality', 'CEO'):
    check(exw_rls_counts(exw_identity(role=exw_role)) == [0] * 6,
          'Forged export visibility cannot give unsupported role standalone SQL access: ' + exw_role)
check(exw_rls_counts(exw_identity(exports=False)) == [0] * 6,
      'Removing export visibility hides standalone invoices and every child in RLS')
exw_hidden_update = exw_runtime_sql(f'UPDATE export_invoices SET invoice_amount=1 WHERE id={exw_id} RETURNING id',
                                  exw_identity(role='Marketing'))
check('UPDATE 0' in exw_hidden_update, 'Unsupported role cannot update hidden standalone invoice through runtime SQL')
exw_linked_update = exw_runtime_sql(f'UPDATE export_invoices SET invoice_amount=1 WHERE id={ex_id} RETURNING id', exw_identity())
check('UPDATE 0' in exw_linked_update, 'Standalone access cannot update an unassigned company-linked invoice through SQL')
for statement, message in (
    (f"INSERT INTO export_invoices(company_id,party_type,party_id,party_name,invoice_number,currency,invoice_amount,created_by) VALUES(NULL,'manual',NULL,'Denied','EX-SQL-DENIED','USD',1,{exw_unassigned_id})", 'row-level security'),
    (f"INSERT INTO export_comments(invoice_id,author_id,note) VALUES({exw_id},{exw_unassigned_id},'Denied SQL comment')", 'row-level security'),
):
    try:
        exw_runtime_sql(statement, exw_identity(role='Marketing'))
        raise AssertionError('Runtime SQL bypassed standalone role isolation')
    except subprocess.CalledProcessError as error:
        check(message in error.stderr.lower(), 'RLS rejects direct unsupported-role writes to standalone parent or comments')
try:
    exw_runtime_sql(f'UPDATE export_invoices SET order_id={po_a} WHERE id={exw_id}', exw_identity())
    raise AssertionError('Standalone database constraints accepted a linked order')
except subprocess.CalledProcessError as error:
    check('check constraint' in error.stderr.lower(), 'Database prevents smuggling a linked production order into a standalone row')
try:
    sql(f"UPDATE export_comments SET note='Rewritten history' WHERE invoice_id={exw_id}", name)
    raise AssertionError('Export comments were mutable')
except subprocess.CalledProcessError:
    check(True, 'Export correction comments remain immutable even through direct owner SQL')
print('Export workflow HTTP/RLS checks passed: ' + str(checks - exw_start), flush=True)
