import {test} from 'node:test';
import assert from 'node:assert/strict';
import {exportPartyKey,exportRegisterQuery,exportInvoiceBody,newJVForm,jvFields,jvTabs,moneyMinor,minorDecimal,exportWorkflowStatuses,invoiceCompanyChoices,invoiceFileKey,invoiceFileError,invoiceContent,exportRequiredDocument,bankingStages,additionalReceiptFields,allReceiptFields,receiptFields,conversionFields,newExportReceipt,ensureReceiptKeys,receiptDocumentSlots,documentInSlot,receiptDocumentLabel,savedEbrcNumbers,reconciliationSelectionValid,reconciliationRegisterQuery,canonicalSavedEbrcNumber,reconciliationAlreadyCleared,foreignChargeDifference,bankEmailRecipients,bankEmailRecipientError,bankEmailDocumentKinds,bankEmailCanConfirm,bankEmailAttachmentError,uncertainInvoiceWrite,hasRequiredInvoiceDocument,exportFileCanPreview,saveInvoiceFiles} from '../src/lib/exportDocumentation.ts';

test('unassigned register omits company ID instead of sending an invalid zero',()=>{
 assert.equal(exportRegisterQuery('').toString(),'scope=standalone');
 assert.equal(exportRegisterQuery('42').toString(),'scope=company&company_id=42');
});
test('manual party filters normalize identity without losing the displayed customer name',()=>{
 assert.equal(exportPartyKey({party_type:'manual',party_name:'  Coast & Co  '}),exportPartyKey({party_type:'manual',name:'coast & co'}));
 assert.equal(exportPartyKey({party_type:'buyer',party_id:42,id:99}),'buyer:42');
 const query=exportRegisterQuery('',{party:{party_type:'manual',name:' Coast & Co '},status:'Submitted',search:'A&B'});
 assert.equal(query.get('party_name'),'Coast & Co');
 assert.equal(query.get('party_id'),null);
 assert.equal(query.get('status'),'Submitted');
 assert.equal(query.get('search'),'A&B');
});
test('standalone payload never includes stale linked company, buyer or PO identifiers',()=>{
 const profile={shipping_bill_number:'SB-12'},receipts=[{payment_received:'990.00',bank_charges:'10.00'}];
 const body=exportInvoiceBody({company_id:null,party_type:'buyer',party_id:44,order_id:55,party_name:' Customer A ',invoice_number:'INV-1',currency:'USD',invoice_amount:'1000.00',profile,receipts,ebrcs:[],version:3});
 assert.equal(body.company_id,null);assert.equal(body.party_id,null);assert.equal(body.order_id,null);
 assert.equal(body.party_type,'manual');assert.equal(body.party_name,'Customer A');
 assert.equal(body.profile,profile);assert.equal(body.receipts,receipts);assert.equal(body.version,3);
});
test('linked invoice payload keeps existing identity and money strings',()=>{
 const body=exportInvoiceBody({company_id:'5',party_type:'buyer',party_id:'8',order_id:'12',party_name:'ignored',invoice_amount:'123456789.90',profile:{},receipts:[],ebrcs:[]});
 assert.equal(body.company_id,5);assert.equal(body.party_id,8);assert.equal(body.order_id,12);
 assert.equal(body.invoice_amount,'123456789.90');assert.equal('party_name' in body,false);
});
test('approval states remain separate from exact settlement arithmetic',()=>{
 assert.deepEqual(exportWorkflowStatuses,['Draft','Submitted','Changes requested','Approved']);
 assert.equal(minorDecimal(moneyMinor('1000.00')-moneyMinor('990.00')-moneyMinor('10.00')),'0.00');
 assert.equal(minorDecimal(moneyMinor('0.30')-moneyMinor('0.10')),'0.20');
});

