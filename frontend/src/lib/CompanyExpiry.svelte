<script lang="ts">
 import {Plus,X} from '@lucide/svelte';
 import type {Row} from './types';
 let {target,legacyEmails=[],editable=false,busy=false,label='Document',onpatch}:{target:Row;legacyEmails?:string[];editable?:boolean;busy?:boolean;label?:string;onpatch:(patch:Row)=>void}=$props();
 let enabled=$derived(target.reminder_enabled==null?true:target.reminder_enabled===true);
 let recipients:string[]=$derived(target.reminder_enabled==null?legacyEmails:(target.reminder_emails||[]));
 function setRecipients(next:string[]){onpatch({reminder_enabled:true,reminder_emails:next});}
 function setEnabled(checked:boolean){const next=checked?[...recipients]:[];if(checked&&!next.length)next.push('');onpatch({reminder_enabled:checked,reminder_emails:next});}
 function setExpiry(checked:boolean){onpatch(checked?{expiry:true}:{expiry:false,reminder_enabled:false,reminder_emails:[]});}
</script>
<div class="company-expiry">
 <label class="inline-check expiry-check"><input type="checkbox" checked={!!target.expiry} disabled={!editable||busy} onchange={e=>setExpiry(e.currentTarget.checked)}/>Has validity / expiry date</label>
 {#if target.expiry}
  <div class="company-form-grid"><label>Valid from<input type="date" value={target.valid_from||''} disabled={!editable||busy} onchange={e=>onpatch({valid_from:e.currentTarget.value})}/></label><label>Expires on<input type="date" value={target.expires_on||''} disabled={!editable||busy} onchange={e=>onpatch({expires_on:e.currentTarget.value})}/></label></div>
  <label class="inline-check expiry-check"><input type="checkbox" checked={enabled} disabled={!editable||busy} onchange={e=>setEnabled(e.currentTarget.checked)}/>Email expiry reminders for this document</label>
  {#if enabled}
   <div class="company-reminder-recipients">
    <p class="muted">Reminders for {label} go only to these addresses, starting two calendar months before expiry.</p>
    {#each recipients as email,i}
     <div class="company-recipient-row"><label>Reminder email {i+1}<input type="email" value={email} maxlength="254" required disabled={!editable||busy} oninput={e=>setRecipients(recipients.map((value,index)=>index===i?e.currentTarget.value:value))}/></label>{#if editable}<button type="button" class="text-button danger" disabled={busy} aria-label={'Remove reminder email '+(i+1)+' for '+label} onclick={()=>setRecipients(recipients.filter((_,index)=>index!==i))}><X size={16}/>Remove</button>{/if}</div>
    {:else}<p class="company-warning">Add at least one email address to send reminders for this document.</p>{/each}
    {#if editable}<button type="button" class="button secondary" disabled={busy||recipients.length>=20} onclick={()=>setRecipients([...recipients,''])}><Plus size={16}/>Add reminder email</button>{/if}
    <small>Up to 20 recipients. Other documents keep their own reminder addresses.</small>
   </div>
  {/if}
 {/if}
</div>
