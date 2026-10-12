import {csrf} from './api';
let lastReport=0;
export function reportClientIssue(kind:'runtime'|'promise'|'network'|'upload'){
 if(typeof window==='undefined'||!csrf||Date.now()-lastReport<10000)return;
 lastReport=Date.now();
 // Never send exception text, form values, filenames, query strings or stacks.
 const page=window.location.pathname.replace(/[^a-zA-Z/-]/g,'').slice(0,80);
 void fetch('/api/client-issues',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify({kind,page})}).catch(()=>{});
}
