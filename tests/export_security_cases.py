# Supplemental real HTTP/RLS checks, loaded after export_cases.py by the
# disposable company_setup.py harness. Never run against a normal database.
import xml.etree.ElementTree as EXS_XML

assert name.startswith('erp_test_') and runtime_role == name + '_api'
exs_start = checks


def exs_setup_export_user(username):
    designation = admin.call('/access-designations', {
        'name': 'Export security ' + username, 'department': 'Export QA',
        'role': 'Finance Head', 'features': ['view', 'create', 'edit', 'submit'],
        'permissions': export_grants,
    })['id']
    uid = admin.call('/setup-users', {
        'username': username, 'name': 'Export security reviewer', 'password': password,
        'designation_id': designation, 'inherit_designation': True,
        'scope': 'company', 'companies': [opscompany], 'active': True,
    })['id']
    return uid, Client(username)


exs_owner_id, exs_owner = exs_setup_export_user('qa.export.master.owner')
exs_other_id, exs_other = exs_setup_export_user('qa.export.master.other')
exs_header = {
    'legal_name': 'EXPORT QA ASSIGNED BUYER', 'category': 'Buyer',
    'company_type': 'Retailer', 'country': 'GB', 'city': 'London',
}
exs_master = exs_owner.call('/company-setup', exs_header)['id']
exs_setup_path = '/company-setup/' + str(exs_master)
exs_profile = copy.deepcopy(buyerprofile)
exs_owner.call(exs_setup_path, {
    **exs_header, 'action': 'save', 'version': 1, 'profile': exs_profile,
})
exs_owner.call(exs_setup_path, {
    'action': 'submit', 'version': exs_owner.call(exs_setup_path)['version'],
})
admin.call(exs_setup_path, {
    'action': 'approve', 'version': admin.call(exs_setup_path)['version'],
})
exs_owner_options = exs_owner.call('/exports/options?company_id=' + str(opscompany))
check(any(p['party_type'] == 'company_setup' and p['id'] == exs_master
          for p in exs_owner_options['parties']),
      'Assigned approved Buyer master is offered to its authorised Finance Head')
for actor in (exs_other, ex_writer):
    options = actor.call('/exports/options?company_id=' + str(opscompany))
    check(not any(p['party_type'] == 'company_setup' and p['id'] == exs_master
                  for p in options['parties']),
          'Unassigned Finance Head and Finance without setup visibility cannot enumerate Buyer master')

exs_body = {
    'company_id': opscompany, 'party_type': 'company_setup', 'party_id': exs_master,
    'order_id': None, 'invoice_number': 'EX-ASSIGNED-MASTER-001', 'currency': 'USD',
    'invoice_amount': '10.00', 'profile': {}, 'receipts': [], 'ebrcs': [],
}
exs_id = exs_owner.call('/exports', exs_body)['id']
exs_path = '/exports/' + str(exs_id)
exs_version = lambda: exs_owner.call(exs_path)['record']['version']
exs_file = exs_owner.call(
    exs_path + '/files?kind=invoice&version=' + str(exs_version()), multipart,
    headers={'Content-Type': 'multipart/form-data; boundary=' + boundary},
)['id']
exs_owner.call(exs_path + '/discount', {
    'version': exs_version(), 'action': 'request', 'amount': '1.00',
    'reason': 'Synthetic scope check',
})
exs_history = exs_owner.call(exs_path)['history'][0]['id']
for actor in (exs_other, ex_writer):
    actor.call(exs_path, expect=404)
    actor.call(exs_path, {**exs_body, 'version': exs_version()}, 404)
    actor.call('/exports', {**exs_body, 'invoice_number': 'EX-DENIED-MASTER'},
               404 if actor is exs_other else 403)
    actor.call(exs_path + '/history/' + str(exs_history), expect=404)
    actor.call('/export-files/' + str(exs_file), expect=404)
    actor.call(exs_path + '/export', expect=404)
    check(not any(r['id'] == exs_id for r in actor.call('/exports')['records']),
          'Master-backed invoices inherit setup visibility in register and detail')
    for endpoint in [
        '/exports/export?company_id=' + str(opscompany),
        '/exports/statement?company_id=' + str(opscompany) +
        '&party_type=company_setup&party_id=' + str(exs_master),
    ]:
        with zipfile.ZipFile(io.BytesIO(actor.call(endpoint))) as archive:
            check(all(exs_body['invoice_number'].encode() not in archive.read(entry)
                      for entry in archive.namelist() if entry.endswith('.xml')),
                  'Register and statement workbooks do not leak hidden Buyer-master invoices')