test('invoice company choices deduplicate stored names and preserve a current legacy name',()=>{
 assert.deepEqual(invoiceCompanyChoices([{id:1,name:'  EXAMPLE RETAIL LTD '},{id:2,name:'example   retail ltd'},{name:'Other Company'}],'Legacy Buyer'),['EXAMPLE RETAIL LTD','Other Company','Legacy Buyer']);
 assert.deepEqual(invoiceCompanyChoices([{name:'Saved buyer'}],'Saved buyer'),['Saved buyer']);
});
test('invoice document queue rejects empty, oversized and unsupported documents',()=>{
 const valid=new File(['%PDF-1.4'],'invoice.PDF',{lastModified:12,type:'application/pdf'});
 assert.equal(invoiceFileError(valid),'');
 assert.equal(invoiceFileKey(valid),invoiceFileKey(new File(['%PDF-1.4'],'invoice.PDF',{lastModified:12,type:'application/pdf'})));
 assert.match(invoiceFileError(new File([],'empty.pdf')),/not empty/);
 assert.match(invoiceFileError(new File([new Uint8Array(10*1024*1024+1)],'large.pdf')),/10 MB/);
 assert.match(invoiceFileError(new File(['x'],'script.html')),/Choose a PDF/);
});
const draft=()=>({company_id:null,party_type:'manual',party_name:'Customer',invoice_number:'INV-1',currency:'USD',invoice_amount:'100.00',profile:{},receipts:[],ebrcs:[]});
const queued=(name,kind='invoice')=>({kind,file:new File(['%PDF-1.4'],name,{lastModified:1}),key:name,status:'queued'});
test('a partial upload retry keeps one invoice and skips successfully attached files',async()=>{
 const form=draft(),state={savedContent:'',pendingSave:null,files:[queued('a.pdf'),queued('b.pdf')]};
 let saves=0,failed=false;const uploads=[];
 const mutate=async()=>{saves++;return {id:9,version:1}};
 const upload=async(path,file)=>{uploads.push([path,file.name]);if(file.name==='b.pdf'&&!failed){failed=true;throw Object.assign(new Error('Busy'),{status:429})}return {id:file.name==='a.pdf'?11:12,version:file.name==='a.pdf'?2:3}};
 await assert.rejects(saveInvoiceFiles(form,state,mutate,upload),/Busy/);
 assert.equal(form.id,9);assert.equal(form.version,2);assert.equal(state.files[0].status,'uploaded');assert.equal(state.files[1].status,'failed');
 await saveInvoiceFiles(form,state,mutate,upload);
 assert.equal(saves,1);assert.equal(form.version,3);
 assert.deepEqual(uploads.map(x=>x[1]),['a.pdf','b.pdf','b.pdf']);
 assert.equal(uploads[1][0],uploads[2][0]);
});
test('changed fields after a partial upload save against the acknowledged version',async()=>{
 const form=draft(),state={savedContent:'',pendingSave:null,files:[queued('a.pdf'),queued('b.pdf')]},saved=[];let failed=false;
 const mutate=async(path,body)=>{saved.push({path,body});return {id:9,version:body.version?body.version+1:1}};
 const upload=async(path,file)=>{if(file.name==='b.pdf'&&!failed){failed=true;throw Object.assign(new Error('Busy'),{status:429})}return {id:4,version:Number(new URL('https://example.test'+path).searchParams.get('version'))+1}};
 await assert.rejects(saveInvoiceFiles(form,state,mutate,upload));form.invoice_amount='200.00';
 await saveInvoiceFiles(form,state,mutate,upload);
 assert.equal(saved.length,2);assert.equal(saved[1].path,'/exports/9');assert.equal(saved[1].body.version,2);assert.equal(saved[1].body.invoice_amount,'200.00');assert.equal(form.version,4);
});
test('unconfirmed creates retain their exact request even if the form is later edited',async()=>{
 const form=draft(),state={savedContent:'',pendingSave:null,files:[]},requests=[];
 const mutate=async(path,body)=>{requests.push({path,body});throw Object.assign(new Error('Unconfirmed'),requests.length===1?{}:{status:409,code:'action_already_submitted'})};
 await assert.rejects(saveInvoiceFiles(form,state,mutate,async()=>({})));form.invoice_amount='200.00';
 await assert.rejects(saveInvoiceFiles(form,state,mutate,async()=>({})));
 assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1]);assert.equal(requests[1].body.invoice_amount,'100.00');assert.ok(state.pendingSave);assert.equal(form.id,undefined);
});
test('confirmed validation failures allow correction instead of replaying invalid input',async()=>{
 const form=draft(),state={savedContent:'',pendingSave:null,files:[]};let calls=0;
 const mutate=async(path,body)=>{calls++;if(calls===1)throw Object.assign(new Error('Validation'),{status:400});assert.equal(body.invoice_amount,'200.00');return {id:9,version:1}};
 await assert.rejects(saveInvoiceFiles(form,state,mutate,async()=>({})));assert.equal(state.pendingSave,null);
 form.invoice_amount='200.00';await saveInvoiceFiles(form,state,mutate,async()=>({}));assert.equal(form.id,9);
});
test('unconfirmed file attempts keep their URL and File object and block later writes',async()=>{
 const form={...draft(),id:9,version:5},state={savedContent:'',pendingSave:null,files:[queued('a.pdf')]},uploads=[];state.savedContent=invoiceContent(form);let saves=0;
 const upload=async(path,file)=>{uploads.push({path,file});throw Object.assign(new Error('Unconfirmed'),uploads.length===1?{}:{status:409,code:'action_already_submitted'})};
 const mutate=async()=>{saves++;return {}};
 await assert.rejects(saveInvoiceFiles(form,state,mutate,upload));form.invoice_amount='200.00';
 await assert.rejects(saveInvoiceFiles(form,state,mutate,upload));
 assert.equal(saves,0);assert.equal(uploads[0].path,uploads[1].path);assert.equal(uploads[0].file,uploads[1].file);assert.equal(state.files[0].status,'uncertain');assert.equal(form.version,5);
});

