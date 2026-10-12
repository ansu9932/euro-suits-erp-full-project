export function convertMeasurement(row: Record<string, any>) {
 const widthUnit=row.width_unit||'cm', weightUnit=row.weight_unit||'GSM';
 const factors:Record<string,number>={cm:1,inch:2.54,metre:100};
 const widthCm=Object.hasOwn(factors,widthUnit)?Number(row.width||0)*factors[widthUnit]:null;
 const weight=Number(row.weight||0);
 const gsm=weightUnit==='GSM'?weight:weightUnit==='oz/yd²'?weight*33.905747:weightUnit==='g/linear metre'&&widthCm!==null&&widthCm>0?weight/(widthCm/100):null;
 return {widthCm,gsm,gramsPerMetre:gsm===null||widthCm===null?null:gsm*widthCm/100};
}
