import type {Row} from './types';

export function linkedCompanyFiles(target:Row, files:Row[]):Row[] {
 const ids=new Set((target.documents||[]).map(Number));
 return files.filter(file=>ids.has(Number(file.id)));
}

export function unlinkedCompanyFiles(profile:Row, files:Row[]):Row[] {
 const ids=new Set<number>();
 for(const section of ['fields','banks','ports','trade_registrations']) {
  for(const item of profile[section]||[]) for(const id of item.documents||[]) ids.add(Number(id));
 }
 return files.filter(file=>!ids.has(Number(file.id)));
}

export function companyFileCanPreview(file:Row):boolean {
 return /\.(pdf|png|jpe?g|docx|xlsx)$/i.test(String(file.name||''));
}

export function companyFilePreviewLabel(file:Row):string {
 return /\.(docx|xlsx)$/i.test(String(file.name||''))?'Text preview':'Preview';
}

export function companyFileStorageNote(file:Row):string {
 const saved=Number(file.saved_bytes),stored=Number(file.stored_size);
 if(file.compression!=='gzip'||!Number.isSafeInteger(saved)||saved<=0||!Number.isSafeInteger(stored)||stored<=0)return '';
 const percent=Math.floor(saved/(stored+saved)*100);
 return 'Lossless compression · '+(percent?percent+'% less storage':'under 1% storage saved');
}

export function companyHistoryTime(value:unknown):string {
 if(!value)return '—';
 const date=new Date(String(value));
 if(Number.isNaN(date.getTime()))return '—';
 return new Intl.DateTimeFormat('en-GB',{day:'2-digit',month:'short',year:'numeric',hour:'2-digit',minute:'2-digit',second:'2-digit',timeZone:'Asia/Kolkata',timeZoneName:'short'}).format(date);
}