test('only the invoice checklist category satisfies the submission requirement',()=>{
 assert.equal(hasRequiredInvoiceDocument([{kind:'packing_list'}],[queued('evidence.pdf','bank_advice')]),false);
 assert.equal(hasRequiredInvoiceDocument([{kind:'invoice'}],[]),true);
 assert.equal(hasRequiredInvoiceDocument([],[queued('invoice.pdf')]),true);
});
test('checklist upload queue keeps categories across saves and retries',async()=>{
 const form=draft(),state={savedContent:'',pendingSave:null,files:[queued('same.pdf','invoice'),queued('same.pdf','packing_list')]},kinds=[];
 await saveInvoiceFiles(form,state,async()=>({id:5,version:1}),async(path)=>{const query=new URL('https://example.test'+path).searchParams;kinds.push(query.get('kind'));return {id:kinds.length,version:Number(query.get('version'))+1}});
 assert.deepEqual(kinds,['invoice','packing_list']);assert.equal(form.version,3);
});
test('secure previews use validated PDF/image metadata and Office falls back to download',()=>{
 for(const mime of ['application/pdf','image/png','image/jpeg'])assert.equal(exportFileCanPreview({mime}),true);
 for(const mime of ['application/vnd.openxmlformats-officedocument.wordprocessingml.document','application/vnd.openxmlformats-officedocument.spreadsheetml.sheet','text/html',''])assert.equal(exportFileCanPreview({mime,filename:'looks-like.pdf'}),false);
});
test('JV register queries never inherit linked company scope or mix with default exports',()=>{
 const jv=exportRegisterQuery('42',{record_kind:'jv',status:'Draft',currency:'EUR',party:{party_type:'manual',name:'EURO SUITS MFG'}});
 assert.equal(jv.get('record_kind'),'jv');assert.equal(jv.get('scope'),'standalone');assert.equal(jv.get('company_id'),null);
 assert.equal(jv.get('party_name'),'EURO SUITS MFG');assert.equal(jv.get('status'),'Draft');assert.equal(jv.get('currency'),'EUR');
 assert.equal(exportRegisterQuery('42').get('record_kind'),null);
});
test('new JV defaults are isolated and never infer a currency or current date',()=>{
 const first=newJVForm(),second=newJVForm();
 assert.equal(first.party_name,'EURO SUITS MFG');assert.equal(first.profile.shipping_mode,'HANDO');assert.equal(first.currency,'');assert.equal(first.profile.export_date,'');
 assert.equal(first.company_id,null);assert.equal(first.party_id,null);assert.equal(first.order_id,null);
 first.profile.shipping_mode='Sea';assert.equal(second.profile.shipping_mode,'HANDO');
});
test('JV payload contains only its eight field values and no stale settlement or linked identity',()=>{
 const form={...newJVForm(),company_id:4,party_id:8,order_id:12,party_type:'buyer',party_name:' Other JV party ',invoice_number:'JV/26/1',currency:'EUR',invoice_amount:'1234.56',version:7,profile:{export_date:'2026-10-09',shipping_mode:'',jv_by_date:'Asha / 09 Oct',scrip_tally_reference:'T-1',shipping_bill_number:'stale',drawback_claim_amount:'999'},receipts:[{payment_received:'100'}],ebrcs:[{ebrc_number:'stale'}]};
 const body=exportInvoiceBody(form);
 assert.equal(body.record_kind,'jv');assert.equal(body.party_name,'Other JV party');assert.equal(body.company_id,null);assert.equal(body.party_id,null);assert.equal(body.order_id,null);assert.equal(body.party_type,'manual');
 assert.deepEqual(body.profile,{export_date:'2026-10-09',shipping_mode:'',jv_by_date:'Asha / 09 Oct',scrip_tally_reference:'T-1'});
 assert.deepEqual(body.receipts,[]);assert.deepEqual(body.ebrcs,[]);assert.equal(body.invoice_amount,'1234.56');assert.equal(body.version,7);
});
test('JV schema maps exactly the workbook headers and offers only its relevant sections',()=>{
 assert.deepEqual(jvFields.map(field=>field.label),['JV Ref','Date','Party Name','Currency','Amount','Mode','By / Date','Tally Ref']);
 assert.deepEqual(jvFields.filter(field=>field.profile).map(field=>field.key),['export_date','shipping_mode','jv_by_date','scrip_tally_reference']);
 assert.deepEqual(jvTabs.map(tab=>tab.key),['shipment','documents','history']);
});
test('JV draft save preserves its classification and exact amount through attachment staging',async()=>{
 const form=newJVForm();Object.assign(form,{invoice_number:'JV-2',currency:'USD',invoice_amount:'0.30'});const state={savedContent:'',pendingSave:null,files:[]};let body;
 await saveInvoiceFiles(form,state,async(path,payload)=>{assert.equal(path,'/exports');body=payload;return {id:20,version:1}},async()=>{throw new Error('No upload expected')});
 assert.equal(body.record_kind,'jv');assert.equal(body.invoice_amount,'0.30');assert.equal(form.id,20);assert.equal(form.version,1);
});


