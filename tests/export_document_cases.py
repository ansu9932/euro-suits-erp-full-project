# Per-kind export attachment regressions, run only in the disposable harness.
assert name.startswith('erp_test_') and runtime_role == name + '_api'
check(not env.get('SMTP_HOST'), 'Export attachment tests cannot send real messages')
exd_start = checks
exd_writer_id, exd_writer = export_user('qa.export.documents')
exd_reviewer_id, exd_reviewer = export_user('qa.export.documents.md', 'MD', ['export_view', 'export_download', 'export_approve'])


def exd_new(label):
    body = {**exw_body, 'invoice_number': 'EX-DOCUMENT-' + label, 'receipts': [], 'ebrcs': []}
    rid = exd_writer.call('/exports', body)['id']
    return '/exports/' + str(rid), rid


def exd_detail(path, actor=exd_writer):
    return actor.call(path)


def exd_version(path):
    return exd_detail(path)['record']['version']


def exd_multipart(filename, content):
    delimiter = 'export-document-proof'
    body = (f'--{delimiter}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\nContent-Type: application/octet-stream\r\n\r\n').encode() + content + f'\r\n--{delimiter}--\r\n'.encode()
    return body, {'Content-Type': 'multipart/form-data; boundary=' + delimiter}


def exd_upload(path, kind='invoice', filename='invoice-proof.pdf', content=payload, actor=exd_writer, expect=200, version=None):
    body, headers = exd_multipart(filename, content)
    return actor.call(path + '/files?kind=' + kind + '&version=' + str(exd_version(path) if version is None else version),
                      body, expect, headers)


