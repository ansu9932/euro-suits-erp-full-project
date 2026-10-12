"""Company-only Additional field access and concurrency in the disposable DB."""
def company_additional_field_cases():
    import copy
    def reset_fixture_request_budgets():
        # Independent scenarios share these synthetic accounts. Give this case
        # its own normal request budget in the suite's disposable Redis only;
        # retain sessions, login/OTP/security counters and production limits.
        assert name.startswith('erp_test_') and redis_created
        fixture_ids = sql("SELECT id FROM users WHERE username IN ('admin','finance','md','limited.finance')", name).splitlines()
        keys = [prefix + user_id for user_id in fixture_ids for prefix in ('session-requests:', 'write:')]
        if keys:
            run('docker', 'exec', name, 'redis-cli', 'DEL', *keys)
    reset_fixture_request_budgets()
    # Exercise the additive upgrade against temporary schema20-shaped tables.
    migration = (ROOT/'backend/internal/app/migration_v21.sql').read_text()
    prepare = """BEGIN; SET LOCAL search_path=pg_temp,public;
CREATE TEMP TABLE schema_versions(version int PRIMARY KEY);
INSERT INTO schema_versions VALUES(20);
CREATE TEMP TABLE company_setups(id bigint PRIMARY KEY,deleted_at timestamptz,assigned_to bigint,status text);
INSERT INTO company_setups VALUES(1,NULL,1,'Draft'),(2,NULL,1,'Draft');
CREATE TEMP TABLE company_field_definitions(id bigserial PRIMARY KEY,label text NOT NULL,country text NOT NULL,section text NOT NULL,
 CONSTRAINT company_field_definitions_label_country_section_key UNIQUE(label,country,section));
INSERT INTO company_field_definitions(label,country,section) VALUES('Existing global','GB','Additional'),('Existing KYC','IN','KYC');
"""
    migrated = sql(prepare + migration + "SELECT json_build_object('globals',(SELECT count(*) FROM company_field_definitions WHERE company_setup_id IS NULL),'version',(SELECT max(version) FROM schema_versions))::text; ROLLBACK;", name)
    summary = json.loads(next(line for line in migrated.splitlines() if line.startswith('{')))
    check(summary == {'globals': 2, 'version': 21}, 'Schema21 upgrade preserves existing global definitions')
    for invalid in ["INSERT INTO company_field_definitions(label,country,section) VALUES('Existing global','GB','Additional');",
                    "INSERT INTO company_field_definitions(label,country,section,company_setup_id) VALUES('Bad local','GB','KYC',1);",
                    "INSERT INTO company_field_definitions(label,country,section,company_setup_id) VALUES('Same','GB','Additional',1),('SAME','GB','Additional',1);"]:
        try:
            sql(prepare + migration + invalid + 'ROLLBACK;', name)
            raise AssertionError('Schema21 constraint accepted invalid field definitions')
        except subprocess.CalledProcessError:
            check(True, 'Schema21 preserves global uniqueness and constrains company-only definitions')
    header = {'legal_name': 'ADDITIONAL FIELD COMPANY A', 'category': 'Buyer', 'company_type': 'Retailer', 'country': 'GB', 'city': 'London'}
    aid = finance.call('/company-setup', header)['id']; apath = '/company-setup/' + str(aid)
    bheader = {**header, 'legal_name': 'ADDITIONAL FIELD COMPANY B'}
    bid = finance.call('/company-setup', bheader)['id']; bpath = '/company-setup/' + str(bid)
    foreign = limited.call('/company-setup', {**header, 'legal_name': 'OTHER FINANCE ADDITIONAL COMPANY'})['id']
    endpoint = apath + '/fields'
    def detail(): return finance.call(apath)
    def body(label='Company reference', **values):
        return {'version': detail()['version'], 'label': label, 'input_type': 'text', 'options': [], 'required': False, 'document_required': False, 'expiry_enabled': False, **values}
    initial = detail()
    finance.call(endpoint, body(version=0), 409)
    for actor, expected in [(Client(), 401), (md, 403), (admin, 403), (defaultuser, 403), (noaccess, 403), (limited, 404)]:
        actor.call(endpoint, body(), expected)
    finance.call('/company-setup/' + str(foreign) + '/fields', body(), 404)
    finance.call('/company-fields', {'label': 'Blocked global', 'section': 'Additional', 'country': 'GB', 'categories': ['Buyer'], 'input_type': 'text'}, 403)
    for extra in [{'section': 'KYC'}, {'company_setup_id': bid}, {'country': '*'}, {'categories': ['Supplier']}, {'business_modes': []}, {'active': True}]:
        finance.call(endpoint, body(**extra), 400)
    for extra in [{'input_type': 'script'}, {'input_type': 'select', 'options': []}, {'input_type': 'select', 'options': ['same', 'same']}, {'input_type': 'select', 'options': ['A']}, {'options': ['not dropdown']}, {'options': 'A,B'}, {'required': 'yes'}, {'label': ''}]:
        finance.call(endpoint, body(**extra), 400)
    check(detail()['version'] == initial['version'], 'Rejected Additional definitions do not modify company version')
    original_profile = copy.deepcopy(initial['profile'])
    request = body('Unique company reference', input_type='select', options=['First', 'Second'], required=True)
    created = finance.call(endpoint, request); field_id = created['id']
    check(created['version'] == initial['version'] + 1, 'Additional field creation increments company version')
    check(created['field']['company_setup_id'] == aid and created['field']['section'] == 'Additional', 'Server fixes company and Additional section')
    check(created['field']['country'] == 'GB' and created['field']['categories'] == ['Buyer'], 'Server fixes initial field applicability metadata')
    check(detail()['profile'] == original_profile, 'Adding a definition never overwrites draft profile')
    check(any('Unique company reference is required' in issue for issue in detail()['summary']['issues']), 'Company-only required field participates in approval validation')
    check(detail()['history'][0]['action'] == 'Additional field added', 'Company-specific field creation is audited')
    finance.call(endpoint, request, 409)
    finance.call(endpoint, body('UNIQUE COMPANY REFERENCE'), 400)
    check(any(f['id'] == field_id for f in finance.call('/company-setup')['fields']), 'Assigned Finance can read its local definition')
    check(all(f['id'] != field_id for f in limited.call('/company-setup')['fields']), 'Other Finance cannot see the local definition')
    check(all(item['label'] != 'Unique company reference' for item in finance.call(bpath)['summary']['checklist']), 'Company-only field does not appear on another owned company')
    other = finance.call(bpath + '/fields', {**body(), 'version': finance.call(bpath)['version'], 'label': 'Unique company reference'})
    check(other['id'] != field_id and other['field']['company_setup_id'] == bid, 'Another company can use the same local field label independently')
    finance.call(bpath, {'action': 'save', 'version': finance.call(bpath)['version'], **bheader, 'profile': {'fields': [{'id': field_id, 'value': 'First'}]}}, 400)
    profile = {'address': 'Synthetic address', 'email': 'qa@example.test', 'phone': '000', 'sample_address': 'Synthetic sample address', 'fields': [{'id': field_id, 'value': 'First'}]}
    # An ordinary country change retains the explicitly company-scoped field.
    changed_header = {**header, 'country': 'US', 'city': 'Boston'}
    finance.call(apath, {'action': 'save', 'version': detail()['version'], **changed_header, 'profile': profile})
    check(any(x['label'] == 'Unique company reference' for x in detail()['summary']['checklist']), 'Local field stays with its company after country changes')
    for definition in finance.call('/company-setup')['fields']:
        if definition['company_setup_id'] is None and definition['section'] == 'KYC' and definition['country'] in ('*', 'US') and 'Buyer' in definition['categories']:
            profile['fields'].append({'id': definition['id'], 'not_applicable': True, 'reason': 'Synthetic test company', 'reminder_enabled': False})
    finance.call(apath, {'action': 'save', 'version': detail()['version'], **changed_header, 'profile': profile})
    # Two requests with the same version cannot both add a field.
    version = detail()['version']
    def race(label):
        payload = {'version': version, 'label': label, 'input_type': 'text', 'options': []}
        req = urllib.request.Request(base + '/api' + endpoint, data=json.dumps(payload).encode(), headers={'Origin': os.getenv('COMPANY_TEST_ORIGIN', base), 'X-CSRF-Token': finance.csrf, 'Content-Type': 'application/json'})
        try: response = finance.opener.open(req, timeout=10)
        except urllib.error.HTTPError as exc: response = exc
        response.read(); return response.status
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        results = sorted(pool.map(race, ['Concurrent field one', 'Concurrent field two']))
    check(results == [200, 409], 'Concurrent Additional additions share optimistic company locking')
    check(detail()['version'] == version + 1, 'Concurrent additions create exactly one version change')
    # Direct runtime SQL must also hide the foreign company's local definition.
    foreign_user_id = limited.call('/session')['user']['id']
    identity = json.dumps({'id': foreign_user_id, 'role': 'Finance Head', 'view_setup': True}).replace("'", "''")
    rls_result = sql(f"BEGIN; SET LOCAL ROLE {runtime_role}; SELECT set_config('erp.identity','{identity}',true); SELECT count(*) FROM company_field_definitions WHERE id={field_id}; ROLLBACK;", name)
    check(rls_result.splitlines()[-2] == '0', 'Database RLS hides company-specific definitions from other Finance')
    finance.call(apath, {'action': 'submit', 'version': detail()['version']})
    finance.call(endpoint, body('Pending cannot change'), 403)
    md.call(apath, {'action': 'approve', 'version': detail()['version']})
    finance.call(endpoint, body('Approved cannot change'), 403)
    finance.call(apath, {'action': 'amend', 'version': detail()['version']})
    finance.call(endpoint, body('Amendment local field'))
    # Removing edit permission must revoke the session and deny newly signed-in writes.
    revoked_id = account('additional.revoked', 'Finance Head', ['view', 'create', 'edit'])
    revoked = Client('additional.revoked')
    revoked_header = {**header, 'legal_name': 'REVOKED ADDITIONAL COMPANY'}
    revoked_company = revoked.call('/company-setup', revoked_header)['id']
    revoked_path = '/company-setup/' + str(revoked_company)
    revoked_before = revoked.call(revoked_path)['version']
    # Create depends on edit in the existing permission model; revoke both
    # while retaining read access to verify that this draft stays unchanged.
    permissions(revoked_id, ['view'])
    revoked.call(revoked_path + '/fields', {'version': revoked_before, 'label': 'Denied after revoke', 'input_type': 'text'}, 401)
    revoked = Client('additional.revoked')
    revoked.call(revoked_path + '/fields', {'version': revoked_before, 'label': 'Denied after sign in', 'input_type': 'text'}, 403)
    check(revoked.call(revoked_path)['version'] == revoked_before, 'Revoked edit permission cannot add fields or change company version')
    # Global Super Admin additions remain available and keep null company scope.
    global_id = admin.call('/company-fields', {'label': 'Global Additional regression', 'section': 'Additional', 'country': 'GB', 'categories': ['Buyer'], 'input_type': 'text'})['id']
    global_field = next(f for f in admin.call('/company-setup')['fields'] if f['id'] == global_id)
    check(global_field['company_setup_id'] is None, 'Super Admin global field controls retain their original scope')
    for company_id, legal_name in [(aid, header['legal_name']), (bid, bheader['legal_name']), (foreign, 'OTHER FINANCE ADDITIONAL COMPANY'), (revoked_company, revoked_header['legal_name'])]:
        company_path = '/company-setup/' + str(company_id)
        admin.call(company_path + '/delete', {'version': admin.call(company_path)['version'], 'confirmation_name': legal_name, 'password': password})
    check(all(f['id'] != field_id for f in finance.call('/company-setup')['fields']), 'Deleted company local definitions no longer appear')
    reset_fixture_request_budgets()
company_additional_field_cases()