test('JV submission accepts supporting JV documents only, including staged files',()=>{
 assert.deepEqual(exportRequiredDocument('jv'),{kind:'jv_supporting',label:'supporting JV document'});
 assert.deepEqual(exportRequiredDocument('export'),{kind:'invoice',label:'invoice document'});
 assert.equal(hasRequiredInvoiceDocument([{kind:'invoice'}],[queued('packing.pdf','packing_list')],'jv_supporting'),false);
 assert.equal(hasRequiredInvoiceDocument([{kind:'jv_supporting'}],[],'jv_supporting'),true);
 assert.equal(hasRequiredInvoiceDocument([],[queued('jv.pdf','jv_supporting')],'jv_supporting'),true);
});
test('a JV supporting document upload preserves classification and the protected version sequence',async()=>{
 const form=newJVForm();Object.assign(form,{invoice_number:'JV-3',currency:'GBP',invoice_amount:'123456789.90'});
 const state={savedContent:'',pendingSave:null,files:[queued('jv.pdf','jv_supporting')]};let request;
 await saveInvoiceFiles(form,state,async(path,body)=>{assert.equal(body.record_kind,'jv');return {id:25,version:1}},async(path,file)=>{request={path,file};return {id:31,version:2}});
 assert.equal(request.path,'/exports/25/files?version=1&kind=jv_supporting');
 assert.equal(request.file.name,'jv.pdf');assert.equal(form.version,2);assert.equal(form.invoice_amount,'123456789.90');
});


