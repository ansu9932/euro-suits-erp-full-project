<script lang="ts">
 import {onMount} from 'svelte';
 import {api} from './api';
 import type {Row} from './types';
 let data:Row|null=$state(null),error=$state(''),busy=$state(false),previous:Row|null=null;
 let waitDelta=$state(0),cancelDelta=$state(0);
 const num=(v:unknown)=>Number(v||0).toLocaleString('en-IN',{maximumFractionDigits:1});
 async function refresh(){
  if(busy)return;busy=true;
  try{
   const next=await api('/developer/performance');
   waitDelta=previous?Math.max(0,next.database.waited_acquires-previous.waited_acquires):0;
   cancelDelta=previous?Math.max(0,next.database.cancelled_acquires-previous.cancelled_acquires):0;
   previous=next.database;data=next;error='';
  }catch(e){error=(e as Error).message}finally{busy=false}
 }
 onMount(()=>{void refresh();const timer=setInterval(()=>{if(!document.hidden)void refresh()},15000);return()=>clearInterval(timer)});
 function download(){if(!data)return;const url=URL.createObjectURL(new Blob([JSON.stringify(data,null,2)],{type:'application/json'}));const a=document.createElement('a');a.href=url;a.download='euro-suits-performance.json';a.click();URL.revokeObjectURL(url)}
</script>
<div class="perf-toolbar"><p>Live API measurements · last 60 seconds</p><div><button class="button secondary" disabled={busy} onclick={refresh}>{busy?'Refreshing…':'Refresh metrics'}</button><button class="button secondary" disabled={!data} onclick={download}>Download snapshot</button></div></div>
{#if error}<p class="form-error" role="alert">{error} Last successful readings remain below.</p>{/if}
{#if data}
 <div class="perf-cards">
  <article><span>Requests / minute</span><strong>{num(data.traffic.requests)}</strong><small>{num(data.traffic.requests_per_second)} per second · {data.traffic.active} active</small></article>
  <article><span>95% of responses</span><strong>{data.traffic.p95_bucket}</strong><small>Average {num(data.traffic.mean_ms)} ms</small></article>
  <article class:attention={data.traffic.server_errors>0}><span>Server errors</span><strong>{num(data.traffic.server_errors)}</strong><small>Responses with a 5xx status</small></article>
  <article class:attention={data.traffic.rate_limited>0}><span>Rate-limit responses</span><strong>{num(data.traffic.rate_limited)}</strong><small>{num(data.traffic.rejected)} total rejected requests</small></article>
 </div>
 <div class="perf-columns">
  <section class="panel"><h2>Database pressure</h2><dl><div><dt>Connections in use</dt><dd>{data.database.in_use} / {data.database.max}</dd></div><div><dt>Idle connections</dt><dd>{data.database.idle}</dd></div><div><dt>Waited since previous refresh</dt><dd>{num(waitDelta)}</dd></div><div class:attention={cancelDelta>0}><dt>Cancelled while waiting</dt><dd>{num(cancelDelta)}</dd></div><div><dt>Wait time since API restart</dt><dd>{num(data.database.wait_ms_total)} ms</dd></div></dl><p>Repeated waiting with slow responses means queries or capacity need attention.</p></section>
  <section class="panel"><h2>Background processing</h2><dl><div class:attention={!data.worker.healthy}><dt>Reminder scheduler</dt><dd>{data.worker.healthy?'Running':'Needs attention'}</dd></div><div><dt>Last successful reminder refresh</dt><dd>{data.worker.last_success_unix?new Date(data.worker.last_success_unix*1000).toLocaleTimeString():'Not recorded'}</dd></div><div><dt>Pending issue reports</dt><dd>{data.diagnostics.queued} / {data.diagnostics.capacity}</dd></div><div class:attention={data.diagnostics.dropped_total>0}><dt>Issue events not stored</dt><dd>{num(data.diagnostics.dropped_total)}</dd></div><div><dt>Issue storage failures</dt><dd>{num(data.diagnostics.write_failures_total)}</dd></div></dl><p>Scheduler health confirms its database refresh. Email and WhatsApp delivery are shown in their own sections.</p></section>
  <section class="panel"><h2>API process</h2><dl><div><dt>Running for</dt><dd>{num(data.uptime_seconds/3600)} hours</dd></div><div><dt>Heap memory</dt><dd>{num(data.process.heap_mb)} MB</dd></div><div><dt>Memory reserved by Go</dt><dd>{num(data.process.reserved_mb)} MB</dd></div><div><dt>Go tasks</dt><dd>{num(data.process.goroutines)}</dd></div><div><dt>Responses over 1 second</dt><dd>{num(data.traffic.slow_requests)}</dd></div></dl><p>These are API process readings. VPS totals remain available in Hostinger monitoring.</p></section>
  <section class="panel"><h2>Office traffic limits</h2><dl><div><dt>Requests per user / minute</dt><dd>{num(data.limits.requests_per_user_per_minute)}</dd></div><div><dt>Shared-IP logins / 15 minutes</dt><dd>{num(data.limits.office_logins_per_15_minutes)}</dd></div><div><dt>Shared-IP OTP checks / 15 minutes</dt><dd>{num(data.limits.office_otp_checks_per_15_minutes)}</dd></div><div><dt>Parallel password checks</dt><dd>{data.limits.parallel_password_checks}</dd></div></dl><p>Account-level password and OTP protections still apply. Limits are managed in the server configuration.</p></section>
 </div>
 <p class="perf-footnote">Updated {new Date(data.observed_at).toLocaleString()} · Refreshes every 15 seconds while visible. Counters reset when the API restarts. Daily backup setup is currently paused.</p>
{:else if !error}<p role="status">Loading performance readings…</p>{/if}
<style>
 .perf-toolbar{display:flex;justify-content:space-between;align-items:center;gap:16px;margin-bottom:20px}.perf-toolbar>div{display:flex;gap:8px}.perf-toolbar p{margin:0;color:#334c62}.perf-cards{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:16px;margin-bottom:24px}.perf-cards article{padding:22px;background:#fff;border:1px solid #d6e1e8;border-top:3px solid #168782;border-radius:12px;display:flex;flex-direction:column;gap:10px}.perf-cards span{font-weight:700;color:#334c62}.perf-cards strong{font-size:clamp(22px,2.2vw,30px);color:#122b40}.perf-cards small{color:#536679;font-weight:500}.perf-columns{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px}.perf-columns section{padding:24px}.perf-columns h2{margin:0 0 20px;font-size:19px;color:#122b40}dl{margin:0}dl>div{display:flex;justify-content:space-between;gap:20px;padding:11px 0;border-bottom:1px solid #e4ebef}dt{color:#344e63;font-size:14px}dd{margin:0;font-size:14px;font-weight:750;color:#172d40;text-align:right}.perf-columns p,.perf-footnote{font-size:13px;line-height:1.6;color:#55697b;margin:18px 0 0}.attention{background:#fff4e5!important}.attention dd{color:#8d460a}.perf-footnote{margin-top:24px}@media(max-width:1000px){.perf-cards{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:650px){.perf-columns{grid-template-columns:1fr}.perf-toolbar{align-items:flex-start;flex-direction:column}.perf-cards{gap:10px}.perf-cards article{padding:16px}dl>div{gap:10px}}
</style>
