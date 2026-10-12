export const BUSINESS_FLOWS = ['Domestic', 'Import', 'Export'] as const;
export type BusinessFlow = typeof BUSINESS_FLOWS[number];

type CompanyFlows = {business_flows?:unknown;business_mode?:unknown};

// The array is authoritative. The scalar fallback keeps older records readable.
export function businessFlows(company:CompanyFlows|null|undefined):BusinessFlow[] {
 const stored=company?.business_flows;
 if(Array.isArray(stored))return BUSINESS_FLOWS.filter(flow=>stored.includes(flow));
 const mode=company?.business_mode;
 if(mode==='Import & export')return ['Import','Export'];
 return BUSINESS_FLOWS.filter(flow=>flow===mode);
}

export function businessFlowMatches(modes:unknown, company:CompanyFlows|null|undefined):boolean {
 if(!Array.isArray(modes)||!modes.length)return true;
 const flows=businessFlows(company);
 const applicable:string[]=flows.length?[...flows]:['Not specified'];
 if(flows.includes('Import')&&flows.includes('Export'))applicable.push('Import & export');
 return modes.some(mode=>applicable.includes(mode));
}
