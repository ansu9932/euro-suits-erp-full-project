// Exercise the actual TypeScript API module with delayed transport responses.
// Temporary JS compilation uses the project's TypeScript; no browser automation.
import assert from 'node:assert/strict';
import {mkdtemp,readFile,writeFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
import ts from '../frontend/node_modules/typescript/lib/typescript.js';
const dir=await mkdtemp(join(tmpdir(),'erp-actions-'));
try{
 for(const name of ['api','actionLock','diagnostics']){
  const source=await readFile(new URL(`../frontend/src/lib/${name}.ts`,import.meta.url),'utf8');
  const js=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.ES2022}}).outputText.replace(/from '(\.\/\w+)'/g,"from '$1.mjs'");
  await writeFile(join(dir,name+'.mjs'),js);
 }
 let requests=[];
 globalThis.fetch=(url,options)=>new Promise((resolve,reject)=>requests.push({url,options,resolve,reject}));
 const {api,uploadTo}=await import(pathToFileURL(join(dir,'api.mjs')));
 const succeed=(r,data={ok:true})=>r.resolve(new Response(JSON.stringify(data),{status:200,headers:{'Content-Type':'application/json'}}));
 let a=api('/login',{username:'test',password:'fake'}),b=api('/login',{username:'test',password:'fake'});
 assert.equal(requests.length,1,'Concurrent sign-ins share one request');
 assert.match(requests[0].options.headers['Idempotency-Key'],/^[a-f0-9-]{36}$/);
 succeed(requests[0],{otp_required:true});assert.deepEqual(await a,await b);
 a=api('/records',{value:1});b=api('/records',{value:2});
 assert.equal(requests.length,3,'Different intended values stay independent');
 succeed(requests[1]);succeed(requests[2]);await Promise.all([a,b]);
 a=api('/retry',{value:1});b=api('/retry',{value:1});
 const failed=Promise.allSettled([a,b]);requests[3].reject(new TypeError('Test network failure'));
 assert.deepEqual((await failed).map(r=>r.status),['rejected','rejected']);
 a=api('/retry',{value:1});assert.equal(requests.length,5,'Network failure releases the pending lock');assert.equal(requests[4].options.headers['Idempotency-Key'],requests[3].options.headers['Idempotency-Key'],'Uncertain retry keeps its original action key');succeed(requests[4]);await a;
 a=api('/validation',{value:1});requests[5].resolve(new Response(JSON.stringify({error:'Fix this value'}),{status:400}));await assert.rejects(a,/Fix this value/);
 a=api('/validation',{value:2});succeed(requests[6]);await a;
 const file=new File(['test document'],'proof.txt',{type:'text/plain'});
 a=uploadTo('/company-setup/1/files',file);b=uploadTo('/company-setup/1/files',file);
 assert.equal(requests.length,8,'Concurrent uploads share one request');
 assert.ok(requests[7].options.headers['Idempotency-Key']);succeed(requests[7],{id:1});assert.deepEqual(await a,await b);
 a=uploadTo('/orders/1/documents',file);b=uploadTo('/orders/2/documents',file);
 assert.equal(requests.length,10,'Different upload destinations stay independent');succeed(requests[8]);succeed(requests[9]);await Promise.all([a,b]);
 a=api('/session');b=api('/session');assert.equal(requests.length,11,'Concurrent read refreshes share one request');assert.equal(requests[10].options.method,'GET');succeed(requests[10]);await Promise.all([a,b]);
 console.log('PASS: frontend duplicate-submit, retry, validation, request-key and upload checks');
}finally{await rm(dir,{recursive:true,force:true});}
