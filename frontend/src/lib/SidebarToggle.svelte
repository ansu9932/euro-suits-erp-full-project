<script lang="ts">
 import {onMount} from 'svelte';
 let hidden=$state(false);
 function apply(){document.body.classList.toggle('sidebar-collapsed',hidden)}
 function toggle(){hidden=!hidden;apply();try{localStorage.setItem('erp-sidebar-hidden',String(hidden))}catch{}}
 onMount(()=>{const tablet=window.matchMedia('(min-width:768px) and (max-width:1100px)').matches;try{const saved=localStorage.getItem('erp-sidebar-hidden');hidden=saved===null?tablet:saved==='true'}catch{hidden=tablet}apply();return()=>document.body.classList.remove('sidebar-collapsed')});
</script>
<button type="button" class="sidebar-toggle" onclick={toggle} aria-label={hidden?'Show sidebar':'Hide sidebar'} aria-expanded={!hidden} title={hidden?'Show sidebar':'Hide sidebar'}><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/></svg><span>{hidden?'Show menu':'Hide menu'}</span></button>
<style>
 .sidebar-toggle{display:inline-flex;align-items:center;gap:7px;flex-shrink:0;border:1px solid #c6d2da;border-radius:6px;background:#f1f6fa;color:#172d3c;padding:8px 10px;font-size:12px;font-weight:700;cursor:pointer}.sidebar-toggle:focus-visible{outline:3px solid #087f7c;outline-offset:2px}
 :global(body.sidebar-collapsed .sidebar),:global(body.sidebar-collapsed .company-sidebar),:global(body.sidebar-collapsed .dev-sidebar){display:none!important}
 :global(body.sidebar-collapsed .main-shell),:global(body.sidebar-collapsed .company-main),:global(body.sidebar-collapsed .dev-main){margin-left:0!important;width:100%!important;flex:1}
 @media(max-width:700px){.sidebar-toggle span{display:none}}
</style>
