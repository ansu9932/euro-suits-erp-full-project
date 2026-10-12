<script lang="ts">
 import {onMount,onDestroy,tick} from 'svelte';
 import {beforeNavigate} from '$app/navigation';
 import {Plus,ArrowLeft,Download,FileText,Search,RefreshCw,Trash2,Paperclip,Check,ChevronRight,Eye} from '@lucide/svelte';
 import {api,uploadTo} from './api';
 import ExportDocumentField from './ExportDocumentField.svelte';
 import ExportReconciliationWorkspace from './ExportReconciliationWorkspace.svelte';
 import {enter} from './motion';
 import type {Row,User} from './types';
 import {invoiceFields,jvFields,jvTabs,newJVForm,incentiveFields,bankingStages,allReceiptFields,newExportReceipt,ensureReceiptKeys,receiptDocumentSlots,documentInSlot,receiptDocumentLabel,ebrcFields,scripFields,clearanceFields,exportTabs,moneyText,moneyMinor,minorDecimal,exportDate,exportReference,exportWorkflowStatuses,exportPartyKey,exportRegisterQuery,invoiceCompanyChoices,invoiceFileKey,invoiceFileError,invoiceContent,exportFileCanPreview,exportRequiredDocument,hasRequiredInvoiceDocument,saveInvoiceFiles,type InvoiceSaveState,type ExportRecordKind,type ExportField,type ExportTab} from './exportDocumentation';
 let {user,dirty=$bindable(false)}:{user:User&{portal?:string;setup_grants?:string[]};dirty?:boolean}=$props();
 let options:Row=$state({companies:[],parties:[],orders:[],currencies:[],permissions:{},document_kinds:[]});
 let listing:Row=$state({records:[],totals:[]}),detail:Row|null=$state(null),form:Row=$state({profile:{},receipts:[],ebrcs:[]});
 let registerKind:ExportRecordKind=$state('export');
 let reconciliationSection=$state(false),reconciliationBusy=$state(false),initialReconciliationInvoice=$state(0);
 let isJV=$derived.by(()=>registerKind==='jv');
 let tabs=$derived(isJV?jvTabs:exportTabs.filter(item=>item.key!=='ebrc'));
 let recordLabel=$derived(isJV?'JV':'invoice');
 let company=$state(''),search=$state(''),year=$state(''),partyFilter=$state(''),currencyFilter=$state(''),workflowFilter=$state(''),tab:ExportTab=$state('shipment');
 let working=$state(false);
 let busy=$derived(working||reconciliationBusy);
 let editing=$state(false),loading=$state(true),error=$state(''),notice=$state(''),reviewNote=$state(''),reviewStatus=$state('Checked');
 let discountAmount=$state(''),discountReason=$state(''),decisionNotes:Record<number,string>=$state({}),revision:Row|null=$state(null);
 let workflowNote=$state(''),commentNote=$state('');
 let addingCompany=$state(false),newCompanyName=$state(''),companyError=$state('');
 let invoiceSave:InvoiceSaveState=$state({savedContent:'',pendingSave:null,files:[]});
 let root:HTMLElement; let returnID='export-new'; let savedID=$state(0); let focusFrame=0;
 const coreFields=invoiceFields.filter(f=>['invoice_number','invoice_date','currency','invoice_amount'].includes(f.key));
 const shipmentFields=invoiceFields.filter(f=>!['invoice_number','invoice_date','currency','invoice_amount'].includes(f.key));
 let permissions=$derived.by(()=>detail?.permissions||options.permissions||{});
 let invoiceCurrencyLocked=$derived.by(()=>!!form.id&&((detail?.record?.receipts||[]).length>0||(detail?.discounts||[]).some((d:Row)=>d.status==='Approved')));
 let parties=$derived(options.parties||[]);
 let registerParties=$derived.by(()=>{const choices=new Map<string,Row>();for(const party of parties)choices.set(exportPartyKey(party),party);for(const record of listing.records||[])if(record.party_type==='manual')choices.set(exportPartyKey(record),{party_type:'manual',name:record.party_name});return [...choices.values()]});
 let actions=$derived.by(()=>detail?.actions||{});
 let invoiceCompanies=$derived(invoiceCompanyChoices(options.standalone_parties||[],String(form.party_name||'')));
 let requiredDocument=$derived(exportRequiredDocument(registerKind));
 let requiredFiles=$derived.by(()=>(detail?.files||[]).filter((file:Row)=>file.kind===requiredDocument.kind));
 let canAttachInvoice=$derived(editing?!!permissions.files&&(!detail||!!actions.attach):!!actions.attach);
 let hasRequiredDocument=$derived(hasRequiredInvoiceDocument(requiredFiles,invoiceSave.files,requiredDocument.kind));
 let previewFallback=$state(0);
 let uncertainSave=$derived(!!invoiceSave.pendingSave||invoiceSave.files.some(file=>file.status==='uncertain'));
 let requiresOrder=$derived.by(()=>!!form.company_id&&!!options.permissions?.requires_order);
 let companySetupURL=$derived.by(()=>user.setup_grants?.includes('view')&&['admin','md','finance'].includes(user.portal||'')?'/'+user.portal:'');
 let linkedParty=$derived(editing?`${form.party_type||''}:${form.party_id||''}`:'');
 let availableOrders=$derived((options.orders||[]).filter((o:Row)=>form.party_type==='buyer'&&Number(o.buyer_id)===Number(form.party_id)));
 const partyKey=exportPartyKey;
 function fiscalYear(r:Row){const value=r.profile?.export_date||r.created_at;if(!value)return 'Undated';const d=new Date(String(value).slice(0,10)+'T12:00:00');const start=d.getMonth()<3?d.getFullYear()-1:d.getFullYear();return Number.isFinite(start)?`${start}-${String(start+1).slice(-2)}`:'Undated'}
 let years=$derived([...new Set<string>((listing.records||[]).map((r:Row)=>fiscalYear(r)))].sort().reverse());
 let records=$derived((listing.records||[]).filter((r:Row)=>(!year||fiscalYear(r)===year)&&(!currencyFilter||r.currency===currencyFilter)&&(!partyFilter||partyKey(r)===partyFilter)&&(!workflowFilter||r.workflow_status===workflowFilter)&&(!search||[r.invoice_number,r.party_name,r.profile?.export_reference,r.profile?.shipping_bill_number,...(isJV?[r.profile?.jv_by_date,r.profile?.scrip_tally_reference]:[exportReference(r.id)])].join(' ').toLowerCase().includes(search.trim().toLowerCase()))));
 let filteredTotals=$derived.by(()=>{const amounts=new Map<string,{count:number;invoice:bigint;outstanding:bigint}>();for(const r of records){const v=amounts.get(r.currency)||{count:0,invoice:0n,outstanding:0n};v.count++;v.invoice+=moneyMinor(r.invoice_amount);v.outstanding+=moneyMinor(r.balance?.outstanding);amounts.set(r.currency,v)}return [...amounts].map(([currency,value])=>({currency,count:value.count,invoice:minorDecimal(value.invoice),outstanding:minorDecimal(value.outstanding)}))});
 const query=()=>exportRegisterQuery(company,{record_kind:registerKind});
 const dateTime=(s:string)=>s?new Date(s).toLocaleString('en-GB',{dateStyle:'medium',timeStyle:'short'}):'—';
 const present=(v:unknown)=>v!==undefined&&v!==null&&v!=='';
 const fieldValue=(f:ExportField,v:unknown)=>!present(v)?'—':f.type==='date'?exportDate(v):f.type==='money'?moneyText(v):String(v);
 async function focus(id:string){await tick();root?.querySelector<HTMLElement>('#'+id)?.focus()}
 async function run(work:()=>Promise<void>){if(busy)return;working=true;error='';try{await work()}catch(e){error=(e as Error).message;await tick();root?.querySelector<HTMLElement>('[role="alert"]')?.focus()}finally{working=false;loading=false}}
 async function loadOptions(){options=await api('/exports/options'+(isJV?'?record_kind=jv':company?'?company_id='+encodeURIComponent(company):''))}
 async function loadRegister(){listing=await api('/exports?'+query())}
 async function start(){await loadOptions();if(!isJV&&company||options.permissions?.standalone)await loadRegister()}
 onMount(()=>{const query=new URLSearchParams(window.location.search);if(query.get('section')==='reconciliation'){reconciliationSection=true;initialReconciliationInvoice=Math.max(0,Number(query.get('invoice_id'))||0);loading=false}else void run(start)});
 onDestroy(()=>{if(focusFrame)cancelAnimationFrame(focusFrame)});
 function revealFocusedField(event:FocusEvent){
  const target=event.target;
  if(!(target instanceof HTMLElement)||!target.matches('input,select,textarea'))return;
  const editor=target.closest<HTMLElement>('.ex-editor');
  if(!editor)return;
  cancelAnimationFrame(focusFrame);
  focusFrame=requestAnimationFrame(()=>{
   if(!target.isConnected||document.activeElement!==target)return;
   const viewport=window.visualViewport;
   const viewTop=viewport?.offsetTop||0,viewBottom=viewTop+(viewport?.height||window.innerHeight);
   const heading=editor.querySelector<HTMLElement>('.ex-task-heading')?.getBoundingClientRect();
   const footer=editor.querySelector<HTMLElement>('.ex-task-footer')?.getBoundingClientRect();
   const top=Math.max(viewTop,heading&&heading.bottom>viewTop&&heading.top<viewBottom?heading.bottom:viewTop)+34;
   const bottom=Math.min(viewBottom,footer&&footer.top<viewBottom&&footer.bottom>viewTop?footer.top:viewBottom)-18;
   const bounds=target.getBoundingClientRect();
   target.style.scrollMarginTop=`${Math.max(34,top-viewTop)}px`;
   target.style.scrollMarginBottom=`${Math.max(18,viewBottom-bottom)}px`;
   if(bounds.top<top||bounds.bottom>bottom)target.scrollIntoView({block:'nearest',inline:'nearest',behavior:'instant'});
  });
 }

 beforeNavigate(n=>{if(!dirty)return;if(n.type==='leave'){n.cancel();return}if(!window.confirm(`Discard the unsaved ${isJV?'JV':'export'} changes?`))n.cancel()});
 async function selectCompany(value:string){if(value===company||busy)return;if(dirty&&!window.confirm(`Discard the unsaved ${isJV?'JV':'export'} changes?`))return;company=value;detail=null;editing=false;dirty=false;resetInvoiceStaging();listing={records:[],totals:[]};options={...options,parties:[],orders:[],permissions:{}};partyFilter='';year='';currencyFilter='';workflowFilter='';search='';notice='';await run(async()=>{await loadOptions();if(company||options.permissions?.standalone)await loadRegister()})}
 async function selectRegister(value:ExportRecordKind){if(busy||value===registerKind&&!reconciliationSection)return;if(dirty&&!window.confirm('Leave unsaved reconciliation changes? Saved invoice details and documents remain.'))return;reconciliationSection=false;registerKind=value;detail=null;editing=false;dirty=false;savedID=0;resetInvoiceStaging();listing={records:[],totals:[]};options={...options,parties:[],orders:[],permissions:{}};partyFilter='';year='';currencyFilter='';workflowFilter='';search='';notice='';loading=true;await run(start);await focus('export-register-title')}
 async function selectReconciliation(invoiceID=0){if(busy||reconciliationSection)return;if(dirty&&!window.confirm('Leave unsaved invoice changes?'))return;detail=null;editing=false;dirty=false;resetInvoiceStaging();initialReconciliationInvoice=invoiceID;reconciliationSection=true;notice='';error=''}
 async function openReconciledInvoice(id:number){if(busy)return;reconciliationSection=false;dirty=false;await open(id);if(detail){company=detail.record.company_id?String(detail.record.company_id):'';await run(async()=>{await loadOptions();await loadRegister()})}}
 function resetInvoiceStaging(saved=false){invoiceSave={savedContent:saved?invoiceContent(form):'',pendingSave:null,files:[]};addingCompany=false;newCompanyName='';companyError=''}
 function changeParty(value:string){const p=parties.find((p:Row)=>partyKey(p)===value);form.party_type=p?.party_type||'';form.party_id=p?.id||'';form.order_id=null;dirty=true}
 function fresh(){if(busy||!options.permissions?.edit||((isJV||!company)&&!options.permissions?.standalone))return;returnID='export-new';error='';notice='';detail=null;form=isJV?newJVForm():{company_id:company?Number(company):null,party_type:company?'':'manual',party_id:null,party_name:'',order_id:null,workflow_status:'Draft',invoice_number:'',currency:'',invoice_amount:'',profile:{statuses:{}},receipts:[],ebrcs:[]};resetInvoiceStaging();editing=true;dirty=true;tab='shipment';void focus('export-task-title')}
 async function open(id:number){returnID='export-open-'+id;await run(async()=>{const loaded:Row=await api('/exports/'+id);detail=loaded;registerKind=loaded.record.record_kind==='jv'?'jv':'export';resetInvoiceStaging();savedID=0;editing=false;dirty=false;tab='shipment';revision=null;reviewNote='';workflowNote='';commentNote='';discountAmount='';discountReason='';notice='';await focus('export-task-title')})}
 function edit(){if(!detail||busy||!actions.edit)return;if(invoiceSave.files.length){error=`Finish confirming the queued attachments before editing the ${recordLabel}.`;return}form=structuredClone($state.snapshot(detail.record));form.profile??={};form.profile.statuses??={};form.receipts??=[];form.ebrcs??=[];resetInvoiceStaging(true);const missingKeys=form.receipts.some((receipt:Row)=>!receipt.receipt_key);ensureReceiptKeys(form.receipts);editing=true;dirty=missingKeys;error='';notice='';revision=null;void focus('export-task-title')}
 async function cancel(){
  if(busy||dirty&&!window.confirm(uncertainSave?`A previous save or upload is unconfirmed. Leave this form and check the saved ${recordLabel} or register before starting it again?`:`Discard unsaved changes and queued documents? Saved ${recordLabel} details and attached files will remain.`))return;
  const id=form.id;editing=false;dirty=false;error='';resetInvoiceStaging();
  if(id)await run(async()=>{savedID=id;detail=null;detail=await api('/exports/'+id);savedID=0;await loadRegister()});
  await focus(detail?'export-task-title':returnID);
 }
 async function back(){if(busy)return;if(editing){await cancel();if(editing)return}else if(dirty&&!window.confirm(`Leave the queued attachments? Check this ${recordLabel} before attaching an unconfirmed file again.`))return;resetInvoiceStaging();dirty=false;detail=null;revision=null;notice='';error='';await focus(returnID)}
 async function refreshDetail(){detail=await api('/exports/'+detail!.record.id);await loadRegister()}
 async function addInvoiceCompany(){
  if(busy)return;const name=newCompanyName.trim();companyError='';
  if(!name||name.length>160||/[\r\n]/.test(name)){companyError='Enter a company name between 1 and 160 characters on one line.';await focus('export-new-company');return}
  await run(async()=>{const item=await api('/exports/parties',{name});options.standalone_parties=[...(options.standalone_parties||[]).filter((choice:Row)=>choice.id!==item.id),item];form.party_name=item.name;addingCompany=false;newCompanyName='';dirty=true;notice=item.created?'Company added to the export invoice list.':'The existing company name is selected.';await focus('export-invoice-company')});
 }
 function queueInvoiceFiles(event:Event,kind:string,receipt_key='',field_key=''){
  const input=event.currentTarget as HTMLInputElement,files=Array.from(input.files||[]);input.value='';if(busy||!canAttachInvoice)return;
  const invalid=files.map(file=>invoiceFileError(file)).find(Boolean);if(invalid){error=invalid;return}
  if((detail?.files||[]).length+invoiceSave.files.length+files.length>100){error=`Each ${recordLabel} can have at most 100 documents.`;return}
  for(const file of files){const key=[kind,receipt_key,field_key,invoiceFileKey(file)].join(':');if(!invoiceSave.files.some(item=>item.key===key))invoiceSave.files.push({file,key,kind,...(receipt_key?{receipt_key,field_key}:{}),status:'queued'})}
  dirty=true;error='';notice='Documents queued under their checklist categories. Save the draft to attach them.';
 }
 function removeQueuedInvoiceFile(key:string){if(busy)return;const item=invoiceSave.files.find(file=>file.key===key);if(!item||item.status==='uploaded'||item.path||!window.confirm(`Remove ${item.file.name} from the document queue?`))return;invoiceSave.files=invoiceSave.files.filter(file=>file.key!==key);dirty=editing||invoiceSave.files.length>0}
 async function flushInvoiceDocuments(){
  await run(async()=>{await saveInvoiceFiles(form,invoiceSave,api,uploadTo);detail=await api('/exports/'+form.id);resetInvoiceStaging();dirty=false;notice='Documents attached under their checklist categories.';await loadRegister()});
 }
 async function attachInvoiceFiles(event:Event,kind:string,receipt_key='',field_key=''){
  if(!invoiceSave.files.length){form=structuredClone($state.snapshot(detail!.record));resetInvoiceStaging(true)}
  queueInvoiceFiles(event,kind,receipt_key,field_key);if(!error&&invoiceSave.files.length)await flushInvoiceDocuments();
 }
 async function removeSavedDocument(file:Row){
  if(busy||!detail||!actions.remove||uncertainSave)return;
  if(!window.confirm(`Remove ${file.filename} from this ${recordLabel}? It will no longer be available in the active checklist. The removal stays in the audit history.`))return;
  await run(async()=>{
   const id=detail!.record.id,version=editing||invoiceSave.files.length?form.version:detail!.record.version;
   const removed=await api(`/exports/${id}/files/${file.id}/remove`,{version});
   if(editing||invoiceSave.files.length){form.version=removed.version;form.workflow_status='Draft';form.documentation_cleared=null}
   detail!.record.version=removed.version;detail!.record.workflow_status='Draft';detail!.files=detail!.files.filter((entry:Row)=>entry.id!==file.id);
   invoiceSave.files=invoiceSave.files.filter(entry=>entry.id!==file.id);previewFallback=0;
   notice='Document removed from the active checklist. The action remains in the audit history.';
   detail=await api('/exports/'+id);await loadRegister();
  });
 }
 async function save(event:SubmitEvent){
  event.preventDefault();
  const intent=event.submitter instanceof HTMLButtonElement?event.submitter.value:'save';
  if((form.company_id?!form.party_id:!String(form.party_name||'').trim())||!String(form.invoice_number||'').trim()||!form.currency||!present(form.invoice_amount)||(requiresOrder&&!form.order_id)){
   error=isJV?'Enter the JV reference, party name, currency and amount.':form.company_id?'Choose a party, enter the invoice number, currency and amount, and select a linked PO when required.':'Choose the invoice company, and enter the invoice number, currency and amount.';
   tab='shipment';await focus('export-task-title');return;
  }
  if(intent==='submit'&&!hasRequiredDocument){error=`Attach at least one ${requiredDocument.label} before submitting for approval. You can save a draft without one.`;tab='documents';await focus('document-input-'+requiredDocument.kind);return}
  await run(async()=>{
   try{await saveInvoiceFiles(form,invoiceSave,api,uploadTo)}catch(error){
    notice=form.id?'Draft saved. Your entered details and document queue are retained. Completed attachments will not be uploaded again.':'Your entered details and queued documents are retained.';
    throw error;
   }
   const id=form.id,version=form.version;editing=false;dirty=false;savedID=id;detail=null;listing={records:[],totals:[]};resetInvoiceStaging();
   notice=`Draft saved. Submit it when the ${recordLabel} is ready for MD or Super Admin review.`;
   detail=await api('/exports/'+id);savedID=0;
   if(intent==='submit'){
    await api('/exports/'+id+'/workflow',{version,action:'submit'});
    notice=`${isJV?'JV':'Invoice'} submitted to MD or Super Admin for approval.`;savedID=id;detail=null;
    detail=await api('/exports/'+id);savedID=0;
   }
   await loadRegister();await focus('export-task-title');
  });
 }
 async function workflow(action:'submit'|'approve'|'request_changes'|'reopen'){
  if(!detail||!actions[action]||busy)return;
  if(action==='submit'&&(invoiceSave.pendingSave||invoiceSave.files.some(file=>file.status!=='uploaded'))){error='Finish or remove queued documents before submitting. Retry / confirm any unconfirmed attachment first.';tab='documents';await focus('export-panel-title');return}
  if(action==='submit'&&!requiredFiles.length){error=`Attach at least one ${requiredDocument.label} before submitting for approval.`;tab='documents';await focus('document-input-'+requiredDocument.kind);return}
  if(['request_changes','reopen'].includes(action)&&!workflowNote.trim()){error='Enter a clear reason before continuing.';await focus('export-workflow-note');return}
  await run(async()=>{
   const result=await api('/exports/'+detail!.record.id+'/workflow',{version:detail!.record.version,action,note:workflowNote.trim()});
   detail!.record.version=result.version;detail!.record.workflow_status=result.workflow_status;detail!.actions={};workflowNote='';
   notice=action==='submit'?`${isJV?'JV':'Invoice'} submitted to MD or Super Admin for approval.`:action==='approve'?`${isJV?'JV':'Invoice'} approved.`:action==='request_changes'?`Changes requested. Finance can correct and resubmit this ${recordLabel}.`:`${isJV?'JV':'Invoice'} reopened as a draft. Save corrections and submit it for approval again.`;
   await refreshDetail();await focus('export-task-title');
  });
 }
 async function addComment(event:SubmitEvent){event.preventDefault();if(!detail||!actions.comment||!commentNote.trim())return;await run(async()=>{await api('/exports/'+detail!.record.id+'/comments',{version:detail!.record.version,note:commentNote.trim()});commentNote='';notice=`Comment added to the ${recordLabel} history.`;await refreshDetail()})}

 function addReceipt(){form.receipts.push(newExportReceipt(form.currency||''));dirty=true;void focus('receipt-'+(form.receipts.length-1)+'-inward_reference')}
 function removeRow(key:'receipts'|'ebrcs',i:number){if(busy||uncertainSave)return;if(key==='receipts'){const receiptKey=form.receipts[i]?.receipt_key;if(receiptKey&&[...(detail?.files||[]),...invoiceSave.files].some(file=>file.receipt_key===receiptKey)){error='Remove this receipt’s attached or queued documents before removing the receipt.';return}}if(window.confirm(`Remove ${key==='receipts'?'receipt':'eBRC'} ${i+1} from these unsaved changes?`)){form[key].splice(i,1);dirty=true}}
 async function check(kind:'finance'|'audit'){await run(async()=>{await api(`/exports/${detail!.record.id}/check`,{version:detail!.record.version,kind,status:reviewStatus,note:reviewNote});await refreshDetail();reviewNote='';notice=`${kind==='finance'?'Finance':'Audit'} check recorded.`})}
 async function requestDiscount(e:SubmitEvent){e.preventDefault();await run(async()=>{await api(`/exports/${detail!.record.id}/discount`,{version:detail!.record.version,action:'request',amount:discountAmount,reason:discountReason});await refreshDetail();discountAmount='';discountReason='';notice='Shortfall request sent to Finance Head for the first review.'})}
 async function discountDecision(d:Row,action:string){await run(async()=>{await api(`/exports/${detail!.record.id}/discount`,{version:detail!.record.version,action,discount_id:d.id,reason:decisionNotes[d.id]||''});await refreshDetail();decisionNotes[d.id]='';notice=action==='final_approve'?'Shortfall approved and balance updated.':'Shortfall review recorded.'})}
 async function history(id:number){await run(async()=>{revision=await api(`/exports/${detail!.record.id}/history/${id}`);await focus('export-revision-title')})}
 function exportURL(kind:'register'|'statement'){const q=exportRegisterQuery(company,{record_kind:registerKind,fy:year,currency:currencyFilter,search,status:workflowFilter,party:registerParties.find((p:Row)=>partyKey(p)===partyFilter)});return '/api/exports/'+(kind==='statement'?'statement':'export')+'?'+q}

 function showTab(value:ExportTab){if(busy)return;tab=value;error='';void focus('export-panel-title')}
 async function showDocuments(kind:string){if(busy)return;tab='documents';error='';await tick();const target=root?.querySelector<HTMLElement>('#document-category-'+kind);target?.focus();target?.scrollIntoView({block:'start',behavior:'instant'})}
 const docKinds=()=>isJV?[{key:'jv_supporting',label:'Supporting JV document'}]:options.document_kinds||[];
 const profileStatusFields=[...shipmentFields,...incentiveFields,...scripFields,...clearanceFields].filter(f=>!['textarea','select'].includes(f.type||'')&&!f.key.endsWith('_currency'));