test('banking sequence preserves every historical receipt field exactly once',()=>{
 const keys=allReceiptFields.map(field=>field.key);
 assert.equal(keys.length,new Set(keys).size);
 for(const field of [...receiptFields,...conversionFields])assert.ok(keys.includes(field.key),field.key+' retained');
 assert.deepEqual(bankingStages.map(stage=>stage.key),['inward','disposal','credit']);
 assert.deepEqual(bankingStages[0].fields.slice(0,2).map(field=>[field.key,field.label]),[['inward_reference','Our reference number'],['receipt_date','Advice / received date']]);
 assert.deepEqual(bankingStages[2].fields.slice(0,2).map(field=>field.key),['credit_advice_bill_number','reference']);
 assert.equal(bankingStages[0].fields.some(field=>field.key==='reference'),false);
 assert.ok(additionalReceiptFields.some(field=>field.key==='bank_advice_reference'));
 assert.ok(additionalReceiptFields.some(field=>field.key==='invoice_currency_amount'));
 assert.ok(additionalReceiptFields.some(field=>field.key==='net_payment_received'));
 assert.equal(bankingStages[1].fields.some(field=>field.key==='gross_payment_received'),false);
 assert.ok(additionalReceiptFields.some(field=>field.key==='payment_received'));
 assert.ok(additionalReceiptFields.some(field=>field.key==='fcr_amount'));
});
test('new receipt defaults never invent settlement amounts, instructions or conversion rates',()=>{
 const first=newExportReceipt('USD'),second=newExportReceipt('GBP');
 assert.equal(first.forex_account,'BOB FOREX ACCOUNT');assert.equal(first.currency,'USD');assert.equal(first.inr_currency,'INR');assert.equal(first.gst_currency,'INR');
 for(const key of ['payment_received','fcr_amount','bank_advice_amount','inr_conversion_rate','instruction_action'])assert.equal(first[key],undefined);
 first.statuses.note='changed';assert.deepEqual(second.statuses,{});assert.equal(second.currency,'GBP');
});
test('foreign bank charge difference uses exact decimal subtraction and rejects over-credit',()=>{
 assert.deepEqual(foreignChargeDifference({fcr_amount:'1000.00',bank_advice_amount:'990.00'}),{amount:'10.00',error:''});
 assert.deepEqual(foreignChargeDifference({fcr_amount:'0.30',bank_advice_amount:'0.10'}),{amount:'0.20',error:''});
 assert.deepEqual(foreignChargeDifference({fcr_amount:'999999999999999.99',bank_advice_amount:'0.01'}),{amount:'999999999999999.98',error:''});
 assert.equal(foreignChargeDifference({fcr_amount:'10',bank_advice_amount:'11'}).amount,'');
 assert.match(foreignChargeDifference({fcr_amount:'10',bank_advice_amount:'11'}).error,/cannot exceed/);
 for(const pair of [{},{fcr_amount:'12'},{fcr_amount:'12',bank_advice_amount:''},{fcr_amount:'1e3',bank_advice_amount:'3'},{fcr_amount:'1.234',bank_advice_amount:'1'}])assert.equal(foreignChargeDifference(pair),null);
 const receipt={fcr_amount:'100',bank_advice_amount:'90',payment_received:'70',foreign_bank_charges:'5'};foreignChargeDifference(receipt);assert.equal(receipt.payment_received,'70');assert.equal(receipt.foreign_bank_charges,'5');
});
test('bank email recipients normalize separators and reject missing, excessive or malformed recipients',()=>{
 assert.deepEqual(bankEmailRecipients(' One@Bank.example, two@bank.example; ONE@bank.example\nthree@bank.example '),['one@bank.example','two@bank.example','three@bank.example']);
 assert.equal(bankEmailRecipientError('one@bank.example;two@bank.example'),'');
 assert.match(bankEmailRecipientError(''),/Enter the bank/);assert.match(bankEmailRecipientError('missing-domain'),/valid/);
 assert.match(bankEmailRecipientError('x@a.test, y@a.test, z@a.test, a@a.test, b@a.test, c@a.test'),/five/);
 assert.deepEqual(bankEmailDocumentKinds,['invoice','shipping_bill','bill_of_lading','packing_list','bank_advice','payment_details']);
 assert.equal(bankEmailDocumentKinds.includes('jv_supporting'),false);
});


