<script lang="ts">
 import {onMount} from 'svelte';
 import {Mail,RefreshCw,Paperclip,Send,Eye} from '@lucide/svelte';
 import {api} from './api';
 import {enter} from './motion';
 import {bankEmailDocumentKinds,receiptDocumentLabel,bankEmailRecipients,bankEmailRecipientError,bankEmailCanConfirm,bankEmailAttachmentError,uncertainInvoiceWrite,exportFileCanPreview} from './exportDocumentation';
 import type {Row} from './types';
 let {record,files,userID,busy=$bindable(false)}:{record:Row;files:Row[];userID:number;busy?:boolean}=$props();
 let items:Row[]=$state([]),prepared:Row|null=$state(null),configured=$state(false),canSend=$state(false),loaded=$state(false);
 let recipients=$state(''),subject=$state(''),body=$state(''),selected:number[]=$state([]),error=$state(''),notice=$state(''),uncertain=$state(false);
 let available=$derived(files.filter(file=>bankEmailDocumentKinds.includes(file.kind)));
 const dateTime=(value:string)=>value?new Date(value).toLocaleString('en-GB',{dateStyle:'medium',timeStyle:'short'}):'—';
 const statusLabel=(status:string)=>status==='Accepted'?'Accepted by mail service':status==='Uncertain'?'Send result unconfirmed':status==='Sending'?'Sending':status==='Failed'?'Not sent':status==='Prepared'?'Prepared for review':status;
 async function refresh(){
  if(busy)return;busy=true;error='';
  try{const result=await api(`/exports/${record.id}/bank-email`);items=result.items||[];configured=!!result.configured;canSend=!!result.can_send;loaded=true;if(prepared)prepared=items.find(item=>item.id===prepared!.id)||prepared;uncertain=false}
  catch(e){error=(e as Error).message}finally{busy=false}
 }
 onMount(()=>{subject=`Export documents · ${record.invoice_number}`;void refresh()});
 async function prepare(){
  if(busy)return;error=bankEmailRecipientError(recipients);if(error)return;
  if(!subject.trim()||!body.trim()){error='Enter an email subject and message before reviewing.';return}
  error=bankEmailAttachmentError(selected,available);if(error)return;
  busy=true;notice='';
  try{prepared=await api(`/exports/${record.id}/bank-email/prepare`,{version:record.version,recipients:bankEmailRecipients(recipients),subject:subject.trim(),body:body.trim(),file_ids:selected});items=[prepared!,...items.filter(item=>item.id!==prepared!.id)];uncertain=false}
  catch(e){error=(e as Error).message;uncertain=uncertainInvoiceWrite(e)}finally{busy=false}
 }
 async function send(){
  if(busy||!bankEmailCanConfirm(prepared,userID,Number(record.version))||!prepared||uncertain||!canSend||!configured)return;
  busy=true;error='';notice='';
  try{const result:Row=await api(`/exports/${record.id}/bank-email/${prepared.id}/send`,{confirmed:true,version:record.version});prepared=result;items=[result,...items.filter(item=>item.id!==result.id)];notice=result.status==='Accepted'?'The mail service accepted this message. This does not confirm bank inbox delivery.':result.status==='Uncertain'?'The send result is unconfirmed. Check the mail service or bank before sending again.':result.status==='Failed'?'The message was not sent. Check its recipients and configuration, then review before confirming a retry.':'Email status updated.'}
  catch(e){error=(e as Error).message;uncertain=uncertainInvoiceWrite(e)}finally{busy=false}
 }
 function resume(item:Row){if(busy)return;prepared=item;error='';notice='';uncertain=false}
</script>