</script>

{#snippet fields(items:ExportField[],values:Row,prefix:string,readonly:boolean)}
 <div class="ex-grid" class:read={readonly}>
  {#each items as field}
   {@const fieldValues=field.profile?(values.profile||{}):values}
   {@const required=field.required||(field.key==='ebrc_number'&&!['Pending','Not required'].includes(fieldValues.status))}
   <div class="ex-field" class:wide={field.type==='textarea'}>
    {#if readonly}<span class="ex-label">{field.label}</span><span class="ex-value">{fieldValue(field,fieldValues?.[field.key])}</span>{#if fieldValues?.statuses?.[field.key]}<small>{fieldValues.statuses[field.key]}</small>{/if}
    {:else}
     <label for={prefix+'-'+field.key}><span class="ex-caption">{field.label}{#if required}<span class="ex-required"> *</span>{/if}</span></label>
     {#if field.type==='textarea'}<textarea id={prefix+'-'+field.key} bind:value={fieldValues[field.key]} rows="3" maxlength="2000" disabled={busy}></textarea>
     {:else if field.type==='currency'||field.type==='select'}
      <select id={prefix+'-'+field.key} bind:value={fieldValues[field.key]} required={required} disabled={busy||(prefix==='invoice'&&field.key==='currency'&&invoiceCurrencyLocked)}><option value="">Choose…</option>{#each [...new Set([...(field.type==='currency'?options.currencies||[]:field.choices||[]),...(fieldValues[field.key]?[fieldValues[field.key]]:[])])] as item}<option value={item}>{item}</option>{/each}</select>
     {:else if field.type==='date'}<input id={prefix+'-'+field.key} type="date" bind:value={fieldValues[field.key]} required={required} disabled={busy}/>
     {:else if field.type==='money'||field.type==='number'}<input id={prefix+'-'+field.key} type="text" inputmode="decimal" pattern={field.type==='money'?'(0|[1-9][0-9]{0,14})([.][0-9]{1,2})?':'(0|[1-9][0-9]{0,14})([.][0-9]{1,6})?'} bind:value={fieldValues[field.key]} required={required} disabled={busy} maxlength="30" title={field.type==='money'?'Enter an amount with up to two decimal places.':'Enter a positive decimal number.'}/>
     {:else}<input id={prefix+'-'+field.key} bind:value={fieldValues[field.key]} required={required} disabled={busy} maxlength={field.maxLength||500}/>{/if}
     {#if field.hint}<small>{field.hint}</small>{/if}{#if prefix==='invoice'&&field.key==='currency'&&invoiceCurrencyLocked}<small>Currency is fixed while the export has saved receipts or approved adjustments.</small>{/if}
    {/if}
   </div>
  {/each}
 </div>
{/snippet}

{#snippet empty(title:string,message:string)}
 <div class="ex-empty"><FileText size={28}/><h3>{title}</h3><p>{message}</p></div>
{/snippet}

{#snippet workflowSummary(r:Row)}
 <section class="ex-workflow-summary" class:correction={r.workflow_status==='Changes requested'} aria-label={isJV?'JV approval status':'Invoice approval status'}>
  <div class="ex-workflow-status"><span class="ex-status" class:complete={r.workflow_status==='Approved'} class:submitted={r.workflow_status==='Submitted'} class:correction={r.workflow_status==='Changes requested'}>{r.workflow_status||'Draft'}</span><p>{r.workflow_status==='Submitted'?`Awaiting MD or Super Admin approval. ${isJV?'JV':'Invoice'} details and documents are locked during review.`:r.workflow_status==='Approved'?`Approved ${recordLabel}. Reopen with a reason before changing ${recordLabel} details or documents.`:r.workflow_status==='Changes requested'?`Review the comments, correct the ${recordLabel} and submit it again.`:`Save the ${recordLabel}, attach its documents and submit it for approval when ready.`}</p></div>
  {#if !isJV}<small>eBRC numbers and certificates can be managed separately in Reconciliation.</small>{/if}
  {#if !isJV&&r.documentation_cleared?.at}<p class="ex-documentation-state"><span class="ex-status complete">Documentation cleared</span> {dateTime(r.documentation_cleared.at)} · eBRC {r.documentation_cleared.ebrc_number}</p>{/if}
  {#if r.review_note}<div class="ex-correction-note"><strong>{r.workflow_status==='Changes requested'?'Corrections requested':'Latest review note'}</strong><p class="ex-prewrap">{r.review_note}</p><small>{r.reviewed_by_name||'Reviewer'}{#if r.reviewed_at} · {dateTime(r.reviewed_at)}{/if}</small></div>{/if}
  {#if r.workflow_status==='Submitted'&&r.submitted_by_name}<small>Submitted by {r.submitted_by_name} · {dateTime(r.submitted_at)}</small>{/if}
  {#if r.workflow_status==='Submitted'&&permissions.approve&&(Number(r.created_by)===user.id||Number(r.submitted_by)===user.id)}<small>Another MD or Super Admin must review {isJV?'a JV':'an invoice'} you created or submitted.</small>{/if}
 </section>
{/snippet}

{#snippet documentField(kind:string,label:string,readonly:boolean,receiptKey='',fieldKey='',prefix='inline-document',compact=false)}
 <ExportDocumentField {label} {compact} inputID={prefix+'-'+kind} files={(detail?.files||[]).filter((file:Row)=>documentInSlot(file,kind,receiptKey,fieldKey))} queued={invoiceSave.files.filter(file=>documentInSlot(file,kind,receiptKey,fieldKey))} canAttach={canAttachInvoice} canRemove={!!actions.remove} canDownload={!!permissions.download} {busy} uncertain={uncertainSave} onattach={event=>readonly?attachInvoiceFiles(event,kind,receiptKey,fieldKey):queueInvoiceFiles(event,kind,receiptKey,fieldKey)} onremove={removeSavedDocument} onremovequeued={removeQueuedInvoiceFile}/>
{/snippet}

{#snippet panels(r:Row,readonly:boolean)}
 {#if tab==='shipment'&&isJV}
  <section class="ex-section"><h3 id="export-panel-title" tabindex="-1">JV details</h3>{@render fields(jvFields,r,'jv',readonly)}</section>
 {:else if tab==='shipment'}
  <section class="ex-section"><h3 id="export-panel-title" tabindex="-1">Shipment & invoice</h3>
   {#if readonly}<div class="ex-record-links"><div><span class="ex-label">Party</span><strong>{r.party_name}</strong></div><div><span class="ex-label">Linked production PO</span><strong>{(options.orders||[]).find((o:Row)=>o.id===r.order_id)?.po||r.order_po||'Not linked'}</strong></div><div><span class="ex-label">Register reference</span><strong>{exportReference(r.id)}</strong></div></div>
   {:else if !form.company_id}<div class="ex-link-fields ex-invoice-company"><label for="export-invoice-company"><span class="ex-caption">Company name <span class="ex-required">*</span></span></label><div class="ex-company-choice"><select id="export-invoice-company" bind:value={form.party_name} required disabled={busy}><option value="">Choose invoice company</option>{#each invoiceCompanies as name}<option value={name}>{name}</option>{/each}</select><button type="button" class="ex-button secondary" disabled={busy} aria-expanded={addingCompany} onclick={()=>{addingCompany=!addingCompany;companyError='';if(addingCompany)void focus('export-new-company')}}><Plus size={16}/>Add more company</button></div><small>These company names are saved for export invoices only.</small>
    {#if addingCompany}<div class="ex-company-add" use:enter={'add-invoice-company'}><label for="export-new-company">New company name<input id="export-new-company" bind:value={newCompanyName} maxlength="160" disabled={busy} autocomplete="organization" onkeydown={event=>{if(event.key==='Enter'){event.preventDefault();void addInvoiceCompany()}}}/></label>{#if companyError}<p class="ex-error" role="alert">{companyError}</p>{/if}<div class="ex-actions"><button type="button" class="ex-button primary" disabled={busy||!newCompanyName.trim()} onclick={addInvoiceCompany}>Save company</button><button type="button" class="ex-button secondary" disabled={busy} onclick={()=>{addingCompany=false;companyError='';void focus('export-invoice-company')}}>Cancel</button></div><small>This adds a choice here; it does not create a Company Setup record.</small></div>{/if}
   </div>{:else}<div class="ex-grid ex-link-fields"><label><span class="ex-caption">Party <span class="ex-required">*</span></span><select value={linkedParty} onchange={e=>changeParty(e.currentTarget.value)} required disabled={busy||!!form.id}><option value="">Choose an approved party</option>{#each parties as p}<option value={partyKey(p)}>{p.name}{p.party_type==='company_setup'?' · company master':''}</option>{/each}</select></label><label><span class="ex-caption">Linked production PO{#if requiresOrder}<span class="ex-required"> *</span>{/if}</span><select bind:value={form.order_id} required={requiresOrder} disabled={busy||!!form.id||!form.party_id||form.party_type!=='buyer'}><option value={null}>{requiresOrder?'Choose an authorised PO':'No linked PO'}</option>{#each availableOrders as order}<option value={order.id}>{order.po}</option>{/each}</select><small>{requiresOrder?'Your access is limited to assigned production orders.':'Select the related PO when available.'}</small></label></div>{/if}
   {@render fields(coreFields,r,'invoice',readonly)}
   <div class="ex-subsection">{@render fields(shipmentFields,r.profile,'shipment',readonly)}</div>
  </section>
  <section class="ex-section"><h3>Drawback & RoSCTL</h3>{@render fields(incentiveFields,r.profile,'incentive',readonly)}</section>
  {#if !readonly}<details class="ex-section"><summary>Special codes & field notes</summary><p class="ex-muted">Use these for FOC, HC, NIL or other explanations. Amount fields accept numbers only.</p><div class="ex-grid">{#each profileStatusFields as f}<label>{f.label}<input bind:value={r.profile.statuses[f.key]} maxlength="160" disabled={busy} placeholder="Optional code or note"/></label>{/each}</div></details>{/if}
 {:else if tab==='receipts'}
  <section class="ex-section ex-banking"><div class="ex-section-heading"><div><h3 id="export-panel-title" tabindex="-1">Receipts & banking</h3><p class="ex-muted">Complete the three sections for each receipt. Manage eBRC documentation in the Reconciliation section.</p></div>{#if !readonly}<button type="button" class="ex-button secondary" disabled={busy||(r.receipts||[]).length>=100} onclick={addReceipt}><Plus size={16}/>Add receipt</button>{/if}</div>
   <div class="ex-bank-context">{@render fields([{key:'invoice_number',label:'Invoice number'},{key:'invoice_date',label:'Invoice date',type:'date',profile:true},{key:'invoice_amount',label:'Invoice amount',type:'money'},{key:'currency',label:'Invoice currency'}],r,'bank-invoice-context',true)}</div>
  {#each r.receipts||[] as receipt,i(receipt.receipt_key||receipt)}
   <article class="ex-repeat ex-bank-receipt"><div class="ex-section-heading"><h4>Receipt {i+1}{#if receipt.reference} · {receipt.reference}{/if}</h4>{#if !readonly}<button type="button" class="ex-button quiet" disabled={busy||uncertainSave} aria-label={'Remove receipt '+(i+1)} onclick={()=>removeRow('receipts',i)}><Trash2 size={16}/>Remove</button>{/if}</div>
    <ol class="ex-bank-sequence">{#each bankingStages as stage,stageIndex}{@const slot=receiptDocumentSlots[stage.key]}
     <li class="ex-bank-stage"><div class="ex-bank-stage-heading"><span class="ex-bank-number" aria-hidden="true">{stageIndex+1}</span><h5>{stage.title}</h5></div>
      {#if stage.key==='disposal'}<div class="ex-bank-carried"><span>Bill / remittance amount</span><strong>{moneyText(receipt.gross_payment_received,receipt.currency||'')}</strong><small>From this receipt’s inward advice.</small></div>{/if}
      <div class="ex-bank-stage-main"><div class="ex-bank-stage-fields">{@render fields(stage.fields,receipt,'receipt-'+i,readonly)}</div>
       <div class="ex-bank-stage-attachment" class:full-row={stage.key==='credit'}>{#if receipt.receipt_key}{@render documentField(slot.kind,slot.label,readonly,receipt.receipt_key,slot.field,'receipt-'+i+'-'+slot.field,true)}{:else}<p class="ex-muted">Edit and save this receipt to attach its {slot.label.toLowerCase()} document. Earlier shared documents remain in Documents.</p>{/if}</div>
      </div>
      {#if stage.key==='inward'}
       {#if i===0}<div class="ex-bank-shipping"><h6>Invoice & transport details · shared by all receipts</h6>
        <div class="ex-bank-document-row"><div>{@render fields([{key:'invoice_number',label:'Invoice number'},{key:'invoice_date',label:'Invoice date',type:'date',profile:true}],r,'inward-invoice',true)}</div>{@render documentField('invoice','Invoice document',readonly,'','','bank-invoice',true)}</div>
        <div class="ex-bank-document-row"><div>{@render fields([{key:'transport_document_number',label:'BL / AWB number'},{key:'transport_document_date',label:'BL / AWB date',type:'date'}],r.profile,'inward-bl',readonly)}</div>{@render documentField('bill_of_lading','BL / AWB document',readonly,'','','bank-bl',true)}</div>
        <div class="ex-bank-ports">{@render fields([{key:'port_of_loading',label:'Port of loading'},{key:'port_of_departure',label:'Port of departure'}],r.profile,'inward-ports',readonly)}</div>
       </div>{:else}<p class="ex-muted">Invoice, BL / AWB and port details are shared by this invoice and shown under Receipt 1.</p>{/if}
      {/if}
      {#if stage.key==='credit'}<p class="ex-muted">Foreign credited amount uses {receipt.currency||'the inward advice currency'}. Conversion, received amount and charges above are in INR.</p>{/if}
     </li>
    {/each}</ol>

   </article>
  {:else}{@render empty('No receipts recorded',readonly?'Payments and bank advice will appear here when added.':'Add a receipt to start its inward remittance advice.')}{/each}
  </section>

 {:else if tab==='clearance'}
  <section class="ex-section"><h3 id="export-panel-title" tabindex="-1">Scrip sale</h3><p class="ex-muted">Record the agreed sale amount, percentage and currency separately.</p>{@render fields(scripFields,r.profile,'scrip',readonly)}</section>
  <section class="ex-section"><h3>Clearance & transport partners</h3>{@render fields(clearanceFields,r.profile,'clearance',readonly)}<div class="ex-subsection">{@render fields([{key:'linked_bank',label:'Linked bank'}],r.profile,'linked-bank',readonly)}</div></section>
 {:else if tab==='documents'}
  <section class="ex-section"><h3 id="export-panel-title" tabindex="-1">Document checklist</h3><p class="ex-muted">{isJV?'Attach a supporting JV document before submission. You can attach the JV, BL or AWB copy here.':'Attach each document in its own category. An invoice document is required before submission.'} PDF, PNG, JPG, Word or Excel · up to 10 MB per file.</p>
  {#if !readonly}<p class="ex-notice">Choose documents here, then save the draft to attach them. Queued files stay on this form until saved.</p>{/if}
  <ul class="ex-checklist">{#each docKinds() as kind}{@const files=(detail?.files||[]).filter((f:Row)=>f.kind===kind.key)}{@const queued=invoiceSave.files.filter(item=>item.kind===kind.key&&!files.some((file:Row)=>file.id===item.id))}<li id={'document-category-'+kind.key} tabindex="-1">
   <div class="ex-document-kind"><span class:attached={files.length>0} class="ex-document-indicator">{#if files.length}<Check size={15}/>{:else}<FileText size={15}/>{/if}</span><strong>{kind.label}{#if kind.key===requiredDocument.kind}<span class="ex-required"> *</span>{/if}</strong>{#if files.length}<small>{files.length} attached</small>{/if}
    {#if canAttachInvoice}<label class="ex-button secondary ex-upload-label ex-checklist-attach"><Paperclip size={16}/>Attach document<input id={'document-input-'+kind.key} type="file" multiple accept=".pdf,.png,.jpg,.jpeg,.docx,.xlsx" aria-label={'Attach '+kind.label+' document'} disabled={busy} onchange={event=>readonly?attachInvoiceFiles(event,kind.key):queueInvoiceFiles(event,kind.key)}/></label>{:else if !files.length}<small>No document attached</small>{/if}
   </div>
   {#if files.length||queued.length}<ul class="ex-document-files">{#each files as file}<li><div class="ex-document-info"><strong>{file.filename}</strong><small>{receiptDocumentLabel(file,r.receipts||[])}</small><small>{Math.max(1,Math.round(file.size/1024))} KB · {exportDate(file.created_at)}</small></div><div class="ex-actions">
    {#if permissions.download}{#if exportFileCanPreview(file)}<a data-sveltekit-reload data-sveltekit-preload-data="false" class="ex-button quiet" href={'/api/export-files/'+file.id+'?preview=1'} target="_blank" rel="noopener noreferrer" aria-label={'Preview '+file.filename}><Eye size={15}/>Preview</a>{:else}<button type="button" class="ex-button quiet" disabled={busy} onclick={()=>previewFallback=previewFallback===file.id?0:file.id} aria-expanded={previewFallback===file.id}><Eye size={15}/>Download / open</button>{/if}<a data-sveltekit-reload data-sveltekit-preload-data="false" class="ex-button quiet" href={'/api/export-files/'+file.id+'?download=1'} download aria-label={'Download '+file.filename}><Download size={15}/>Download</a>{/if}
    {#if actions.remove}<button type="button" class="ex-button quiet" disabled={busy||uncertainSave} aria-label={'Remove '+file.filename} onclick={()=>removeSavedDocument(file)}><Trash2 size={15}/>Remove</button>{/if}
   </div>{#if previewFallback===file.id}<p class="ex-document-fallback" role="status">Word and Excel documents open in their own apps. Download this file to view it.</p>{/if}</li>{/each}
   {#each queued as item (item.key)}<li><div class="ex-document-info"><strong>{item.file.name}</strong><small>{receiptDocumentLabel(item,r.receipts||[])}</small><small>{Math.max(1,Math.round(item.file.size/1024))} KB · {item.status==='uploaded'?'Attached':item.status==='uploading'?'Uploading…':item.status==='uncertain'?'Awaiting upload confirmation':item.status==='failed'?'Upload failed — retry or remove':'Queued for the next save'}</small>{#if item.error}<small class="ex-file-error">{item.error}</small>{/if}</div><div class="ex-actions">{#if item.status==='uploaded'}<span class="ex-status complete">Attached</span>{#if permissions.download}{#if exportFileCanPreview({mime:item.file.type})}<a data-sveltekit-reload data-sveltekit-preload-data="false" class="ex-button quiet" href={'/api/export-files/'+item.id+'?preview=1'} target="_blank" rel="noopener noreferrer" aria-label={'Preview '+item.file.name}><Eye size={15}/>Preview</a>{/if}<a data-sveltekit-reload data-sveltekit-preload-data="false" class="ex-button quiet" href={'/api/export-files/'+item.id+'?download=1'} download aria-label={'Download '+item.file.name}><Download size={15}/>Download</a>{/if}{#if detail&&actions.remove}<button type="button" class="ex-button quiet" disabled={busy||uncertainSave} aria-label={'Remove '+item.file.name} onclick={()=>removeSavedDocument({id:item.id,filename:item.file.name})}><Trash2 size={15}/>Remove</button>{:else if !detail&&editing}<button type="button" class="ex-button quiet" disabled={busy} onclick={async()=>{await cancel();if(!editing){tab='documents';await focus('export-panel-title')}}}>Open saved draft</button>{/if}{:else if !item.path}<button type="button" class="ex-button quiet" disabled={busy} aria-label={'Remove queued document '+item.file.name} onclick={()=>removeQueuedInvoiceFile(item.key)}><Trash2 size={15}/>Remove queued</button>{/if}</div></li>{/each}</ul>{/if}
  </li>{/each}</ul>
  {#if readonly&&invoiceSave.files.length}<div class="ex-upload"><button type="button" class="ex-button secondary" disabled={busy} onclick={flushInvoiceDocuments}>Retry / confirm attachments</button><small>Completed uploads are kept. Unconfirmed uploads use the same protected request.</small></div>{/if}
  </section>
 {:else if tab==='history'}
  <section class="ex-section"><h3 id="export-panel-title" tabindex="-1">{isJV?'JV comments':'Invoice comments'}</h3><p class="ex-muted">Questions, correction reasons and approval notes stay with the {recordLabel}.</p>
   <ol class="ex-history ex-comments">{#each detail?.comments||[] as comment}<li><div><strong>{comment.author_name}</strong><small>{dateTime(comment.created_at)}</small><p class="ex-prewrap">{comment.note}</p></div></li>{:else}<li><p class="ex-muted">No comments yet.</p></li>{/each}</ol>
   {#if readonly&&actions.comment}<form class="ex-comment-form" onsubmit={addComment}><label>Add a comment<textarea bind:value={commentNote} rows="3" maxlength="2000" required disabled={busy}></textarea></label><button class="ex-button secondary" disabled={busy||!commentNote.trim()}>Add comment</button></form>{/if}
  </section>
  {#if !isJV}
  <section class="ex-section"><h3>Finance & audit file checks</h3><div class="ex-review-grid">{#each ['finance','audit'] as kind}{@const check=r[kind+'_check']}<div><h4>{kind==='finance'?'Finance file check':'Audit file check'}</h4><span class="ex-status" class:complete={check?.status==='Checked'}>{check?.status||'Not checked'}</span>{#if check?.actor_name}<p>{check.actor_name} · {dateTime(check.at)}</p>{/if}{#if check?.note}<p class="ex-prewrap">{check.note}</p>{/if}</div>{/each}</div>
  {#if !readonly}<p class="ex-notice">Saving edits resets the file checks so the revised export can be reviewed.</p>
  {:else if permissions.finance_check||permissions.audit_check}<div class="ex-review-form"><div class="ex-grid"><label>Check result<select bind:value={reviewStatus} disabled={busy}><option>Checked</option><option>Needs correction</option></select></label><label class="wide">Review note<textarea bind:value={reviewNote} maxlength="2000" rows="2" disabled={busy}></textarea></label></div><div class="ex-actions">{#if permissions.finance_check}<button class="ex-button secondary" disabled={busy||!reviewNote.trim()} onclick={()=>check('finance')}>Record finance check</button>{/if}{#if permissions.audit_check}<button class="ex-button secondary" disabled={busy||!reviewNote.trim()} onclick={()=>check('audit')}>Record audit check</button>{/if}</div></div>{/if}
  </section>
  <section class="ex-section"><h3>Shortfall approval</h3><p class="ex-muted">Finance Head reviews first. A different MD or Super Admin gives final approval. Only the final approval reduces the outstanding balance.</p>
   {#each detail?.discounts||[] as d}<article class="ex-approval"><div class="ex-section-heading"><strong>{moneyText(d.amount,d.currency)}</strong><span class="ex-status" class:complete={d.status==='Approved'}>{d.status}</span></div><p class="ex-prewrap">{d.reason}</p><p class="ex-meta">Requested by {d.requested_by_name} · {dateTime(d.created_at)}</p>{#if d.first_by_name}<p class="ex-meta">Finance review: {d.first_by_name} · {dateTime(d.first_at)}</p>{/if}{#if d.final_by_name}<p class="ex-meta">Final review: {d.final_by_name} · {dateTime(d.final_at)}</p>{/if}{#if d.review_note}<p class="ex-prewrap">{d.review_note}</p>{/if}
   {#if readonly&&(d.status==='Pending finance'&&permissions.discount_first||d.status==='Pending final'&&permissions.discount_final&&Number(d.first_by)!==user.id)}<label>Decision reason<textarea bind:value={decisionNotes[d.id]} rows="2" maxlength="2000" disabled={busy}></textarea></label><div class="ex-actions"><button class="ex-button primary" disabled={busy||!decisionNotes[d.id]?.trim()} onclick={()=>discountDecision(d,d.status==='Pending finance'?'first_approve':'final_approve')}>{d.status==='Pending finance'?'Approve finance review':'Give final approval'}</button><button class="ex-button secondary" disabled={busy||!decisionNotes[d.id]?.trim()} onclick={()=>discountDecision(d,'reject')}>Reject request</button></div>{/if}</article>{:else}<p class="ex-muted">No shortfall requests for this invoice.</p>{/each}
   {#if readonly&&permissions.edit&&moneyMinor(r.balance?.outstanding)>0n&&!(detail?.discounts||[]).some((d:Row)=>['Pending finance','Pending final'].includes(d.status))}<form class="ex-request-form" onsubmit={requestDiscount}><h4>Request shortfall adjustment</h4><div class="ex-grid"><label>Requested amount ({r.currency})<input type="text" inputmode="decimal" pattern={'(0|[1-9][0-9]{0,14})([.][0-9]{1,2})?'} bind:value={discountAmount} required disabled={busy}/></label><label class="wide">Reason / supporting reference<textarea bind:value={discountReason} rows="3" maxlength="2000" required disabled={busy}></textarea></label></div><button class="ex-button primary" disabled={busy||!discountAmount||!discountReason.trim()}>Send to Finance Head</button></form>{/if}
  </section>
  {/if}
  <section class="ex-section"><h3>Revision history</h3><ol class="ex-history">{#each detail?.history||[] as item}<li><div><strong>{item.action}</strong><small>{item.actor_name} · {dateTime(item.created_at)}</small></div>{#if readonly}<button class="ex-button quiet" disabled={busy} onclick={()=>history(item.id)}>View revision<ChevronRight size={16}/></button>{/if}</li>{:else}<li><p class="ex-muted">The first saved {isJV?'JV':'export'} starts its history.</p></li>{/each}</ol>
  {#if revision}<section class="ex-revision"><div class="ex-section-heading"><h4 id="export-revision-title" tabindex="-1">Saved revision · {dateTime(revision.created_at)}</h4><button class="ex-button quiet" onclick={()=>revision=null}>Close revision</button></div><p>{revision.action} · {revision.actor_name}</p>{#if isJV}{@render fields(jvFields,revision.snapshot,'past-jv',true)}{:else}{@render fields(coreFields,revision.snapshot,'past-invoice',true)}<div class="ex-subsection">{@render fields([...shipmentFields,...incentiveFields,...scripFields,...clearanceFields],revision.snapshot.profile||{},'past-profile',true)}</div>{#each revision.snapshot.receipts||[] as receipt,i}<h4>Receipt {i+1}</h4>{@render fields(allReceiptFields,receipt,'past-receipt-'+i,true)}{/each}{#each revision.snapshot.ebrcs||[] as ebrc,i}<h4>eBRC {i+1}</h4>{@render fields(ebrcFields,ebrc,'past-ebrc-'+i,true)}{/each}{/if}</section>{/if}
  </section>
 {/if}
{/snippet}

<section class="export-workspace" bind:this={root} aria-busy={busy} onfocusin={revealFocusedField}>
 {#if !editing&&!detail}<nav class="ex-tabs ex-register-tabs" aria-label="Documentation registers"><button class:active={!reconciliationSection&&!isJV} aria-current={!reconciliationSection&&!isJV?'page':undefined} disabled={busy} onclick={()=>selectRegister('export')}>Export invoices</button><button class:active={!reconciliationSection&&isJV} aria-current={!reconciliationSection&&isJV?'page':undefined} disabled={busy} onclick={()=>selectRegister('jv')}>JV</button><button class:active={reconciliationSection} aria-current={reconciliationSection?'page':undefined} disabled={busy} onclick={()=>selectReconciliation()}>Reconciliation</button></nav>{/if}
 {#if reconciliationSection}<ExportReconciliationWorkspace initialInvoice={initialReconciliationInvoice} bind:dirty bind:busy={reconciliationBusy} onopeninvoice={openReconciledInvoice}/>
 {:else if !editing&&!detail}
  <header class="ex-register-heading"><div><h1 id="export-register-title" tabindex="-1">{isJV?'JV register':'Export documentation'}</h1><p>{isJV?'JV references, dates, parties and amounts in one register.':'Invoices, shipping documents and payment follow-up in one register.'}</p></div><div class="ex-actions"><button class="ex-button secondary" disabled={busy} onclick={()=>run(start)}><RefreshCw size={16}/>Refresh</button>{#if options.permissions?.edit}<button id="export-new" class="ex-button primary" disabled={busy||((isJV||!company)&&!options.permissions?.standalone)} onclick={fresh}><Plus size={17}/>{isJV?'Add JV':'Add export invoice'}</button>{/if}</div></header>
  <div class="ex-company-row">{#if !isJV}<label>Company filter<select value={company} disabled={busy} onchange={e=>selectCompany(e.currentTarget.value)}><option value="">No company / unassigned</option>{#each options.companies||[] as c}<option value={String(c.id)}>{c.name}</option>{/each}</select></label>{/if}{#if options.permissions?.download&&(!isJV&&company||options.permissions?.standalone)}<div class="ex-actions"><a data-sveltekit-reload data-sveltekit-preload-data="false" class="ex-button secondary" href={exportURL('register')}><Download size={16}/>Excel register</a>{#if !isJV}<div class="ex-statement-download">{#if partyFilter}<a data-sveltekit-reload data-sveltekit-preload-data="false" class="ex-button secondary" href={exportURL('statement')}><Download size={16}/>Party outstanding</a>{:else}<button class="ex-button secondary" disabled aria-describedby="export-statement-help"><Download size={16}/>Party outstanding</button><small id="export-statement-help">Select a party to download its outstanding statement.</small>{/if}</div>{/if}</div>{/if}</div>
  {#if error}<p class="ex-error" role="alert" tabindex="-1">{error}</p>{/if}{#if notice}<p class="ex-notice" role="status">{notice}</p>{/if}
  {#if savedID}<div class="ex-saved-recovery"><p>The {isJV?'JV':'export'} was saved, but its refreshed details could not be loaded.</p><button class="ex-button secondary" disabled={busy} onclick={()=>open(savedID)}>{isJV?'Open saved JV':'Open saved export'}</button></div>{/if}
  {#if !loading&&(isJV||!company)&&!options.permissions?.standalone&&!error}<section class="ex-company-required"><h2>{isJV?'JV access is not assigned':'Select an assigned company'}</h2><p>{isJV?'JV records are available to authorised Finance and management users.':'Your account can work with company-linked exports. Choose an assigned company above to view its invoices.'}</p>{#if !isJV&&companySetupURL}<a class="ex-button secondary" href={companySetupURL}>Open Company Setup</a>{/if}</section>{:else}
  <div class="ex-filters"><label class="ex-search"><span>Search register</span><div><Search size={17}/><input bind:value={search} placeholder={isJV?'JV reference, party or Tally reference':'Invoice, party, shipping bill or reference'}/></div></label><label>Financial year<select bind:value={year}><option value="">All years</option>{#each years as value}<option>{value}</option>{/each}</select></label><label>Party<select bind:value={partyFilter}><option value="">All parties</option>{#each registerParties as party}<option value={partyKey(party)}>{party.name}</option>{/each}</select></label><label>Currency<select bind:value={currencyFilter}><option value="">All currencies</option>{#each options.currencies||[] as value}<option>{value}</option>{/each}</select></label><label>Approval status<select bind:value={workflowFilter}><option value="">All statuses</option>{#each exportWorkflowStatuses as value}<option>{value}</option>{/each}</select></label></div>
  <div class="ex-register-meta"><strong>{records.length} {isJV?(records.length===1?'JV':'JVs'):(records.length===1?'export':'exports')}</strong><span>{isJV?'Amounts are totalled separately for each currency.':'Outstanding is kept separately for each currency.'}</span></div>
  {#if isJV}<div class="ex-table-scroll"><table class="ex-jv-table"><thead><tr>{#each jvFields as field}<th class:numeric={field.type==='money'}>{field.label}</th>{/each}</tr></thead><tbody>{#each records as r}<tr><td><button id={'export-open-'+r.id} class="ex-reference" disabled={busy} onclick={()=>open(r.id)}>{r.invoice_number}</button><small><span class="ex-status" class:complete={r.workflow_status==='Approved'} class:submitted={r.workflow_status==='Submitted'} class:correction={r.workflow_status==='Changes requested'}>{r.workflow_status||'Draft'}</span></small></td><td>{exportDate(r.profile?.export_date)}</td><td>{r.party_name}</td><td>{r.currency}</td><td class="numeric">{moneyText(r.invoice_amount)}</td><td>{r.profile?.shipping_mode||'—'}</td><td>{r.profile?.jv_by_date||'—'}</td><td>{r.profile?.scrip_tally_reference||'—'}</td></tr>{:else}<tr><td colspan="8">{@render empty(loading?'Loading JV register…':search||year||partyFilter||currencyFilter||workflowFilter?'No matching JVs':'No JV records yet',loading?'Fetching your authorised JV records.':search||year||partyFilter||currencyFilter||workflowFilter?'Change the search or filters to see more records.':options.permissions?.edit?'Select Add JV to enter its reference, party and amount.':'Authorised JV records will appear here.')}</td></tr>{/each}</tbody></table></div>{:else}
  <div class="ex-table-scroll"><table><thead><tr><th>S.No.</th><th>Invoice / reference</th><th>Party</th><th>Approval</th><th>Shipment</th><th class="numeric">Invoice amount</th><th class="numeric">Outstanding</th><th>File checks</th></tr></thead><tbody>{#each records as r,i}<tr><td class="ex-sequence">{i+1}</td><td><button id={'export-open-'+r.id} class="ex-reference" disabled={busy} onclick={()=>open(r.id)}>{r.invoice_number}</button><small>{r.profile?.export_reference||exportReference(r.id)} · {fiscalYear(r)}</small></td><td>{r.party_name}</td><td><span class="ex-status" class:complete={r.workflow_status==='Approved'} class:submitted={r.workflow_status==='Submitted'} class:correction={r.workflow_status==='Changes requested'}>{r.workflow_status||'Draft'}</span></td><td>{r.profile?.shipping_mode||'—'}<small>{r.profile?.shipping_bill_number||'No shipping bill'}</small></td><td class="numeric">{moneyText(r.invoice_amount,r.currency)}</td><td class="numeric"><strong>{moneyText(r.balance?.outstanding,r.currency)}</strong>{#if moneyMinor(r.balance?.outstanding)<0n}<small>Overpaid</small>{/if}</td><td><span class="ex-check-state" class:checked={r.finance_check?.status==='Checked'}>Finance: {r.finance_check?.status||'Pending'}</span><span class="ex-check-state" class:checked={r.audit_check?.status==='Checked'}>Audit: {r.audit_check?.status||'Pending'}</span></td></tr>{:else}<tr><td colspan="8">{@render empty(loading?'Loading export register…':search||year||partyFilter||currencyFilter||workflowFilter?'No matching exports':'No export records yet',loading?'Fetching your authorised invoices.':search||year||partyFilter||currencyFilter||workflowFilter?'Change the search or filters to see more records.':options.permissions?.edit?'Select Add export invoice to start its invoice, payment and document file.':'Exports available to your company and assigned buyers will appear here.')}</td></tr>{/each}</tbody></table></div>
  {/if}
  {#if filteredTotals.length}<div class="ex-currency-totals">{#each filteredTotals as total}<p><strong>{total.currency}</strong><span>{total.count} {isJV?'JVs':'invoices'}</span><span>{isJV?'Amount total':'Invoice total'} {moneyText(total.invoice)}</span>{#if !isJV}<span>Outstanding <strong>{moneyText(total.outstanding)}</strong></span>{/if}</p>{/each}</div>{/if}
  {/if}
 {:else}
  {#if editing}
   <form class="ex-editor" onsubmit={save} oninput={()=>dirty=true} onchange={()=>dirty=true} use:enter={'edit-'+(form.id||'new')}>
    <header class="ex-task-heading"><div><h1 id="export-task-title" tabindex="-1">{isJV?(form.id?'Edit JV':'Add JV'):(form.id?'Edit export invoice':'Add export invoice')}</h1><p>{form.invoice_number||(isJV?'JV details':'Invoice & shipping file')}{#if form.id&&!isJV} · {exportReference(form.id)}{/if}</p></div><div class="ex-actions"><button type="button" class="ex-button header-secondary" disabled={busy} onclick={cancel}>Cancel</button><button class="ex-button header-secondary" name="intent" value="save" disabled={busy}>{busy?'Saving…':'Save draft'}</button><button class="ex-button primary" name="intent" value="submit" disabled={busy}>Save & submit</button></div></header>
    {#if error}<p class="ex-error" role="alert" tabindex="-1">{error}</p>{/if}
    {#if notice}<p class="ex-notice" role="status">{notice}</p>{/if}
    {#if uncertainSave}<p class="ex-notice" role="status">A save or upload is not confirmed yet. Retry the same action to check its result. If the server says it was already submitted, use Cancel to check the saved {recordLabel} or register before starting again.</p>{/if}
    {@render workflowSummary(form)}
    <nav class="ex-tabs" aria-label={isJV?'JV sections':'Export sections'}>{#each tabs as item}<button type="button" class:active={tab===item.key} aria-current={tab===item.key?'page':undefined} onclick={()=>showTab(item.key)}>{item.label}</button>{/each}</nav>
    <div class="ex-panel" use:enter={tab}>{@render panels(form,false)}</div>
    <footer class="ex-task-footer"><span>{dirty?'Unsaved changes':'No unsaved changes'}{#if !hasRequiredDocument} · {isJV?'Supporting JV document':'Invoice document'} required to submit{/if}</span><button type="button" class="ex-button secondary" disabled={busy} onclick={cancel}>Cancel</button><button class="ex-button secondary" name="intent" value="save" disabled={busy}>{busy?'Saving…':'Save draft'}</button><button class="ex-button primary" name="intent" value="submit" disabled={busy}>Save & submit</button></footer>
   </form>
  {:else if detail}
   {@const r=detail.record}{@const balance=r.balance||{}}
   <button class="ex-back" disabled={busy} onclick={back}><ArrowLeft size={16}/>{isJV?'JV register':'Export register'}</button>
   <header class="ex-task-heading"><div><h1 id="export-task-title" tabindex="-1">{r.invoice_number}</h1><p>{r.party_name}{#if !isJV} · {exportReference(r.id)}{/if}</p></div><div class="ex-actions"><button class="ex-button header-secondary" disabled={busy} onclick={()=>run(refreshDetail)} aria-label={isJV?'Refresh JV details':'Refresh export details'}><RefreshCw size={16}/></button>{#if permissions.download}<a data-sveltekit-reload data-sveltekit-preload-data="false" class="ex-button header-secondary" href={'/api/exports/'+r.id+'/export'}><Download size={16}/>Excel file</a>{/if}{#if !isJV}<button class="ex-button header-secondary" disabled={busy||dirty} onclick={()=>selectReconciliation(r.id)}>Reconciliation</button>{/if}{#if actions.edit}<button class="ex-button primary" disabled={busy} onclick={edit}>{isJV?'Edit JV':'Edit invoice'}</button>{/if}</div></header>
   {#if error}<p class="ex-error" role="alert" tabindex="-1">{error}</p>{/if}{#if notice}<p class="ex-notice" role="status">{notice}</p>{/if}
   {@render workflowSummary(r)}
   {#if !isJV}
   <dl class="ex-balance"><div><dt>Bill amount</dt><dd>{moneyText(r.invoice_amount,r.currency)}</dd></div><div><dt>Payment received</dt><dd>{moneyText(balance.payment_received,r.currency)}</dd></div><div><dt>Foreign bank charges</dt><dd>{moneyText(balance.foreign_bank_charges,r.currency)}</dd></div><div><dt>Bank charges</dt><dd>{moneyText(balance.bank_charges,r.currency)}</dd></div><div><dt>Approved adjustment</dt><dd>{moneyText(balance.approved_discount,r.currency)}</dd></div><div class="outstanding"><dt>{moneyMinor(balance.outstanding)<0n?'Overpaid balance':'Outstanding / short amount'}</dt><dd>{moneyText(balance.outstanding,r.currency)}</dd></div></dl>
   <p class="ex-balance-basis">Invoice less payments, recorded foreign and local bank charges, and finally approved adjustments. Gross/net advice figures and eBRC amounts are not counted again.</p>
   {/if}
   <nav class="ex-tabs" aria-label={isJV?'JV sections':'Export sections'}>{#each tabs as item}<button type="button" class:active={tab===item.key} aria-current={tab===item.key?'page':undefined} onclick={()=>showTab(item.key)}>{item.label}</button>{/each}</nav>
   <div class="ex-panel" use:enter={tab}>{@render panels(r,true)}</div>
   {#if actions.submit||actions.approve||actions.request_changes||actions.reopen}
    <footer class="ex-task-footer ex-workflow-footer">
     {#if actions.approve||actions.request_changes||actions.reopen}<label class="ex-workflow-input" for="export-workflow-note">{actions.reopen?'Reason to reopen for correction':'Review comment'}<textarea id="export-workflow-note" bind:value={workflowNote} rows="2" maxlength="2000" disabled={busy} aria-describedby="export-workflow-help"></textarea><small id="export-workflow-help">{actions.reopen?`A reason is required. Reopening removes the approval until the ${recordLabel} is reviewed again.`:`Include a clear correction reason when sending the ${recordLabel} back.`}</small></label>{:else}<span>Ready for MD or Super Admin review</span>{/if}
     <div class="ex-actions">{#if actions.reopen}<button class="ex-button secondary" disabled={busy||!workflowNote.trim()} onclick={()=>workflow('reopen')}>Reopen for correction</button>{/if}{#if actions.request_changes}<button class="ex-button secondary" disabled={busy||!workflowNote.trim()} onclick={()=>workflow('request_changes')}>Request changes</button>{/if}{#if actions.approve}<button class="ex-button primary" disabled={busy} onclick={()=>workflow('approve')}>{isJV?'Approve JV':'Approve invoice'}</button>{/if}{#if actions.submit}<button class="ex-button primary" disabled={busy} onclick={()=>workflow('submit')}>{r.workflow_status==='Changes requested'?'Resubmit for approval':'Submit for approval'}</button>{/if}</div>
    </footer>
   {/if}
  {/if}
 {/if}
</section>

<style>
 .export-workspace{--ex-navy:#132b40;--ex-teal:#176d67;--ex-ink:#142e41;--ex-muted:#47616f;--ex-line:#ccdcdf;color:var(--ex-ink);background:#fff;border:1px solid var(--ex-line);border-radius:14px;min-width:0;font-size:14px;overflow:clip}.ex-register-heading,.ex-section-heading,.ex-company-row,.ex-task-heading,.ex-register-meta{display:flex;align-items:center;justify-content:space-between;gap:20px}.ex-register-heading{padding:28px 28px 20px;align-items:flex-start}.export-workspace h1{font-size:25px;line-height:1.25;letter-spacing:-.02em;margin:0;font-weight:750}.ex-register-heading p{color:var(--ex-muted);line-height:1.6;margin:9px 0 0;max-width:65ch}.ex-actions{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.ex-button{display:inline-flex;align-items:center;justify-content:center;gap:7px;min-height:42px;border:1px solid transparent;border-radius:8px;padding:10px 15px;font-family:inherit;font-size:13px;font-weight:750;line-height:1.3;text-decoration:none;cursor:pointer;white-space:nowrap;background:none;color:var(--ex-ink)}.ex-button.primary{background:var(--ex-teal);color:#fff}.ex-button.primary:hover{background:#0c5954}.ex-button.secondary{background:#edf4f5;border-color:#bfced3;color:#183f50}.ex-button.secondary:hover{background:#deecec}.ex-button.quiet{color:var(--ex-teal);padding:8px;min-height:36px}.ex-button.quiet:hover{background:#e9f3ef}.ex-button.header-secondary{background:#e8f0f3;border-color:#cadde4;color:#19374b}.export-workspace button:disabled,.export-workspace input:disabled,.export-workspace select:disabled,.export-workspace textarea:disabled{cursor:not-allowed;opacity:.6}.export-workspace :is(button,a,input,textarea,select,summary):focus-visible{outline:3px solid #58a9a1;outline-offset:3px}.export-workspace :is(h1,h3,h4,[role=alert]):focus{outline:none}.ex-company-row{padding:0 28px 24px;border-bottom:1px solid var(--ex-line);align-items:end}.ex-company-row>label{width:min(360px,100%)}.export-workspace label,.ex-field>label{font-size:13px;font-weight:700;color:var(--ex-ink);display:flex;flex-direction:column;gap:7px;min-width:0}.export-workspace input,.export-workspace select,.export-workspace textarea{box-sizing:border-box;width:100%;min-width:0;min-height:44px;border:1px solid #b8ccd4;border-radius:8px;padding:10px 12px;background:#eff5f8;color:var(--ex-ink);font-family:inherit;font-size:14px;line-height:1.45;caret-color:#087d76}.export-workspace input::placeholder{color:#56717e;opacity:1}.export-workspace textarea{resize:vertical}.export-workspace small{font-size:12px;font-weight:400;color:var(--ex-muted);line-height:1.5}.ex-filters{display:grid;grid-template-columns:minmax(230px,1.5fr) minmax(130px,.7fr) minmax(180px,1fr) minmax(130px,.7fr);gap:16px;padding:24px 28px 18px}.ex-search>div{position:relative}.ex-search :global(svg){position:absolute;left:12px;top:13px;color:var(--ex-muted)}.export-workspace .ex-search input{padding-left:40px!important}.ex-register-meta{padding:0 28px 15px;gap:14px;font-size:13px}.ex-register-meta>span{color:var(--ex-muted)}.ex-table-scroll{overflow:auto;max-width:100%;scrollbar-color:#91b3b3 #edf4f5;scrollbar-width:thin}.ex-table-scroll table{border-collapse:collapse;width:100%;min-width:850px;table-layout:auto;font-variant-numeric:tabular-nums}.ex-table-scroll th{background:#edf4f5;color:#365260;font-size:12px;text-align:left;font-weight:750;white-space:nowrap}.ex-table-scroll th,.ex-table-scroll td{padding:15px 18px;border-bottom:1px solid #d9e5e7;vertical-align:middle;height:auto;line-height:1.45}.ex-table-scroll td{font-size:13px}.ex-table-scroll td small{display:block;margin-top:5px}.ex-table-scroll .numeric{text-align:right;white-space:nowrap}.ex-reference{background:none;border:0;padding:0;color:var(--ex-teal);font:inherit;font-weight:750;text-align:left;text-decoration:underline;text-underline-offset:3px;cursor:pointer;white-space:nowrap}.ex-check-state{display:block;color:#536a76;font-size:12px;white-space:nowrap;margin:4px 0}.ex-check-state.checked{color:#176549}.ex-empty{padding:44px 24px;max-width:520px;margin:auto;text-align:center}.ex-empty :global(svg){color:var(--ex-teal)}.ex-empty h3{font-size:18px;margin:15px 0 8px}.ex-empty p{color:var(--ex-muted);line-height:1.6;margin:0}.ex-currency-totals{padding:18px 28px;background:#f6f9f8}.ex-currency-totals p{display:flex;align-items:center;gap:22px;flex-wrap:wrap;line-height:1.5;margin:7px 0;font-variant-numeric:tabular-nums}.ex-currency-totals p>strong{color:var(--ex-teal);min-width:42px}.ex-currency-totals span{font-size:13px}.ex-statement-download{display:flex;flex-direction:column;align-items:flex-start;gap:6px;max-width:250px}.ex-saved-recovery{margin:18px 26px;display:flex;align-items:center;gap:18px;flex-wrap:wrap;color:#365c4b;line-height:1.5}.ex-error,.ex-notice{padding:14px 18px;margin:18px 26px;border-radius:8px;line-height:1.6;overflow-wrap:anywhere}.ex-error{color:#8e2c27;background:#fbecea;border:1px solid #e8c8c4}.ex-notice{color:#185842;background:#e5f2e9;border:1px solid #c2ddcc}.ex-panel .ex-notice{margin:18px 0}.ex-task-heading{position:sticky;top:0;z-index:6;background:var(--ex-navy);padding:20px 28px;color:#fff}.ex-task-heading h1{font-size:23px;color:#fff;overflow-wrap:anywhere}.ex-task-heading p{margin:7px 0 0;color:#d4e6ed;line-height:1.5}.ex-task-heading>div:first-child{min-width:0}.ex-task-heading .ex-button.primary{background:#147b71}.ex-back{background:none;border:0;display:flex;align-items:center;gap:7px;font:inherit;font-size:13px;font-weight:750;color:var(--ex-teal);padding:19px 28px;cursor:pointer}.ex-tabs{display:flex;gap:0;overflow:auto;background:#f1f6f5;border-bottom:1px solid var(--ex-line);padding:0 20px;scrollbar-color:#91b3b3 #edf4f5;scrollbar-width:thin}.ex-tabs>button{border:0;border-bottom:3px solid transparent;background:none;min-height:52px;padding:15px 13px;color:#3f5f6c;font-family:inherit;font-size:13px;font-weight:700;white-space:nowrap;cursor:pointer}.ex-tabs>button:hover{background:#e5efeb}.ex-tabs>button.active{border-bottom-color:var(--ex-teal);color:#105b55;background:#e4f0ea}.ex-panel{padding:0 28px}.ex-section{padding:28px 0;border-bottom:1px solid var(--ex-line)}.ex-section:last-child{border-bottom:0}.export-workspace h3{font-size:18px;margin:0 0 18px;font-weight:750;line-height:1.35}.export-workspace h4{font-size:15px;margin:0 0 16px;font-weight:750;line-height:1.45}.ex-section-heading{margin-bottom:22px;align-items:flex-start}.ex-section-heading h3,.ex-section-heading h4{margin-bottom:0}.ex-section-heading>div{max-width:72ch}.ex-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:22px}.ex-grid.read{gap:22px 26px}.ex-grid .wide{grid-column:1/-1}.ex-field{display:flex;flex-direction:column;gap:7px;min-width:0}.ex-label{font-size:12px;font-weight:700;color:var(--ex-muted);display:block;margin-bottom:2px}.ex-value{font-weight:600;white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.6;min-height:22px}.ex-caption{display:inline;line-height:1.5}.export-workspace .ex-caption .ex-required{display:inline}.export-workspace .ex-editor :is(input,select,textarea){scroll-margin-top:140px;scroll-margin-bottom:110px}.ex-required{color:#a83129;font-size:13px}.ex-link-fields{margin-bottom:24px;grid-template-columns:2fr 1fr}.ex-record-links{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:22px;margin-bottom:26px;padding-bottom:22px;border-bottom:1px solid #dce7e8}.ex-record-links strong{display:block;margin-top:7px;line-height:1.5;overflow-wrap:anywhere}.ex-subsection{margin-top:26px;padding-top:24px;border-top:1px solid #d4e1e3}.ex-muted{color:var(--ex-muted);line-height:1.65;margin:8px 0 20px;max-width:75ch}.ex-repeat{padding:26px 0;border-top:1px solid #d5e3e5}.ex-repeat:last-child{padding-bottom:0}.ex-repeat .ex-section-heading{align-items:center}.ex-repeat h4{color:#155c58}.export-workspace summary{cursor:pointer;font-weight:750;color:var(--ex-ink);line-height:1.5;padding:2px 0}.export-workspace details[open]>summary{margin-bottom:18px}.ex-task-footer{position:sticky;bottom:0;z-index:5;background:#e8f0ec;border-top:1px solid #c2d3cb;padding:16px 28px;display:flex;gap:12px;justify-content:flex-end;align-items:center}.ex-task-footer>span{margin-right:auto;font-size:12px;color:#446759}.ex-balance{display:grid;grid-template-columns:repeat(6,minmax(0,1fr));margin:0;background:#f5f8f6;border-bottom:1px solid var(--ex-line);font-variant-numeric:tabular-nums}.ex-balance>div{padding:20px 18px;border-right:1px solid #dde7e3;min-width:0}.ex-balance>div:last-child{border-right:0}.ex-balance dt{font-size:11px;font-weight:700;color:#4b665b;margin-bottom:10px;line-height:1.45}.ex-balance dd{font-size:15px;font-weight:750;margin:0;line-height:1.5;overflow-wrap:anywhere}.ex-balance .outstanding{background:#deeee5}.ex-balance-basis{padding:12px 28px;margin:0;background:#f5f8f6;color:#486558;font-size:12px;line-height:1.6}.ex-checklist{list-style:none;margin:0;padding:0;display:grid;grid-template-columns:1fr 1fr;gap:0 28px}.ex-checklist>li{padding:16px 0;border-bottom:1px solid var(--ex-line);min-width:0}.ex-document-kind{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.ex-document-kind strong{font-size:13px}.ex-document-kind small{margin-left:auto}.ex-document-indicator{display:flex;align-items:center;justify-content:center;flex-shrink:0;width:29px;height:29px;border-radius:6px;background:#edf2f4;color:#5d737b}.ex-document-indicator.attached{background:#ddefe4;color:#1a634d}.ex-checklist ul{list-style:none;margin:12px 0 0;padding:0 0 0 39px}.ex-checklist ul li{margin:10px 0;line-height:1.5;overflow-wrap:anywhere}.ex-checklist a{color:#176d67;text-underline-offset:3px;font-size:13px}.ex-upload{display:flex;gap:16px;align-items:end;flex-wrap:wrap;margin-top:26px}.ex-upload small{align-self:center}.ex-upload-label{position:relative;overflow:hidden;flex-direction:row!important;font-size:13px!important}.ex-upload-label input{position:absolute;inset:0;opacity:0;cursor:pointer;min-height:100%;padding:0}.ex-upload-label:focus-within{outline:3px solid #58a9a1;outline-offset:3px}.ex-review-grid{display:grid;grid-template-columns:1fr 1fr;gap:28px}.ex-review-grid>div{padding-bottom:18px}.ex-review-grid p{font-size:13px;color:var(--ex-muted);line-height:1.6}.ex-status{font-size:12px;font-weight:750;padding:6px 9px;background:#eef2f6;color:#385264;border-radius:6px;display:inline-block}.ex-status.complete{background:#e1f0e7;color:#205f48}.ex-review-form,.ex-request-form{padding-top:22px;border-top:1px solid var(--ex-line)}.ex-review-form .ex-actions,.ex-approval .ex-actions{margin-top:16px}.ex-approval{padding:22px 0;border-bottom:1px solid var(--ex-line)}.ex-approval:last-of-type{border-bottom:0}.ex-approval p{line-height:1.6}.ex-approval .ex-section-heading{margin-bottom:12px}.ex-meta{font-size:12px;color:var(--ex-muted)}.ex-prewrap{white-space:pre-wrap;overflow-wrap:anywhere}.ex-request-form{margin-top:24px}.ex-request-form>button{margin-top:20px}.ex-history{padding:0;margin:0;list-style:none}.ex-history li{display:flex;align-items:center;justify-content:space-between;gap:20px;padding:15px 0;border-bottom:1px solid var(--ex-line)}.ex-history strong{font-size:13px}.ex-history small{display:block;margin-top:6px}.ex-history li:last-child{border-bottom:0}.ex-revision{margin-top:22px;padding:24px;background:#f0f5f3;border-radius:12px}.ex-revision>h4{margin-top:28px}.ex-revision>.ex-section-heading{gap:18px}.ex-revision>p{color:var(--ex-muted);font-size:13px}.export-workspace ::selection{color:#132b40;background:#c7e5dd}
 @media(min-width:1024px){.ex-company-required{padding:32px 28px;max-width:76ch}.ex-company-required h2{font-size:19px;margin:0 0 12px;color:var(--ex-ink)}.ex-company-required p{line-height:1.65;color:var(--ex-muted);margin:0 0 16px}}
 @media(max-width:1150px){.ex-filters{grid-template-columns:1fr 1fr}.ex-grid,.ex-grid.read{grid-template-columns:repeat(2,minmax(0,1fr))}.ex-balance{grid-template-columns:repeat(3,minmax(0,1fr))}.ex-balance>div{border-bottom:1px solid #dde7e3}.ex-register-heading,.ex-company-row{flex-wrap:wrap}.ex-register-heading,.ex-company-row,.ex-filters{padding-left:22px;padding-right:22px}.ex-panel{padding:0 22px}.ex-task-heading{padding:18px 22px;align-items:flex-start}.ex-task-heading .ex-actions{justify-content:flex-end}.ex-checklist{grid-template-columns:1fr}.ex-record-links{grid-template-columns:repeat(2,minmax(0,1fr))}.ex-table-scroll th,.ex-table-scroll td{height:auto!important;min-height:0;padding:14px 15px!important}.ex-table-scroll td button{min-width:max-content}.ex-upload small{width:100%}}
 @media(max-width:620px){.ex-grid,.ex-grid.read,.ex-filters,.ex-review-grid,.ex-record-links{grid-template-columns:1fr}.ex-task-heading{flex-wrap:wrap}.ex-task-heading h1{font-size:21px}.ex-task-heading .ex-actions{width:100%;justify-content:flex-start}.ex-balance{grid-template-columns:repeat(2,minmax(0,1fr))}.ex-register-meta{align-items:flex-start;flex-direction:column}.ex-section-heading{flex-wrap:wrap}.ex-task-footer{padding:14px 18px}.ex-task-footer>span{display:none}.ex-currency-totals p{gap:9px 18px}.ex-history li{align-items:flex-start;flex-wrap:wrap}}
 @media(min-width:1024px){
  .ex-register-tabs{padding-top:4px}
  .ex-jv-table td{max-width:280px;overflow-wrap:anywhere}
  .ex-filters{grid-template-columns:repeat(auto-fit,minmax(150px,1fr))}
  .ex-task-heading{flex-wrap:wrap}
  .ex-workflow-summary{padding:20px 28px;border-bottom:1px solid var(--ex-line);background:#f4f7f9}
  .ex-workflow-summary.correction{background:#fff6e8;border-color:#e2cda9}
  .ex-workflow-status{display:flex;align-items:center;gap:16px}
  .ex-workflow-status p{margin:0;line-height:1.6;color:#365260;max-width:75ch}
  .ex-workflow-summary>small{display:block;margin-top:10px}
  .ex-status.submitted{background:#e7eff8;color:#264d73}
  .ex-status.correction{background:#fce8c3;color:#714808}
  .ex-correction-note{margin-top:16px;padding-top:16px;border-top:1px solid #e2cda9;overflow-wrap:anywhere}
  .ex-correction-note p{margin:8px 0;max-width:75ch;line-height:1.65}
  .ex-workflow-footer{align-items:flex-end;gap:20px;flex-wrap:wrap}
  .ex-workflow-input{flex:1 1 320px;max-width:780px}
  .ex-workflow-footer>.ex-actions{flex-shrink:0;justify-content:flex-end}
  .ex-workflow-footer .ex-workflow-input textarea{scroll-margin-bottom:24px}
  .ex-comments p{margin:10px 0 0;max-width:75ch;line-height:1.65}
  .ex-comment-form{display:grid;gap:14px;margin-top:22px;max-width:780px}
  .ex-comment-form>.ex-button{justify-self:start}
  .ex-link-fields.ex-invoice-company{max-width:900px;display:grid;grid-template-columns:minmax(0,1fr);gap:9px}
  .ex-company-choice{display:flex;align-items:center;gap:12px;flex-wrap:wrap}
  .ex-company-choice>select{flex:1 1 320px;width:auto}
  .ex-company-add{display:grid;gap:14px;margin-top:12px;padding-top:20px;border-top:1px solid var(--ex-line)}
  .ex-company-add .ex-error{margin:0}
  .ex-checklist-attach{margin-left:auto;min-height:36px;padding:7px 10px}
  .ex-checklist .ex-document-files{padding-left:0}
  .ex-document-files>li{display:flex;gap:8px 16px;align-items:flex-start;justify-content:space-between;flex-wrap:wrap;padding:10px 0}
  .ex-document-info{flex:1 1 170px;min-width:0;overflow-wrap:anywhere}
  .ex-document-info strong{font-size:13px;line-height:1.6}
  .ex-document-info small{display:block;margin-top:4px}
  .ex-document-files .ex-actions{gap:2px}
  .ex-document-files .ex-button{font-size:12px;padding:7px}
  .ex-document-files .ex-file-error{color:#8e2c27}
  .ex-document-fallback{flex-basis:100%;font-size:12px;line-height:1.6;color:var(--ex-muted);margin:2px 0 0}

  .ex-banking .ex-section-heading{margin-bottom:16px}.ex-banking .ex-muted{font-size:12px;line-height:1.6;margin:7px 0 14px}
  .ex-bank-receipt{padding-top:16px}.ex-bank-receipt> .ex-section-heading{padding-bottom:10px;margin-bottom:0}
  .ex-bank-sequence{list-style:none;margin:0;padding:0}.ex-bank-stage{padding:16px 0;border-top:1px solid var(--ex-line)}
  .ex-bank-stage-heading{display:flex;align-items:center;gap:10px;margin-bottom:12px}.ex-bank-stage-heading h5{font-size:15px;line-height:1.5;color:var(--ex-ink);margin:0;font-weight:750}
  .ex-bank-number{display:inline-flex;align-items:center;justify-content:center;width:25px;height:25px;flex-shrink:0;border-radius:6px;background:#e8f1ef;color:#175c55;font-size:12px;font-weight:750;font-variant-numeric:tabular-nums}
  .ex-banking .ex-grid{grid-template-columns:repeat(3,minmax(0,1fr));gap:12px 18px}.ex-banking .ex-field{gap:5px}.ex-banking .ex-caption,.ex-banking .ex-label{font-size:12px}.ex-banking :is(input,select){min-height:40px;padding:8px 10px;font-size:13px}.ex-banking .ex-value{font-size:13px;line-height:1.5}
  .ex-checklist>li{scroll-margin-top:125px}.ex-checklist>li:focus{outline:3px solid #58a9a1;outline-offset:5px}

  .ex-bank-context{padding:14px 16px;background:#f4f7f9;margin:0 0 8px;border-radius:8px}.ex-bank-context .ex-grid{grid-template-columns:repeat(4,minmax(0,1fr));gap:12px 18px}
  .ex-bank-carried{display:flex;align-items:center;gap:8px 18px;flex-wrap:wrap;padding:10px 12px;background:#edf5f1;border-radius:8px;font-size:12px;margin:0 0 16px}.ex-bank-carried strong{font-variant-numeric:tabular-nums}.ex-bank-carried small{margin-left:auto}
  .ex-bank-stage-main{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px 18px;align-items:start}.ex-bank-stage-fields,.ex-bank-stage-fields>.ex-grid{display:contents}.ex-bank-stage-attachment{min-width:0;align-self:start}.ex-bank-stage-attachment.full-row{grid-column:1/-1;padding-top:2px}
  .ex-bank-shipping{margin-top:16px;padding-top:14px;border-top:1px solid var(--ex-line)}.ex-bank-shipping h6{font-size:12px;line-height:1.5;margin:0 0 10px;font-weight:750}.ex-bank-document-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1.25fr);gap:18px;align-items:start;padding:8px 0}.ex-bank-document-row>div{min-width:0}.ex-bank-document-row .ex-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.ex-bank-document-row+.ex-bank-document-row{border-top:1px solid #e1e9eb;margin-top:2px;padding-top:12px}.ex-bank-ports{padding-top:10px}.ex-bank-ports .ex-grid{grid-template-columns:repeat(2,minmax(0,1fr))}

  .ex-documentation-state{display:flex;align-items:center;gap:12px;flex-wrap:wrap;font-size:12px;line-height:1.6;color:#205f48;margin:14px 0 0}

 }

</style>
