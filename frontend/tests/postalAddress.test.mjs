import test from 'node:test';import assert from 'node:assert/strict';
import {indianPIN,postalSuggestions,lookupPostalAddress} from '../src/lib/postalAddress.ts';
test('India PIN lookup only accepts six digits with a nonzero first digit',()=>{
 for(const value of ['751001',' 110001 '])assert.equal(indianPIN(value),true);
 for(const value of ['012345','75100','7510012','A51001','751 001',''])assert.equal(indianPIN(value),false);
});
test('Only unanimous city/state values are autofilled; ambiguous areas stay available',()=>{
 const a={city:'Bhubaneswar',state:'Odisha',district:'Khorda'};
 assert.deepEqual(postalSuggestions([a,a]),{cities:['Bhubaneswar'],states:['Odisha'],city:'Bhubaneswar',state:'Odisha'});
 const multi=postalSuggestions([a,{...a,city:'Jatni'}]);assert.equal(multi.city,'');assert.equal(multi.state,'Odisha');assert.equal(multi.cities.length,2);
 assert.deepEqual(postalSuggestions([]),{cities:[],states:[],city:'',state:''});
});
test('Lookup sends only PIN through ERP and validates returned country/PIN',async()=>{
 const original=globalThis.fetch;const signal=new AbortController().signal;
 try{
  globalThis.fetch=async(url,options)=>{assert.equal(url,'/api/company-postal-lookup?country=IN&postal_code=751001');assert.equal(options.signal,signal);assert.equal(options.credentials,'same-origin');return Response.json({country:'IN',postal_code:'751001',locations:[{city:'Bhubaneswar',state:'Odisha',district:'Khorda'}]})};
  assert.equal((await lookupPostalAddress('751001',signal))[0].city,'Bhubaneswar');
  globalThis.fetch=async()=>Response.json({country:'IN',postal_code:'110001',locations:[]});await assert.rejects(lookupPostalAddress('751001',signal));
  globalThis.fetch=async()=>new Response('',{status:503});await assert.rejects(lookupPostalAddress('751001',signal));
 }finally{globalThis.fetch=original;}
});
