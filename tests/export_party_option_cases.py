# Invoice-only company names. Loaded after export_workflow_cases.py, within the
# disposable test DB and SMTP-disabled server; never use against live records.
assert name.startswith('erp_test_') and runtime_role == name + '_api'
check(not env.get('SMTP_HOST'), 'Invoice option tests cannot send real employee email')
exp_start = checks


def exp_names(actor=exw_unassigned):
    return actor.call('/exports/options')['standalone_parties']


def exp_company_fingerprint():
    return sql("SELECT json_build_array((SELECT md5(coalesce(string_agg(to_jsonb(c)::text,'' ORDER BY c.id),'')) FROM companies c),"
               "(SELECT md5(coalesce(string_agg(to_jsonb(c)::text,'' ORDER BY c.id),'')) FROM company_setups c),"
               "(SELECT count(*) FROM company_setup_history))", name)


exp_before_companies = exp_company_fingerprint()
exp_existing = exp_names()
check(all((row['id'] is None or isinstance(row['id'], int)) and isinstance(row['name'], str) for row in exp_existing),
      'Invoice dropdown provides saved identifiers and compatible legacy display names')
exp_seed_names = {'EURO TROUSERS MFG,CO, (FZC)', 'P & C RETAIL UK LTD', 'ERRY ELLIS EUROPE LIMITED'}
check(exp_seed_names <= {row['name'] for row in exp_existing if row['id'] is not None},
      'All three user-confirmed invoice company names are seeded exactly as supplied')
check(any(row['id'] is None and row['name'] == exw_body['party_name'] for row in exp_existing),
      'Approved legacy manual-party names remain selectable without creating Company Setup records')
exp_name = 'QA Export Option Client'
exp_added = exw_unassigned.call('/exports/parties', {'name': '  QA   Export Option Client  '})
check(exp_added['id'] > 0 and exp_added['name'] == exp_name and exp_added['created'] is True,
      'Adding an invoice company persists a normalized display name')
for actor in (ex_writer, ex_finance, ex_team_b, ex_line, ex_buyer, ex_reader, ex_final, exw_sa):
    check(any(row['id'] == exp_added['id'] and row['name'] == exp_name for row in exp_names(actor)),
          'Invoice-only company names are shared across authorized Finance and management scopes')
for index, actor in enumerate((ex_writer, ex_finance, ex_team_b, exw_sa)):
    added = actor.call('/exports/parties', {'name': 'QA Export Role Option ' + str(index)})
    check(any(row['id'] == added['id'] for row in exp_names()),
          'Finance, Finance Head and Super Admin can persist an invoice company option')
for actor in (ex_reader, ex_final, exw_md, exw_noview, exw_unsupported, merch_b):
    actor.call('/exports/parties', {'name': 'Unauthorized invoice company'}, 403)
Client().call('/exports/parties', {'name': 'Anonymous invoice company'}, 401)
exw_unassigned.call('/exports/parties', {'name': 'Missing CSRF company'}, 403, {'X-CSRF-Token': ''})
for invalid in ({}, {'name': ''}, {'name': '   '}, {'name': None}, {'name': 123},
                {'name': 'A' * 161}, {'name': 'Invalid\ncompany'}, {'name': 'Invalid\x00company'},
                {'name': 'Unexpected parent', 'company_id': opscompany},
                {'name': 'Forged creator', 'created_by': exw_sa_id}):
    exw_unassigned.call('/exports/parties', invalid, 400)
exp_audit_before_duplicate = sql('SELECT count(*) FROM audit', name)
exp_duplicate = exw_unassigned.call('/exports/parties', {'name': 'qa   export option client'})
check(exp_duplicate['id'] == exp_added['id'] and exp_duplicate['name'] == exp_name and exp_duplicate['created'] is False,
      'A normalized duplicate reuses the existing invoice company selection safely')
check(sql('SELECT count(*) FROM audit', name) == exp_audit_before_duplicate,
      'Reusing an existing company option does not emit a duplicate creation audit')
check(sql("SELECT count(*) FROM export_party_options WHERE lower(name)='qa export option client'", name) == '1',
      'Case and whitespace variants cannot create duplicate invoice company names')

# Retries and concurrent requests cannot add multiple copies of the same name.
exp_idempotency = secrets.token_hex(16)
exw_unassigned.call('/exports/parties', {'name': 'QA Idempotent Company'}, headers={'Idempotency-Key': exp_idempotency})
exp_retry = exw_unassigned.call('/exports/parties', {'name': 'QA Idempotent Company'}, 409,
                               {'Idempotency-Key': exp_idempotency})
check(exp_retry['code'] == 'action_already_submitted' and
      sql("SELECT count(*) FROM export_party_options WHERE name='QA Idempotent Company'", name) == '1',
      'Duplicate submission key creates only one invoice company option')


def exp_add_race(username):
    actor = Client(username)
    request = urllib.request.Request(base + '/api/exports/parties',
        data=json.dumps({'name': 'QA Concurrent Company'}).encode(),
        headers={'Origin': os.getenv('COMPANY_TEST_ORIGIN', base), 'X-CSRF-Token': actor.csrf,
                 'Content-Type': 'application/json'})
    try:
        return actor.opener.open(request, timeout=15).status
    except urllib.error.HTTPError as error:
        return error.code


