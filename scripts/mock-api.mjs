#!/usr/bin/env node
/**
 * Mock ERP API for sandboxed UI/layout testing (no Go toolchain / Postgres / Redis here).
 * Mirrors the JSON contracts of backend/internal/app (core.go, security.go, company_setup.go)
 * closely enough for the Company Setup (finance/md/admin) workspace to render every screen.
 *
 *   GET  /api/login-config            -> {turnstile_site_key:''}   (Turnstile skipped locally)
 *   GET  /api/session                 -> {user|null, csrf, local:true}
 *   POST /api/login                   -> {user, csrf}              (sets erp_session cookie)
 *   POST /api/logout                  -> {ok:true}
 *   GET  /api/company-setup           -> register list payload
 *   GET  /api/company-setup/:id       -> detail incl. profile/files/history/summary
 *   POST /api/company-setup[/:id]     -> save/submit/approve/amend/request_changes + delete (validated),
 *                                        /fields, /files (multipart upload), /responsibility, /export
 *   POST /api/company-fields|company-types|company-ports -> persisted register writes (visible after reload)
 *
 * Demo accounts (password shown):  finance / finance123   md / md123   admin / admin123
 * Run:  node scripts/mock-api.mjs            (listens on 127.0.0.1:8081, the vite proxy target)
 */
