# Loaded by the disposable company suite after export workflow/document cases.
# No SMTP, bank connection, payment execution or production record is involved.
assert name.startswith('erp_test_') and runtime_role == name + '_api'
check(not env.get('SMTP_HOST'), 'Banking field tests cannot send real bank or employee email')
exb_start = checks
exb_writer_id, exb_writer = export_user('qa.export.banking.writer', companies=[])
exb_reviewer_id, exb_reviewer = export_user('qa.export.banking.md', 'MD', ['export_view', 'export_download', 'export_approve'], companies=[])
exb_body = {
    **exw_body, 'invoice_number': 'EX-BANKING-001', 'invoice_amount': '1000.00',
    'receipts': [{
        'reference': 'FOREIGN-BANK-REF', 'receipt_date': '2026-10-10',
        'currency': 'USD', 'gross_payment_received': '1000.00',
        'payment_received': '990.00', 'fcr_amount': '1000.00', 'bank_advice_amount': '990.00',
        'forex_account': 'BOB FOREX ACCOUNT', 'instruction_action': 'Credit FC account',
        'fc_account_currency': 'USD', 'bank_instruction_reference': 'INST-001',
        'bank_advice_reference': 'ADV-001', 'bank_advice_date': '2026-10-10',
        'inr_advice_reference': 'BOB-INR-001', 'inr_advice_date': '2026-10-10',
        'inr_received_amount': '81000.00', 'inr_bank_charges': '100.00',
        'inr_currency': 'INR', 'inr_conversion_rate': '81.123456',
        'credit_bank_344': '344', 'credit_bank_344_amount': '81000.00',
        'credit_bank_344_currency': 'INR', 'conversion_debit_account': 'D/ACCOUNT',
        'gst_amount': '18.00', 'gst_currency': 'INR',
    }], 'ebrcs': [],
}
exb_id = exb_writer.call('/exports', exb_body)['id']
exb_path = '/exports/' + str(exb_id)


def exb_record():
    return exb_writer.call(exb_path)['record']


def exb_save(body, expect=200, version=None, actor=exb_writer):
    return actor.call(exb_path, {**body, 'version': exb_record()['version'] if version is None else version}, expect)


exb_saved = exb_record()
check(exb_saved['receipts'][0]['foreign_bank_charges'] == '10.00' and
      exb_saved['receipts'][0]['foreign_bank_charges_currency'] == 'USD',
      'Banking FCR minus bank advice produces exact same-currency foreign charges')
check(exb_saved['balance']['outstanding'] == '0.00' and
      exb_saved['balance']['payment_received'] == '990.00' and
      exb_saved['balance']['bank_charges'] == '0.00',
      'INR advice, GST, FCR, and account allocations are not counted again in settlement')
for exb_key in ('forex_account', 'instruction_action', 'fc_account_currency', 'bank_instruction_reference',
                'inr_advice_reference', 'inr_advice_date', 'inr_received_amount', 'inr_bank_charges',
                'inr_currency', 'inr_conversion_rate', 'credit_bank_344', 'credit_bank_344_amount',
                'credit_bank_344_currency', 'conversion_debit_account', 'gst_amount', 'gst_currency'):
    check(exb_saved['receipts'][0][exb_key] == exb_body['receipts'][0][exb_key],
          'Banking field round-trips: ' + exb_key)

# Invalid writes preserve the saved invoice and its history, including version.
exb_fingerprint = sql(f'SELECT md5(to_jsonb(d)::text) FROM export_invoices d WHERE id={exb_id}', name)
for exb_patch in (
    {'bank_advice_amount': '1000.01'}, {'fcr_amount': '-1'}, {'fcr_amount': '1.001'},
    {'bank_advice_amount': 1000}, {'instruction_action': 'Transfer automatically'},
    {'fc_account_currency': 'EUR'}, {'disposal_credit_target': 'Unknown'},
    {'inr_currency': 'USD'}, {'inr_conversion_rate': '0'}, {'inr_conversion_rate': '1.1234567'},
    {'inr_advice_date': '2026-02-30'}, {'forex_account': 'BOB\nOther'},
    {'automatic_send': True}, {'bank_advice_currency': 'GBP'},
):
    exb_save({**exb_body, 'receipts': [{**exb_body['receipts'][0], **exb_patch}]}, 400)
