<script lang="ts">
 import {onMount} from 'svelte';
 import LoginFields from './LoginFields.svelte';
 import {api} from './api';
 import type {Row} from './types';
 let {portal,initialUsername='',onauthenticated}:{portal:string;initialUsername?:string;onauthenticated:(result:Row)=>void|Promise<void>}=$props();
 let username=$state(''),password=$state(''),botToken=$state(''),verificationReady=$state(false),attempts=$state(0);
 let busy=$state(false),error=$state(''),pending=$state(false),emailHint=$state(''),code=$state('');
 let resendAt=0,resendSeconds=$state(0);
 onMount(()=>{username=initialUsername;const timer=setInterval(()=>{resendSeconds=Math.max(0,Math.ceil((resendAt-Date.now())/1000))},1000);return()=>clearInterval(timer)});
 async function submit(e:SubmitEvent){e.preventDefault();if(busy)return;busy=true;error='';try{
  const result=await api(pending?'/login/otp':'/login',pending?{code}:{username,password,portal,bot_token:botToken});
  password='';code='';
  if(result.otp_required){pending=true;emailHint=result.email_hint;resendAt=Date.now()+60000;resendSeconds=60;return;}
  await onauthenticated(result);
 }catch(e){error=(e as Error).message;}finally{busy=false;attempts++;}}
</script>
<form onsubmit={submit}>
 {#if pending}
  <p class="company-notice" role="status">Enter the code sent to {emailHint}. It expires in 5 minutes.</p>
  <label>Email verification code<input bind:value={code} aria-label="Email verification code" inputmode="numeric" autocomplete="one-time-code" pattern={'[0-9]{6}'} minlength="6" maxlength="6" required placeholder="6-digit code"/></label>
 {:else}
  <LoginFields bind:username bind:password bind:botToken bind:verificationReady resetKey={attempts}/>
 {/if}
 {#if error}<p class="form-error" role="alert">{error}</p>{/if}
 <button class="button primary" disabled={busy||(!pending&&!verificationReady)}>{busy?'Please wait…':pending?'Verify & sign in':'Sign in'}</button>
 {#if pending}<button type="button" class="button secondary" disabled={busy||resendSeconds>0} onclick={()=>{if(busy||resendSeconds>0)return;pending=false;code='';error=''}}>{resendSeconds>0?'Request another code in '+resendSeconds+'s':'Use password again / request a new code'}</button>{/if}
</form>