with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
    exp_race_codes = sorted(pool.map(exp_add_race, ['qa.export.writer', 'qa.export.head']))
check(exp_race_codes == [200, 200] and
      sql("SELECT count(*) FROM export_party_options WHERE name='QA Concurrent Company'", name) == '1',
      'Concurrent Add more requests safely converge on one persisted company option')
check(exp_company_fingerprint() == exp_before_companies,
      'Invoice company options do not create or change Company Setup, operational companies or company history')
check(int(sql(f"SELECT count(*) FROM audit WHERE actor_id={exw_unassigned_id} AND detail LIKE '%QA Export Option Client%'", name)) >= 1,
      'Adding an invoice company name records the acting employee in audit history')
exp_legacy_before = sql(f'SELECT md5(to_jsonb(d)::text) FROM export_invoices d WHERE id={exw_own_id}', name)
exp_saved_legacy = exw_unassigned.call('/exports/parties', {'name': exw_body['party_name']})
exp_legacy_choices = [row for row in exp_names() if row['name'].lower() == exw_body['party_name'].lower()]
check(len(exp_legacy_choices) == 1 and exp_legacy_choices[0]['id'] == exp_saved_legacy['id'],
      'Saving an existing legacy name replaces its temporary choice without duplicate dropdown entries')
check(sql(f'SELECT md5(to_jsonb(d)::text) FROM export_invoices d WHERE id={exw_own_id}', name) == exp_legacy_before,
      'Saving a legacy name never rewrites its existing invoice')
exp_invoice_id = exw_unassigned.call('/exports', {**exw_body, 'party_name': exp_name,
                                                'invoice_number': 'EX-SAVED-COMPANY-OPTION'})['id']
exp_invoice = exw_unassigned.call('/exports/' + str(exp_invoice_id))['record']
check(exp_invoice['party_name'] == exp_name and all(exp_invoice.get(key) is None
      for key in ('company_id', 'party_id', 'buyer_id', 'party_setup_id', 'order_id')),
      'Selecting an invoice-only company retains the standalone invoice boundary')


def exp_rls_count(identity):
    output = exw_runtime_sql(f'SELECT json_build_array(count(*)) FROM export_party_options WHERE id={exp_added["id"]}', identity)
    return json.loads(next(line for line in output.splitlines() if line.startswith('[')))[0]


for role in ('Finance', 'Finance Head', 'MD', 'Super Admin'):
    check(exp_rls_count(exw_identity(role=role)) == 1,
          'Option RLS allows export-enabled Finance/management reads without company assignments: ' + role)
for identity in (exw_identity(exports=False), exw_identity(role='Marketing'), exw_identity(role='CEO')):
    check(exp_rls_count(identity) == 0, 'Option RLS rejects missing export visibility or unsupported role')
for identity in (exw_identity(exports_edit=False), exw_identity(role='MD', exports_edit=True),
                 exw_identity(role='Marketing', exports_edit=True), exw_identity(exports=False, exports_edit=True)):
    try:
        exw_runtime_sql(f"INSERT INTO export_party_options(name,created_by) VALUES('QA SQL Denied Option',{identity['id']})", identity)
        raise AssertionError('Option INSERT bypassed explicit edit visibility or role requirements')
    except subprocess.CalledProcessError as error:
        check('row-level security' in error.stderr.lower(), 'Direct SQL option insertion enforces both role and explicit export edit capability')
try:
    exw_runtime_sql(f"INSERT INTO export_party_options(name,created_by) VALUES('QA SQL Forged Creator',{exw_sa_id})",
                    exw_identity(uid=exw_unassigned_id, exports_edit=True))
    raise AssertionError('Option creator attribution was forgeable through SQL')
except subprocess.CalledProcessError as error:
    check('row-level security' in error.stderr.lower(), 'Runtime cannot attribute a company option to a different employee')
for role in ('Finance', 'Finance Head', 'Super Admin'):
    output = exw_runtime_sql(f"INSERT INTO export_party_options(name,created_by) VALUES('QA SQL Allowed Option',{exw_unassigned_id}) RETURNING id",
                             exw_identity(role=role, uid=exw_unassigned_id, exports_edit=True))
    check('INSERT 0 1' in output, 'Option RLS allows explicit Finance/Super Admin edit capability: ' + role)
for verb in ('UPDATE', 'DELETE'):
    statement = (f"UPDATE export_party_options SET name='Rewritten option' WHERE id={exp_added['id']}" if verb == 'UPDATE'
                 else f"DELETE FROM export_party_options WHERE id={exp_added['id']}")
    try:
        exw_runtime_sql(statement, exw_identity(role='Super Admin', exports_edit=True))
        raise AssertionError('Runtime could rewrite/remove a persisted invoice company option')
    except subprocess.CalledProcessError as error:
        check('permission denied' in error.stderr.lower(), 'Runtime cannot ' + verb.lower() + ' invoice company option history')
check(exp_company_fingerprint() == exp_before_companies,
      'Option security checks preserve every pre-existing company record')
print('Export company option HTTP/RLS checks passed: ' + str(checks - exp_start), flush=True)
