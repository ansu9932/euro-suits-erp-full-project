<script lang="ts">
 import {onMount} from 'svelte';
 import {mascotGesture} from './motion';
 import {registerMascot, resolveMascotContext} from './mascot/garment-erp-mascot.js';

 let {userId, role='', portal='', page=''}:{userId:number|string;role?:string;portal?:string;page?:string}=$props();
 let mascot=$state<HTMLElement|null>(null);
 const module=$derived(resolveMascotContext(page,role,portal));

 // Only visual context and a local preference key cross this boundary.
 // The element never receives ERP records, permissions, credentials or API access.
 $effect(()=>{
  if(!mascot)return;
  mascot.setAttribute('user-key',String(userId));
  mascot.setAttribute('user-role',role);
  mascot.setAttribute('dashboard-module',module);
 });

 onMount(()=>{
  registerMascot({gesture:mascotGesture});
  const element=document.createElement('garment-erp-mascot');
  element.setAttribute('user-key',String(userId));
  element.setAttribute('user-role',role);
  element.setAttribute('dashboard-module',module);
  // Body placement avoids transforms or overflow in any role's workspace.
  document.body.append(element);
  mascot=element;
  let actionbar:Element|null=null;
  let positionFrame=0;
  const position=()=>{
   positionFrame=0;
   const rect=actionbar?.getBoundingClientRect();
   const bottom=rect&&rect.top<window.innerHeight&&rect.bottom>0
    ?Math.max(20,window.innerHeight-rect.top+12):20;
   element.style.bottom=`${bottom}px`;
  };
  const schedulePosition=()=>{if(!positionFrame)positionFrame=requestAnimationFrame(position)};
  const barObserver=new ResizeObserver(schedulePosition);
  const suspend=()=>{
   const nextBar=document.querySelector('.company-actionbar, .ex-task-footer');
   if(nextBar!==actionbar){
    barObserver.disconnect();
    actionbar=nextBar;
    if(actionbar)barObserver.observe(actionbar);
   }
   schedulePosition();
   const editing=document.activeElement?.matches('input,textarea,select,[contenteditable="true"]')
    ||Array.from(document.querySelectorAll('.ex-editor')).some(node=>node.getClientRects().length>0);
   const dialog=Array.from(document.querySelectorAll(
    'dialog[open],[aria-modal="true"],.drawer-backdrop,.company-modal-backdrop,.sample-overlay'
   )).some(node=>node.getClientRects().length>0);
   element.toggleAttribute('suspended',!!editing||dialog);
  };
  // Observe only the ERP DOM, never the mascot's isolated shadow tree.
  const observer=new MutationObserver(suspend);
  observer.observe(document.body,{subtree:true,childList:true,attributes:true,attributeFilter:['open','aria-modal']});
  const afterFocus=()=>queueMicrotask(()=>{if(element.isConnected)suspend()});
  document.addEventListener('focusin',suspend);
  document.addEventListener('focusout',afterFocus);
  window.addEventListener('resize',schedulePosition);
  document.addEventListener('scroll',schedulePosition,{passive:true,capture:true});
  suspend();
  return()=>{
   observer.disconnect();
   barObserver.disconnect();
   cancelAnimationFrame(positionFrame);
   document.removeEventListener('focusin',suspend);
   document.removeEventListener('focusout',afterFocus);
   window.removeEventListener('resize',schedulePosition);
   document.removeEventListener('scroll',schedulePosition,true);
   element.remove();
  };
 });
</script>
