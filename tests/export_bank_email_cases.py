# Runs at the end of developer.py against its disposable database and local,
# certificate-verified STARTTLS receiver. No externally reachable SMTP is used.
assert name.startswith('erp_dev_test_') and runtime == name + '_api'
assert env['SMTP_HOST'] == 'localhost' and env['SMTP_PORT'] == str(mailserver.server_address[1])
assert mailserver.server_address[0] == '127.0.0.1'
exbm_start = len(report)
check(True, 'Bank email tests use only the isolated loopback STARTTLS receiver')
exbm_admin = Client()
exbm_admin.login('admin')
exbm_grants = ['export_view', 'export_edit', 'export_files', 'export_download']


def exbm_user(suffix, role='Finance', grants=None):
    username = 'qa.bankmail.' + suffix
    designation = exbm_admin.call('/access-designations', {
        'name': 'Bank email QA ' + suffix, 'department': 'Isolated banking QA',
        'role': role, 'features': [], 'permissions': exbm_grants if grants is None else grants,
    })['id']
    uid = exbm_admin.call('/setup-users', {
        'name': 'Bank email QA ' + suffix, 'username': username, 'password': '1234',
        'email': username + '@example.test',
        'designation_id': designation, 'inherit_designation': True,
        'scope': 'company', 'companies': [], 'active': True,
    })['id']
    client = Client()
    client.login(username)
    return uid, client


exbm_uid, exbm_writer = exbm_user('writer')
exbm_other_uid, exbm_other = exbm_user('other')
exbm_read_uid, exbm_read = exbm_user('read', grants=['export_view', 'export_download'])
exbm_nodownload_uid, exbm_nodownload = exbm_user('no-download', grants=['export_view', 'export_edit', 'export_files'])
exbm_md_uid, exbm_md = exbm_user('md', 'MD', ['export_view', 'export_download', 'export_approve'])
exbm_payload = b'%PDF-1.4\nSynthetic bank invoice document\n' + bytes(range(256)) + b'\n%%EOF'
exbm_body = {'party_type': 'manual', 'party_name': 'Synthetic bank email customer',
             'invoice_number': 'BANK-EMAIL-QA-001', 'currency': 'USD', 'invoice_amount': '1000.00',
             'profile': {}, 'receipts': [], 'ebrcs': []}
exbm_id = exbm_writer.call('/exports', exbm_body)['id']
exbm_path = '/exports/' + str(exbm_id)


def exbm_version(path=exbm_path):
    return exbm_writer.call(path)['record']['version']


def exbm_upload(path=exbm_path, kind='invoice', filename='invoice original.pdf', content=exbm_payload):
    boundary_value = 'isolated-bank-mail-' + secrets.token_hex(8)
    raw = (f'--{boundary_value}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\n'
           'Content-Type: application/octet-stream\r\n\r\n').encode() + content + f'\r\n--{boundary_value}--\r\n'.encode()
    request = urllib.request.Request(base + '/api' + path + '/files?kind=' + kind + '&version=' + str(exbm_version(path)),
        data=raw, headers={'Origin': base, 'X-CSRF-Token': exbm_writer.csrf,
                           'Content-Type': 'multipart/form-data; boundary=' + boundary_value})
    try:
        response = exbm_writer.o.open(request, timeout=20)
    except urllib.error.HTTPError as error:
        response = error
    result = json.load(response)
    check(response.status == 200, 'Bank email fixture uploads a real encrypted supporting document: ' + str(result if response.status != 200 else 'ok'))
    return result['id']


exbm_file = exbm_upload()
exbm_base_input = {'recipients': ['bank.operations@example.test'], 'subject': 'Conversion advice – synthetic invoice',
                   'body': 'Please process the attached synthetic invoice.\nUSD 1,000.00\nNo real bank instruction.',
                   'file_ids': [exbm_file]}


def exbm_prepare(subject=None, actor=exbm_writer, path=exbm_path, status=200, **patch):
    return actor.call(path + '/bank-email/prepare', {
        **exbm_base_input, 'version': exbm_version(path),
        **({'subject': subject} if subject else {}), **patch,
    }, status)


def exbm_send(eid, actor=exbm_writer, path=exbm_path, status=200, **patch):
    return actor.call(path + '/bank-email/' + str(eid) + '/send',
                      {'version': exbm_version(path), 'confirmed': True, **patch}, status)


def exbm_clear_limit(uid=exbm_uid):
    run('docker', 'exec', name, 'redis-cli', 'DEL', 'bank-mail:' + str(uid))


exbm_before = len(messages)
exbm_prepared = exbm_prepare()
check(exbm_prepared['status'] == 'Prepared' and len(messages) == exbm_before,
      'Preparing bank email stores a reviewable draft without sending mail')
