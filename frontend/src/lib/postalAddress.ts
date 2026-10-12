export type PostalLocation={city:string;state:string;district:string};
export type AddressPatch=Partial<{city:string;state:string;postalCode:string;address:string}>;
export const indianPIN=(value:string)=>/^[1-9]\d{5}$/.test(value.trim());

// A PIN can cover several towns. Only fill a field when every result agrees.
export function postalSuggestions(locations:PostalLocation[]){
 const cities=[...new Set(locations.map(item=>item.city).filter(Boolean))];
 const states=[...new Set(locations.map(item=>item.state).filter(Boolean))];
 return {cities,states,city:cities.length===1?cities[0]:'',state:states.length===1?states[0]:''};
}

export async function lookupPostalAddress(pin:string,signal:AbortSignal):Promise<PostalLocation[]>{
 const response=await fetch('/api/company-postal-lookup?country=IN&postal_code='+encodeURIComponent(pin),{signal,credentials:'same-origin'});
 if(response.status===401)window.dispatchEvent(new Event('erp:session-expired'));
 if(!response.ok)throw new Error('Postal lookup unavailable');
 const data=await response.json();
 if(data.country!=='IN'||data.postal_code!==pin||!Array.isArray(data.locations)||data.locations.length>200)throw new Error('Invalid postal response');
 return data.locations.filter((item:unknown):item is PostalLocation=>{
  if(!item||typeof item!=='object')return false;
  const row=item as Record<string,unknown>;
  return ['city','state','district'].every(key=>typeof row[key]==='string'&&(row[key] as string).length<=100);
 });
}
