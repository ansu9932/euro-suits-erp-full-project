<script lang="ts">
 import { onMount } from 'svelte';
 import { Eye, EyeOff } from '@lucide/svelte';
 import { api } from './api';
 import { loadTurnstile, type Turnstile } from './turnstile';
 let { username = $bindable(''), password = $bindable(''), botToken = $bindable(''), verificationReady = $bindable(false), resetKey = 0 }:
  {username?: string; password?: string; botToken?: string; verificationReady?: boolean; resetKey?: number} = $props();
 let visible = $state(false), message = $state(''), checking = $state(true);
 let host: HTMLDivElement, widget: string | undefined, provider: Turnstile | undefined;
 $effect(() => { void resetKey; if (provider && widget) { botToken=''; verificationReady=false; provider.reset(widget); } });
 onMount(() => {
  let active=true;
  (async () => {
   try {
    const config=await api('/login-config');
    if (!active) return;
    if (!config.turnstile_site_key) { verificationReady=true; return; }
    provider=await loadTurnstile(); if (!active) return;
    widget=provider.render(host, {sitekey:config.turnstile_site_key, action:'erp-login', theme:'light',
     callback: token => {botToken=token;verificationReady=true;message='';},
     'expired-callback': () => {botToken='';verificationReady=false;},
     'error-callback': () => {botToken='';verificationReady=false;message='Verification failed. Reload this page to retry.';}
    });
   } catch (e) { if(active) message=(e as Error).message; }
   finally { if(active) checking=false; }
  })();
  return () => {active=false;if(widget)provider?.remove(widget);botToken='';};
 });
</script>
<label>Username<input bind:value={username} autocomplete="username" autocapitalize="none" spellcheck="false" maxlength="80" required placeholder="Your username"/></label>
<label>Password<span class="password-control"><input type={visible?'text':'password'} bind:value={password} aria-label="Password" autocomplete="current-password" maxlength="128" required placeholder="Your password"/><button type="button" class="password-toggle" aria-label={visible?'Hide password':'Show password'} aria-pressed={visible} onclick={()=>visible=!visible}>{#if visible}<EyeOff size={18}/>{:else}<Eye size={18}/>{/if}</button></span></label>
<div bind:this={host} class="login-verification"></div>
{#if checking}<p class="verification-status" role="status">Preparing sign-in…</p>{/if}
{#if message}<p class="form-error" role="alert">{message}</p>{/if}
