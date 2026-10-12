// A short click guard covers every button, including actions that only edit a
// draft locally. Requests extend the guard until they settle. No form data is stored.
type Lock = {until:number; pending:number; timer:ReturnType<typeof setTimeout>|null; ariaDisabled:string|null; ariaBusy:string|null};
const locks = new Map<HTMLElement,Lock>();
let current:HTMLElement[]=[];
let contextTimer:ReturnType<typeof setTimeout>|undefined;

function restore(node:HTMLElement, lock:Lock){
 node.removeAttribute('data-action-locked');
 for(const [name,value] of [['aria-disabled',lock.ariaDisabled],['aria-busy',lock.ariaBusy]]){
  if(value===null)node.removeAttribute(name!);else node.setAttribute(name!,value!);
 }
 locks.delete(node);
}
function schedule(node:HTMLElement,lock:Lock){
 if(lock.timer)clearTimeout(lock.timer);
 if(lock.pending)return;
 lock.timer=setTimeout(()=>{if(!lock.pending)restore(node,lock)},Math.max(0,lock.until-Date.now()));
}
function acquire(node:HTMLElement){
 let lock=locks.get(node);
 if(!lock){lock={until:Date.now()+650,pending:0,timer:null,ariaDisabled:node.getAttribute('aria-disabled'),ariaBusy:node.getAttribute('aria-busy')};locks.set(node,lock);node.setAttribute('data-action-locked','');node.setAttribute('aria-disabled','true');}
 schedule(node,lock);
}
function context(nodes:HTMLElement[]){
 current=nodes;
 if(contextTimer)clearTimeout(contextTimer);
 contextTimer=setTimeout(()=>{current=[]},0);
}
function block(event:Event){event.preventDefault();event.stopImmediatePropagation();}

export function installActionLocks(){
 const click=(event:MouseEvent)=>{
  const target=event.target instanceof Element?event.target.closest<HTMLElement>('button,input[type="submit"],input[type="button"],a.button,[role="button"]'):null;
  if(!target||event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
  if(locks.has(target)||target.matches(':disabled')||target.getAttribute('aria-disabled')==='true'){block(event);return;}
  acquire(target);context([target]);
 };
 const submit=(event:SubmitEvent)=>{
  const form=event.target;
  if(!(form instanceof HTMLFormElement))return;
  if(locks.has(form)){block(event);return;}
  const targets:HTMLElement[]=[form];
  if(event.submitter instanceof HTMLElement)targets.push(event.submitter);
  // Do not set native disabled in click capture: that would cancel the first
  // button's default form submission before validation/onsubmit can run.
  targets.forEach(acquire);context(targets);
 };
 const change=(event:Event)=>{
  const input=event.target;
  if(!(input instanceof HTMLInputElement)||input.type!=='file')return;
  if(locks.has(input)){block(event);return;}
  acquire(input);context([input]);
 };
 document.addEventListener('click',click,true);
 document.addEventListener('submit',submit,true);
 document.addEventListener('change',change,true);
 return()=>{
  document.removeEventListener('click',click,true);document.removeEventListener('submit',submit,true);document.removeEventListener('change',change,true);
  if(contextTimer)clearTimeout(contextTimer);current=[];
  for(const [node,lock] of locks){if(lock.timer)clearTimeout(lock.timer);restore(node,lock);}
 };
}

export function trackAction<T>(request:Promise<T>):Promise<T>{
 const targets=[...current].filter(node=>locks.has(node));
 for(const node of targets){const lock=locks.get(node)!;lock.pending++;node.setAttribute('aria-busy','true');schedule(node,lock);}
 return request.finally(()=>{
  for(const node of targets){const lock=locks.get(node);if(!lock)continue;lock.pending--;lock.until=Math.max(lock.until,Date.now()+250);schedule(node,lock);}
 });
}