check(exbm_prepared['recipients'] == exbm_base_input['recipients'] and
      exbm_prepared['files'][0]['filename'] == 'invoice original.pdf' and
      'ciphertext' not in json.dumps(exbm_prepared), 'Review shows exact bank recipients and file metadata without encrypted bytes')
check(exbm_prepare()['id'] == exbm_prepared['id'] and len(messages) == exbm_before,
      'Identical preparation is idempotent and never duplicates a bank message')
check(exbm_writer.call(exbm_path + '/bank-email')['configured'], 'Bank mail reports the isolated TLS sender as configured')
exbm_send(exbm_prepared['id'], confirmed=False, status=400)
exbm_send(exbm_prepared['id'], confirmed='true', status=400)
exbm_writer.call(exbm_path + '/bank-email/' + str(exbm_prepared['id']) + '/send', {'version': exbm_version()}, 400)
check(len(messages) == exbm_before, 'Missing, false or string confirmation cannot send bank email')
for exbm_denied in (exbm_read, exbm_nodownload, exbm_md):
    exbm_prepare(actor=exbm_denied, status=403)
    exbm_denied.call(exbm_path + '/bank-email', status=403)
    exbm_send(exbm_prepared['id'], actor=exbm_denied, status=403)
Client().call(exbm_path + '/bank-email', status=401)
exbm_send(exbm_prepared['id'], actor=exbm_other, status=404)
check(len(messages) == exbm_before, 'Read-only, missing download grant, reviewer and another author cannot send the prepared email')
for exbm_patch in (
    {'recipients': []}, {'recipients': ['Bank <bank@example.test>']},
    {'recipients': ['bad\r\n@example.test']}, {'subject': 'Safe\r\nBcc: injected@example.test'},
    {'file_ids': []}, {'file_ids': [exbm_file, exbm_file]}, {'file_ids': [999999999]},
):
    exbm_prepare(status=400, **exbm_patch)
exbm_non_bank_file = exbm_upload(kind='other', filename='other.pdf')
exbm_prepare(status=400, file_ids=[exbm_non_bank_file])
# A file upload advances the source version, invalidating the earlier review.
exbm_clear_limit()
exbm_send(exbm_prepared['id'], status=409)
check(len(messages) == exbm_before, 'A source revision changed after review cannot be emailed')
exbm_current = exbm_prepare()
exbm_accepted = exbm_send(exbm_current['id'])
check(exbm_accepted['status'] == 'Accepted' and exbm_accepted['sent_at'] and len(messages) == exbm_before + 1,
      'Confirmed send records provider acceptance once, distinct from inbox delivery')
exbm_mime = Parser(policy=policy.default).parsestr(messages[-1])
check(str(exbm_mime['Subject']) == exbm_base_input['subject'] and
      str(exbm_mime['To']) == 'bank.operations@example.test', 'MIME carries exactly the reviewed recipient and Unicode subject')
check(exbm_mime.get_body(preferencelist=('plain',)).get_content().replace('\r\n', '\n').rstrip('\n') == exbm_base_input['body'],
      'MIME carries the reviewed plain-text bank instruction')
exbm_parts = list(exbm_mime.iter_attachments())
check(len(exbm_parts) == 1 and exbm_parts[0].get_filename() == 'invoice original.pdf' and
      exbm_parts[0].get_payload(decode=True) == exbm_payload,
      'MIME attachment decodes to the exact original filename and file bytes')
exbm_send(exbm_current['id'], status=400)
check(len(messages) == exbm_before + 1, 'Accepted bank email cannot be replayed with a fresh request')

# Prepare→remove is invalid even with the latest parent version supplied.
exbm_remove_file = exbm_upload(kind='bank_advice', filename='removable advice.pdf')
exbm_removal_review = exbm_prepare(subject='Review before removal', file_ids=[exbm_remove_file])
exbm_writer.call(exbm_path + '/files/' + str(exbm_remove_file) + '/remove', {'version': exbm_version()})
exbm_clear_limit()
exbm_send(exbm_removal_review['id'], status=409)
exbm_prepare(status=400, file_ids=[exbm_remove_file])
check(len(messages) == exbm_before + 1, 'Removed documents cannot be sent from an old review or selected in a new review')
exbm_jv_id = exbm_writer.call('/exports', {'record_kind': 'jv', 'party_type': 'manual', 'invoice_number': 'BANK-MAIL-JV-DENIED',
                                         'currency': 'USD', 'invoice_amount': '10.00', 'profile': {}})['id']
exbm_jv_path = '/exports/' + str(exbm_jv_id)
exbm_jv_file = exbm_upload(path=exbm_jv_path, kind='jv_supporting', filename='jv.pdf')
exbm_prepare(path=exbm_jv_path, status=400, file_ids=[exbm_jv_file])
exbm_writer.call(exbm_jv_path + '/bank-email', status=400)
check(len(messages) == exbm_before + 1, 'JV entries cannot use the export bank email service')

