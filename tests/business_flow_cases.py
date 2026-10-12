"""Company multi-flow regression cases; invoked only by the disposable HTTP suite."""
def run_company_business_flow_cases():
    # Rehearse the additive migration against temporary schema-17-shaped tables.
    migration=(ROOT/'backend/internal/app/migration_v18.sql').read_text()
    prepare="""BEGIN;
CREATE TEMP TABLE company_setups(id int, business_mode text NOT NULL);
CREATE TEMP TABLE schema_versions(version int PRIMARY KEY);
INSERT INTO company_setups VALUES(1,'Not specified'),(2,'Domestic'),(3,'Import'),(4,'Export'),(5,'Import & export');
"""
    result=sql(prepare+migration+"SELECT json_agg(json_build_object('mode',business_mode,'flows',business_flows) ORDER BY id)::text FROM company_setups; ROLLBACK;",name)
    migrated=json.loads(next(line for line in result.splitlines() if line.startswith('[{"mode"')))
    check([r['flows'] for r in migrated]==[[],['Domestic'],['Import'],['Export'],['Import','Export']],'Migration18 preserves all five legacy flow modes')
    check([r['mode'] for r in migrated]==['Not specified','Domestic','Import','Export','Import & export'],'Migration18 leaves compatibility scalars unchanged')
    for badarray,mode in [("ARRAY['Domestic','Domestic']",'Domestic'),("ARRAY['Unknown']",'Domestic'),("ARRAY['Import','Domestic']",'Import'),("ARRAY['Domestic',NULL]",'Domestic'),("ARRAY['Domestic']",'Import')]:
        try:
            sql(prepare+migration+f"INSERT INTO company_setups(id,business_mode,business_flows) VALUES(6,'{mode}',{badarray}); ROLLBACK;",name)
            raise AssertionError('Invalid stored business flows accepted')
        except subprocess.CalledProcessError:
            check(True,'Database rejects noncanonical or inconsistent business-flow array '+badarray)
    check(sql('SELECT max(version) FROM schema_versions',name)=='26','Disposable runtime uses schema26')

    hdr={'legal_name':'QA MULTI BUSINESS FLOW','category':'Buyer','company_type':'Retailer','country':'IN','city':'Delhi'}
    defs=[]
    for label,modes in [('Domestic only',['Domestic']),('Import only',['Import']),('Export only',['Export']),('Import and export',['Import & export']),('Unspecified only',['Not specified'])]:
        identifier=admin.call('/company-fields',{'label':'Flow QA '+label,'section':'Additional','country':'IN','categories':['Buyer'],'input_type':'text','expiry_enabled':True,'business_modes':modes})['id']
        defs.append((identifier,'Flow QA '+label,modes))
    choices=['Domestic','Import','Export']
    companies=[]
    for mask in range(8):
        flows=[flow for bit,flow in enumerate(choices) if mask&(1<<bit)]
        identifier=finance.call('/company-setup',{**hdr,'legal_name':hdr['legal_name']+' '+str(mask),'business_flows':list(reversed(flows))+flows})['id']
        companypath='/company-setup/'+str(identifier)
        entry=finance.call(companypath)
        check(entry['business_flows']==flows,'Canonical selection roundtrip '+str(flows))
        labels={field['label'] for field in entry['summary']['checklist']}
        check(('IEC' in labels)==(not flows or 'Import' in flows or 'Export' in flows),'IEC union applicability '+str(flows))
        applicable=set(flows or ['Not specified'])
        if 'Import' in flows and 'Export' in flows:applicable.add('Import & export')
        for _,label,modes in defs:
            check((label in labels)==bool(applicable.intersection(modes)),'Custom field applicability '+label+' '+str(flows))
        companies.append((identifier,companypath,entry))
    selected_id,selected_path,selected=companies[-1]
    listed=next(item for item in finance.call('/company-setup')['companies'] if item['id']==selected_id)
    check(listed['business_flows']==choices,'Register returns every selected flow')
    out=finance.call(selected_path+'/export')
    check(b'Domestic, Import, Export' in zipfile.ZipFile(io.BytesIO(out)).read('xl/worksheets/sheet1.xml'),'Excel includes every selected flow')
    save_header={**hdr,'legal_name':selected['legal_name']}
    def save(payload,expect=200,profile=None):
        return finance.call(selected_path,{'action':'save','version':finance.call(selected_path)['version'],**save_header,'profile':profile or {},**payload},expect)
    save({'business_mode':selected['business_mode']})
    check(finance.call(selected_path)['business_flows']==choices,'Old-client scalar echo preserves all three flows')
    save({})
    check(finance.call(selected_path)['business_flows']==choices,'Omitted flow fields preserve selections')
    for invalid in [None,'Domestic',{},['Not specified'],['Import & export'],['bad'],['Domestic',2],['Domestic',None]]:
        finance.call('/company-setup',{**hdr,'business_flows':invalid},400)
        save({'business_flows':invalid},400)
    check(finance.call(selected_path)['business_flows']==choices,'Rejected values leave saved selections unchanged')
    # Legacy clients can still explicitly change the old scalar; new arrays can clear it.
    save({'business_mode':'Domestic'})
    check(finance.call(selected_path)['business_flows']==['Domestic'],'Changed legacy scalar remains accepted')
    save({'business_flows':[]})
    cleared=finance.call(selected_path)
    check(cleared['business_flows']==[] and cleared['business_mode']=='Not specified','Empty array clears to Not specified')
    for legacy,expected in [('Not specified',[]),('Domestic',['Domestic']),('Import',['Import']),('Export',['Export']),('Import & export',['Import','Export'])]:
        legacy_id=finance.call('/company-setup',{**hdr,'legal_name':hdr['legal_name']+' legacy '+legacy,'business_mode':legacy})['id']
        check(finance.call('/company-setup/'+str(legacy_id))['business_flows']==expected,'Legacy request conversion '+legacy)
    save({'business_flows':['Domestic','Import']})
    combined_field=next(identifier for identifier,label,_ in defs if label=='Flow QA Import and export')
    save({'business_flows':['Domestic','Import']},400,{'fields':[{'id':combined_field,'value':'must not apply'}]})
    # New selections still use normal versions, permissions, KYC completeness and approval.
    entry=finance.call(selected_path)
    finance.call(selected_path,{'action':'save','version':entry['version']-1,**save_header,'profile':{},'business_flows':choices},409)
    md.call(selected_path,{'action':'save','version':entry['version'],**save_header,'profile':{},'business_flows':choices},403)
    profile={'address':'QA flow address','email':'qa-flow@example.test','phone':'000','reminder_email':'qa-flow-reminder@example.test','sample_address':'QA sample address','brands':'QA','departments':['Menswear']}
    flow_defs=finance.call('/company-setup')['fields']
    profile['fields']=[{'id':d['id'],'not_applicable':True,'reason':'Synthetic regression fixture'} for d in flow_defs if d['section']=='KYC' and d['country'] in ['*','IN'] and 'Buyer' in d['categories']]
    renewal=next(identifier for identifier,label,_ in defs if label=='Flow QA Import only')
    profile['fields'].append({'id':renewal,'value':'QA IMPORT RENEWAL','expiry':True,'valid_from':datetime.date.today().isoformat(),'expires_on':expires})
    save({'business_flows':['Domestic','Import']},profile=profile)
    entry=finance.call(selected_path)
    finance.call(selected_path,{'action':'submit','version':entry['version']})
    md.call(selected_path,{'action':'approve','version':finance.call(selected_path)['version']})
    check(sql(f"SELECT count(*) FROM company_reminders WHERE company_id={selected_id} AND active AND obligation_key='field:{renewal}'",name)=='1','Mixed Domestic/Import approval schedules applicable expiry reminder')
    check('QA IMPORT RENEWAL' not in json.dumps(md.call(selected_path)),'Mixed-flow approval summary preserves KYC privacy')
    # Remove Import via the normal amendment workflow; old reminder remains until approval.
    finance.call(selected_path,{'action':'amend','version':finance.call(selected_path)['version']})
    domestic_profile={**profile,'fields':[f for f in profile['fields'] if f['id']!=renewal and next((d['label'] for d in flow_defs if d['id']==f['id']),'')!='IEC']}
    save({'business_flows':['Domestic']},profile=domestic_profile)
    check(sql(f"SELECT count(*) FROM company_reminders WHERE company_id={selected_id} AND active",name)=='1','Flow amendment preserves approved reminder until reapproval')
    finance.call(selected_path,{'action':'submit','version':finance.call(selected_path)['version']})
    md.call(selected_path,{'action':'approve','version':finance.call(selected_path)['version']})
    check(sql(f"SELECT count(*) FROM company_reminders WHERE company_id={selected_id} AND active",name)=='0','Reapproved Domestic-only flow deactivates obsolete Import renewal')
    check(any(h['action']=='approve' for h in finance.call(selected_path)['history']),'Mixed-flow transitions retain company audit history')

run_company_business_flow_cases()
