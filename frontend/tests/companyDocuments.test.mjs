import {test} from 'node:test';
import assert from 'node:assert/strict';
import {linkedCompanyFiles,unlinkedCompanyFiles,companyFileCanPreview,companyFilePreviewLabel,companyFileStorageNote,companyHistoryTime} from '../src/lib/companyDocuments.ts';

const files=[{id:1,name:'GST.pdf'},{id:2,name:'Bank.jpg'},{id:3,name:'Port.pdf'},{id:4,name:'Trade.pdf'},{id:5,name:'Retained.xlsx'}];
test('each company field displays only its linked originals',()=>{
 assert.deepEqual(linkedCompanyFiles({documents:[1]},files),[files[0]]);
 assert.deepEqual(linkedCompanyFiles({documents:[2]},files),[files[1]]);
 assert.deepEqual(linkedCompanyFiles({},files),[]);
 assert.deepEqual(linkedCompanyFiles({documents:[1,1,999]},files),[files[0]]);
});
test('full document register retains every original while recovery excludes attached sections',()=>{
 const profile={fields:[{documents:[1]}],banks:[{documents:[2]}],ports:[{documents:[3]}],trade_registrations:[{documents:[4]}]};
 assert.deepEqual(unlinkedCompanyFiles(profile,files),[files[4]]);
 profile.fields[0].documents=[];
 assert.deepEqual(unlinkedCompanyFiles(profile,files),[files[0],files[4]]);
 assert.deepEqual(files.map(file=>file.name),['GST.pdf','Bank.jpg','Port.pdf','Trade.pdf','Retained.xlsx']);
});
test('legacy shared links remain attached until every reference is removed',()=>{
 const profile={fields:[{documents:[1]}],banks:[{documents:[1]}]};
 profile.fields[0].documents=[];
 assert.deepEqual(unlinkedCompanyFiles(profile,[files[0]]),[]);
 assert.deepEqual(linkedCompanyFiles(profile.banks[0],files),[files[0]]);
});
test('supported previews distinguish Office text from PDF and image originals',()=>{
 for(const name of ['GST.PDF','proof.png','scan.jpeg','scan.jpg']){
  assert.equal(companyFileCanPreview({name}),true);
  assert.equal(companyFilePreviewLabel({name}),'Preview');
 }
 for(const name of ['proof.DOCX','accounts.xlsx']){
  assert.equal(companyFileCanPreview({name}),true);
  assert.equal(companyFilePreviewLabel({name}),'Text preview');
 }
 for(const name of ['page.html','scan.svg','scan.pdf.exe','legacy.doc','macros.xlsm'])assert.equal(companyFileCanPreview({name}),false);
});
test('company record history includes date and precise India time',()=>{
 const time=companyHistoryTime('2026-10-09T06:30:12Z');
 assert.match(time,/09 Oct 2026/);
 assert.match(time,/12:00:12/);
 assert.equal(companyHistoryTime('bad timestamp'),'—');
});
test('lossless storage claims use measured encrypted storage and hide absent or non-saving metadata',()=>{
 assert.equal(companyFileStorageNote({compression:'gzip',original_size:1000,stored_size:846,saved_bytes:200}),'Lossless compression · 19% less storage');
 for(const metadata of [{},{compression:'none',stored_size:100,saved_bytes:0},{compression:'gzip',stored_size:100,saved_bytes:-1},{compression:'gzip',stored_size:0,saved_bytes:20},{compression:'gzip',stored_size:100,saved_bytes:'unavailable'}])assert.equal(companyFileStorageNote(metadata),'');
 assert.equal(companyFileStorageNote({compression:'gzip',stored_size:10000,saved_bytes:1}),'Lossless compression · under 1% storage saved');
});
