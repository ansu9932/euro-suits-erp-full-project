import {reportClientIssue} from './diagnostics';
import {trackAction} from './actionLock';
export let csrf='';
export function setCSRF(token:string){csrf=token;}
const pending=new Map<string,Promise<any>>();
const fileIDs=new WeakMap<File,string>();
// Keep the same action ID after an uncertain response (network loss / 5xx).
// Retrying unchanged data must not repeat a write that may already have finished.
// This bounded map is memory-only and clears with the page or account session.
const uncertain=new Map<string,{id:string;expires:number}>();
function actionID(identity:string){
 for(const [key,value] of uncertain)if(value.expires<Date.now())uncertain.delete(key);
 if(!uncertain.has(identity)){
  if(uncertain.size>=100)uncertain.delete(uncertain.keys().next().value!);
  uncertain.set(identity,{id:crypto.randomUUID(),expires:Date.now()+86400000});
 }
 return uncertain.get(identity)!.id;
}
function once<T>(key:string,work:()=>Promise<T>):Promise<T>{
 const existing=pending.get(key);
 if(existing)return trackAction(existing);
 const request=work().finally(()=>{if(pending.get(key)===request)pending.delete(key)});
 pending.set(key,request);
 return trackAction(request);
}
export function api(path:string,body?:unknown,method='POST'):Promise<any>{
 const verb=body===undefined?'GET':method;
 const json=body===undefined?undefined:JSON.stringify(body);
 const identity=csrf+':'+verb+':'+path+':'+(json||'');
 return once(identity,async()=>{
  const response=await fetch('/api'+path,{method:verb,credentials:'same-origin',headers:body===undefined?{}:{'Content-Type':'application/json','X-CSRF-Token':csrf,'Idempotency-Key':actionID(identity)},body:json}).catch(e=>{reportClientIssue('network');throw e});
  const data=await response.json().catch(()=>{throw new Error('The server response was interrupted. Refresh to check whether the action finished.');});
  if(response.status<500&&data.code!=='action_already_submitted')uncertain.delete(identity);
  if(!response.ok){if(response.status===401&&path!=='/login'&&path!=='/login/otp'&&typeof window!=='undefined')window.dispatchEvent(new Event('erp:session-expired'));throw Object.assign(new Error(data.error||'Request failed.'),{status:response.status,code:data.code});}
  if(data.csrf){if(csrf!==data.csrf)uncertain.clear();setCSRF(data.csrf);}
  if(path==='/logout'){csrf='';uncertain.clear();}
  return data;
 });
}
export function uploadTo(path:string,file:File):Promise<any>{
 if(!fileIDs.has(file))fileIDs.set(file,crypto.randomUUID());
 const identity=csrf+':UPLOAD:'+path+':'+fileIDs.get(file);
 return once(identity,async()=>{
  const body=new FormData();body.append('file',file);
  const response=await fetch('/api'+path,{method:'POST',credentials:'same-origin',headers:{'X-CSRF-Token':csrf,'Idempotency-Key':actionID(identity)},body}).catch(e=>{reportClientIssue('upload');throw e});
  const data=await response.json().catch(()=>{reportClientIssue('upload');throw new Error('Upload confirmation was interrupted. Refresh to check the document list before retrying.');});
  if(response.status<500&&data.code!=='action_already_submitted')uncertain.delete(identity);
  if(!response.ok){reportClientIssue('upload');throw Object.assign(new Error(data.error||'Upload failed.'),{status:response.status,code:data.code});}
  return data;
 });
}
export const upload=(order:number,file:File)=>uploadTo(`/orders/${order}/documents`,file);
export const fmt=(n:unknown)=>Number(n||0).toLocaleString('en-GB',{maximumFractionDigits:2});
export const day=(s:unknown)=>s?new Date(String(s).slice(0,10)+'T12:00:00').toLocaleDateString('en-GB',{day:'2-digit',month:'short'}):'—';
export const dateISO=()=>new Date().toLocaleDateString('en-CA');
