"""Isolated document settings and preview regressions. No messages are sent."""
def company_document_cases():
    import copy
    header = {'legal_name': 'DOCUMENT SETTINGS REGRESSION', 'category': 'Buyer', 'company_type': 'Retailer', 'country': 'GB', 'city': 'London'}
    cid = finance.call('/company-setup', header)['id']; path = '/company-setup/' + str(cid)
    field = admin.call('/company-fields', {'label': 'Document reminder regression', 'section': 'Additional', 'country': 'GB', 'categories': ['Buyer'], 'input_type': 'text', 'expiry_enabled': True})['id']
    def detail(actor=finance): return actor.call(path)
    def save(profile, expect=200): return finance.call(path, {'action': 'save', 'version': detail()['version'], **header, 'profile': profile}, expect)
    def action(actor, value): return actor.call(path, {'action': value, 'version': detail(admin)['version']})
    upload_results = {}
    def upload(filename, content):
        boundary = 'company-document-settings'
        body = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\nContent-Type: application/octet-stream\r\n\r\n').encode() + content + f'\r\n--{boundary}--\r\n'.encode()
        result = finance.call(path + '/files', body, headers={'Content-Type': 'multipart/form-data; boundary=' + boundary})
        upload_results[result['id']] = result
        return result['id']
    pdf = b'%PDF-1.4\nPrivate synthetic company document\n'; pdf_id = upload('document-proof.pdf', pdf)
    office = io.BytesIO()
    with zipfile.ZipFile(office, 'w') as archive:
        archive.writestr('[Content_Types].xml', '<Types/>'); archive.writestr('word/document.xml', '<document><p><r><t>Private bank proof</t></r></p></document>')
    office_id = upload('bank-proof.docx', office.getvalue())
    for suffix, disposition, content_type in [('?preview=1', 'inline', 'application/pdf'), ('', 'attachment', 'application/octet-stream')]:
        with finance.opener.open(base + '/api/company-files/' + str(pdf_id) + suffix) as response:
            check(response.headers['Content-Disposition'].startswith(disposition), 'Company PDF uses requested preview/download mode')
            check(response.headers['Content-Type'] == content_type, 'Company document uses safe content type')
            check('no-store' in response.headers['Cache-Control'], 'Company preview does not cache private content')
            check(response.headers['X-Content-Type-Options'] == 'nosniff', 'Company preview prevents content sniffing')
            check(response.read() == pdf, 'Preview returns decrypted original bytes')
    with finance.opener.open(base + '/api/company-files/' + str(office_id) + '?preview=1') as response:
        check(response.headers['Content-Disposition'].startswith('inline'), 'Office text preview opens inline')
        check(response.headers['Content-Type'] == 'text/plain; charset=utf-8', 'Office preview uses inert text')
        check(b'Private bank proof' in response.read(), 'Office preview includes readable document text')
    check(finance.call('/company-files/' + str(office_id)) == office.getvalue(), 'Office Download preserves original bytes')
    Client().call('/company-files/' + str(pdf_id) + '?preview=1', expect=401)
    md.call('/company-files/' + str(pdf_id) + '?preview=1', expect=403)
    limited.call('/company-files/' + str(pdf_id) + '?preview=1', expect=404)
    # Lossless compression applies to new attachments only, before encryption.
    compressed_pdf = b'%PDF-1.7\n' + b'Synthetic repeated company proof line.\n' * 10000
    compressed_id = upload('compressed-proof.pdf', compressed_pdf)
    compressed_file = next(f for f in detail()['files'] if f['id'] == compressed_id)
    check(compressed_file['compression'] == 'gzip' and compressed_file['saved_bytes'] > 0,
          'Compressible new document reports real storage savings')
    check(compressed_file['original_size'] == len(compressed_pdf) and compressed_file['size'] == len(compressed_pdf),
          'Compression preserves original document size metadata')
    check(compressed_file['stored_size'] < len(compressed_pdf), 'Encrypted compressed file consumes less disk space')
    check(upload_results[compressed_id]['saved_bytes'] == compressed_file['saved_bytes'] and
          upload_results[compressed_id]['stored_size'] == compressed_file['stored_size'],
          'Upload and file list report the same actual storage savings')
    compressed_key = sql(f'SELECT storage_key FROM company_setup_files WHERE id={compressed_id}', name)
    check((Path(temp)/'files'/compressed_key).stat().st_size == compressed_file['stored_size'],
          'Reported stored size matches the encrypted file on disk')
    check('storage_key' not in compressed_file, 'Storage savings metadata never exposes private storage key')
    check(finance.call('/company-files/' + str(compressed_id)) == compressed_pdf,
          'Lossless compressed download restores exact original bytes')
    check(finance.call('/company-files/' + str(compressed_id) + '?preview=1') == compressed_pdf,
          'PDF preview uses restored original bytes')
    check(finance.call('/company-files/' + str(pdf_id)) == pdf, 'Existing uncompressed attachment still downloads unchanged')
    large_office = io.BytesIO()
    with zipfile.ZipFile(large_office, 'w') as archive:
        archive.writestr('[Content_Types].xml', '<Types/>')
        archive.writestr('word/document.xml', '<document><p><r><t>' + 'Lossless Office text. ' * 10000 + '</t></r></p></document>')
    large_office_bytes = large_office.getvalue()
    large_office_id = upload('compressed-office.docx', large_office_bytes)
    check(upload_results[large_office_id]['compression'] == 'gzip', 'Office originals also use selective lossless storage compression')
    check(finance.call('/company-files/' + str(large_office_id)) == large_office_bytes, 'Compressed Office download restores exact original ZIP bytes')
    check(b'Lossless Office text.' in finance.call('/company-files/' + str(large_office_id) + '?preview=1'),
          'Office text preview works after bounded storage decompression')

    expiry = (datetime.date.today() + datetime.timedelta(days=30)).isoformat()
    profile = {'address': 'Document test address', 'email': 'company@example.test', 'phone': '0000', 'sample_address': 'Sample address',
        'banks': [{'id': 'doc-bank', 'name': 'Test Bank', 'holder': 'Test holder', 'account': 'synthetic', 'account_type': 'Current', 'currency': 'GBP', 'documents': [office_id]}],
        'fields': [{'id': field, 'value': 'REG-ONLY-TEST', 'expiry': True, 'expires_on': expiry, 'documents': [pdf_id], 'reminder_enabled': True, 'reminder_emails': [' GST@Example.test ', 'gst@example.test', 'finance@example.test']}],
        'trade_registrations': [{'id': 'doc-trade', 'portal': 'DGFT', 'registration': 'TEST-DGFT', 'expiry': True, 'expires_on': expiry, 'documents': [pdf_id], 'reminder_enabled': True, 'reminder_emails': ['trade@example.test']}]}
    for definition in finance.call('/company-setup')['fields']:
        if definition['section'] == 'KYC' and definition['country'] in ('*', 'GB') and 'Buyer' in definition['categories']:
            profile['fields'].append({'id': definition['id'], 'not_applicable': True, 'reason': 'Synthetic fixture', 'reminder_enabled': False})
    save(profile); saved = detail()
    check(saved['profile']['fields'][0]['reminder_emails'] == ['gst@example.test', 'finance@example.test'], 'Document recipients trimmed and case-insensitively deduplicated')
    check(saved['profile']['banks'][0]['account_type'] == 'Current', 'Bank account type persists')
    check('legacy_reminder_emails' in saved and 'legacy_reminder_emails' not in detail(md), 'Legacy recipient metadata remains private')
    check(saved['summary']['ready'], 'Document recipients replace global reminder requirement')
    for invalid in [[], ['invalid'], ['person@example.test'] * 21, ['x@example.test\r\nBcc: y@example.test']]:
        changed = copy.deepcopy(profile); changed['fields'][0]['reminder_emails'] = invalid; save(changed, 400)
    for invalid in [dict(reminder_enabled='yes'), dict(expiry=False), dict(not_applicable=True, value='', documents=[], expiry=False)]:
        changed = copy.deepcopy(profile); changed['fields'][0].update(invalid); save(changed, 400)
    changed = copy.deepcopy(profile); changed['banks'][0]['account_type'] = 'Invented account type'; save(changed, 400)
    changed = copy.deepcopy(profile); changed['banks'][0].pop('account_type'); save(changed)
    check(detail()['profile']['banks'][0]['account_type'] == '', 'Legacy bank without type remains accepted')
    changed = copy.deepcopy(profile); changed['fields'][0]['documents'] = []; save(changed); detached = detail()
    check(detached['profile']['fields'][0]['documents'] == [], 'Field unlink leaves only that field empty')
    check(detached['profile']['banks'][0]['documents'] == [office_id], 'Field unlink preserves bank attachment')
    check(any(f['id'] == pdf_id for f in detached['files']), 'Unlinked original retained for history')
    check('1 document link(s) removed' in detached['history'][0]['note'], 'Document unlink has audit note')
    check('T' in detached['history'][0]['created_at'], 'Record history includes timestamp')
    check(admin.call('/company-files/' + str(pdf_id)) == pdf, 'Unlinked original remains recoverable')
    save(profile); action(finance, 'submit')
    for actor in (finance, md):
        hidden = detail(actor)
        check('legacy_reminder_emails' not in hidden and 'profile' not in hidden, 'Pending summary hides reminder recipients')
        actor.call('/company-files/' + str(pdf_id) + '?preview=1', expect=403)
    action(md, 'approve')
    check(sql(f'SELECT count(*) FROM company_reminders WHERE company_id={cid} AND active', name) == '2', 'Approved document and trade expiry tracked')
    action(finance, 'amend'); changed = copy.deepcopy(profile)
    changed['fields'][0]['reminder_enabled'] = False; changed['trade_registrations'][0]['reminder_enabled'] = False; save(changed)
    check(sql(f'SELECT count(*) FROM company_reminders WHERE company_id={cid} AND active', name) == '2', 'Email opt-out cannot remove renewal obligation')
    action(finance, 'submit'); action(md, 'approve')
    check(sql(f'SELECT count(*) FROM company_reminders WHERE company_id={cid} AND active', name) == '2', 'Email-disabled documents still track expiry after approval')
    # Start a second isolated worker once. SMTP is explicitly absent, and only
    # disposable fixture rows are due. Never send any real message in this suite.
    sql(f"UPDATE company_reminders SET next_attempt=now()-interval '1 minute',attempts=0,sent_at=NULL WHERE company_id={cid}", name)
    worker_env = {**env, 'HTTP_ADDR': '127.0.0.1:' + str(freeport()), 'SMTP_HOST': '', 'SMTP_FROM': ''}
    worker = subprocess.Popen([str(ROOT/'backend/erp-server')], env=worker_env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        for _ in range(40):
            state = sql(f"SELECT count(*) FROM company_reminders WHERE company_id={cid} AND active AND email_status='Disabled' AND attempts=0 AND sent_at IS NULL", name)
            if state == '2': break
            if worker.poll() is not None: raise AssertionError('Isolated reminder worker exited before checking disabled rows')
            time.sleep(.1)
        check(state == '2', 'Worker marks email Disabled without delivery attempts, false Sent status or expiry deactivation')
    finally:
        worker.terminate(); worker.wait(timeout=5)
    admin.call(path + '/delete', {'version': detail(admin)['version'], 'confirmation_name': header['legal_name'], 'password': password})
    check(sql(f'SELECT count(*) FROM company_reminders WHERE company_id={cid} AND active', name) == '0', 'Disposable reminder fixture deactivated')
company_document_cases()