test('bank email confirmation requires a retry-safe status, ownership and the reviewed invoice version',()=>{
 const prepared={status:'Prepared',created_by:7,invoice_version:3};
 assert.equal(bankEmailCanConfirm(prepared,7,3),true);
 assert.equal(bankEmailCanConfirm({...prepared,status:'Failed'},7,3),true);
 for(const status of ['Sending','Accepted','Uncertain','unknown'])assert.equal(bankEmailCanConfirm({...prepared,status},7,3),false);
 assert.equal(bankEmailCanConfirm(prepared,8,3),false);assert.equal(bankEmailCanConfirm(prepared,7,4),false);assert.equal(bankEmailCanConfirm(null,7,3),false);
});
test('bank email attachment review rejects stale, duplicate, excessive and oversized selections',()=>{
 const files=Array.from({length:13},(_,index)=>({id:index+1,size:1024}));
 assert.equal(bankEmailAttachmentError([1,2],files),'');
 assert.match(bankEmailAttachmentError([],files),/Select the saved/);
 assert.match(bankEmailAttachmentError([1,1],files),/current saved/);
 assert.match(bankEmailAttachmentError([30],files),/current saved/);
 assert.match(bankEmailAttachmentError(files.map(file=>file.id),files),/twelve/);
 assert.equal(bankEmailAttachmentError([1],[{id:1,size:15*1024*1024}]),'');
 assert.match(bankEmailAttachmentError([1],[{id:1,size:15*1024*1024+1}]),/15 MB/);
});
test('email validation failures remain editable while lost or replayed responses need status verification',()=>{
 assert.equal(uncertainInvoiceWrite({status:400}),false);assert.equal(uncertainInvoiceWrite({status:409}),false);
 assert.equal(uncertainInvoiceWrite({status:409,code:'action_already_submitted'}),true);assert.equal(uncertainInvoiceWrite({status:503}),true);assert.equal(uncertainInvoiceWrite(new Error('network failed')),true);
});


