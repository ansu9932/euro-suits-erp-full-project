<script lang="ts">
 import {onMount} from 'svelte';
 import {ArrowLeft, LockKeyhole, LogOut} from '@lucide/svelte';
 import {api} from '$lib/api';
 import {enter} from '$lib/motion';
 import type {User, Row} from '$lib/types';
 import ExportWorkspace from '$lib/ExportWorkspace.svelte';
 import SignInForm from '$lib/SignInForm.svelte';
 import Spooly from '$lib/Spooly.svelte';
 import {workspaceRedirect} from '$lib/workspaceAccess';
 import '../../app.css';
 import '../../company.css';
 import '../../dashboard.css';

 type SessionUser = User & {portal:string; renewal_blocks?:Row[]; setup_grants?:string[]};
 let user:SessionUser|null=$state(null);
 let ready=$state(false), busy=$state(false), error=$state(''), exportDirty=$state(false);
 let hasWorkspaceHome=$derived.by(()=>!!user&&workspaceRedirect(user,user.portal,'companies')!=='/exports');
 let generation=0;
 onMount(()=>{
  let active=true;
  const ticket=++generation;
  const expired=()=>{generation++;user=null;exportDirty=false;error='Your session changed. Please sign in again.';};
  window.addEventListener('erp:session-expired',expired);
  api('/session').then(session=>{if(active&&ticket===generation)user=session.user;})
   .catch(e=>{if(active)error=e.message;}).finally(()=>{if(active)ready=true;});
  return()=>{active=false;generation++;window.removeEventListener('erp:session-expired',expired);};
 });
 function signedIn(result:Row){generation++;user=result.user;exportDirty=false;error='';}
 async function logout(){
  if(busy||(exportDirty&&!window.confirm('Discard unsaved export changes and sign out?')))return;
  busy=true;error='';
  try{await api('/logout',{});generation++;exportDirty=false;user=null;}
  catch(e){error=(e as Error).message;}finally{busy=false;}
 }
</script>

<svelte:head><title>Export Documentation · Euro Suits ERP</title><meta name="robots" content="noindex"/></svelte:head>
{#if !ready}
 <main class="loading-screen"><img src="/logo.png" alt="Euro Suits"/><p>Opening Export Documentation…</p></main>
{:else if !user}
 <main class="login-layout">
  <section class="login-brand"><div class="login-brand-name"><img src="/logo.png" alt="Euro Suits logo"/><span>EURO SUITS</span></div><div><h1>Every shipment.<br/>Every receipt.<br/><span>One record.</span></h1><p>Export documentation and outstanding balances.</p></div><small>EURO SUITS OPERATIONS</small></section>
  <section class="login-panel"><div class="login-form-wrap" use:enter><span class="login-symbol"><LockKeyhole size={24}/></span><h1>Export Documentation</h1><p class="muted">Sign in with your assigned ERP account.</p><SignInForm portal="" onauthenticated={signedIn}/>{#if error}<p class="form-error" role="alert">{error}</p>{/if}</div></section>
 </main>
{:else}
 <div class="export-shell erp-workspace">
  <a class="workspace-skip" href="#export-content">Skip to export workspace</a>
  <header class="export-topbar">{#if hasWorkspaceHome}<a class="back-link" href={'/'+user.portal}><ArrowLeft size={17}/><span>Back to workspace</span></a>{/if}<div class="export-account"><span><strong>{user.name}</strong><small>{user.role}</small></span><button class="button secondary" disabled={busy} onclick={logout}><LogOut size={16}/>Sign out</button></div></header>
  <main id="export-content" tabindex="-1">
   {#if error}<p class="form-error" role="alert">{error}</p>{/if}
   {#if user.renewal_blocks?.length}
    <section class="panel access-message"><h1>Renewal action required</h1><p>Your workspace is locked because a required document is at least 30 days overdue. Finance must renew it and obtain approval before export access resumes.</p>{#each user.renewal_blocks as block}<p><strong>{block.legal_name}</strong> · {block.label} · {block.expires_on}</p>{/each}<a class="button primary" href={'/'+user.portal}>Open renewal workspace</a></section>
   {:else if !user.grants.includes('export_view')}
    <section class="panel access-message"><h1>Export access is not assigned</h1><p>Ask Super Admin to enable Export Documentation for your account.</p><a class="button primary" href={'/'+user.portal}>Return to workspace</a></section>
   {:else}
    {#key user.id}<ExportWorkspace {user} bind:dirty={exportDirty}/>{/key}
   {/if}
  </main>
  {#if !user.renewal_blocks?.length}<Spooly userId={user.id} role={user.role} portal={user.portal} page="Warehouse & dispatch"/>{/if}
 </div>
{/if}

<style>
 .export-shell{min-height:100dvh;background:var(--bg,#f3f6fa)}
 .export-topbar{min-height:72px;padding:12px 32px;background:#fff;border-bottom:1px solid var(--border,#dce4ed);display:flex;align-items:center;justify-content:space-between;gap:20px}
 .back-link{display:inline-flex;align-items:center;gap:8px;color:#18334e;text-decoration:none;font-weight:700;min-height:40px}
 .back-link:hover{text-decoration:underline;text-underline-offset:4px}
 .export-account{display:flex;align-items:center;gap:20px}
 .export-account>span{display:flex;flex-direction:column;gap:3px;font-size:13px}
 .export-account small{color:#526378}
 #export-content{max-width:1720px;margin:auto;padding:28px 32px 80px;outline:none}
 .access-message{padding:28px;max-width:760px;margin:40px auto}
 .access-message p{line-height:1.65;margin:16px 0}
 @media(min-width:1024px){.export-account{margin-left:auto}}
 @media(max-width:900px){.export-topbar{padding:12px 20px}#export-content{padding:20px 20px 72px}.export-account{gap:12px}}
 @media(max-width:560px){.export-topbar{align-items:flex-start;flex-wrap:wrap;padding:12px 16px}#export-content{padding:16px 12px 72px}.export-account{margin-left:auto}.export-account>span{display:none}}
</style>
