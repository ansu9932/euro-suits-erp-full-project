<script lang="ts">
 import {BUSINESS_FLOWS, type BusinessFlow} from './businessFlows';
 let {values=[],disabled=false,onchange}:{values?:BusinessFlow[];disabled?:boolean;onchange:(next:BusinessFlow[])=>void}=$props();
 function toggle(flow:BusinessFlow,checked:boolean){
  onchange(BUSINESS_FLOWS.filter(item=>item===flow?checked:values.includes(item)));
 }
</script>

<fieldset class="business-flow-select" {disabled}>
 <legend>Business flow</legend>
 <div class="flow-options">
  {#each BUSINESS_FLOWS as flow}
   <label class:chosen={values.includes(flow)}>
    <input type="checkbox" checked={values.includes(flow)} onchange={event=>toggle(flow,event.currentTarget.checked)}/>
    <span>{flow}</span>
   </label>
  {/each}
 </div>
 <small>Select all that apply. Leave unticked if not yet specified.</small>
</fieldset>

<style>
 .business-flow-select{grid-column:1/-1;min-width:0;margin:0;padding:0;border:0}
 legend{padding:0;margin-bottom:8px;font-size:14px;font-weight:700;color:#22394b}
 .flow-options{display:flex;flex-wrap:wrap;gap:10px}
 .flow-options label{display:flex;align-items:center;gap:9px;min-height:44px;min-width:125px;padding:10px 14px;margin:0;border:1px solid #c6d2da;border-radius:8px;background:#f1f6fa;color:#172d3c;cursor:pointer}
 .flow-options label.chosen{border-color:#087f7c;background:#e4f3f0}
 .flow-options input{width:18px;height:18px;min-height:0;padding:0;margin:0;flex:0 0 18px;accent-color:#087f7c}
 .flow-options label:focus-within{outline:3px solid #087f7c;outline-offset:3px}
 fieldset:disabled label{cursor:default}
 small{display:block;margin-top:8px;color:#536b7c;font-size:12px;line-height:1.5}
</style>
