<script lang="ts">
 import {onMount} from 'svelte';
 let {paused=false,mode='hero',progress=0}:{paused?:boolean;mode?:'hero'|'layers';progress?:number}=$props();
 let host:HTMLDivElement;
 let ready=$state(false);
 let unavailable=$state(false);
 let spun=$state(false);
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
<div class="scene" bind:this={host} aria-hidden="true" onpointerdown={()=>spun=true} onwheel={()=>spun=true}>
 {#if !ready}<div class="fallback"><svg viewBox="0 0 360 420" fill="none"><path d="M135 55L70 83 20 262 74 278 106 174 92 375 177 390 180 290 183 390 268 375 254 174 286 278 340 262 290 83 225 55 180 128Z" fill="#294a68" stroke="#7eabae"/><path d="M135 55L145 154 180 235 215 154 225 55M145 154L120 181 180 270 240 181 215 154M180 128V235" stroke="#8ac7bf"/><path d="M127 277H162M206 277H243" stroke="#8ac7bf"/></svg><span>{unavailable?'GARMENT / DESIGN STUDY':'BUILDING THE CONNECTION'}</span></div>{/if}
 {#if ready&&!spun}<div class="hint"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M4 12a8 8 0 0 1 13.7-5.6M20 12a8 8 0 0 1-13.7 5.6"/><path d="M17.7 3.2v3.4h-3.4M6.3 20.8v-3.4h3.4"/></svg>DRAG TO TURN</div>{/if}
</div>
<style>
 .scene{position:absolute;inset:0;pointer-events:none}
 .scene :global(canvas){display:block;width:100%;height:100%;pointer-events:auto;cursor:grab;transition:opacity .8s;touch-action:pan-y}
 .scene :global(canvas.grabbing){cursor:grabbing}
 .fallback{position:absolute;inset:15% 15%;display:grid;place-items:center}
 .fallback svg{height:85%;width:100%;max-width:360px}
 .fallback span{font:10px monospace;letter-spacing:.18em;color:#687079}
 .hint{position:absolute;bottom:18px;left:50%;transform:translateX(-50%);display:flex;align-items:center;gap:8px;
  font:9px monospace;letter-spacing:.16em;color:#3d5b62;background:#f4faf7d9;border:1px solid #b9d3cd;
  border-radius:20px;padding:6px 12px;white-space:nowrap;animation:hint-breathe 2.6s ease-in-out infinite}
 .hint svg{width:13px;height:13px;animation:hint-spin 3.4s linear infinite}
 @keyframes hint-breathe{0%,100%{opacity:.7}50%{opacity:1}}
 @keyframes hint-spin{to{transform:rotate(360deg)}}
 @media(max-width:700px){.hint{font-size:8px;padding:5px 10px;bottom:10px}}
</style>
