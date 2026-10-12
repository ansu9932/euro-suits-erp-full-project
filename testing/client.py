#!/usr/bin/env python3
"""Populate the dedicated UAT deployment through the same APIs the UI uses.
Never points at port 8080 or uses the live application's .env.
Private manifest preserves generated passwords across interrupted runs.
"""
from pathlib import Path
import json,os,secrets,datetime,http.cookiejar,urllib.request,urllib.error,time,re
ROOT=Path(__file__).resolve().parent;PRIVATE=ROOT/'private';os.umask(0o077)
BASE='http://localhost:8082';env=dict(x.split('=',1) for x in (PRIVATE/'environment.env').read_text().splitlines() if '=' in x)
statefile=PRIVATE/'accounts.json';state=json.loads(statefile.read_text()) if statefile.exists() else {'accounts':[],'resources':{},'orders':{}}
def save():statefile.write_text(json.dumps(state,indent=2))
class Client:
 def __init__(self,username=None,password=None):
  self.o=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()));self.csrf=''
  if username:self.call('/login',{'username':username,'password':password})
 def call(self,path,body=None,expect=200,headers=None):
  h={'Origin':BASE,'X-CSRF-Token':self.csrf,**(headers or {})}
  if isinstance(body,dict):body=json.dumps(body).encode();h['Content-Type']='application/json'
  req=urllib.request.Request(BASE+'/api'+path,data=body,headers=h)
  try:r=self.o.open(req,timeout=40)
  except urllib.error.HTTPError as e:r=e
  raw=r.read();d=json.loads(raw) if 'application/json' in r.headers.get('Content-Type','') else raw
  if r.status!=expect:raise RuntimeError(f'{path}: HTTP {r.status}: '+str(d)[:220])
  if isinstance(d,dict) and d.get('csrf'):self.csrf=d['csrf']
  return d
 def action(self,oid,action,**kw):return self.call('/orders/'+str(oid),{'action':action,'version':self.call('/orders/'+str(oid))['version'],**kw})
