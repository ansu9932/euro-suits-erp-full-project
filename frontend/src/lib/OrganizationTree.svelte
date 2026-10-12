<script lang="ts">
 import type {Row} from './types';
 let {users, currentUserId, search='', onedit}:{users:Row[];currentUserId:number;search?:string;onedit:(u:Row)=>void}=$props();
 let staff=$derived(users.filter(u=>!u.is_developer));
 let included=$derived.by(()=>{
  const ids=new Set<number>();
  for(const u of staff){
   if(!`${u.name} ${u.username} ${u.designation} ${u.department} ${u.territory}`.toLowerCase().includes(search.toLowerCase()))continue;
   let item:Row|undefined=u;const seen=new Set<number>();
   while(item&&!seen.has(item.id)){seen.add(item.id);ids.add(item.id);item=staff.find(p=>p.id===item!.reports_to_id)}
  }
  return ids;
 });
 let roots=$derived(staff.filter(u=>!staff.some(p=>p.id===u.reports_to_id)));
</script>
{#snippet branch(u:Row, trail:number[])}
 {#if included.has(u.id)&&!trail.includes(u.id)}
  {@const children=staff.filter(p=>p.reports_to_id===u.id&&included.has(p.id))}
 <li>
  <article class:inactive={!u.active}>
   <span class="avatar" aria-hidden="true">{u.name.trim().split(/\s+/).map((n:string)=>n[0]).slice(0,2).join('')}</span>
   <div class="person"><strong>{u.name}</strong><span>{u.designation||u.role}</span><small>{u.department}{u.territory?' · '+u.territory:''}{!u.active?' · Disabled':''}</small></div>
   <button class="button secondary small-button" disabled={u.id===currentUserId} onclick={()=>onedit(u)}>Edit access</button>
  </article>
  {#if children.length}<ul>{@render branches(children,[...trail,u.id])}</ul>{/if}
 </li>
 {/if}
{/snippet}
{#snippet branches(items:Row[], trail:number[])}{#each items as u(u.id)}{@render branch(u,trail)}{/each}{/snippet}
<div class="org-tree"><p class="muted">Reporting lines show responsibility. Each user’s companies, teams, buyers and actions are assigned separately by Admin.</p><ul aria-label="Organisation reporting structure">{@render branches(roots,[])}</ul>{#if !included.size}<p class="empty">No matching employees. Add a user and choose their reporting manager.</p>{/if}</div>
<style>
 .org-tree{padding:24px;overflow:auto}.org-tree>p{margin:0 0 22px}.org-tree ul{list-style:none;padding:0;margin:0}.org-tree li>ul{margin:0 0 16px 25px;padding:8px 0 0 24px;border-left:1px solid #cedbe0}.org-tree li{position:relative}.org-tree li>ul>li:before{content:'';position:absolute;width:24px;left:-24px;top:35px;border-top:1px solid #cedbe0}.org-tree article{display:flex;align-items:center;gap:14px;padding:16px;background:#fff;border:1px solid #dae4e8;border-radius:10px;margin-bottom:10px;min-width:310px;max-width:750px}.person{display:grid;gap:4px;flex:1}.person span{font-size:13px;color:#425c67}.person small{font-size:11px;color:#70828c}.avatar{width:38px;height:38px;border-radius:10px;display:grid;place-items:center;background:#edf4f3;color:#285950;font-size:12px;font-weight:700}.inactive{opacity:.62}@media(max-width:640px){.org-tree{padding:14px}.org-tree article{gap:9px}.org-tree li>ul{margin-left:12px;padding-left:14px}.org-tree li>ul>li:before{left:-14px;width:14px}}
</style>