def exs_rls_counts(uid, role='Finance Head', setup=True, exports=True):
    # Use the same transaction-local identity keys as the production wrapper,
    # with the runtime role, so SQL policies are tested independently of routes.
    identity = {
        'id': uid, 'role': role, 'scope': 'company', 'companies': [opscompany],
        'teams': [], 'lines': [], 'orders': [], 'units': [], 'buyer_ids': [],
        'production_areas': [], 'restrict_buyers': False, 'view_orders': False,
        'view_setup': setup, 'exports': exports,
    }
    raw = json.dumps(identity).replace("'", "''")
    result = sql(
        'BEGIN; SET LOCAL ROLE euroerp_runtime; '
        "SET LOCAL erp.identity = '" + raw + "'; SET LOCAL erp.service = ''; "
        'SELECT json_build_array('
        f'(SELECT count(*) FROM export_invoices WHERE id={exs_id}),'
        f'(SELECT count(*) FROM export_history WHERE invoice_id={exs_id}),'
        f'(SELECT count(*) FROM export_files WHERE invoice_id={exs_id}),'
        f'(SELECT count(*) FROM export_discounts WHERE invoice_id={exs_id})); '
        'ROLLBACK;', name,
    )
    return json.loads(next(line for line in result.splitlines() if line.startswith('[')))


exs_counts = exs_rls_counts(exs_owner_id)
check(exs_counts[0] == 1 and all(n > 0 for n in exs_counts[1:]),
      'Runtime RLS permits assigned master invoice and its document/history/discount children')
check(exs_rls_counts(exs_other_id) == [0, 0, 0, 0],
      'Runtime RLS denies unassigned master invoice and every child table')
check(exs_rls_counts(ex_writer_id, 'Finance', False) == [0, 0, 0, 0],
      'Runtime RLS does not expose setup-backed exports to ordinary Finance')
check(exs_rls_counts(exs_owner_id, exports=False) == [0, 0, 0, 0],
      'Runtime RLS requires export visibility even for assigned Buyer owner')

# Approved discounts stay immutable; a subsequent financial edit cannot make
# the retained approved reduction exceed the newly calculated shortfall.
exs_before = ex_record()
check(exs_before['balance']['approved_discount'] == '9.00',
      'Approved-discount edit regression uses a real two-stage approved discount')
exs_reduced = copy.deepcopy(ex_body)
exs_reduced['invoice_amount'] = '999.99'
ex_update(exs_reduced, expect=400)
exs_extra_receipt = copy.deepcopy(ex_body)
exs_extra_receipt['receipts'].append({'currency': 'USD', 'payment_received': '0.01'})
ex_update(exs_extra_receipt, expect=400)
exs_after = ex_record()
check(exs_after['version'] == exs_before['version'] and
      exs_after['balance'] == exs_before['balance'],
      'Rejected edit preserves the approved discount, prior facts and exact settled balance')

# Spreadsheet formula-looking business text must remain inert text, with no
# formula nodes or outbound relationships in any generated workbook.
exs_literals = ['=1+1', '+SUM(1,1)', '-1+2', '@SUM(1,1)', '<tag>&"quoted"']
exs_formula = copy.deepcopy(ex_body)
exs_formula.update(invoice_number=exs_literals[0], receipts=[], ebrcs=[])
exs_formula['profile'].update(zip(
    ['export_reference', 'shipping_bill_number', 'transport_document_number', 'clearance_note'],
    exs_literals[1:],
))
exs_formula_id = ex_writer.call('/exports', exs_formula)['id']


def exs_workbook_literals(document):
    main = '{http://schemas.openxmlformats.org/spreadsheetml/2006/main}'
    texts = []
    with zipfile.ZipFile(io.BytesIO(document)) as archive:
        check(not any(p.startswith('xl/externalLinks/') for p in archive.namelist()),
              'Generated Excel contains no external-link parts')
        shared = []
        if 'xl/sharedStrings.xml' in archive.namelist():
            tree = EXS_XML.fromstring(archive.read('xl/sharedStrings.xml'))
            shared = [''.join(n.itertext()) for n in tree.findall(main + 'si')]
        for filename in archive.namelist():
            if filename.endswith('.rels'):
                tree = EXS_XML.fromstring(archive.read(filename))
                check(not any(n.attrib.get('TargetMode', '').lower() == 'external'
                              for n in tree), 'Generated Excel has no external relationships')
            if not filename.startswith('xl/worksheets/') or not filename.endswith('.xml'):
                continue
            tree = EXS_XML.fromstring(archive.read(filename))
            check(not list(tree.iter(main + 'f')), 'Generated Excel stores no executable formulas')
            for cell in tree.iter(main + 'c'):
                kind = cell.attrib.get('t')
                if kind == 'inlineStr':
                    texts.append(''.join(n.text or '' for n in cell.iter(main + 't')))
                elif kind == 's':
                    texts.append(shared[int(cell.find(main + 'v').text)])
    return texts


for endpoint in [
    '/exports/' + str(exs_formula_id) + '/export',
    '/exports/export?company_id=' + str(opscompany),
    '/exports/statement?company_id=' + str(opscompany) + '&party_type=buyer&party_id=' + str(buyer_a),
]:
    exs_cells = exs_workbook_literals(ex_writer.call(endpoint))
    check(any(t in (exs_literals[0], "'" + exs_literals[0]) for t in exs_cells),
          'Formula-looking invoice number remains a literal text cell in every export')
    if endpoint.endswith('/' + str(exs_formula_id) + '/export'):
        check(all(any(t in (value, "'" + value) for t in exs_cells)
                  for value in exs_literals),
              'Detailed workbook preserves formula prefixes and XML punctuation as literal text')

print('Export security HTTP/RLS checks passed: ' + str(checks - exs_start), flush=True)
