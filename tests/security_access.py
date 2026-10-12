# Runs in the isolated access test, while both team orders still exist.
identity={'id':merch_a_id,'role':'Marketing','view_orders':True,'scope':'team','companies':[opscompany],'teams':[team_a],'lines':[],'orders':[],'units':[]}
def scoped_sql(statement,identity=None):
    claims=json.dumps(identity or {}).replace("'","''")
    return sql("BEGIN; SET LOCAL ROLE euroerp_runtime; SELECT set_config('erp.identity','"+claims+"',true); "+statement+"; COMMIT;",name).splitlines()
check(str(po_a) in scoped_sql('SELECT id FROM orders',identity) and str(po_b) not in scoped_sql('SELECT id FROM orders',identity),'Database RLS filters a query even without an application WHERE clause')
check('0' in scoped_sql('SELECT count(*) FROM orders'),'Database returns no orders without request identity')
line_identity={**identity,'role':'Production','scope':'line','teams':[],'lines':[line_a]}
check(str(po_a) in scoped_sql('SELECT id FROM orders',line_identity) and str(po_b) not in scoped_sql('SELECT id FROM orders',line_identity),'Database enforces assigned sewing lines')
for statement in ['ALTER TABLE orders DISABLE ROW LEVEL SECURITY','CREATE TABLE unsafe_table(id int)','DELETE FROM access_history','UPDATE schema_versions SET version=99']:
    try:scoped_sql(statement,identity);raise AssertionError('Runtime privilege escalation: '+statement)
    except subprocess.CalledProcessError:check(True,'Runtime denies '+statement.split()[0]+' on '+statement.split()[2])
# Optimised RLS must return exactly the same records as the original v10 helpers.
legacy="erp_order_allowed(company_id,team_id,line_id,id,finishing_unit) AND erp_buyer_allowed(buyer_id) AND erp_area_allowed(company_id,cutting_table,sub_line)"
claims_cases=[{},identity,line_identity,
 {**identity,'scope':'company'},
 {**identity,'companies':[]},
 {**identity,'view_orders':False},
 {**identity,'teams':[team_b]},
 {**identity,'teams':[],'orders':[po_b]},
 {**identity,'scope':'po','orders':[po_b]},
 {**identity,'scope':'po','orders':[]},
 {**identity,'scope':'company','restrict_buyers':True,'buyer_ids':[buyer_a]},
 {**identity,'scope':'company','restrict_buyers':True,'buyer_ids':[]},
 {**identity,'role':'Finishing','scope':'company','units':['Suits']},
 {**identity,'role':'Finishing','scope':'company','units':[]},
 {**identity,'scope':'cutting_table','production_areas':[]},
 {**identity,'scope':'finishing_line','production_areas':[]},
 {**identity,'role':'Super Admin','companies':[],'view_orders':False}]
for claims in claims_cases:
 raw=json.dumps(claims).replace("'","''")
 query="SELECT COALESCE(string_agg(id::text,',' ORDER BY id),'EMPTY') FROM orders"
 expected=sql("BEGIN;SELECT set_config('erp.identity','"+raw+"',true);"+query+" WHERE "+legacy+";ROLLBACK;",name).splitlines()[-2]
 actual=scoped_sql(query,claims)[-2]
 check(actual==expected,'Optimised database policy matches legacy access: '+str(claims.get('role','no identity'))+'/'+str(claims.get('scope','none')))
check(sql("SELECT bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE relname IN ('orders','documents','company_setups','company_setup_files','purchases')",name)=='t','Sensitive record tables force row security')
def file_body(filename,content):
    return (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\nContent-Type: application/octet-stream\r\n\r\n'.encode()+content+f'\r\n--{boundary}--\r\n'.encode())
upload_headers={'Content-Type':'multipart/form-data; boundary='+boundary}
for filename,content in [('renamed.pdf',b'<script>alert(1)</script>'),('fake.png',b'MZ executable'),('blocked.html',b'html')]:
    merch_a.call('/orders/'+str(po_a)+'/documents',file_body(filename,content),400,upload_headers)
doc=merch_a.call('/orders/'+str(po_a)+'/documents',multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})['id']
check(merch_a.call('/documents/'+str(doc))==payload,'Encrypted operational document downloads with original bytes')
merch_b.call('/documents/'+str(doc),expect=404)
key=sql('SELECT storage_key FROM documents WHERE id='+str(doc),name)
check(payload not in Path(temp+'/files',key).read_bytes(),'Operational documents are encrypted at rest')
check('storage_key' not in json.dumps(merch_a.order(po_a)),'Document responses do not expose storage paths')
# Bounds, field tampering, CSRF, content-type and parameter binding.
merch_a.call('/orders/'+str(po_a),{'action':'advance','version':1,'created_by':999},400)
merch_a.call('/orders/'+str(po_a),b'{"action":"allocate","action":"advance"}',400,{'Content-Type':'application/json'})
merch_a.call('/orders/'+str(po_a),{},403,{'X-CSRF-Token':'wrong'})
merch_a.call('/orders/'+str(po_a),{},403,{'Origin':'https://attacker.test'})
merch_a.call('/orders/'+str(po_a),{},415,{'Content-Type':'application/json-evil'})
merch_a.call('/data',expect=403,headers={'Sec-Fetch-Site':'cross-site'})
Client().call('/login',{'username':"x' OR 1=1 --",'password':password},401)
check(sql('SELECT count(*)>0 FROM users',name)=='t','Injection-shaped username cannot change authentication query')
response=merch_a.opener.open(urllib.request.Request(base+'/api/data',headers={'Origin':os.getenv('COMPANY_TEST_ORIGIN',base)}))
check(response.headers.get('Cache-Control')=='no-store' and response.headers.get('X-Content-Type-Options')=='nosniff','Sensitive responses cannot be cached or MIME-sniffed')
check("default-src 'none'" in response.headers.get('Content-Security-Policy',''),'API response has a restrictive CSP')
cookies=next(h.cookiejar for h in merch_a.opener.handlers if isinstance(h,urllib.request.HTTPCookieProcessor))
session_cookie=next(c for c in cookies if c.name=='erp_session')
check(session_cookie.has_nonstandard_attr('HttpOnly') and session_cookie.get_nonstandard_attr('SameSite')=='Lax','Session cookie is HttpOnly and SameSite')
old_token=session_cookie.value
merch_a.call('/login',{'username':'qa.merch.a','password':password})
Client().call('/data',expect=401,headers={'Cookie':'erp_session='+old_token})
# The per-account limit survives a new client/session and spoofed proxy headers.
for attempt in range(9):Client().call('/login',{'username':'qa.security.missing','password':'wrong'},429 if attempt==8 else 401,{'X-Real-IP':f'203.0.113.{attempt+1}'})
# Unauthenticated requests cannot see the configured server-side verification secret.
check(set(Client().call('/login-config'))=={'turnstile_site_key'},'Bot configuration returns only the public site key')
