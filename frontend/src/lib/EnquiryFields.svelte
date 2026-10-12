<script lang="ts">
 import type {EnquiryField} from './enquiryFields';
 import type {Row} from './types';
 import {Plus,X} from '@lucide/svelte';
 let {fields,values=$bindable(),options,busy=false,onadd}:{fields:EnquiryField[];values:Row;options:(key:string)=>string[];busy?:boolean;onadd:(key:string,label:string)=>Promise<void>}=$props();
 let adding=$state(''),newValue=$state(''),error=$state(''),saving=$state(false);
 const optionKind=(key:string)=>key==='garment_currency'?'currency':key;
 async function add(key:string){if(saving||busy)return;saving=true;error='';try{await onadd(optionKind(key),newValue);values[key]=optionKind(key)==='currency'?newValue.trim().toUpperCase():newValue.trim().replace(/\s+/g,' ');adding='';newValue=''}catch(e){error=(e as Error).message}finally{saving=false}}
</script>
<div class="enquiry-grid">
 {#each fields as f}
 <div class:wide={f.type==='textarea'} class="enquiry-field">
  <label for={'enquiry-'+f.key}>{f.label}{#if f.required}<span class="required"> *</span>{/if}</label>
  {#if f.type==='option'||f.type==='select'}
   <div class="field-control"><select id={'enquiry-'+f.key} bind:value={values[f.key]} required={f.required} disabled={busy||saving}><option value="">Select…</option>{#each [...new Set([...(f.options||options(optionKind(f.key))),...(values[f.key]?[String(values[f.key])]:[])])] as choice}<option>{choice}</option>{/each}</select>{#if f.type==='option'}<button type="button" class="button secondary add-option" aria-label={'Add '+f.label.toLowerCase()} disabled={busy||saving} onclick={()=>{adding=adding===f.key?'':f.key;newValue='';error=''}}><Plus size={17}/></button>{/if}</div>
  {:else if f.type==='textarea'}<textarea id={'enquiry-'+f.key} bind:value={values[f.key]} rows="3" maxlength="2000" disabled={busy}></textarea>
  {:else if f.type==='number'}<input id={'enquiry-'+f.key} type="number" min="0" max={f.key.endsWith('_percent')?100:9999999999} step={f.key.endsWith('_days')?'1':'0.000001'} bind:value={values[f.key]} required={f.required} disabled={busy}/>
  {:else if f.type==='date'}<input id={'enquiry-'+f.key} type="date" bind:value={values[f.key]} required={f.required} disabled={busy}/>
  {:else}<input id={'enquiry-'+f.key} bind:value={values[f.key]} maxlength="2000" required={f.required} disabled={busy}/>{/if}
  {#if f.hint}<small>{f.hint}</small>{/if}
  {#if adding===f.key}<div class="option-editor"><label for={'new-'+f.key}>New {f.label.toLowerCase()}</label><input id={'new-'+f.key} bind:value={newValue} maxlength="100" disabled={saving}/><div class="actions"><button type="button" class="button primary" disabled={saving||!newValue.trim()} onclick={()=>add(f.key)}>{saving?'Adding…':'Add option'}</button><button type="button" class="button secondary" aria-label="Cancel new option" disabled={saving} onclick={()=>adding=''}><X size={16}/></button></div>{#if error}<p role="alert" class="form-error">{error}</p>{/if}</div>{/if}
 </div>
 {/each}
</div>
<style>
 .enquiry-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:20px 22px}.enquiry-field{min-width:0;display:flex;flex-direction:column;gap:7px}.wide{grid-column:1/-1}.enquiry-field label{font-size:14px;font-weight:700;color:#142e41}.field-control{display:flex;gap:6px}.field-control select{min-width:0;flex:1}input,select,textarea{width:100%;min-height:44px;padding:10px 12px;background:#edf4f8;border:1px solid #b7c9d3;border-radius:8px;color:#142e41;font-family:inherit;font-weight:600;font-size:14px;line-height:1.4;caret-color:#087d76}textarea{resize:vertical}.add-option{flex-shrink:0;padding:10px}.enquiry-field small{color:#47616f;line-height:1.5;font-size:12px}.enquiry-field label .required{display:inline;color:#ae332b}.option-editor{padding:14px;background:#dceee7;border-radius:10px;display:grid;gap:9px}.actions{display:flex;gap:8px}input:focus-visible,select:focus-visible,textarea:focus-visible{outline:3px solid #52a59f;outline-offset:2px}@media(max-width:1150px){.enquiry-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