<section class="bank-email" id="export-bank-email" aria-busy={busy}>
 <div class="bank-email-heading"><h5><Mail size={17}/>Email to bank · this invoice</h5><button type="button" class="bank-button quiet" disabled={busy} onclick={refresh}><RefreshCw size={15}/>Refresh status</button></div>
 <p class="bank-help">Select saved documents, check the recipients and review the message. Nothing is sent until you confirm.</p>
 {#if error}<p class="bank-alert" role="alert">{error}</p>{/if}{#if notice}<p class="bank-notice" role="status">{notice}</p>{/if}
 {#if uncertain}<p class="bank-alert">The last request is unconfirmed. Refresh its status before preparing or sending another message.</p>{/if}
 {#if loaded&&!configured}<p class="bank-help">Bank email sending is not configured. You can prepare the message for review.</p>{/if}
 {#if !loaded}<p class="bank-help" role="status">Loading bank email history…</p>
 {:else if prepared}
  <div class="bank-review" use:enter={prepared.id}>
   <h5>Review email #{prepared.id} · {statusLabel(prepared.status)}</h5>
   <dl><div><dt>To</dt><dd>{(prepared.recipients||[]).join(', ')}</dd></div><div><dt>Subject</dt><dd>{prepared.subject}</dd></div><div><dt>Message</dt><dd class="bank-message">{prepared.body}</dd></div></dl>
   <h6>Attachments · {(prepared.files||[]).length}</h6><ul class="bank-files">{#each prepared.files||[] as file}<li><Paperclip size={15}/><strong>{file.filename}</strong><span>{receiptDocumentLabel(file,record.receipts||[])}</span><span>{Math.max(1,Math.round(file.size/1024))} KB</span>{#if exportFileCanPreview(file)}<a data-sveltekit-reload data-sveltekit-preload-data="false" href={'/api/export-files/'+file.id+'?preview=1'} target="_blank" rel="noopener noreferrer"><Eye size={14}/>Preview</a>{/if}</li>{/each}</ul>
   {#if prepared.error_message}<p class="bank-alert">{prepared.error_message}</p>{/if}
   {#if prepared.status==='Uncertain'||prepared.status==='Sending'}<p class="bank-alert">Check the mail service or bank before sending another copy. This message cannot be sent again here.</p>{/if}
   <div class="bank-actions"><button type="button" class="bank-button secondary" disabled={busy||uncertain} onclick={()=>{prepared=null;notice=''}}>Back to message</button>{#if ['Prepared','Failed'].includes(prepared.status)&&Number(prepared.created_by)===userID}<button type="button" class="bank-button primary" disabled={busy||uncertain||!configured||!canSend||!bankEmailCanConfirm(prepared,userID,Number(record.version))} onclick={send}><Send size={15}/>{busy?'Sending…':prepared.status==='Failed'?'Confirm and retry':'Confirm and send'}</button>{/if}</div>
   {#if ['Prepared','Failed'].includes(prepared.status)&&Number(prepared.invoice_version)!==Number(record.version)}<p class="bank-help">The invoice changed after this email was prepared. Refresh the invoice and prepare a new review.</p>{/if}
  </div>
 {:else}
  <div class="bank-compose">
   <label for="bank-email-recipients">Bank email recipients<input id="bank-email-recipients" type="text" bind:value={recipients} placeholder="bank@example.com" maxlength="1500" disabled={busy||uncertain}/><small>Up to five addresses, separated by commas.</small></label>
   <label for="bank-email-subject">Subject<input id="bank-email-subject" bind:value={subject} maxlength="160" disabled={busy||uncertain}/></label>
   <label class="wide" for="bank-email-body">Message<textarea id="bank-email-body" bind:value={body} maxlength="10000" rows="4" disabled={busy||uncertain}></textarea></label>
  </div>
  <fieldset class="bank-selection" disabled={busy||uncertain}><legend>Select attachments · up to 12 files / 15 MB</legend>{#each available as file}<label><input type="checkbox" value={Number(file.id)} bind:group={selected}/><span><strong>{file.filename}</strong><small>{receiptDocumentLabel(file,record.receipts||[])} · {Math.max(1,Math.round(file.size/1024))} KB</small></span>{#if exportFileCanPreview(file)}<a data-sveltekit-reload data-sveltekit-preload-data="false" href={'/api/export-files/'+file.id+'?preview=1'} target="_blank" rel="noopener noreferrer">Preview</a>{/if}</label>{:else}<p class="bank-help">Attach and save the invoice, shipping bill, BL, packing list, bank advice or receipt documents first.</p>{/each}</fieldset>
  <button type="button" class="bank-button secondary" disabled={busy||uncertain||!selected.length} onclick={prepare}><Eye size={15}/>{busy?'Preparing…':'Review email'}</button>
 {/if}
 {#if items.length}<details class="bank-history"><summary>Bank email history · {items.length}</summary><ol>{#each items as item}<li><div><strong>{item.subject}</strong><span>{statusLabel(item.status)} · {dateTime(item.sent_at||item.created_at)}</span><small>{(item.recipients||[]).join(', ')}{#if item.created_by_name} · {item.created_by_name}{/if}</small></div><button type="button" class="bank-button quiet" disabled={busy} onclick={()=>resume(item)}>View email</button></li>{/each}</ol></details>{/if}
</section>

<style>
 @media(min-width:1024px){
  .bank-email{margin-top:18px;padding-top:18px;border-top:1px solid var(--ex-line);color:var(--ex-ink);min-width:0}
  .bank-email-heading,.bank-actions{display:flex;align-items:center;gap:12px;flex-wrap:wrap;justify-content:space-between}.bank-actions{justify-content:flex-start;margin-top:16px}
  h5{font-size:14px;line-height:1.5;margin:0;font-weight:750;display:flex;align-items:center;gap:8px}h6{font-size:13px;margin:18px 0 8px;font-weight:750}
  .bank-help{font-size:12px;color:var(--ex-muted);line-height:1.6;max-width:75ch;margin:8px 0 16px}
  .bank-compose{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.wide{grid-column:1/-1}
  .bank-compose label{display:flex;flex-direction:column;gap:7px;font-size:12px;font-weight:700;min-width:0}
  input:not([type=checkbox]),textarea{width:100%;box-sizing:border-box;min-height:40px;padding:9px 11px;border:1px solid #b8ccd4;border-radius:8px;background:#eff5f8;color:var(--ex-ink);font:inherit;line-height:1.5;caret-color:#087d76}textarea{resize:vertical}input::placeholder{color:#56717e;opacity:1}
  small{font-size:12px;line-height:1.5;font-weight:400;color:var(--ex-muted)}
  .bank-button{display:inline-flex;align-items:center;justify-content:center;gap:7px;min-height:36px;padding:8px 11px;border:1px solid transparent;border-radius:8px;font:inherit;font-size:12px;font-weight:750;line-height:1.4;cursor:pointer;white-space:nowrap;background:none;color:var(--ex-ink)}.bank-button.primary{background:var(--ex-teal);color:white}.bank-button.primary:hover{background:#0c5954}.bank-button.secondary{background:#edf4f5;border-color:#bfced3;color:#183f50}.bank-button.secondary:hover{background:#deecec}.bank-button.quiet{color:var(--ex-teal)}.bank-button.quiet:hover{background:#e9f3ef}
  :is(button,input,textarea):disabled{opacity:.6;cursor:not-allowed}:is(button,input,textarea,a,summary):focus-visible{outline:3px solid #58a9a1;outline-offset:3px}
  .bank-alert,.bank-notice{padding:12px 14px;line-height:1.6;font-size:13px;border-radius:8px}.bank-alert{color:#8e2c27;background:#fbecea;border:1px solid #e8c8c4}.bank-notice{color:#185842;background:#e5f2e9;border:1px solid #c2ddcc}
  .bank-selection{padding:0;margin:20px 0 16px;border:0;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px 18px}.bank-selection legend{font-size:13px;font-weight:750;margin-bottom:12px}.bank-selection label{display:flex;gap:10px;align-items:flex-start;min-width:0;font-size:12px;padding:10px 0;border-bottom:1px solid var(--ex-line)}.bank-selection input{width:17px;height:17px;flex-shrink:0;accent-color:var(--ex-teal);margin:2px 0 0}.bank-selection span{min-width:0;flex:1;overflow-wrap:anywhere}.bank-selection strong,.bank-selection small{display:block;line-height:1.5}.bank-selection small{margin-top:4px}
  a{display:inline-flex;align-items:center;gap:5px;color:var(--ex-teal);font-size:12px;text-underline-offset:3px}
  .bank-review{padding:18px 0 0;border-top:1px solid var(--ex-line)}dl{margin:16px 0}dl>div{display:grid;grid-template-columns:75px minmax(0,1fr);gap:12px;margin:12px 0}dt{font-size:12px;font-weight:700;color:var(--ex-muted)}dd{margin:0;overflow-wrap:anywhere;font-size:13px;line-height:1.6}.bank-message{white-space:pre-wrap}
  .bank-files{list-style:none;padding:0;margin:0}.bank-files li{display:flex;gap:8px;flex-wrap:wrap;align-items:center;font-size:12px;line-height:1.6;padding:8px 0}.bank-files strong{overflow-wrap:anywhere}.bank-files span{color:var(--ex-muted)}
  .bank-history{margin-top:20px;padding-top:16px;border-top:1px solid var(--ex-line)}.bank-history summary{font-size:13px;font-weight:750;cursor:pointer}.bank-history ol{list-style:none;padding:0;margin:12px 0 0}.bank-history li{display:flex;align-items:center;gap:16px;justify-content:space-between;border-bottom:1px solid var(--ex-line);padding:12px 0;min-width:0}.bank-history li>div{min-width:0;overflow-wrap:anywhere}.bank-history strong{font-size:13px}.bank-history span,.bank-history small{display:block;margin-top:4px;font-size:12px;line-height:1.5;color:var(--ex-muted)}
 }
</style>
