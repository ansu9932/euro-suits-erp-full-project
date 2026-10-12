<script lang="ts">
 import {enter} from "./motion";
 import {onMount} from 'svelte';
 import {X,LoaderCircle} from '@lucide/svelte';
 import type {FormSpec,Row} from './types';
 let {spec,onclose}:{spec:FormSpec;onclose:()=>void}=$props();
 let dialog:HTMLDialogElement;let values:Row=$state({});let error=$state('');let busy=$state(false);
 onMount(()=>{values=Object.fromEntries(spec.fields.map(f=>[f.key,f.value??(f.type==='multiselect'?[]:'')]));dialog.showModal();const motion=enter(dialog);return()=>{motion.destroy();dialog.close();};});
 const permissionNames:Record<string,string>={orders:'Create buyer orders',approve:'Approve / reject orders & purchases',plan:'Allocate lines, materials & milestones',develop:'Manage style and CAD work',sample:'Manage samples',cut:'Record cutting output',sew:'Record sewing output',finish:'Record finishing output',pack:'Record packing output',quality:'Record quality inspections',warehouse:'Confirm dispatch',finance:'Record invoice references',suppliers:'Create suppliers',inventory:'Receive & issue stock',purchases:'Create supplier purchases',companies:'Submit company setup',users:'Manage user access & masters',audit:'View audit trail',documents:'Upload order documents'};
 function visible(key:string){return key==='teams'?values.scope==='team':key==='lines'?values.scope==='line':key==='orders'?values.scope==='po':key==='finishing_units'?values.role==='Finishing':true;}
 function fieldOptions(f:FormSpec['fields'][number]){return f.key==='permissions'&&spec.permissionsByRole?(spec.permissionsByRole[values.role]||[]).map(v=>({value:v,label:permissionNames[v]||v})):f.options||[];}
 function toggle(key:string,v:string|number,checked:boolean){const list=values[key]||[];values[key]=checked?[...list,v]:list.filter((x:unknown)=>x!==v);}
 function roleChanged(){if(spec.permissionsByRole)values.permissions=[...(spec.permissionsByRole[values.role]||[])];}
 async function submit(e:SubmitEvent){e.preventDefault();if(busy)return;error='';for(const f of spec.fields){if(f.type==='multiselect'&&visible(f.key)&&f.required!==false&&!values[f.key]?.length){error='Select at least one '+f.label.toLowerCase()+'.';return;}}busy=true;try{await spec.submit(values);onclose();}catch(e){error=(e as Error).message;}finally{busy=false;}}
</script>
<dialog bind:this={dialog} class="form-dialog" oncancel={(e)=>{if(busy)e.preventDefault();else onclose();}}>
 <div class="dialog-header"><div><h2>{spec.title}</h2></div><button type="button" class="icon-button" aria-label="Close form" onclick={onclose} disabled={busy}><X size={20}/></button></div>
 {#if spec.description}<p class="dialog-description">{spec.description}</p>{/if}
 <form onsubmit={submit}>
  <div class="form-grid">
  {#each spec.fields as f}
   {#if visible(f.key)}
   {#if f.type==='multiselect'}
   <fieldset class="checkbox-field wide"><legend>{f.label}{#if f.required!==false}<b class="required"> *</b>{/if}</legend><div class="checkbox-actions"><span>{values[f.key]?.length||0} selected</span><button class="text-button" type="button" onclick={()=>values[f.key]=fieldOptions(f).map(o=>o.value)}>Select all</button><button class="text-button" type="button" onclick={()=>values[f.key]=[]}>Clear</button></div><div class="checkbox-grid">{#each fieldOptions(f) as o}<label class="checkbox-option"><input type="checkbox" checked={(values[f.key]||[]).includes(o.value)} onchange={(e)=>toggle(f.key,o.value,e.currentTarget.checked)}/><span>{o.label}</span></label>{:else}<p class="muted">Choose a department role to see its available actions.</p>{/each}</div>{#if f.hint}<small>{f.hint}</small>{/if}</fieldset>
   {:else}
   <label class:wide={f.type==='textarea'}><span>{f.label}{#if f.required!==false}<b class="required"> *</b>{/if}</span>
   {#if f.type==='select'}<select bind:value={values[f.key]} onchange={()=>{if(f.key==='role')roleChanged();}} required={f.required!==false}><option value="">Select {f.label.toLowerCase()}</option>{#each f.options||[] as o}<option value={o.value}>{o.label}</option>{/each}</select>
   {:else if f.type==='textarea'}<textarea bind:value={values[f.key]} rows="3" maxlength="2000" required={f.required!==false}></textarea>
   {:else}<input type={f.type||'text'} bind:value={values[f.key]} required={f.required!==false} min={f.min} max={f.max} step={f.step} maxlength={f.type==='password'?128:160} autocomplete={f.type==='password'?'new-password':'off'}/>{/if}
   {#if f.hint}<small>{f.hint}</small>{/if}</label>{/if}{/if}
  {/each}
  </div>
  {#if error}<p class="form-error" role="alert">{error}</p>{/if}
  <footer class="dialog-footer"><button type="button" class="button secondary" onclick={onclose} disabled={busy}>Cancel</button><button class="button primary" disabled={busy}>{#if busy}<LoaderCircle size={16} class="spin"/>{/if}{busy?'Saving…':spec.submitLabel||'Save record'}</button></footer>
 </form>
</dialog>
