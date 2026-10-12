# Executed only by company_setup.py against its disposable database, with SMTP off.
password_confirmation_start_checks = checks
check(not env.get('SMTP_HOST'), 'Password confirmation tests cannot send employee emails')
confirmation_body = {
    'username': 'qa.password.confirmation',
    'name': 'Password confirmation QA',
    'password': password,
    'password_confirmation': password,
    'role': 'Finance Head',
    'features': ['view'],
    'active': True,
}
confirmation_before_users = sql('SELECT count(*) FROM users', name)
confirmation_before_audit = sql('SELECT count(*) FROM access_history', name)
for invalid_confirmation in ['', None, 1234, password + '-different']:
    admin.call('/setup-users', {**confirmation_body, 'password_confirmation': invalid_confirmation}, 400)
    check(sql('SELECT count(*) FROM users', name) == confirmation_before_users,
          'Invalid new-user confirmation creates no account')
    check(sql('SELECT count(*) FROM access_history', name) == confirmation_before_audit,
          'Invalid new-user confirmation writes no access audit')
finance.call('/setup-users', confirmation_body, 403)
confirmation_created = admin.call('/setup-users', confirmation_body)
confirmation_source_id = confirmation_created['id']
check(confirmation_created['welcome_email'] == 'not_configured',
      'Matching confirmation creates an account without sending email')
confirmation_client = Client(confirmation_body['username'])
check(confirmation_client.call('/session')['user']['id'] == confirmation_source_id,
      'Matching confirmation preserves the chosen login password')
confirmation_source = next(u for u in admin.call('/setup-users')['users'] if u['id'] == confirmation_source_id)
confirmation_replace_body = {
    'username': 'qa.password.replacement',
    'name': 'Password replacement QA',
    'password': password,
    'password_confirmation': password,
    'version': confirmation_source['version'],
}
confirmation_replace_path = '/setup-users/' + str(confirmation_source_id) + '/replace'
confirmation_before_source = sql(f'SELECT md5(row_to_json(u)::text) FROM users u WHERE id={confirmation_source_id}', name)
confirmation_before_users = sql('SELECT count(*) FROM users', name)
confirmation_before_audit = sql('SELECT count(*) FROM access_history', name)
for invalid_confirmation in ['', None, 1234, password + '-different']:
    admin.call(confirmation_replace_path,
               {**confirmation_replace_body, 'password_confirmation': invalid_confirmation}, 400)
    check(sql('SELECT count(*) FROM users', name) == confirmation_before_users,
          'Invalid replacement confirmation creates no replacement account')
    check(sql(f'SELECT md5(row_to_json(u)::text) FROM users u WHERE id={confirmation_source_id}', name) == confirmation_before_source,
          'Invalid replacement confirmation leaves source account and access unchanged')
    check(sql('SELECT count(*) FROM access_history', name) == confirmation_before_audit,
          'Invalid replacement confirmation writes no access audit')
finance.call(confirmation_replace_path, confirmation_replace_body, 403)
confirmation_replaced = admin.call(confirmation_replace_path, confirmation_replace_body)
check(confirmation_replaced['welcome_email'] == 'not_configured',
      'Matching replacement confirmation sends no email during isolated testing')
check(sql(f'SELECT active FROM users WHERE id={confirmation_source_id}', name) == 'f',
      'Matching replacement confirmation completes the existing employee handover')
Client().call('/login', {'username': confirmation_body['username'], 'password': password}, 401)
confirmation_replacement = Client(confirmation_replace_body['username'])
check(confirmation_replacement.call('/session')['user']['id'] == confirmation_replaced['id'],
      'Replacement account signs in with the confirmed password')
confirmation_history = json.dumps(admin.call('/setup-users')['history'])
check('password_confirmation' not in confirmation_history and password not in confirmation_history,
      'Neither initial password nor confirmation is stored in access history')
print(f'Password confirmation HTTP checks passed: {checks-password_confirmation_start_checks}', flush=True)
