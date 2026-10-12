import type {Row} from './types';

// Company fields follow their record even when its country or business flow changes.
// A global definition still uses the existing category, country and flow filters.
export function companyFieldApplies(field:Row,company:Row|null|undefined,matchesGlobalFilters:boolean):boolean {
 if(!company)return false;
 if(field.company_setup_id!==undefined&&field.company_setup_id!==null){
  const scope=Number(field.company_setup_id),companyID=Number(company.id);
  return Number.isSafeInteger(scope)&&scope>0&&scope===companyID;
 }
 return matchesGlobalFilters;
}

export function companyFieldRequest(form:Row,version:number):Row {
 return {
  version,
  label:String(form.label||'').trim(),
  input_type:form.input_type,
  options:form.input_type==='select'?String(form.options_text||'').split('\n').map(value=>value.trim()).filter(Boolean):[],
  required:!!form.required,
  document_required:!!form.document_required,
  expiry_enabled:!!form.expiry_enabled
 };
}