test('receipt keys remain stable through edits and reordering while legacy rows receive independent IDs',()=>{
 const first=newExportReceipt('USD'),second=newExportReceipt('USD');assert.notEqual(first.receipt_key,second.receipt_key);
 const rows=[{reference:'legacy'},{receipt_key:first.receipt_key,reference:'saved'}];ensureReceiptKeys(rows);
 const legacy=rows[0].receipt_key;assert.match(legacy,/^[0-9a-f-]{36}$/);assert.equal(rows[1].receipt_key,first.receipt_key);
 rows.reverse();ensureReceiptKeys(rows);assert.equal(rows[0].receipt_key,first.receipt_key);assert.equal(rows[1].receipt_key,legacy);
});
test('receipt document slots never borrow shared documents or another receipt’s advice',()=>{
 assert.deepEqual(receiptDocumentSlots.inward,{kind:'payment_details',field:'inward_remittance',label:'Inward remittance advice'});
 const scoped={kind:'payment_details',receipt_key:'receipt-a',field_key:'inward_remittance'};
 assert.equal(documentInSlot(scoped,'payment_details','receipt-a','inward_remittance'),true);
 assert.equal(documentInSlot(scoped,'payment_details','receipt-b','inward_remittance'),false);
 assert.equal(documentInSlot(scoped,'payment_details','receipt-a','disposal_instruction'),false);
 assert.equal(documentInSlot(scoped,'payment_details'),false);
 assert.equal(documentInSlot({kind:'payment_details'},'payment_details','receipt-a','inward_remittance'),false);
 assert.equal(documentInSlot({kind:'invoice',receipt_key:null,field_key:null},'invoice'),true);
 assert.equal(receiptDocumentLabel(scoped,[{receipt_key:'receipt-b'},{receipt_key:'receipt-a'}]),'Receipt 2 · Inward remittance advice');
 assert.equal(receiptDocumentLabel({kind:'bank_advice'},[]),'Shared invoice document');
});
test('receipt-specific upload scope survives the protected save and retry sequence',async()=>{
 const form=draft();form.receipts=[newExportReceipt('USD')];const key=form.receipts[0].receipt_key;
 const file={...queued('advice.pdf','payment_details'),receipt_key:key,field_key:'inward_remittance'};const state={savedContent:'',pendingSave:null,files:[file]},paths=[];let failed=false,saves=0;
 await assert.rejects(saveInvoiceFiles(form,state,async(path,body)=>{saves++;assert.equal(body.receipts[0].receipt_key,key);return {id:8,version:1}},async(path)=>{paths.push(path);failed=true;throw Object.assign(new Error('busy'),{status:429})}));
 await saveInvoiceFiles(form,state,async()=>{saves++;return {}},async(path)=>{paths.push(path);return {id:40,version:2}});
 assert.equal(failed,true);assert.equal(saves,1);assert.equal(paths[0],paths[1]);
 const query=new URL('https://example.test'+paths[0]).searchParams;assert.equal(query.get('receipt_key'),key);assert.equal(query.get('field_key'),'inward_remittance');assert.equal(query.get('kind'),'payment_details');
});
test('documentation clearance requires only a saved eBRC number and active eBRC file, independently of approval or balance',()=>{
 const record={workflow_status:'Draft',balance:{outstanding:'1000.00'},ebrcs:[{status:'Pending',ebrc_number:' E-1 '},{status:'Received',ebrc_number:'E-2'},{ebrc_number:'E-1'}]},files=[{id:1,kind:'ebrc'},{id:2,kind:'invoice'}];
 assert.deepEqual(savedEbrcNumbers(record),['E-1','E-2']);assert.equal(reconciliationSelectionValid(record,files,'E-1',1),true);
 assert.equal(reconciliationSelectionValid(record,files,'E-3',1),false);assert.equal(reconciliationSelectionValid(record,files,'E-1',2),false);assert.equal(reconciliationSelectionValid(record,files,'E-1',3),false);
 assert.equal(reconciliationSelectionValid({...record,workflow_status:'Approved',balance:{outstanding:'0.00'}},files,'E-1',1),true);
});

test('reconciliation lists all authorised exports without a company, unassigned or JV filter',()=>{
 const query=reconciliationRegisterQuery();assert.deepEqual([...query.entries()],[['record_kind','export']]);
});
test('saved eBRC selection adopts existing canonical spelling after a duplicate add',()=>{
 const record={ebrcs:[{ebrc_number:' BANK /  E-01 '},{ebrc_number:'E-02',legacy_reference:'retained'}]};
 assert.equal(canonicalSavedEbrcNumber(record,'bank / e-01'),'BANK /  E-01');
 assert.equal(canonicalSavedEbrcNumber(record,' E-02 '),'E-02');
 assert.equal(canonicalSavedEbrcNumber(record,'E-03'),'');
 assert.equal(record.ebrcs[1].legacy_reference,'retained');
});
test('the same cleared evidence cannot be submitted twice and replacement evidence stays actionable',()=>{
 const record={documentation_cleared:{at:'2026-10-10T12:00:00Z',ebrc_number:'E-1',file_id:40}};
 assert.equal(reconciliationAlreadyCleared(record,'E-1',40),true);
 assert.equal(reconciliationAlreadyCleared(record,'E-2',40),false);
 assert.equal(reconciliationAlreadyCleared(record,'E-1',41),false);
 assert.equal(reconciliationAlreadyCleared({...record,documentation_cleared:{}},'E-1',40),false);
});
