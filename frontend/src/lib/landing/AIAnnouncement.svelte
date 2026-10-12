<script lang="ts">
 import {onMount} from 'svelte';
 import {RotateCcw, Construction} from '@lucide/svelte';
 import {constructionAnnouncement} from '$lib/motion';

 let {paused = false}: {paused?: boolean} = $props();
 let replay = $state(0), reduced = $state(false);
 onMount(() => {
  const preference = matchMedia('(prefers-reduced-motion: reduce)');
  const changed = () => { reduced = preference.matches; };
  changed();
  preference.addEventListener('change', changed);
  return () => preference.removeEventListener('change', changed);
 });
</script>

<aside class="ai-announcement" aria-label="Spooly AI announcement">
 <div class="ai-announcement-inner">
  <div class="ai-construction" use:constructionAnnouncement={{paused, replay}} aria-hidden="true">
   <svg viewBox="0 0 144 88" width="144" height="88" fill="none">
    <path d="M8 76H136" stroke="#416779" stroke-width="1.5" stroke-linecap="round"/>
    <path d="M19 70C6 70 6 45 23 45H66C80 45 70 70 86 70H131" stroke="#74d3c2" stroke-opacity=".22" stroke-width="2"/>
    <path data-build-thread d="M19 70C6 70 6 45 23 45H66C80 45 70 70 86 70H131" stroke="#74d3c2" stroke-width="2" stroke-dasharray="160"/>
    <g data-build-block><rect x="92" y="59" width="18" height="16" rx="3" fill="#217f7b"/><path d="M96 63H106" stroke="#96e1d3" stroke-linecap="round"/></g>
    <g data-build-block><rect x="114" y="59" width="18" height="16" rx="3" fill="#217f7b"/><path d="M118 63H128" stroke="#96e1d3" stroke-linecap="round"/></g>
    <g data-build-block><rect x="103" y="39" width="18" height="16" rx="3" fill="#74d3c2"/><path d="M107 43H117" stroke="#e5fff8" stroke-linecap="round"/></g>
    <g data-build-spool>
     <path d="M28 67L24 74M58 67L63 74" stroke="#97c4cd" stroke-width="4" stroke-linecap="round"/>
     <rect x="25" y="29" width="37" height="37" rx="10" fill="#a4e4d8"/>
     <path d="M27 35H59M27 42H59M27 49H59M27 56H59" stroke="#228a83" stroke-opacity=".35" stroke-width="2"/>
     <rect x="21" y="61" width="45" height="8" rx="4" fill="#e3f5ed"/>
     <rect x="28" y="37" width="31" height="21" rx="8" fill="#102b3b"/>
     <g data-build-eyes><circle cx="37" cy="45" r="2.5" fill="#e5fff8"/><circle cx="50" cy="45" r="2.5" fill="#e5fff8"/></g>
     <path d="M40 51Q44 54 48 51" stroke="#74d3c2" stroke-width="1.5" stroke-linecap="round"/>
     <path d="M24 29C24 5 63 5 63 29" fill="#f2bf63"/>
     <path d="M39 11V23M49 11V23" stroke="#a36c25" stroke-width="2" stroke-linecap="round"/>
     <rect x="19" y="25" width="49" height="7" rx="3.5" fill="#ffe0a1"/>
     <path d="M62 51L75 44" stroke="#a4e4d8" stroke-width="4" stroke-linecap="round"/>
    </g>
    <g data-build-hammer><path d="M76 48L85 26" stroke="#f2bf63" stroke-width="4" stroke-linecap="round"/><path d="M78 23L92 28" stroke="#c9e5e8" stroke-width="7" stroke-linecap="round"/></g>
   </svg>
  </div>
  <div class="ai-copy">
   <strong>Spooly AI is coming soon<span class="ai-period">.</span></strong>
   <span>Your garment assistant is under construction.</span>
  </div>
  <span class="ai-build-status"><Construction size={17} strokeWidth={1.6}/> Work in progress</span>
  <button class="ai-replay" type="button" onclick={() => replay++} disabled={paused || reduced}
   aria-label="Replay Spooly construction animation" title={paused || reduced ? 'Animation is paused by your motion preference' : 'Replay construction animation'}>
   <RotateCcw size={16} strokeWidth={1.7}/><span>Replay</span>
  </button>
 </div>
</aside>

<style>
 .ai-announcement{background:#0c2233;color:#f2faf8;border-bottom:1px solid #315667;position:relative}
 .ai-announcement-inner{max-width:1440px;margin:auto;padding:10px clamp(24px,4vw,72px);display:flex;align-items:center;gap:24px;min-height:108px}
 .ai-construction{width:144px;height:88px;flex:none;overflow:hidden}
 .ai-construction svg{display:block;width:100%;height:100%}
 .ai-construction [data-build-eyes]{transform-box:fill-box;transform-origin:center}
 .ai-construction [data-build-hammer]{transform-origin:76px 48px}
 .ai-copy{display:flex;flex-direction:column;gap:4px;flex:1;min-width:0}
 .ai-copy strong{font-size:clamp(18px,1.65vw,23px);font-weight:700;letter-spacing:-.025em;line-height:1.3;text-wrap:balance}
 .ai-copy>span{font-size:13px;line-height:1.5;color:#bbd5de}
 .ai-period{color:#74d3c2}
 .ai-build-status{display:inline-flex;align-items:center;gap:9px;color:#f4d496;font-size:12px;white-space:nowrap}
 .ai-announcement .ai-replay{display:flex;align-items:center;justify-content:center;gap:8px;min-height:44px;min-width:44px;padding:10px 12px;background:transparent;color:#c8e5e9;border:1px solid #507583;border-radius:12px;font-size:12px;transition:background .15s,color .15s}
 .ai-announcement .ai-replay:hover:not(:disabled){background:#1e4353;color:white}
 .ai-announcement .ai-replay:focus-visible{outline:3px solid #74d3c2;outline-offset:4px}
 .ai-announcement .ai-replay:disabled{color:#9bb9c5;border-color:#355869;cursor:default}
 .ai-announcement ::selection{background:#74d3c2;color:#0c2233}
 @media(max-width:1000px){.ai-announcement-inner{gap:18px}.ai-build-status{display:none}}
 @media(max-width:600px){.ai-announcement-inner{padding:12px 20px;gap:12px;min-height:106px}.ai-construction{width:82px;height:64px}.ai-copy strong{font-size:17px}.ai-copy>span{font-size:11px;line-height:1.5}.ai-replay span{display:none}.ai-announcement .ai-replay{padding:8px}.ai-copy{max-width:300px}}
 @media(max-width:360px){.ai-announcement-inner{padding-inline:12px;gap:8px}.ai-construction{width:64px}.ai-copy strong{font-size:16px}}
</style>
