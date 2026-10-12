<script lang="ts">
 import CompanyAttachments from './CompanyAttachments.svelte';
 import CompanyExpiry from './CompanyExpiry.svelte';
 import type {Row} from './types';
 let {definitions,profile=$bindable(),files=[],recoveryFiles=[],legacyEmails=[],editable=false,busy=false,onchange=()=>{},onupload}:{definitions:Row[];profile:Row;files?:Row[];recoveryFiles?:Row[];legacyEmails?:string[];editable?:boolean;busy?:boolean;onchange?:()=>void;onupload?:(target:Row,file:File)=>Promise<void>}=$props();
 function toggleDoc(f:Row,id:number,checked:boolean){f.documents=checked?[...new Set([...(f.documents||[]),id])]:(f.documents||[]).filter((v:number)=>v!==id);onchange()}
 function markNA(f:Row){if(f.not_applicable){f.value='';f.documents=[];f.expiry=false;f.reminder_enabled=false;f.reminder_emails=[];f.valid_from='';f.expires_on=''}onchange()}
</script>
{#each definitions as def (def.id)}
 {@const f=profile.fields.find((v:Row)=>v.id===def.id)}
 {#if f}
 <fieldset disabled={!editable||busy} class="detail-card" oninput={onchange} onchange={onchange}>
 <div class="detail-card-heading"><h3>{def.label}{#if def.required}<span class="required"> *</span>{/if}</h3><label class="inline-check"><input type="checkbox" bind:checked={f.not_applicable} onchange={()=>markNA(f)}/>Not applicable</label></div>
 {#if f.not_applicable}<label>Reason<input bind:value={f.reason} maxlength="500"/></label>
 {:else}
 {#if def.input_type==='textarea'}<label>{def.label}<textarea rows="3" bind:value={f.value} maxlength="2000"></textarea></label>
 {:else if def.input_type==='select'}<label>{def.label}<select bind:value={f.value}><option value="">Choose an option</option>{#each def.options||[] as option}<option>{option}</option>{/each}</select></label>
 {:else if def.input_type==='checkbox'}<label class="inline-check"><input type="checkbox" checked={f.value==='true'} onchange={e=>{f.value=e.currentTarget.checked?'true':'false';onchange()}}/>{def.label}</label><label>Recorded answer<select bind:value={f.value}><option value="">Not answered</option><option value="true">Yes</option><option value="false">No</option></select></label>
 {:else}<label>{def.label}{def.input_type==='text'?' number / reference':''}<input type={def.input_type==='number'?'text':def.input_type} inputmode={def.input_type==='number'?'decimal':undefined} bind:value={f.value} maxlength="2000"/></label>{/if}
 <CompanyExpiry target={f} {legacyEmails} {editable} {busy} label={def.label} onpatch={patch=>{Object.assign(f,patch);onchange()}}/>
 <CompanyAttachments target={f} {files} {recoveryFiles} {editable} {busy} label={def.label+' supporting copies'+(def.document_required?' (required)':'')} onselect={(id,checked)=>toggleDoc(f,id,checked)} onupload={onupload?(file)=>onupload!(f,file):undefined}/>
 {/if}</fieldset>
 {/if}
{/each}