def exd_get(fid, suffix='', actor=exd_writer, expect=200):
    request = urllib.request.Request(base + '/api/export-files/' + str(fid) + suffix)
    try:
        response = actor.opener.open(request, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    raw = response.read()
    check(response.status == expect, 'Export preview/download returns expected HTTP ' + str(expect))
    return response.headers, raw


exd_path, exd_id = exd_new('PREVIEW')
exd_pdf = exd_upload(exd_path)['id']
exd_text = exd_upload(exd_path, 'packing_list', 'packing-list.txt', b'Synthetic packing list & reference <inert>.')['id']
exd_office_stream = io.BytesIO()
with zipfile.ZipFile(exd_office_stream, 'w') as archive:
    archive.writestr('[Content_Types].xml', '<Types/>')
    archive.writestr('word/document.xml', '<document><p><r><t>Synthetic &lt;script&gt;alert(1)&lt;/script&gt; office proof</t></r></p></document>')
exd_office_bytes = exd_office_stream.getvalue()
exd_office = exd_upload(exd_path, 'shipping_bill', 'shipping-proof.docx', exd_office_bytes)['id']
exd_image_bytes = base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a5gAAAABJRU5ErkJggg==')
exd_image = exd_upload(exd_path, 'delivery_challan', 'delivery-proof.png', exd_image_bytes)['id']
for exd_preview_id, expected_type, expected_bytes in (
    (exd_pdf, 'application/pdf', payload),
    (exd_image, 'image/png', exd_image_bytes),
):
    headers, raw = exd_get(exd_preview_id, '?preview=1')
    check(headers['Content-Disposition'].startswith('inline') and headers['Content-Type'] == expected_type,
          'Validated PDF/raster preview uses an inline safe content type')
    check(raw == expected_bytes, 'Validated preview preserves original document bytes')
    check('no-store' in headers['Cache-Control'] and headers['X-Content-Type-Options'] == 'nosniff' and
          headers['Content-Security-Policy'] == 'sandbox', 'Export preview remains private, sandboxed and non-sniffable')
    download_headers, download = exd_get(exd_preview_id, '?download=1')
    check(download_headers['Content-Disposition'].startswith('attachment') and download == expected_bytes,
          'Explicit download always preserves original attachment bytes')
headers, office_preview = exd_get(exd_office, '?preview=1')
check(headers['Content-Disposition'].startswith('attachment') and headers['Content-Type'] == 'application/octet-stream' and
      office_preview == exd_office_bytes, 'Office preview request uses the original-file download fallback')
check(headers['Content-Security-Policy'] == 'sandbox' and headers['X-Content-Type-Options'] == 'nosniff',
      'Office fallback is sandboxed and cannot be sniffed as HTML')
check(exd_get(exd_office, '?download=1')[1] == exd_office_bytes, 'Office download retains the exact original ZIP document')
exd_sheet_stream = io.BytesIO()
with zipfile.ZipFile(exd_sheet_stream, 'w') as archive:
    archive.writestr('[Content_Types].xml', '<Types/>')
    archive.writestr('xl/workbook.xml', '<workbook/>')
exd_sheet_bytes = exd_sheet_stream.getvalue()
exd_sheet = exd_upload(exd_path, 'bank_advice', 'bank-advice.xlsx', exd_sheet_bytes)['id']
headers, sheet_preview = exd_get(exd_sheet, '?preview=1')
check(headers['Content-Disposition'].startswith('attachment') and headers['Content-Type'] == 'application/octet-stream' and
      sheet_preview == exd_sheet_bytes, 'Spreadsheet preview uses the exact original-file download fallback')
headers, raw = exd_get(exd_text, '?preview=1')
check(headers['Content-Disposition'].startswith('attachment') and headers['Content-Type'] == 'application/octet-stream',
      'Formats without a safe inline viewer use an attachment fallback')
exd_kinds = {row['id']: row['kind'] for row in exd_detail(exd_path)['files']}
check(exd_kinds == {exd_pdf: 'invoice', exd_text: 'packing_list', exd_office: 'shipping_bill', exd_image: 'delivery_challan', exd_sheet: 'bank_advice'},
      'Each export attachment retains only its own document category')
for actor, status in ((Client(), 401), (ex_reader, 403), (exw_noview, 403), (exw_unsupported, 403)):
    exd_get(exd_pdf, '?preview=1', actor, status)
for actor in (ex_team_b, ex_buyer):
    exd_get(ex_file_id, '?preview=1', actor, 404)

# Stored MIME hints never turn content into executable HTML or change download bytes.
sql(f"UPDATE export_files SET mime='text/html' WHERE id={exd_pdf}", name)
headers, raw = exd_get(exd_pdf, '?preview=1')
check(headers['Content-Type'] == 'application/pdf' and raw == payload,
      'Preview derives safe presentation from validated bytes rather than a forged stored MIME hint')
sql(f"UPDATE export_files SET mime='application/pdf' WHERE id={exd_pdf}", name)


def exd_remove(path, fid, actor=exd_writer, expect=200, version=None):
    return actor.call(path + '/files/' + str(fid) + '/remove',
                      {'version': exd_version(path) if version is None else version}, expect)


def exd_workflow(path, action, actor=exd_writer, expect=200, **values):
    return actor.call(path + '/workflow', {'version': exd_version(path), 'action': action, **values}, expect)


def exd_file_hash(fid):
    return sql(f'SELECT md5(to_jsonb(f)::text) FROM export_files f WHERE id={fid}', name)


exd_original_file_hash = exd_file_hash(exd_pdf)
exd_pre_remove = exd_detail(exd_path)
exd_old_version = exd_pre_remove['record']['version']
exd_comment = exd_writer.call(exd_path + '/comments', {'version': exd_old_version, 'note': 'Verify file version before removal'})
exd_remove(exd_path, exd_pdf, version=exd_old_version, expect=409)
for actor in (ex_reader, exw_noview, exw_unsupported, exd_reviewer):
    exd_remove(exd_path, exd_pdf, actor, 403)
for actor in (ex_team_b, ex_buyer):
    actor.call(ex_path + '/files/' + str(ex_file_id) + '/remove', {'version': ex_record()['version']}, 404)
exd_remove(exd_path, ex_file_id, expect=404)
exd_writer.call(exd_path + '/files/' + str(exd_pdf) + '/remove', {'version': exd_version(exd_path)},
                403, {'X-CSRF-Token': ''})
exd_before_remove = exd_detail(exd_path)['record']
exd_removed = exd_remove(exd_path, exd_pdf)
exd_after_remove = exd_detail(exd_path)
check(exd_removed['removed'] is True and exd_removed['id'] == exd_pdf and exd_removed['invoice_id'] == exd_id,
      'Removal confirms the exact invoice attachment identity')
check(exd_file_hash(exd_pdf) == exd_original_file_hash,
      'Removing a document preserves its original metadata and encrypted bytes without physical deletion')
check(not any(f['id'] == exd_pdf for f in exd_after_remove['files']) and
      {f['id'] for f in exd_after_remove['files']} == {exd_text, exd_office, exd_image, exd_sheet},
      'Removing the invoice file preserves unrelated checklist categories')
check(exd_after_remove['record']['version'] == exd_before_remove['version'] + 1 and
      exd_after_remove['record']['content_revision'] == exd_before_remove['content_revision'] + 1,
      'Attachment removal advances both invoice concurrency and content revisions')
exd_removal = json.loads(sql(f"SELECT row_to_json(x)::text FROM export_file_removals x WHERE file_id={exd_pdf}", name))
check(exd_removal['removed_by'] == exd_writer_id and exd_removal['removed_at'],
      'Removal records the acting employee and timestamp')
for suffix in ('', '?preview=1', '?download=1'):
    exd_get(exd_pdf, suffix, expect=404)
exd_remove(exd_path, exd_pdf, expect=404)
exd_workflow(exd_path, 'submit', expect=400)
exd_latest_history = exd_after_remove['history'][0]
exd_history = exd_writer.call(exd_path + '/history/' + str(exd_latest_history['id']))['snapshot']
check(any(f['id'] == exd_pdf and f['removed_by'] == exd_writer_id for f in exd_history['removed_files']) and
      not any(f['id'] == exd_pdf for f in exd_history['files']),
      'Audit snapshot distinguishes removed documents from active category attachments')
check('ciphertext' not in json.dumps(exd_history), 'Document history exposes metadata without encrypted payloads or private bytes')
exd_previous_snapshot = exd_writer.call(exd_path + '/history/' + str(exd_pre_remove['history'][0]['id']))['snapshot']
check(any(f['id'] == exd_pdf for f in exd_previous_snapshot['files']),
      'Earlier immutable audit snapshot still records the original attached document')

# A replacement invoice document restores submission; submitted and approved
# files stay frozen until the invoice is explicitly sent back or reopened.
exd_replacement = exd_upload(exd_path, filename='replacement-invoice.pdf')['id']
exd_workflow(exd_path, 'submit')
exd_remove(exd_path, exd_replacement, expect=400)
exd_upload(exd_path, expect=400)
exd_workflow(exd_path, 'request_changes', exd_reviewer, note='Replace the unreadable invoice scan')
exd_remove(exd_path, exd_replacement)
exd_workflow(exd_path, 'submit', expect=400)
exd_approved_file = exd_upload(exd_path, filename='corrected-invoice.pdf')['id']
exd_workflow(exd_path, 'submit')
exd_workflow(exd_path, 'approve', exd_reviewer)
exd_remove(exd_path, exd_approved_file, expect=400)
exd_workflow(exd_path, 'reopen', note='Refresh final invoice attachment')
exd_remove(exd_path, exd_approved_file)
exd_workflow(exd_path, 'submit', expect=400)

# Tampered display metadata never permits HTML execution under an inline name.
exd_office_mime = sql(f'SELECT mime FROM export_files WHERE id={exd_office}', name)
sql(f"UPDATE export_files SET filename='unsafe.html',mime='text/html' WHERE id={exd_office}", name)
headers, unsafe_preview = exd_get(exd_office, '?preview=1')
check(headers['Content-Disposition'].startswith('attachment') and headers['Content-Type'] == 'application/octet-stream' and
      headers['Content-Security-Policy'] == 'sandbox' and unsafe_preview == exd_office_bytes,
      'Unexpected filename or MIME uses a sandboxed download fallback rather than executing content')
sql(f"UPDATE export_files SET filename='shipping-proof.docx',mime='{exd_office_mime}' WHERE id={exd_office}", name)


def exd_race_request(path, request_kind, fid, version):
    actor = Client('qa.export.documents')
    headers = {'Origin': os.getenv('COMPANY_TEST_ORIGIN', base), 'X-CSRF-Token': actor.csrf}
    if request_kind == 'upload':
        data, upload_headers = exd_multipart('concurrent-invoice.pdf', payload)
        headers.update(upload_headers)
        url = path + '/files?kind=invoice&version=' + str(version)
    else:
        data = json.dumps({'version': version, **({'action': 'submit'} if request_kind == 'submit' else {})}).encode()
        headers['Content-Type'] = 'application/json'
        url = path + ('/workflow' if request_kind == 'submit' else '/files/' + str(fid) + '/remove')
    request = urllib.request.Request(base + '/api' + url, data=data, headers=headers)
    try:
        response = actor.opener.open(request, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    return request_kind, response.status


for exd_pair in (('remove', 'remove'), ('remove', 'upload'), ('remove', 'submit'), ('upload', 'submit')):
    exd_race_path, exd_race_id = exd_new('-'.join(exd_pair))
    exd_race_file = exd_upload(exd_race_path)['id']
    exd_race_original_hash = exd_file_hash(exd_race_file)
    exd_race_version = exd_version(exd_race_path)
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        exd_race_results = list(pool.map(lambda operation: exd_race_request(exd_race_path, operation, exd_race_file, exd_race_version), exd_pair))
    check(sorted(code for _, code in exd_race_results) == [200, 409],
          'Concurrent ' + '/'.join(exd_pair) + ' commits exactly one invoice revision')
    exd_race_detail = exd_detail(exd_race_path)
    if exd_race_detail['record']['workflow_status'] == 'Submitted':
        check(any(f['kind'] == 'invoice' for f in exd_race_detail['files']),
              'Concurrent submit never leaves a submitted revision without an active invoice document')
    check(exd_file_hash(exd_race_file) == exd_race_original_hash,
          'Concurrent attachment operations retain the exact original metadata and encrypted bytes')

# SQL policies enforce file capability, parent visibility/state and attribution.
def exd_removal_count(identity):
    output = exw_runtime_sql(f'SELECT json_build_array(count(*)) FROM export_file_removals WHERE file_id={exd_pdf}', identity)
    return json.loads(next(line for line in output.splitlines() if line.startswith('[')))[0]


check(exd_removal_count(exw_identity(uid=exd_writer_id)) == 1, 'Removal metadata follows the visible standalone parent in RLS')
for identity in (exw_identity(exports=False), exw_identity(role='Marketing')):
    check(exd_removal_count(identity) == 0, 'Hidden or unsupported users cannot read removal metadata through SQL')
for identity, expected_denial in (
    (exw_identity(uid=exd_writer_id, exports_files=False), 'row-level security'),
    (exw_identity(uid=exd_writer_id, exports=False, exports_files=True), 'only editable documents or authorized reconciliation ebrc evidence'),
    (exw_identity(uid=exd_writer_id, role='Marketing', exports_files=True), 'only editable documents or authorized reconciliation ebrc evidence'),
):
    try:
        exw_runtime_sql(f'INSERT INTO export_file_removals(file_id,removed_by) VALUES({exd_text},{exd_writer_id})', identity)
        raise AssertionError('Direct SQL removed a file without capability/visibility')
    except subprocess.CalledProcessError as error:
        check(expected_denial in error.stderr.lower(), 'Direct SQL removal requires export view and file capabilities')
try:
    exw_runtime_sql(f'INSERT INTO export_file_removals(file_id,removed_by) VALUES({exd_text},{exw_sa_id})',
                    exw_identity(uid=exd_writer_id, exports_files=True))
    raise AssertionError('Removal attribution was forgeable')
except subprocess.CalledProcessError as error:
    check('row-level security' in error.stderr.lower(), 'Runtime cannot attribute a document removal to another employee')
for statement in (f'UPDATE export_file_removals SET removed_by={exw_sa_id} WHERE file_id={exd_pdf}',
                  f'DELETE FROM export_file_removals WHERE file_id={exd_pdf}'):
    try:
        sql(statement, name)
        raise AssertionError('Removal audit was mutable')
    except subprocess.CalledProcessError:
        check(True, 'Document removal audit is immutable even to an owner connection')
exd_sql_path, exd_sql_id = exd_new('SQL-STATE-GUARD')
exd_sql_file = exd_upload(exd_sql_path)['id']
exd_workflow(exd_sql_path, 'submit')
for expected_state in ('Submitted', 'Approved'):
    if expected_state == 'Approved':
        exd_workflow(exd_sql_path, 'approve', exd_reviewer)
    try:
        exw_runtime_sql(f'INSERT INTO export_file_removals(file_id,removed_by) VALUES({exd_sql_file},{exd_writer_id})',
                        exw_identity(uid=exd_writer_id, exports_files=True))
        raise AssertionError('Direct SQL removed a frozen invoice attachment')
    except subprocess.CalledProcessError as error:
        check('only editable documents or authorized reconciliation ebrc evidence' in error.stderr.lower(),
              'Database removal guard protects ' + expected_state.lower() + ' invoice attachments independently of HTTP')
check(exd_get(exd_sql_file, '?download=1')[1] == payload,
      'Rejected direct SQL removals leave approved attachment downloadable and unchanged')
print('Export document HTTP/RLS checks passed: ' + str(checks - exd_start), flush=True)
