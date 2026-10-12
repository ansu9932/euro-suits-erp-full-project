<script lang="ts">
 import {FileText,Trash2} from '@lucide/svelte';
 import CompanyFileActions from './CompanyFileActions.svelte';
 import {linkedCompanyFiles,companyFileStorageNote} from './companyDocuments';
 import type {Row} from './types';
 let {target,files=[],recoveryFiles=[],editable=false,busy=false,label='Supporting copies',onselect,onupload}:{target:Row;files?:Row[];recoveryFiles?:Row[];editable?:boolean;busy?:boolean;label?:string;onselect:(id:number,checked:boolean)=>void;onupload?:(file:File)=>Promise<void>}=$props();
 let linked=$derived(linkedCompanyFiles(target,files));
 let removing=$state<number|null>(null),recoverID=$state(''),removed=$state(false);
 function remove(id:number){if(!editable||busy)return;onselect(id,false);removing=null;removed=true;}
</script>
<div class="linked-documents"><strong>{label}</strong>
 {#each linked as doc (doc.id)}
  <div class="company-file-row"><FileText size={20}/><div class="company-file-name"><strong>{doc.name}</strong><small>{Math.ceil(doc.size/1024)} KB · attached to this item</small>{#if companyFileStorageNote(doc)}<small>{companyFileStorageNote(doc)}</small>{/if}</div><div class="company-file-actions"><CompanyFileActions {doc}/>{#if editable}<button type="button" class="text-button danger" disabled={busy} onclick={()=>{removing=doc.id;removed=false}} aria-label={'Remove '+doc.name+' from '+label}><Trash2 size={16}/>Remove</button>{/if}</div></div>
  {#if removing===doc.id}
   <div class="company-remove-confirm" role="group" aria-label="Confirm document removal"><p>Remove <strong>{doc.name}</strong> from this item? Save the draft to apply the removal. The original stays in Documents for recovery and audit.</p><div class="row-actions"><button type="button" class="button secondary" disabled={busy} onclick={()=>removing=null}>Keep document</button><button type="button" class="button secondary danger" disabled={busy} onclick={()=>remove(doc.id)}>Confirm removal</button></div></div>
  {/if}
 {:else}<p class="muted">No supporting copies attached to this item.</p>{/each}
 {#if removed}<small role="status">Document removed from this item. Save the draft to keep the change.</small>{/if}
 {#if editable&&onupload}
  <label class="button secondary">Attach document here<input class="upload-file-input" aria-label={'Upload '+label} type="file" accept=".pdf,.png,.jpg,.jpeg,.docx,.xlsx" disabled={busy} onchange={async e=>{const input=e.currentTarget;const file=input.files?.[0];if(file){await onupload?.(file);input.value='';removed=false}}}/></label>
  <small>PDF, images, Word or Excel · up to 20 MB. Save the draft to keep this link.</small>
  <small>New files are compressed automatically when it saves space, without quality loss. Downloads restore the exact original.</small>
 {/if}
 {#if editable&&recoveryFiles.length}
  <details class="company-file-recovery"><summary>Use an unattached document ({recoveryFiles.length})</summary><label>Unattached document<select bind:value={recoverID} disabled={busy}><option value="">Choose a document</option>{#each recoveryFiles as doc}<option value={String(doc.id)}>{doc.name}</option>{/each}</select></label><button type="button" class="button secondary" disabled={busy||!recoveryFiles.some(doc=>String(doc.id)===recoverID)} onclick={()=>{onselect(Number(recoverID),true);recoverID='';removed=false}}>Attach selected document</button></details>
 {/if}
</div>