# Two separately keyed confirmed send requests contend on one prepared row.
exbm_concurrent = exbm_prepare(subject='Concurrent send proof')
exbm_clear_limit()
mail_test_mode = 'slow_accept'
try:
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as exbm_pool:
        exbm_codes = sorted(exbm_pool.map(lambda _: raw_post(exbm_path + '/bank-email/' + str(exbm_concurrent['id']) + '/send',
            {'version': exbm_version(), 'confirmed': True}, secrets.token_hex(16), exbm_writer), range(2)))
finally:
    mail_test_mode = 'accept'
check(exbm_codes == [200, 400] and len(messages) == exbm_before + 2,
      'Different concurrent send actions deliver one bank message and reject the duplicate')

# A definitive pre-DATA rejection is safely retryable; nothing was accepted.
exbm_failed = exbm_prepare(subject='Pre-DATA rejection proof')
exbm_clear_limit()
mail_test_mode = 'reject_before_data'
try:
    exbm_failure = exbm_send(exbm_failed['id'])
finally:
    mail_test_mode = 'accept'
check(exbm_failure['status'] == 'Failed' and not exbm_failure['sent_at'] and len(messages) == exbm_before + 2,
      'Definitive rejection before DATA records Failed and sends no message body')
exbm_retry = exbm_send(exbm_failed['id'])
check(exbm_retry['status'] == 'Accepted' and len(messages) == exbm_before + 3,
      'An explicitly retried pre-DATA failure can send the unchanged reviewed message once')

# Simulate the receiver taking DATA then dropping the acknowledgement.
exbm_uncertain = exbm_prepare(subject='After-DATA uncertainty proof')
exbm_clear_limit()
mail_test_mode = 'drop_after_data'
try:
    exbm_unknown = exbm_send(exbm_uncertain['id'])
finally:
    mail_test_mode = 'accept'
check(exbm_unknown['status'] == 'Uncertain' and not exbm_unknown['sent_at'] and len(messages) == exbm_before + 4,
      'Lost post-DATA acknowledgement records Uncertain even though receiver obtained the MIME')
exbm_send(exbm_uncertain['id'], status=400)
check(exbm_prepare(subject='After-DATA uncertainty proof')['id'] == exbm_uncertain['id'] and len(messages) == exbm_before + 4,
      'Uncertain send cannot be replayed or silently recreated by identical preparation')
check(any(exbm_item['status'] == 'Uncertain' for exbm_item in exbm_writer.call(exbm_path + '/bank-email')['items']),
      'Bank email history preserves the uncertain outcome for the operator')

# Bank correspondence is protected by effective edit+download permissions at SQL level.
exbm_identity = {'id': exbm_uid, 'role': 'Finance', 'scope': 'company', 'companies': [], 'teams': [],
                 'lines': [], 'orders': [], 'units': [], 'buyer_ids': [], 'production_areas': [],
                 'restrict_buyers': False, 'exports': True, 'exports_edit': True, 'exports_download': False}
exbm_identity_text = json.dumps(exbm_identity).replace("'", "''")
exbm_rls = sql("BEGIN; SET LOCAL ROLE euroerp_runtime; SET LOCAL erp.service=''; SET LOCAL erp.identity='" +
              exbm_identity_text + f"'; SELECT json_build_array(count(*)) FROM export_bank_emails WHERE invoice_id={exbm_id}; ROLLBACK;")
check(json.loads(next(exbm_line for exbm_line in exbm_rls.splitlines() if exbm_line.startswith('['))) == [0],
      'Runtime SQL without download permission cannot read bank correspondence')
for exbm_statement in (
    f"UPDATE export_bank_emails SET subject='tampered' WHERE id={exbm_current['id']}",
    f"DELETE FROM export_bank_emails WHERE id={exbm_current['id']}",
    f"UPDATE export_bank_emails SET status='Prepared' WHERE id={exbm_current['id']}",
):
    try:
        sql('BEGIN; ' + exbm_statement + '; ROLLBACK;')
        raise AssertionError('Reviewed bank correspondence was mutable/replayable')
    except subprocess.CalledProcessError:
        check(True, 'Database prevents alteration, deletion or replay of reviewed bank email')
check(sql(f"SELECT count(*) FROM export_history WHERE invoice_id={exbm_id} AND action LIKE 'Bank email #%'") == '10',
      'Every actual bank send attempt records its sending transition and result in immutable invoice history')
exbm_result = {'passed': len(report) - exbm_start, 'checks': report[exbm_start:],
               'real_external_email_sent': False, 'receiver': 'loopback STARTTLS only',
               'accepted_messages': 3, 'uncertain_messages_received': 1}
(ROOT/'docs/export-bank-email-test-results.json').write_text(json.dumps(exbm_result, indent=2))
print('PASS: ' + str(exbm_result['passed']) + ' isolated bank email HTTP/MIME assertions', flush=True)
