<script lang="ts">
 import {onMount} from 'svelte';
 let {paused=false,mode='hero',progress=0}:{paused?:boolean;mode?:'hero'|'layers';progress?:number}=$props();
 let host:HTMLDivElement;
 let ready=$state(false);
 let unavailable=$state(false);
 onMount(()=>{
  let disposed=false,cleanup=()=>{};
  const load=()=>import('./garmentScene').then(({createGarmentScene})=>{
   if(disposed)return;
   try{cleanup=createGarmentScene(host,()=>paused,()=>{ready=true;unavailable=false},()=>{ready=false;unavailable=true},{mode,progress:()=>progress});}catch{unavailable=true;}
  }).catch(()=>{unavailable=true;});
  const observer=new IntersectionObserver(entries=>{if(entries.some(e=>e.isIntersecting)){observer.disconnect();load();}},{rootMargin:'300px'});observer.observe(host);
  return()=>{disposed=true;observer.disconnect();cleanup();};
 });
</script>
<div class="scene" bind:this={host} aria-hidden="true">
 {#if !ready}<div class="fallback"><svg viewBox="0 0 360 420" fill="none"><path d="M135 55L70 83 20 262 74 278 106 174 92 375 177 390 180 290 183 390 268 375 254 174 286 278 340 262 290 83 225 55 180 128Z" fill="#294a68" stroke="#7eabae"/><path d="M135 55L145 154 180 235 215 154 225 55M145 154L120 181 180 270 240 181 215 154M180 128V235" stroke="#8ac7bf"/><path d="M127 277H162M206 277H243" stroke="#8ac7bf"/></svg><span>{unavailable?'GARMENT / DESIGN STUDY':'BUILDING THE CONNECTION'}</span></div>{/if}
</div>
<style>.scene{position:absolute;inset:0;pointer-events:none}.scene :global(canvas){display:block;width:100%;height:100%;transition:opacity .8s}.fallback{position:absolute;inset:15% 15%;display:grid;place-items:center}.fallback svg{height:85%;width:100%;max-width:360px}.fallback span{font:10px monospace;letter-spacing:.18em;color:#687079}</style>
