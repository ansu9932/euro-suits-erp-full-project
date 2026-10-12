#!/usr/bin/env python3
"""Add team and renewal examples only to the isolated localhost:8082 UAT workspace."""
from client import *
a=next(x for x in state['accounts'] if x['username']=='admin');admin=Client(a['username'],a['password'])
def account(name):
 a=next(x for x in state['accounts'] if x['username']==name);return Client(a['username'],a['password'])
finance=account('finance');md=account('md');meta=admin.call('/setup-users');company=state['company']['id']
team=next((t for t in meta['teams'] if t['name']=='UAT Team C' and t['company_id']==company),None)
if team is None:team={'id':admin.call('/access-resources',{'kind':'teams','company_id':company,'name':'UAT Team C'})['id']}
teamid=team['id'];state['resources']['team_c']=teamid;save()
if not any(b['name']=='UAT Brand Delta' and b['team_id']==teamid for b in meta['buyers']):admin.call('/access-resources',{'kind':'buyers','company_id':company,'team_id':teamid,'name':'UAT Brand Delta'})
u=next(u for u in meta['users'] if u['username']=='test.regional.marketing.manager')
if teamid not in u['teams']:
 admin.call('/setup-users/'+str(u['id']),{**u,'features':u['setup_features'],'permissions':u['effective_permissions'],'teams':list(dict.fromkeys(u['teams']+[state['resources']['team_a'],state['resources']['team_b'],teamid]))})
print('Team C and Brand Delta ready; regional manager assigned Teams A, B and C.')
# One dedicated example company; never amend real company records or change system time.
name='UAT Renewal Documents - Practice';register=finance.call('/company-setup');existing=next((c for c in register['companies'] if c['legal_name']==name),None)
header={'legal_name':name,'category':'Buyer','company_type':'Retailer','country':'GB','city':'Test London','business_mode':'Domestic'}
cid=existing['id'] if existing else finance.call('/company-setup',header)['id'];path='/company-setup/'+str(cid)
responsible=next(u for u in meta['users'] if u['username']=='test.trainee.intern')
detail=admin.call(path)
if detail['responsible_user_id']!=responsible['id']:admin.call(path+'/responsibility',{'user_id':responsible['id'],'version':detail['version']})
scenarios=[('UAT certificate - due in 7 days',7),('UAT certificate - expired 7 days ago',-7),('UAT certificate - expired 31 days ago',-31)]
# A small valid PDF with a single synthetic-test label, generated as a fixture.
def pdf(label,expiry):
 stream=('BT /F1 16 Tf 50 760 Td (SYNTHETIC UAT DOCUMENT) Tj 0 -30 Td /F1 11 Tf ('+label+') Tj 0 -25 Td (Expires: '+expiry+') Tj 0 -25 Td (Not a real legal certificate. Test use only.) Tj ET').encode()
 objects=[b'<< /Type /Catalog /Pages 2 0 R >>',b'<< /Type /Pages /Kids [3 0 R] /Count 1 >>',b'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',b'<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',b'<< /Length '+str(len(stream)).encode()+b' >>\nstream\n'+stream+b'\nendstream']
 out=b'%PDF-1.4\n';offsets=[0]
 for i,obj in enumerate(objects,1):offsets.append(len(out));out+=str(i).encode()+b' 0 obj\n'+obj+b'\nendobj\n'
 start=len(out);out+=b'xref\n0 6\n0000000000 65535 f \n'+b''.join(f'{n:010} 00000 n \n'.encode() for n in offsets[1:]);return out+b'trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n'+str(start).encode()+b'\n%%EOF\n'
detail=finance.call(path)
if detail['status']=='Draft':
 fields=[];today=datetime.date.today()
 for index,(label,delta) in enumerate(scenarios):
  defs=admin.call('/company-setup')['fields'];definition=next((f for f in defs if f['label']==label),None)
  fid=definition['id'] if definition else admin.call('/company-fields',{'label':label,'section':'Additional','country':'*','categories':['Buyer'],'input_type':'text','required':False,'document_required':True,'expiry_enabled':True})['id']
  expiry=(today+datetime.timedelta(days=delta)).isoformat();boundary='uat-renewal-example';filename='UAT-renewal-'+str(index+1)+'.pdf';content=pdf(label,expiry)
  multipart=(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\nContent-Type: application/pdf\r\n\r\n'.encode()+content+f'\r\n--{boundary}--\r\n'.encode())
  fileid=finance.call(path+'/files',multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})['id']
  fields.append({'id':fid,'value':'UAT CERTIFICATE '+str(index+1),'expiry':True,'valid_from':(today-datetime.timedelta(days=365)).isoformat(),'expires_on':expiry,'documents':[fileid]})
 fields += [{'id':f['id'],'not_applicable':True,'reason':'Synthetic renewal practice company only'} for f in finance.call('/company-setup')['fields'] if f['section']=='KYC' and f['country'] in ['*','GB'] and 'Buyer' in f['categories'] and (not f.get('business_modes') or 'Domestic' in f['business_modes'])]
 profile={'address':'Synthetic UAT address','owner':'UAT owner','email':'renewals@example.test','phone':'0000000000','reminder_email':'renewals@example.test','sample_address':'Synthetic sample delivery address','fields':fields}
 finance.call(path,{'action':'save','version':finance.call(path)['version'],**header,'profile':profile});finance.call(path,{'action':'submit','version':finance.call(path)['version']})
if md.call(path)['status']=='Pending':md.call(path,{'action':'approve','version':md.call(path)['version']})
reminders=[r for r in finance.call('/company-setup')['reminders'] if r['company_id']==cid];assert len(reminders)==3
blocked=account('test.trainee.intern');assert blocked.call('/session')['user']['renewal_blocks'];blocked.call('/data',expect=423)
for client in [admin,finance,md]:assert not client.call('/session')['user']['renewal_blocks']
merchant=account('test.junior.merchant');merchant.call('/access-resources',{'kind':'teams','company_id':company,'name':'Not authorized'},expect=403)
manager=account('test.regional.marketing.manager');assert set(manager.call('/session')['user']['teams'])=={state['resources']['team_a'],state['resources']['team_b'],teamid}
state['renewal_example']={'company_id':cid,'company_name':name,'blocked_username':'test.trainee.intern','documents':[{'label':r['label'],'expires_on':r['expires_on']} for r in reminders]};save()
(PRIVATE/'team-renewal-check.json').write_text(json.dumps({'team_c':teamid,'manager_teams':'A, B, C','renewal_example':state['renewal_example'],'checks':['team and brand created','manager has three assigned teams','merchant cannot create teams','three reminders visible','31-day overdue account blocked','admin finance md remain unblocked'],'at':datetime.datetime.now().isoformat()},indent=2))
print('PASS: three renewal reminders, trainee blocked after 31 overdue days, main accounts unaffected; admin-only team creation verified.')