import http from 'node:http';
import {readFileSync, writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

const PORT = Number(process.env.MOCK_API_PORT || 8081);
const PASSWORDS = { finance: 'finance123', md: 'md123', admin: 'admin123' };
// Sessions survive mock restarts; SameSite=None+Secure so the cookie also works
// when the preview is viewed inside the Arena iframe (third-party context).
const SESSION_FILE = join(tmpdir(), 'erp-mock-sessions.json');
const SESSION_COOKIE_FLAGS = 'Path=/; HttpOnly; Secure; SameSite=None; Max-Age=28800';

const users = {
 admin: {id:1, username:'admin', name:'Rahul Mehra', role:'Super Admin', portal:'admin', scope:'company',
  email:'admin@eurosuitsmfg.com', phone:'+91 98300 00001', active:true, mfa_enabled:false, is_developer:false,
  designation:'', department:'', reports_to_id:0, territory:'', restrict_buyers:false, buyer_ids:[], production_areas:[],
  companies:[1,2,3,4,5], teams:[], lines:[], orders:[], finishing_units:[], grants:['view_orders','export_view'],
  permissions:['companies','users','approve','audit','documents'], custom_permissions:false, renewal_blocks:[],
  setup_features:[], setup_custom_access:false,
  setup_grants:['view','edit','approve','submit','create','export','ports','documents','reminders','fields','users']},
 finance: {id:2, username:'finance', name:'Priya Sharma', role:'Finance Head', portal:'finance', scope:'company',
  email:'finance@eurosuitsmfg.com', phone:'+91 98300 11223', active:true, mfa_enabled:false, is_developer:false,
  designation:'', department:'', reports_to_id:0, territory:'', restrict_buyers:false, buyer_ids:[], production_areas:[],
  companies:[1,2,3,4,5], teams:[], lines:[], orders:[], finishing_units:[], grants:['view_orders','export_view'],
  permissions:['companies','export'], custom_permissions:false, renewal_blocks:[],
  setup_features:[], setup_custom_access:false,
  setup_grants:['view','edit','create','submit','export','ports','documents','reminders','fields']},
 md: {id:3, username:'md', name:'Arun Kumar Das', role:'MD', portal:'md', scope:'company',
  email:'md@eurosuitsmfg.com', phone:'+91 98300 22334', active:true, mfa_enabled:false, is_developer:false,
  designation:'', department:'', reports_to_id:0, territory:'', restrict_buyers:false, buyer_ids:[], production_areas:[],
  companies:[1,2,3,4,5], teams:[], lines:[], orders:[], finishing_units:[], grants:['view_orders','export_view'],
  permissions:['approve','audit'], custom_permissions:false, renewal_blocks:[],
  setup_features:[], setup_custom_access:false,
  setup_grants:['view','approve','reminders']},
};

const types = {
 Internal:['Self','Branch','JV support company'],
 Buyer:['Retailer','Brand owner','Online','Uniform supplier','Wholesaler'],
 Supplier:['Raw materials','Finished goods','Fabrics','Trims','Office consumables','Machines and parts','Production consumables','Job workers','Service providers','Housekeeping','Refreshments','Travel','Export logistics','Cargo transport','Import logistics','CHA import/export'],
};

const ports = [
 {id:1,name:'Jawaharlal Nehru (Nhava Sheva)',code:'INJVA',country:'IN',mode:'Sea',postal_code:'400707',address:'Navi Mumbai, Maharashtra',active:true},
 {id:2,name:'Mundra Port',code:'INMUN',country:'IN',mode:'Sea',postal_code:'370610',address:'Kutch, Gujarat',active:true},
 {id:3,name:'Indira Gandhi International Airport',code:'INDEL',country:'IN',mode:'Air',postal_code:'110037',address:'New Delhi',active:true},
 {id:4,name:'ICD Tughlakabad',code:'INTKD',country:'IN',mode:'ICD',postal_code:'110044',address:'New Delhi',active:true},
 {id:5,name:'Port of Felixstowe',code:'GBFXT',country:'GB',mode:'Sea',postal_code:'IP11 8EE',address:'Felixstowe, Suffolk',active:true},
 {id:6,name:'London Heathrow Airport',code:'GBLHR',country:'GB',mode:'Air',postal_code:'TW6',address:'Hounslow, London',active:true},
];

const fieldDefs = [
 {id:1,label:'GSTIN',section:'KYC',country:'IN',categories:['Internal','Supplier'],input_type:'text',required:true,document_required:true,expiry_enabled:false,options:[],business_modes:[],company_setup_id:null,active:true},
 {id:2,label:'PAN',section:'KYC',country:'IN',categories:['Internal','Buyer','Supplier'],input_type:'text',required:true,document_required:true,expiry_enabled:false,options:[],business_modes:[],company_setup_id:null,active:true},
 {id:3,label:'IEC code',section:'KYC',country:'IN',categories:['Internal'],input_type:'text',required:true,document_required:true,expiry_enabled:false,options:[],business_modes:['Export','Import & export'],company_setup_id:null,active:true},
 {id:4,label:'Factory licence',section:'KYC',country:'IN',categories:['Internal'],input_type:'text',required:false,document_required:true,expiry_enabled:true,options:[],business_modes:[],company_setup_id:null,active:true},
 {id:5,label:'Registration certificate',section:'KYC',country:'*',categories:['Buyer'],input_type:'text',required:true,document_required:false,expiry_enabled:false,options:[],business_modes:[],company_setup_id:null,active:true},
 {id:6,label:'VAT number',section:'KYC',country:'GB',categories:['Buyer','Internal'],input_type:'text',required:false,document_required:false,expiry_enabled:false,options:[],business_modes:[],company_setup_id:null,active:true},
 {id:7,label:'Import Export Registration No.',section:'Additional',country:'IN',categories:['Buyer'],input_type:'text',required:false,document_required:true,expiry_enabled:false,options:[],business_modes:['Import','Import & export'],company_setup_id:2,active:true},
];

const field = (id, value, extra={}) => ({id, value, not_applicable:false, reason:'', expiry:false, valid_from:'', expires_on:'', documents:[], reminder_enabled:null, reminder_emails:[], ...extra});

const companies = [
 {id:1, legal_name:'Euro Suits Manufacturing Pvt Ltd', category:'Internal', company_type:'Self', country:'IN', city:'Durgapur',
  business_mode:'Import & export', business_flows:['Import','Export'], status:'Approved', revision:3, version:3,
  created_by:2, assigned_to:2, responsible_user_id:2, created_at:'2026-06-02 09:00:00', updated_at:'2026-09-28 15:42:11',
  profile:{
   short_code:'ESM', address:'Plot 14, Steel Park Industrial Area', state:'West Bengal', postal_code:'713212',
   email:'accounts@eurosuitsmfg.com', phone:'+91 343 250 1100', website:'https://eurosuitsmfg.com', owner:'Arun Kumar Das',
   sample_address:'', brands:'', departments:[], ownership:'Owned', relationship:'', nominated_by:'', supplier_services:'',
   supplier_group:'', supplier_code:'', dial_code:'', fax:'', reminder_email:'', reminder_emails:['finance@eurosuitsmfg.com'],
   notes:'Flagship manufacturing unit. All buyer POs route through this company.',
   contacts:[
    {role:'Director', name:'Arun Kumar Das', email:'md@eurosuitsmfg.com', phone:'+91 98300 22334', website:'', skype:'', speciality:''},
    {role:'Accounts Head', name:'Priya Sharma', email:'finance@eurosuitsmfg.com', phone:'+91 98300 11223', website:'', skype:'priya.esm', speciality:'Statutory compliance'},
   ],
   banks:[
    {id:'b1', name:'HDFC Bank', holder:'Euro Suits Manufacturing Pvt Ltd', account:'50200041234567', account_type:'Current', routing:'HDFC0000123', currency:'INR', documents:[101]},
    {id:'b2', name:'State Bank of India', holder:'Euro Suits Manufacturing Pvt Ltd', account:'33445566778', account_type:'Current', routing:'SBIN0005432', currency:'USD', documents:[]},
   ],
   ports:[
    {id:1, default:true, registered:true, registration:'JNPT/PORT/2024/0041', bank_id:'b1', expiry:true, valid_from:'2024-04-01', expires_on:'2027-03-31', documents:[102], reminder_enabled:true, reminder_emails:['finance@eurosuitsmfg.com']},
    {id:3, default:true, registered:false, registration:'', bank_id:'', expiry:false, valid_from:'', expires_on:'', documents:[], reminder_enabled:false, reminder_emails:[]},
   ],
   trade_registrations:[
    {id:'t1', portal:'DGFT', registration:'0262100412', licence:'AA Licence 4471/2025', bank_id:'b2', port_ids:[1], expiry:true, valid_from:'2025-01-15', expires_on:'2026-11-30', documents:[103], reminder_enabled:true, reminder_emails:[]},
   ],
   fields:[
    field(1,'19ABCDE1234F1Z5',{documents:[101]}),
    field(2,'ABCDE1234F',{documents:[101]}),
    field(3,'0262100412',{documents:[103]}),
    field(4,'WB/FA/2023/1187',{expiry:true, valid_from:'2023-07-01', expires_on:'2026-12-31', documents:[104], reminder_enabled:true, reminder_emails:['finance@eurosuitsmfg.com']}),
   ],
  },
  files:[
   {id:101,name:'GSTIN-certificate.pdf',size:248576,sha256:'a1b2c3',created_at:'2026-06-03 10:12:00',storage_key:'local/101'},
   {id:102,name:'Port-registration-JNPT.pdf',size:517120,sha256:'d4e5f6',created_at:'2026-06-05 11:40:00',storage_key:'local/102'},
   {id:103,name:'IEC-certificate.pdf',size:189440,sha256:'0a1b2c',created_at:'2026-06-05 11:44:00',storage_key:'local/103'},
   {id:104,name:'Factory-licence-2023.pdf',size:1048576,sha256:'3d4e5f',created_at:'2026-06-05 12:02:00',storage_key:'local/104',compression:'gzip',stored_size:838860,saved_bytes:209716},
  ],
  history:[
   {id:4,action:'Approved',note:'KYC complete.',revision:3,created_at:'2026-09-28 15:42:11',actor_role:'MD'},
   {id:3,action:'Submitted for approval',note:'',revision:2,created_at:'2026-09-25 12:05:44',actor_role:'Finance Head'},
   {id:2,action:'Changes requested',note:'Attach the IEC certificate copy.',revision:1,created_at:'2026-09-20 17:31:02',actor_role:'MD'},
   {id:1,action:'Draft created',note:'',revision:0,created_at:'2026-06-02 09:00:00',actor_role:'Finance Head'},
  ]},
 {id:2, legal_name:'Hanes India Retail', category:'Buyer', company_type:'Brand owner', country:'IN', city:'Bengaluru',
  business_mode:'Domestic', business_flows:['Domestic'], status:'Pending', revision:1, version:5,
  created_by:2, assigned_to:2, responsible_user_id:2, created_at:'2026-09-30 11:20:00', updated_at:'2026-10-11 09:15:22',
  profile:{
   short_code:'HANES', address:'Level 9, Prestige Trade Tower, Palace Road', state:'Karnataka', postal_code:'560001',
   email:'sourcing@hanesindia.example', phone:'+91 80 4123 9000', website:'https://hanesindia.example', owner:'Meera Iyer',
   sample_address:'Euro Suits Manufacturing Pvt Ltd, Plot 14, Steel Park Industrial Area, Durgapur 713212, West Bengal',
   brands:'Hanes, Bali, Playtex', departments:['Menswear','Womenswear'], ownership:'', relationship:'', nominated_by:'',
   supplier_services:'', supplier_group:'', supplier_code:'', dial_code:'', fax:'', reminder_email:'', reminder_emails:[],
   notes:'Uniform supplier contract under negotiation for 2027.',
   contacts:[{role:'Sourcing Manager', name:'Meera Iyer', email:'meera.iyer@hanesindia.example', phone:'+91 98450 77881', website:'', skype:'', speciality:'Formal wear'}],
   banks:[{id:'b3', name:'ICICI Bank', holder:'Hanes India Retail Pvt Ltd', account:'012305009876', account_type:'Current', routing:'ICIC0000123', currency:'INR', documents:[]}],
   ports:[{id:4, default:true, registered:false, registration:'', bank_id:'', expiry:false, valid_from:'', expires_on:'', documents:[], reminder_enabled:false, reminder_emails:[]}],
   trade_registrations:[],
   fields:[field(2,'BBFPQ2211K',{documents:[201]}), field(5,'COI/2019/778',{}), field(7,'IER-77120',{documents:[]})],
  },
  files:[{id:201,name:'PAN-hanes.pdf',size:132096,sha256:'b2c3d4',created_at:'2026-10-01 14:02:11',storage_key:'local/201'}],
  history:[
   {id:6,action:'Submitted for approval',note:'',revision:1,created_at:'2026-10-11 09:15:22',actor_role:'Finance Head'},
   {id:5,action:'Draft created',note:'',revision:0,created_at:'2026-09-30 11:20:00',actor_role:'Finance Head'},
  ]},
 {id:3, legal_name:'Arvind Fabric Mills', category:'Supplier', company_type:'Raw materials', country:'IN', city:'Ahmedabad',
  business_mode:'Domestic', business_flows:['Domestic'], status:'Changes requested', revision:2, version:7,
  approval_note:'Bank proof shows a different account holder. Upload the cancelled cheque for the correct current account.',
  created_by:2, assigned_to:2, responsible_user_id:2, created_at:'2026-08-14 10:00:00', updated_at:'2026-10-08 16:44:35',
  profile:{
   short_code:'AFM', address:'Survey 88/2, Naroda GIDC', state:'Gujarat', postal_code:'382330',
   email:'sales@arvindfabrics.example', phone:'+91 79 2281 4400', website:'https://arvindfabrics.example', owner:'Kiran Patel',
   sample_address:'', brands:'', departments:[], ownership:'', relationship:'Nominated', nominated_by:'Hanes India Retail',
   supplier_services:'Suiting fabrics: poly-viscose, wool blends, stretch weaves.', supplier_group:'Raw materials',
   supplier_code:'SUP-0117', dial_code:'+91', fax:'', reminder_email:'', reminder_emails:[], notes:'Nominated supplier for Hanes suiting programme.',
   contacts:[{role:'Owner', name:'Kiran Patel', email:'kiran@arvindfabrics.example', phone:'+91 98250 33445', website:'', skype:'', speciality:''}],
   banks:[{id:'b4', name:'Bank of Baroda', holder:'Arvind Fabric Mills', account:'987654321012', account_type:'Current', routing:'BARB0NARODA', currency:'INR', documents:[]}],
   ports:[], trade_registrations:[],
   fields:[field(1,'24AABCA1234C1ZQ',{documents:[]}), field(2,'AABCA1234C',{documents:[]})],
  },
  files:[{id:301,name:'Mill-inspection-report.pdf',size:402432,sha256:'c3d4e5',created_at:'2026-08-20 09:31:00',storage_key:'local/301'}],
  history:[
   {id:8,action:'Changes requested',note:'Bank proof mismatch.',revision:2,created_at:'2026-10-08 16:44:35',actor_role:'MD'},
   {id:7,action:'Draft created',note:'',revision:0,created_at:'2026-08-14 10:00:00',actor_role:'Finance Head'},
  ]},
 {id:4, legal_name:'Euro Trs Exports Ltd', category:'Internal', company_type:'Branch', country:'GB', city:'London',
  business_mode:'Export', business_flows:['Export'], status:'Draft', revision:0, version:1,
  created_by:2, assigned_to:2, responsible_user_id:0, created_at:'2026-10-10 12:00:00', updated_at:'2026-10-10 12:00:00',
  profile:{
   short_code:'ETE', address:'42 Fenchurch Street', state:'', postal_code:'EC3M 3LP',
   email:'', phone:'', website:'', owner:'', sample_address:'', brands:'', departments:[], ownership:'Leased',
   relationship:'', nominated_by:'', supplier_services:'', supplier_group:'', supplier_code:'', dial_code:'', fax:'',
   reminder_email:'', reminder_emails:[], notes:'UK branch to receive direct buyer payments.',
   contacts:[], banks:[], ports:[], trade_registrations:[], fields:[],
  },
  files:[], history:[{id:9,action:'Draft created',note:'',revision:0,created_at:'2026-10-10 12:00:00',actor_role:'Finance Head'}]},
 {id:5, legal_name:'Next Retail PLC', category:'Buyer', company_type:'Retailer', country:'GB', city:'Leicester',
  business_mode:'Export', business_flows:['Export'], status:'Approved', revision:2, version:4,
  created_by:2, assigned_to:2, responsible_user_id:2, created_at:'2026-07-11 15:30:00', updated_at:'2026-09-02 10:05:00',
  profile:{
   short_code:'NEXT', address:'Desford Road, Enderby', state:'', postal_code:'LE19 4AT',
   email:'supplierhub@next.example', phone:'+44 333 777 1000', website:'https://next.example', owner:'Supplier Hub',
   sample_address:'', brands:'Next, Label 28', departments:['Menswear'], ownership:'', relationship:'', nominated_by:'',
   supplier_services:'', supplier_group:'', supplier_code:'', dial_code:'', fax:'', reminder_email:'', reminder_emails:[], notes:'',
   contacts:[{role:'Merchandiser', name:'Oliver Grant', email:'oliver.grant@next.example', phone:'+44 7700 900123', website:'', skype:'', speciality:''}],
   banks:[], ports:[], trade_registrations:[], fields:[field(5,'RC/2001/44',{documents:[]}), field(6,'GB902118456',{documents:[]})],
  },
  files:[], history:[
   {id:11,action:'Approved',note:'',revision:2,created_at:'2026-09-02 10:05:00',actor_role:'Super Admin'},
   {id:10,action:'Draft created',note:'',revision:0,created_at:'2026-07-11 15:30:00',actor_role:'Finance Head'},
  ]},
];

const today = () => new Date().toISOString().slice(0,10);
const plusDays = n => { const d=new Date(); d.setDate(d.getDate()+n); return d.toISOString().slice(0,10); };
const reminders = [
 {id:1, company_id:1, legal_name:'Euro Suits Manufacturing Pvt Ltd', label:'Factory licence', expires_on:plusDays(79), remind_on:today(), email_status:'Sent 09:05 IST'},
 {id:2, company_id:1, legal_name:'Euro Suits Manufacturing Pvt Ltd', label:'DGFT advance licence 4471/2025', expires_on:plusDays(-4), remind_on:today(), email_status:'Sent · retried once'},
 {id:3, company_id:1, legal_name:'Euro Suits Manufacturing Pvt Ltd', label:'Port registration JNPT/PORT/2024/0041', expires_on:plusDays(5), remind_on:today(), email_status:'Queued'},
];

const sessions = (() => {
 try { return new Map(Object.entries(JSON.parse(readFileSync(SESSION_FILE,'utf8')))); }
 catch { return new Map(); }
})();
const saveSessions = () => { try { writeFileSync(SESSION_FILE, JSON.stringify(Object.fromEntries(sessions))); } catch {} };
const CSRF = 'mock-csrf-token';
const ts = () => new Date().toISOString().replace('T',' ').slice(0,19);

function detail(c, user) {
 const d = JSON.parse(JSON.stringify(c));
 const financeOwner = user.role==='Finance Head' && d.assigned_to===user.id;
 d.can_edit = ['Draft','Pending','Changes requested'].includes(d.status) &&
  (user.role==='Super Admin' || (user.role==='Finance Head' && financeOwner));
 d.can_view_private = user.role!=='Finance Head' || d.status!=='Approved';
 const flows = Array.isArray(d.business_flows)&&d.business_flows.length ? d.business_flows : [];
 const flowApplies = modes => {
  if(!Array.isArray(modes)||!modes.length) return true;
  const applicable = flows.length?[...flows]:['Not specified'];
  if(flows.includes('Import')&&flows.includes('Export')) applicable.push('Import & export');
  return modes.some(m=>applicable.includes(m));
 };
 // Mirror the real backend/frontend filter: country, category and business flow.
 const defs = fieldDefs.filter(f => f.company_setup_id ? f.company_setup_id===d.id
  : ((f.country==='*'||f.country===d.country) && f.categories.includes(d.category) && flowApplies(f.business_modes)));
 const checklist = defs.map(def => {
  const p = (d.profile.fields||[]).find(f=>f.id===def.id);
  const state = !p?'Not entered':p.not_applicable?'Not applicable':p.value?'Provided':'Not entered';
  return {label:def.label, state, documents:p?p.documents.length:0, expires_on:p&&p.expiry?p.expires_on:''};
 });
 const issues = [];
 for(const [i,item] of checklist.entries()) if(defs[i].required && item.state==='Not entered') issues.push(defs[i].label+' is required.');
 if(defs.some((def,i)=>def.document_required) && checklist.some(item=>item.state==='Provided'&&item.documents===0)) issues.push('Attach the supporting document for the provided KYC values.');
 d.summary = {checklist, contacts:d.profile.contacts.length, bank_accounts:d.profile.banks.length,
  ports:d.profile.ports.length, documents:d.files.length, fields:d.profile.fields.length,
  ready:issues.length===0, issues};
 d.legacy_reminder_emails = d.profile.reminder_emails||[];
 return d;
}

function listPayload(user) {
 return {user, local:true, email_configured:true, types, ports,
  fields:fieldDefs, companies:companies.map(c=>({...c, profile:undefined, files:undefined, history:undefined})), reminders};
}

function send(res, code, body, headers={}) {
 const json = JSON.stringify(body);
 res.writeHead(code, {'Content-Type':'application/json', 'Cache-Control':'no-store', ...headers});
 res.end(json);
}

const server = http.createServer((req,res) => {
 const url = new URL(req.url,'http://mock');
 const path = url.pathname.replace(/^\/api/,'') || '/';
 const cookies = Object.fromEntries((req.headers.cookie||'').split(';').map(s=>s.trim().split('=').slice(0,2)).filter(p=>p[0]));
 const me = () => users[sessions.get(cookies.erp_session)] || null;
 const chunks=[]; req.on('data',c=>chunks.push(c)); req.on('end',()=>{
  const buf=Buffer.concat(chunks);
  let body={}; try{body=buf.length?JSON.parse(buf.toString('utf8')):{};}catch{}

  if(path==='/health') return send(res,200,{ok:true,mock:true});
  if(path==='/login-config') return send(res,200,{turnstile_site_key:''});

  if(path==='/session') { const u=me(); return send(res,200,u?{user:u,csrf:CSRF,local:true}:{user:null,local:true}); }

  if(path==='/login' && req.method==='POST') {
   const name = String(body.username||'').toLowerCase();
   if(!users[name] || PASSWORDS[name]!==body.password) return send(res,401,{error:'Incorrect username or password.'});
   if(body.portal && body.portal!==users[name].portal) return send(res,403,{error:'This account belongs to a different role login page.'});
   const token = Math.random().toString(36).slice(2)+Date.now().toString(36);
   sessions.set(token,name); saveSessions();
   return send(res,200,{user:users[name],csrf:CSRF},{'Set-Cookie':`erp_session=${token}; ${SESSION_COOKIE_FLAGS}`});
  }
  if(path==='/logout' && req.method==='POST') {
   sessions.delete(cookies.erp_session); saveSessions();
   return send(res,200,{ok:true},{'Set-Cookie':`erp_session=; ${SESSION_COOKIE_FLAGS.replace('Max-Age=28800','Max-Age=-1')}`});
  }

  if(!me()) return send(res,401,{error:'Please sign in.'});

  if(path==='/company-setup' && req.method==='GET') return send(res,200,listPayload(me()));
  if(path==='/company-setup' && req.method==='POST') {
   const id = Math.max(...companies.map(c=>c.id))+1;
   companies.unshift({id, legal_name:body.legal_name||'New company', category:body.category||'Internal', company_type:body.company_type||'Self',
    country:(body.country||'IN').toUpperCase().slice(0,2), city:body.city||'', business_mode:'Not specified', business_flows:body.business_flows||[],
    status:'Draft', revision:0, version:1, created_by:me().id, assigned_to:me().id, responsible_user_id:0, created_at:ts(), updated_at:ts(),
    profile:{address:body.address||'',state:body.state||'',postal_code:body.postal_code||'',email:'',phone:'',website:'',owner:'',sample_address:'',brands:'',departments:[],ownership:'',relationship:'',nominated_by:'',supplier_services:'',supplier_group:'',supplier_code:'',dial_code:'',fax:'',reminder_email:'',reminder_emails:[],notes:'',short_code:'',contacts:[],banks:[],ports:[],trade_registrations:[],fields:[]},
    files:[], history:[{id:Date.now()%100000,action:'Draft created',note:'',revision:0,created_at:ts(),actor_role:me().role}]});
   return send(res,200,{id});
  }
  if(path==='/setup-users') return send(res,200,{users:Object.values(users).map(u=>({id:u.id,username:u.username,name:u.name,email:u.email,phone:u.phone,role:u.role,active:u.active,mfa_enabled:u.mfa_enabled,ip_restricted:false,allowed_ips:[],version:1,is_developer:false,designation:''}))});

  const m = path.match(/^\/company-setup\/(\d+)(\/(delete|responsibility|export|fields|files))?$/);
  if(m) {
   const c = companies.find(x=>x.id===Number(m[1]));
   if(!c) return send(res,404,{error:'Company not found.'});
   const sub = m[3];
   if(sub==='export') {
    res.writeHead(200,{'Content-Type':'text/csv','Content-Disposition':`attachment; filename="company-${c.id}.csv"`});
    return res.end(`legal_name,status,revision\n"${c.legal_name}",${c.status},${c.revision}\n`);
   }
   if(req.method==='POST') {
    if(sub==='delete') {
     if(String(body.confirmation_name||'')!==c.legal_name) return send(res,403,{error:'Confirmation name does not match the company record.'});
     if(PASSWORDS[me().username]!==body.password) return send(res,403,{error:'Incorrect password. The company was not deleted.'});
     companies.splice(companies.indexOf(c),1); return send(res,200,{ok:true});
    }
    if(sub==='files') {
     const nameMatch=buf.toString('latin1').match(/filename="([^"]+)"/);
     const size=Math.max(buf.length-512,2048);
     const id=Date.now()%100000;
     const stored=Math.round(size*0.78);
     const rec={id,name:nameMatch?nameMatch[1]:'uploaded-document',size,sha256:'mock'+id,created_at:ts(),storage_key:'local/'+id,compression:'gzip',stored_size:stored,saved_bytes:size-stored};
     c.files.push(rec);
     return send(res,200,rec);
    }
    if(sub==='responsibility') { c.responsible_user_id=Number(body.user_id||0); return send(res,200,{ok:true}); }
    if(sub==='fields') { const id=Math.max(...fieldDefs.map(f=>f.id))+1; fieldDefs.push({id,label:body.label||'New field',section:'Additional',country:c.country,categories:[c.category],input_type:body.input_type||'text',required:!!body.required,document_required:!!body.document_required,expiry_enabled:!!body.expiry_enabled,options:body.options||[],business_modes:[],company_setup_id:c.id,active:true}); c.version+=1; return send(res,200,{version:c.version,field:fieldDefs.at(-1)}); }
    const action = body.action;
    if(action==='save'||action==='submit') {
     if(body.legal_name)c.legal_name=String(body.legal_name);
     for(const k of ['category','company_type','city']) if(body[k]!==undefined&&String(body[k]).trim()!=='')c[k]=String(body[k]).trim();
     if(body.country&&String(body.country).trim()!=='')c.country=String(body.country).trim().toUpperCase().slice(0,2);
     if(Array.isArray(body.business_flows)){c.business_flows=body.business_flows;c.business_mode=body.business_flows.includes('Import')&&body.business_flows.includes('Export')?'Import & export':body.business_flows.length?body.business_flows[0]:'Not specified';}
     if(body.profile)c.profile={...c.profile,...body.profile};
     c.updated_at=ts();
    }
    if(action==='save') { c.version+=1; return send(res,200,{ok:true}); }
    if(action==='submit') { c.status='Pending'; c.revision+=1; c.version+=1; c.approval_note=''; c.history.unshift({id:Date.now()%100000,action:'Submitted for approval',note:'',revision:c.revision,created_at:ts(),actor_role:me().role}); return send(res,200,{ok:true}); }
    if(action==='approve') { c.status='Approved'; c.revision+=1; c.version+=1; c.history.unshift({id:Date.now()%100000,action:'Approved',note:body.note||'',revision:c.revision,created_at:ts(),actor_role:me().role}); return send(res,200,{ok:true}); }
    if(action==='amend') { c.status='Draft'; c.version+=1; c.history.unshift({id:Date.now()%100000,action:'Amendment opened',note:'',revision:c.revision,created_at:ts(),actor_role:me().role}); return send(res,200,{ok:true}); }
    if(action==='request_changes') { c.status='Changes requested'; c.approval_note=String(body.note||''); c.version+=1; c.history.unshift({id:Date.now()%100000,action:'Changes requested',note:c.approval_note,revision:c.revision,created_at:ts(),actor_role:me().role}); return send(res,200,{ok:true}); }
    return send(res,200,{ok:true});
   }
   return send(res,200,detail(c,me()));
  }
  if(path==='/company-fields'&&req.method==='POST') {
   const id=Math.max(...fieldDefs.map(f=>f.id))+1;
   const cc=String(body.country||'*').trim().toUpperCase();
   const def={id,label:String(body.label||'New field').trim(),section:body.section||'KYC',country:cc==='*'?'*':cc.slice(0,2),
    categories:Array.isArray(body.categories)&&body.categories.length?body.categories:['Internal','Buyer','Supplier'],
    input_type:body.input_type||'text',required:!!body.required,document_required:!!body.document_required,expiry_enabled:!!body.expiry_enabled,
    options:Array.isArray(body.options)?body.options:[],business_modes:Array.isArray(body.business_modes)?body.business_modes:[],company_setup_id:null,active:true};
   fieldDefs.push(def);
   return send(res,200,{id,field:def});
  }
  if(path==='/company-types'&&req.method==='POST') {
   const cat=body.category||'Internal'; const label=String(body.label||'').trim();
   if(label){ types[cat]=types[cat]||[]; if(!types[cat].includes(label))types[cat].push(label); }
   return send(res,200,{ok:true});
  }
  if(path==='/company-ports'&&req.method==='POST') {
   const id=Math.max(...ports.map(p=>p.id))+1;
   const port={id,name:String(body.name||'New port').trim(),code:String(body.code||'').trim().toUpperCase(),
    country:String(body.country||'IN').trim().toUpperCase().slice(0,2),mode:body.mode||'Sea',
    postal_code:String(body.postal_code||''),address:String(body.address||''),active:true};
   ports.push(port);
   return send(res,200,{id,port});
  }

  // Lenient catch-alls so every button in the UI has a valid response while testing layouts.
  if(req.method==='GET') return send(res,200,{ok:true,mock:true});
  return send(res,200,{ok:true,mock:true});
 });
});

server.listen(PORT,'127.0.0.1',()=>console.log(`Mock ERP API listening on http://127.0.0.1:${PORT}`));
