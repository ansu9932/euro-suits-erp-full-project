<script lang="ts">
 import Badge from './Badge.svelte';import {fmt,day} from './api';import type {Row} from './types';import {ChevronRight,Inbox} from '@lucide/svelte';
 let {rows,columns,onopen,empty='No records to show.'}:{rows:Row[];columns:{key:string;label:string;format?:string}[];onopen?:(r:Row)=>void;empty?:string}=$props();
</script>
<div class="table-wrap"><table><thead><tr>{#each columns as col}<th class:numeric={['number','hundredths','progress'].includes(col.format||'')}>{col.label}</th>{/each}{#if onopen}<th><span class="sr-only">Open</span></th>{/if}</tr></thead><tbody>
{#each rows as row}<tr>{#each columns as col,i}<td class:numeric={['number','hundredths','progress'].includes(col.format||'')}>
 {#if i===0&&onopen}<button class="table-link" onclick={()=>onopen?.(row)}>{row[col.key]||'—'}</button>
 {:else if col.format==='badge'}<Badge value={row[col.key]}/>
 {:else if col.format==='date'}<span class:overdue={String(row[col.key]).slice(0,10)<new Date().toLocaleDateString('en-CA')&&row.stage!=='Dispatched'}>{day(row[col.key])}</span>
 {:else if col.format==='number'}{fmt(row[col.key])}
 {:else if col.format==='hundredths'}{fmt(Number(row[col.key])/100)}
 {:else if col.format==='progress'}<div class="table-progress"><span>{row[col.key]||0}%</span><progress max="100" value={row[col.key]||0}></progress></div>
 {:else}{row[col.key]??'—'}{/if}
 </td>{/each}{#if onopen}<td><button class="icon-button small" aria-label={'Open '+(row[columns[0].key]||'record')} onclick={()=>onopen?.(row)}><ChevronRight size={16}/></button></td>{/if}</tr>{/each}
</tbody></table>{#if !rows.length}<div class="empty"><Inbox size={30}/><p>{empty}</p></div>{/if}</div>
