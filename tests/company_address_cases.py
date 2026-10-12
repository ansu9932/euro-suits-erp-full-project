"""Address regressions in company_setup.py's disposable database.

Provider success/failure responses are stubbed in Go unit tests. These HTTP
checks never make an external postal request or send messages.
"""

def company_address_cases():
    address_header = {
        'legal_name': 'ADDRESS REGRESSION COMPANY', 'category': 'Buyer',
        'company_type': 'Retailer', 'country': 'IN', 'city': 'Manual City',
        'business_flows': ['Domestic'],
    }
    address_input = {
        'address': 'Private address test street', 'state': 'Manual State',
        'postal_code': '560001',
    }
    before_grants = finance.call('/session')['user']['setup_grants']
    anonymous = Client()
    anonymous.call('/company-postal-lookup?country=IN&postal_code=560001', expect=401)
    for actor in [admin, md, defaultuser, noaccess]:
        actor.call('/company-postal-lookup?country=IN&postal_code=560001', expect=403)
    for query in ['country=GB&postal_code=560001', 'country=IN&postal_code=12345',
                  'country=IN&postal_code=000000', 'country=IN&postal_code=5600017',
                  'country=IN&postal_code=56000X', 'postal_code=560001']:
        finance.call('/company-postal-lookup?' + query, expect=400)
    finance_id = finance.call('/session')['user']['id']
    rate_key = 'company-postal:' + str(finance_id)
    run('docker', 'exec', name, 'redis-cli', 'SET', rate_key, '20', 'EX', '60')
    finance.call('/company-postal-lookup?country=IN&postal_code=560001', expect=429)
    run('docker', 'exec', name, 'redis-cli', 'DEL', rate_key)
    for actor in [admin, md, defaultuser, noaccess]:
        actor.call('/company-setup', {**address_header, **address_input}, 403)
    finance.call('/company-setup', {**address_header, 'profile': address_input}, 400)
    for key, limit in [('address', 2000), ('state', 100), ('postal_code', 30)]:
        for value in [None, False, 123, [], {}, 'x' * (limit + 1)]:
            finance.call('/company-setup', {**address_header, **address_input, key: value}, 400)
    address_id = finance.call('/company-setup', {**address_header, **address_input})['id']
    address_path = '/company-setup/' + str(address_id)
    created = finance.call(address_path)
    check(created['city'] == address_header['city'], 'Add company stores its city without a profile.city field')
    check(all(created['profile'][key] == value for key, value in address_input.items()),
          'Add company atomically persists registered address, state and PIN')
    raw = bytes.fromhex(sql(f"SELECT encode(private_data,'hex') FROM company_setups WHERE id={address_id}", name))
    check(b'Private address test street' not in raw and b'Manual State' not in raw,
          'New address fields are encrypted in the existing private profile')
    check('profile' not in md.call(address_path), 'MD address review preserves private-profile restriction')
    limited.call(address_path, expect=404)
    private_export = zipfile.ZipFile(io.BytesIO(finance.call(address_path + '/export'))).read('xl/worksheets/sheet1.xml')
    check(b'Private address test street' in private_export and b'Manual State' in private_export,
          'Private Excel export includes the added address and state')
    summary_export = zipfile.ZipFile(io.BytesIO(md.call(address_path + '/export'))).read('xl/worksheets/sheet1.xml')
    check(b'Private address test street' not in summary_export and b'Manual State' not in summary_export,
          'Summary Excel export does not expose private address fields')
    manual_header = {**address_header, 'city': 'User-entered city'}
    manual_profile = {**created['profile'], 'address': 'Manually corrected street',
                      'state': 'State absent from lookup', 'postal_code': '999999'}
    version = created['version']
    finance.call(address_path, {'action': 'save', 'version': version,
                               **manual_header, 'profile': manual_profile})
    saved = finance.call(address_path)
    check(saved['city'] == 'User-entered city' and saved['profile']['state'] == 'State absent from lookup'
          and saved['profile']['postal_code'] == '999999',
          'Manual city, state and unlisted PIN remain editable and persist on save')
    finance.call(address_path, {'action': 'save', 'version': version,
                               **manual_header, 'profile': manual_profile}, 409)
    for key, value in [('state', 123), ('postal_code', False), ('address', [])]:
        finance.call(address_path, {'action': 'save', 'version': saved['version'],
                                   **manual_header, 'profile': {**manual_profile, key: value}}, 400)
    finance.call(address_path, {'action': 'save', 'version': saved['version'],
                               **manual_header, 'profile': {**manual_profile, 'city': 'Wrong location'}}, 400)
    check(finance.call(address_path)['version'] == saved['version'],
          'Rejected address edits leave the saved company version unchanged')
    check(finance.call('/session')['user']['setup_grants'] == before_grants,
          'Postal lookup and address updates do not change account grants')
    check(sql(f"SELECT count(*) FROM company_setup_history WHERE company_id={address_id}", name) == '2',
          'Address creation and manual correction use existing company audit history')
    legacy_id = finance.call('/company-setup', {**address_header, 'legal_name': 'ADDRESS LEGACY CREATE'})['id']
    check(finance.call('/company-setup/' + str(legacy_id))['profile']['postal_code'] == '',
          'Older company creation requests without address fields still work')
    international_id = finance.call('/company-setup', {
        **address_header, 'legal_name': 'ADDRESS INTERNATIONAL CREATE', 'country': 'GB', 'city': 'London',
        'address': 'Manual international address', 'state': 'Greater London', 'postal_code': 'SW1A 1AA',
    })['id']
    check(finance.call('/company-setup/' + str(international_id))['profile']['postal_code'] == 'SW1A 1AA',
          'International postal codes remain manually enterable')


company_address_cases()
