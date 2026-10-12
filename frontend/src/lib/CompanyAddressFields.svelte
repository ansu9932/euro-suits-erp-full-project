<script lang="ts">
 import {onDestroy} from 'svelte';
 import {enter} from './motion';
 import {indianPIN,lookupPostalAddress,postalSuggestions,type AddressPatch,type PostalLocation} from './postalAddress';
 let {country,city='',state:province='',postalCode='',address='',disabled=false,onchange}:{country:string;city?:string;state?:string;postalCode?:string;address?:string;disabled?:boolean;onchange:(patch:AddressPatch)=>void}=$props();
 const uid=$props.id();
 let status=$state<'idle'|'loading'|'found'|'empty'|'error'>('idle');
 let locations:PostalLocation[]=$state([]);
 let attempted=$state(false);
 const india=$derived(country.toUpperCase()==='IN');
 const suggestions=$derived(postalSuggestions(locations));
 const canApply=$derived(status==='found'&&!!((suggestions.city&&suggestions.city!==city)||(suggestions.state&&suggestions.state!==province)));
 let timer:ReturnType<typeof setTimeout>|undefined;
 let controller:AbortController|undefined;
 let generation=0;
 let previousCountry:string|undefined;
 let autoCity:string|null=null,autoState:string|null=null;
 let cityEdits=0,stateEdits=0;
 function cancel(){clearTimeout(timer);controller?.abort();controller=undefined;generation++;}
 function clearAuto(){
  const patch:AddressPatch={};
  if(autoCity!==null&&city===autoCity)patch.city='';
  if(autoState!==null&&province===autoState)patch.state='';
  autoCity=null;autoState=null;
  if(Object.keys(patch).length)onchange(patch);
 }
 $effect(()=>{
  const nextCountry=country;const locked=disabled;
  if(previousCountry!==undefined&&nextCountry!==previousCountry){cancel();clearAuto();locations=[];status='idle';attempted=false;}
  previousCountry=nextCountry;
  if(locked){cancel();status='idle';}
 });
 onDestroy(cancel);
 function editPIN(value:string){
  if(disabled)return;
  cancel();clearAuto();locations=[];status='idle';attempted=true;
  onchange({postalCode:value});
  if(!india||!indianPIN(value))return;
  const pin=value.trim(),id=generation,edits={city:cityEdits,state:stateEdits};
  timer=setTimeout(()=>lookup(pin,id,edits),350);
 }
 async function lookup(pin:string,id:number,edits:{city:number;state:number}){
  if(disabled||id!==generation||!india)return;
  status='loading';const request=new AbortController();controller=request;
  const timeout=setTimeout(()=>request.abort(),6500);
  try{
   const found=await lookupPostalAddress(pin,request.signal);
   if(id!==generation||disabled||!india||postalCode.trim()!==pin)return;
   locations=found;status=found.length?'found':'empty';
   const result=postalSuggestions(found),patch:AddressPatch={};
   if(result.city&&cityEdits===edits.city&&(!city||city===autoCity)){patch.city=result.city;autoCity=result.city;}
   if(result.state&&stateEdits===edits.state&&(!province||province===autoState)){patch.state=result.state;autoState=result.state;}
   if(Object.keys(patch).length)onchange(patch);
  }catch{
   if(id===generation&&!disabled&&india){locations=[];status='error';}
  }finally{clearTimeout(timeout);if(controller===request)controller=undefined;}
 }
 function useSuggestions(){
  if(disabled||status!=='found')return;
  const patch:AddressPatch={};
  if(suggestions.city){patch.city=suggestions.city;autoCity=suggestions.city;}
  if(suggestions.state){patch.state=suggestions.state;autoState=suggestions.state;}
  onchange(patch);
 }
</script>

<fieldset class="company-address-fields" {disabled}>
 <legend>Address details</legend>
 <div class="address-grid">
  <label>{india?'PIN code':'Postal code'}<input value={postalCode} oninput={event=>editPIN(event.currentTarget.value)} maxlength="30" inputmode={india?'numeric':'text'} autocomplete="postal-code" aria-describedby={uid+'-postal-help'}/></label>
  <label>State / province<input value={province} oninput={event=>{stateEdits++;autoState=null;onchange({state:event.currentTarget.value})}} maxlength="100" autocomplete="address-level1" list={uid+'-states'}/></label>
  <label class="address-wide">City / town<input value={city} oninput={event=>{cityEdits++;autoCity=null;onchange({city:event.currentTarget.value})}} maxlength="100" autocomplete="address-level2" list={uid+'-cities'}/></label>
  <div class="postal-help address-wide" id={uid+'-postal-help'}>
   <p role="status" aria-live="polite" aria-atomic="true">
    {#if !india}Enter the postal code, city and state manually for this country.
    {:else if status==='loading'}Finding city and state… You can keep typing.
    {:else if status==='error'}PIN lookup is unavailable. Enter the city and state yourself, or retry.
    {:else if status==='empty'}No match for this PIN. Enter the city and state yourself.
    {:else if status==='found'&&suggestions.cities.length>1}This PIN covers several areas. Choose a city suggestion or type your own. You can also edit the state.
    {:else if status==='found'&&canApply}PIN suggestions are ready. Your entered details have been kept.
    {:else if status==='found'}City/area and state suggested from the PIN. Check or edit either field.
    {:else if attempted&&postalCode&&!indianPIN(postalCode)}Use a six-digit PIN for lookup, or enter the city and state yourself.
    {:else}Enter a six-digit PIN to suggest city and state. Both fields stay editable.{/if}
   </p>
   {#if canApply}<button type="button" class="text-button" onclick={useSuggestions} use:enter>Use PIN suggestions</button>{/if}
   {#if status==='error'&&india&&indianPIN(postalCode)}<button type="button" class="text-button" onclick={()=>editPIN(postalCode)}>Retry PIN lookup</button>{/if}
  </div>
  <label class="address-wide">Registered address<textarea value={address} oninput={event=>onchange({address:event.currentTarget.value})} rows="3" maxlength="2000" autocomplete="street-address" placeholder="Building, street and locality"></textarea></label>
 </div>
 <datalist id={uid+'-cities'}>{#each suggestions.cities as value}<option {value}></option>{/each}</datalist>
 <datalist id={uid+'-states'}>{#each suggestions.states as value}<option {value}></option>{/each}</datalist>
</fieldset>

<style>
 .company-address-fields{grid-column:1/-1;min-width:0;margin:4px 0 0;padding:20px 0 0;border:0;border-top:1px solid #dce5eb}
 legend{float:left;width:100%;padding:0;margin-bottom:14px;font-size:15px;font-weight:700;color:#22394b}
 .address-grid{clear:both;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px 20px}
 .address-wide{grid-column:1/-1}
 .postal-help{min-height:42px;margin-top:-6px;color:#496174}
 .postal-help p{margin:0;font-size:13px;line-height:1.55;overflow-wrap:anywhere}
 .postal-help .text-button{margin-top:4px;padding:4px 0;color:#076e68;text-decoration:underline;text-underline-offset:3px}
 @media(max-width:760px){.address-grid{grid-template-columns:1fr}}
</style>
