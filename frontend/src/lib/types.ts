export type Row = Record<string, any>;
export type User = {id:number;username:string;name:string;role:string;scope:string;companies:number[];teams:number[];lines:number[];orders:number[];finishing_units:string[];active:boolean;grants:string[]};
export type Dataset = {production_areas?:Row[];user:User;local:boolean;companies:Row[];teams:Row[];buyers:Row[];lines:Row[];orders:Row[];suppliers:Row[];inventory:Row[];purchases:Row[];work_items:Row[];entries:Row[];quality:Row[];users:Row[];audit:Row[];reminders:Row[];stages:string[];roles:string[];products:string[];categories:string[];role_grants:Record<string,string[]>};
export type Option={value:string|number;label:string};
export type Field={key:string;label:string;type?:string;options?:Option[];value?:any;required?:boolean;min?:number;max?:number;step?:string;hint?:string};
export type FormSpec={title:string;description?:string;fields:Field[];permissionsByRole?:Record<string,string[]>;submitLabel?:string;submit:(values:Row)=>Promise<void>};
