# Synthetic-only credential checks, executed inside company_setup.py's disposable DB.
credential_start_checks = checks
check(not env.get('SMTP_HOST'), 'Credential edit tests cannot send real email')
credential_original_username = 'qa.credential.edit'
credential_next_username = 'qa.credential.changed'
credential_next_password = secrets.token_urlsafe(24)
credential_user_id = admin.call('/setup-users', {
    'username': credential_original_username, 'name': 'Credential edit QA',
    'password': password, 'password_confirmation': password, 'role': 'Finance Head',
    'features': ['view'], 'active': True,
})['id']
credential_path = '/setup-users/' + str(credential_user_id)
def credential_record():
    return next(user for user in admin.call('/setup-users')['users'] if user['id'] == credential_user_id)
def credential_body():
    user = credential_record()
    body = {key: user[key] for key in ('name','username','email','phone','active','designation_id','inherit_designation','scope','companies','teams','lines','orders','finishing_units','reports_to_id','territory','restrict_buyers','buyer_ids','production_areas','version')}
    return {**body, 'features': user['setup_features'], 'permissions': user['effective_permissions']}
def credential_fingerprint(exclude_credentials=False):
    expression = "to_jsonb(u)-ARRAY['username','password_hash','session_version']" if exclude_credentials else 'to_jsonb(u)'
    return sql(f'SELECT md5(({expression})::text) FROM users u WHERE id={credential_user_id}', name)
def credential_login(username, login_password):
    client = Client()
    client.call('/login', {'username': username, 'password': login_password})
    client.user = username
    return client

# Existing clients omit credential keys; blank fields must not invoke local defaults.
credential_hash = sql(f'SELECT password_hash FROM users WHERE id={credential_user_id}', name)
credential_old = Client(credential_original_username)
body = credential_body(); body.pop('username')
admin.call(credential_path, body)
check(sql(f'SELECT password_hash FROM users WHERE id={credential_user_id}', name) == credential_hash,
      'Legacy access edits preserve the password hash')
body = credential_body()
admin.call(credential_path, {**body, 'new_password':'', 'new_password_confirmation':''})
check(sql(f'SELECT password_hash FROM users WHERE id={credential_user_id}', name) == credential_hash,
      'Blank new password preserves the existing hash even in local mode')
credential_current = Client(credential_original_username)

# Invalid/unauthorized requests cannot mutate identity, access or password state.
for credentials in [
    {'username':''}, {'username':None}, {'username':'bad username'},
    {'new_password':None}, {'new_password':1234},
    {'new_password':credential_next_password},
    {'new_password':credential_next_password,'new_password_confirmation':'mismatch'},
    {'new_password':'','new_password_confirmation':credential_next_password},
    {'new_password':'short','new_password_confirmation':'short'},
]:
    body = credential_body(); before = credential_fingerprint()
    admin.call(credential_path, {**body, **credentials}, 400)
    check(credential_fingerprint() == before, 'Invalid credential edit leaves account unchanged')
body = credential_body(); before = credential_fingerprint()
conflict = admin.call(credential_path, {**body, 'username':' ADMIN '}, 409)
check('username' in conflict.get('error','').lower(), 'Duplicate username produces a clear validation error')
check(credential_fingerprint() == before, 'Duplicate username preserves all account state')
for actor in [finance, md, credential_current]:
    actor.call(credential_path, {**body, 'new_password':credential_next_password, 'new_password_confirmation':credential_next_password}, 403)
check(credential_fingerprint() == before, 'Non-Super-Admin credential changes are blocked')
admin_id = admin.call('/session')['user']['id']
admin.call('/setup-users/' + str(admin_id), {**body, 'username':'self.rename'}, 400)
# A Developer fixture must satisfy the production role/MFA constraints. Its
# synthetic email already exists, and no Developer login or OTP is requested.
credential_protection_baseline = json.loads(sql(f'SELECT json_build_object(\'role\',role,\'mfa_enabled\',mfa_enabled) FROM users WHERE id={credential_user_id}', name))
sql(f"UPDATE users SET role='Super Admin',mfa_enabled=true,is_developer=true WHERE id={credential_user_id}", name)
before = credential_fingerprint()
admin.call(credential_path, {**body, 'username':'protected.rename'}, 403)
check(credential_fingerprint() == before, 'Developer credentials cannot be changed through User access')
credential_restore_role = credential_protection_baseline['role'].replace("'", "''")
credential_restore_mfa = 'true' if credential_protection_baseline['mfa_enabled'] else 'false'
sql(f"UPDATE users SET is_developer=false,role='{credential_restore_role}',mfa_enabled={credential_restore_mfa} WHERE id={credential_user_id}", name)
sql(f'UPDATE users SET deleted_at=now() WHERE id={credential_user_id}', name)
before = credential_fingerprint()
admin.call(credential_path, {**body, 'username':'protected.rename'}, 403)
check(credential_fingerprint() == before, 'Deleted account credentials cannot be changed through User access')
sql(f'UPDATE users SET deleted_at=NULL WHERE id={credential_user_id}', name)

# Username-only changes invalidate existing sessions while keeping the password.
body = credential_body(); stale_body = dict(body)
credential_preserved = credential_fingerprint(True)
changed = admin.call(credential_path, {**body, 'username':' QA.Credential.Changed ', 'new_password':'', 'new_password_confirmation':'', 'mfa_enabled':True, 'ip_restricted':True, 'allowed_ips':['203.0.113.1/32']})
check('welcome_email' not in changed, 'Credential edits do not email passwords')
check(credential_fingerprint(True) == credential_preserved, 'Username edit preserves OTP/IP, permissions, scopes and account data')
check(sql(f'SELECT password_hash FROM users WHERE id={credential_user_id}', name) == credential_hash,
      'Username-only change retains the password hash')
credential_current.call('/company-setup', expect=401)
Client().call('/login', {'username':credential_original_username, 'password':password}, 401)
credential_current = credential_login(credential_next_username,password)
body = credential_body(); before = credential_fingerprint()
admin.call(credential_path, {**stale_body, 'username':'qa.stale.rename','new_password':credential_next_password,'new_password_confirmation':credential_next_password}, 409)
check(credential_fingerprint() == before, 'Stale edits cannot rename or reset credentials')

# A confirmed password reset preserves all unrelated data and revokes old sessions.
credential_preserved = credential_fingerprint(True)
reset = admin.call(credential_path, {**body, 'new_password':credential_next_password,'new_password_confirmation':credential_next_password})
check('welcome_email' not in reset, 'Password reset sends no welcome email')
check(credential_fingerprint(True) == credential_preserved, 'Password reset preserves user security settings and access scope')
credential_current.call('/company-setup', expect=401)
Client().call('/login', {'username':credential_next_username,'password':password}, 401)
credential_current = credential_login(credential_next_username,credential_next_password)
check(credential_current.call('/session')['user']['id'] == credential_user_id, 'Confirmed new credentials sign in as the same employee identity')
register = admin.call('/setup-users')
credential_history = [item for item in register['history'] if item['subject_type']=='User' and item['subject_id']==credential_user_id]
check(any(item['detail'].get('password_reset') is True for item in credential_history), 'Password reset is recorded without secret values')
check(any(item['detail'].get('before',{}).get('username')==credential_original_username and item['detail'].get('after',{}).get('username')==credential_next_username for item in credential_history), 'Username change retains old and new login names in audit history')
serialized = json.dumps(register)
check(credential_next_password not in serialized and password not in serialized and 'password_hash' not in serialized,
      'User register and audit expose no plaintext password or password hash')
print(f'Credential edit HTTP checks passed: {checks-credential_start_checks}', flush=True)
