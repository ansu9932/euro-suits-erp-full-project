import {test} from 'node:test';
import assert from 'node:assert/strict';
import {workspaceRedirect} from '../src/lib/workspaceAccess.ts';

for (const portal of ['admin','md','finance']) {
 test(`${portal}: setup-only access works through an old operations link`,()=>{
  const user={portal,grants:[],setup_grants:['view']};
  assert.equal(workspaceRedirect(user,portal,'operations'),'/'+portal);
  assert.equal(workspaceRedirect(user,portal,'companies'),null);
 });
 test(`${portal}: operations-only access works through the company login`,()=>{
  const user={portal,grants:['view_orders'],setup_grants:[]};
  assert.equal(workspaceRedirect(user,portal,'companies'),'/'+portal+'/operations');
  assert.equal(workspaceRedirect(user,portal,'operations'),null);
 });
 test(`${portal}: no redirect loop and no implicit grants`,()=>{
  for(const grants of [[],['view_orders']]) for(const setup_grants of [[],['view']]){
   const u={portal,grants,setup_grants};
   if(grants.length===setup_grants.length){
    assert.equal(workspaceRedirect(u,portal,'companies'),null);
    assert.equal(workspaceRedirect(u,portal,'operations'),null);
   }
  }
 });
}
test('all operational portals retain their dashboard',()=>{
 for(const portal of ['management','accounts','merchandising','planning','cad','sourcing','stores','sampling','cutting','sewing','finishing','quality','warehouse']) {
  assert.equal(workspaceRedirect({portal,grants:['view_orders']},portal,'operations'),null);
  assert.equal(workspaceRedirect({portal,grants:[]},portal,'operations'),null);
  assert.equal(workspaceRedirect({portal},'finance','companies'),'/'+portal);
 }
});
test('generic login and developer route resolve to the account portal',()=>{
 assert.equal(workspaceRedirect({portal:'finance'},'','companies'),'/finance');
 assert.equal(workspaceRedirect({portal:'developer'},'admin','companies'),'/developer');
});

test('export-only Finance users reach invoice access from sign-in and old workspace links',()=>{
 const user={portal:'finance',grants:['export_view','export_edit'],setup_grants:[]};
 for(const portal of ['', 'finance', 'admin']) for(const workspace of ['companies','operations']) {
  assert.equal(workspaceRedirect(user,portal,workspace),'/exports');
 }
});
test('export grants preserve company and operations workspaces when those are assigned',()=>{
 assert.equal(workspaceRedirect({portal:'finance',grants:['export_view'],setup_grants:['view']},'finance','companies'),null);
 assert.equal(workspaceRedirect({portal:'finance',grants:['export_view'],setup_grants:['view']},'finance','operations'),'/finance');
 assert.equal(workspaceRedirect({portal:'finance',grants:['export_view','view_orders'],setup_grants:[]},'finance','companies'),'/finance/operations');
 assert.equal(workspaceRedirect({portal:'finance',grants:['export_edit'],setup_grants:[]},'finance','companies'),null);
 assert.equal(workspaceRedirect({portal:'developer',grants:['export_view'],setup_grants:[]},'finance','companies'),'/developer');
});