check(sql(f'SELECT md5(to_jsonb(d)::text) FROM export_invoices d WHERE id={exb_id}', name) == exb_fingerprint,
      'Rejected banking field changes preserve invoice state exactly')
for exb_actor in (ex_reader, exb_reviewer, exw_noview, exw_unsupported):
    exb_save(exb_body, 403, actor=exb_actor)
Client().call(exb_path, expect=401)
exb_writer.call(exb_path, {**exb_body, 'version': exb_record()['version']}, 403, {'X-CSRF-Token': ''})

# Cross-currency conversion is explicit; no rate or net advice is silently used.
exb_cross = {**exb_body, 'invoice_amount': '125.00', 'receipts': [{
    'currency': 'GBP', 'fcr_amount': '100.00', 'bank_advice_amount': '98.00',
    'payment_received': '98.00', 'invoice_currency_amount': '122.50',
}]}
exb_save(exb_cross, 400)
exb_cross['receipts'][0]['foreign_bank_charges_invoice_amount'] = '2.50'
exb_old_version = exb_record()['version']
exb_save(exb_cross)
exb_save(exb_body, 409, version=exb_old_version)
check(exb_record()['balance']['outstanding'] == '0.00',
      'Cross-currency receipt and derived charges use confirmed invoice equivalents')

# Existing receipts with no FCR pair keep their original recorded settlement.
exb_legacy = {**exb_body, 'receipts': [{
    'currency': 'USD', 'payment_received': '980.00', 'foreign_bank_charges': '5.00',
    'foreign_bank_charges_currency': 'USD', 'bank_charges': '15.00', 'bank_charges_currency': 'USD',
}]}
exb_save(exb_legacy)
check(exb_record()['balance']['outstanding'] == '0.00' and
      exb_record()['receipts'][0]['foreign_bank_charges'] == '5.00',
      'Legacy manual foreign and Indian bank charges remain compatible')
exb_partial = {**exb_body, 'receipts': [{'currency': 'USD', 'fcr_amount': '1000.00'}]}
exb_save(exb_partial)
check('foreign_bank_charges' not in exb_record()['receipts'][0] and
      exb_record()['balance']['outstanding'] == '1000.00',
      'An incomplete banking draft never invents credited funds or charges')

# The exact same approval boundary applies after banking changes.
exb_save(exb_body)
exw_upload_invoice(exb_path, actor=exb_writer)
exb_writer.call(exb_path + '/workflow', {'version': exb_record()['version'], 'action': 'submit'})
exb_save(exb_body, 400)
exb_reviewer.call(exb_path + '/workflow', {'version': exb_record()['version'], 'action': 'request_changes', 'note': 'Check bank advice reference.'})
exb_corrected = {**exb_body, 'receipts': [{**exb_body['receipts'][0], 'bank_advice_reference': 'ADV-CORRECTED'}]}
exb_save(exb_corrected)
check(exb_record()['workflow_status'] == 'Draft', 'Banking correction resets the invoice to a fresh draft revision')
exb_writer.call(exb_path + '/workflow', {'version': exb_record()['version'], 'action': 'submit'})
exb_reviewer.call(exb_path + '/workflow', {'version': exb_record()['version'], 'action': 'approve'})
exb_save(exb_body, 400)
check(exb_record()['workflow_status'] == 'Approved' and
      exb_record()['receipts'][0]['bank_advice_reference'] == 'ADV-CORRECTED',
      'Independent MD review approves the corrected banking revision and freezes it')
exb_xlsx = exb_writer.call('/exports/export?search=EX-BANKING-001')
with zipfile.ZipFile(io.BytesIO(exb_xlsx)) as exb_archive:
    exb_workbook_text = ''.join(exb_archive.read(exb_member).decode('utf-8')
                              for exb_member in exb_archive.namelist()
                              if exb_member.endswith('.xml'))
check('inr conversion rate' in exb_workbook_text and '81.123456' in exb_workbook_text,
      'Register Excel includes the INR conversion rate header and exact six-decimal value')
check('bank advice amount' in exb_workbook_text and 'forex account' in exb_workbook_text and
      'BOB FOREX ACCOUNT' in exb_workbook_text,
      'Register Excel retains the banking revision fields alongside existing receipt columns')
print('Export banking HTTP checks passed: ' + str(checks - exb_start), flush=True)
