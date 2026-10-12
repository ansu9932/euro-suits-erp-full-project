import {test} from 'node:test';
import assert from 'node:assert/strict';
import {businessFlows, businessFlowMatches} from '../src/lib/businessFlows.ts';

test('legacy business flow records keep their existing selection',()=>{
 for(const mode of ['Domestic','Import','Export'])assert.deepEqual(businessFlows({business_mode:mode}),[mode]);
 assert.deepEqual(businessFlows({business_mode:'Import & export'}),['Import','Export']);
 assert.deepEqual(businessFlows({business_mode:'Not specified'}),[]);
 assert.deepEqual(businessFlows(null),[]);
});

test('canonical multi-selection is unique, ordered, and takes precedence over the scalar',()=>{
 assert.deepEqual(businessFlows({business_flows:['Export','Domestic','Import','Domestic'],business_mode:'Domestic'}),['Domestic','Import','Export']);
 assert.deepEqual(businessFlows({business_flows:[],business_mode:'Import'}),[]);
});

test('KYC applicability is the union of selected business flows',()=>{
 const company={business_flows:['Domestic','Import']};
 assert.equal(businessFlowMatches(['Domestic'],company),true);
 assert.equal(businessFlowMatches(['Import'],company),true);
 assert.equal(businessFlowMatches(['Export'],company),false);
 assert.equal(businessFlowMatches(['Not specified','Import','Export','Import & export'],company),true);
 assert.equal(businessFlowMatches(['Import & export'],company),false);
 assert.equal(businessFlowMatches(['Import & export'],{business_flows:['Import','Export']}),true);
 assert.equal(businessFlowMatches([],company),true);
});

test('domestic-only and unspecified preserve distinct IEC rules',()=>{
 const iec=['Not specified','Import','Export','Import & export'];
 assert.equal(businessFlowMatches(iec,{business_flows:['Domestic']}),false);
 assert.equal(businessFlowMatches(iec,{business_flows:[]}),true);
});
