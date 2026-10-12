import {test} from 'node:test';
import assert from 'node:assert/strict';
import {companyFieldApplies,companyFieldRequest} from '../src/lib/companyFields.ts';

test('company fields stay attached when the company header stops matching global filters',()=>{
 const localField={company_setup_id:41};
 assert.equal(companyFieldApplies(localField,{id:41,country:'IN'},true),true);
 assert.equal(companyFieldApplies(localField,{id:41,country:'GB'},false),true);
});
test('company fields cannot appear on a different company even when global filters match',()=>{
 assert.equal(companyFieldApplies({company_setup_id:41},{id:42},true),false);
 assert.equal(companyFieldApplies({company_setup_id:41},null,true),false);
 for(const scope of [0,-1,'invalid',1.5])assert.equal(companyFieldApplies({company_setup_id:scope},{id:scope},true),false);
 assert.equal(companyFieldApplies({company_setup_id:'41'},{id:41},false),true);
});
test('global fields preserve existing applicability rules',()=>{
 for(const field of [{},{company_setup_id:null}]){
  assert.equal(companyFieldApplies(field,{id:41},true),true);
  assert.equal(companyFieldApplies(field,{id:41},false),false);
 }
});
test('company field requests send only editable definition properties and record version',()=>{
 const form={label:'  Buyer reference  ',input_type:'select',options_text:' One\n\nTwo \n',required:true,document_required:false,expiry_enabled:true,section:'KYC',country:'*',categories:['Buyer'],business_modes:['Export'],company_setup_id:99};
 const original=structuredClone(form);
 assert.deepEqual(companyFieldRequest(form,7),{version:7,label:'Buyer reference',input_type:'select',options:['One','Two'],required:true,document_required:false,expiry_enabled:true});
 assert.deepEqual(form,original);
});
test('switching from a dropdown to text does not send obsolete choices',()=>{
 assert.deepEqual(companyFieldRequest({label:'Reference',input_type:'text',options_text:'One\nTwo'},8),{version:8,label:'Reference',input_type:'text',options:[],required:false,document_required:false,expiry_enabled:false});
});
