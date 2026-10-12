#!/usr/bin/env python3
"""Repeatable local checks. Scanners run locally; no source is uploaded.
Requires npm, go, govulncheck and gitleaks on PATH.
Supports GO, GOVULNCHECK and GITLEAKS executable path overrides.
"""
import json, os, shutil, subprocess, tempfile
from pathlib import Path

root=Path(__file__).resolve().parents[1]
reports=root/'docs/security-results';reports.mkdir(parents=True,exist_ok=True)
failed=[]
def run(label,args,cwd,report):
    try: result=subprocess.run(args,cwd=cwd,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
    except FileNotFoundError: failed.append(label+' (tool not installed)');return
    (reports/report).write_text(result.stdout)
    print(label+': '+('PASS' if result.returncode==0 else 'REVIEW'))
    if result.returncode:failed.append(label)
run('npm audit',['npm','audit','--json'],root/'frontend','npm-audit.json')
run('Go vulnerability scan',[os.getenv('GOVULNCHECK','govulncheck'),'-show','verbose','./...'],root/'backend','go-vulnerabilities.txt')
# Match each private configured value against the application source and built
# browser bundle, reporting only variable names and paths, never the value.
secrets={}
for envfile in root.glob('.env*'):
    if envfile.name.endswith('.example'):continue
    for line in envfile.read_text().splitlines():
        if '=' not in line or line.startswith('#'):continue
        key,value=line.split('=',1)
        if any(word in key for word in ['SECRET','PASSWORD','ENCRYPTION_KEY']) and len(value)>=16:secrets[key]=value.encode()
    if envfile.stat().st_mode & 0o077:failed.append(envfile.name+' must have mode 600')
findings=[]
for folder in [root/'frontend/src',root/'frontend/static',root/'frontend/build/client',root/'backend']:
    for p in folder.rglob('*'):
        if p.is_file() and p.name!='erp-server' and '/files/' not in str(p):
            data=p.read_bytes()
            for key,value in secrets.items():
                if value in data:findings.append({'variable':key,'file':str(p.relative_to(root))})
(reports/'configured-secret-scan.json').write_text(json.dumps({'findings':findings},indent=2))
print('Configured secret exposure: '+('REVIEW' if findings else 'PASS'))
if findings:failed.append('Configured secret exposure')
with tempfile.TemporaryDirectory(prefix='erp-source-scan-') as temp:
    destination=Path(temp)
    ignored={'node_modules','.git','.svelte-kit','build','backups','certs','files','.claude','security-results','__pycache__'}
    for parent,dirs,files in os.walk(root):
        dirs[:]=[d for d in dirs if d not in ignored]
        for filename in files:
            if filename.startswith('.env') and not filename.endswith('.example'):continue
            if filename in {'LOCAL-ACCESS.txt','erp-server','.DS_Store'} or filename.endswith('.log'):continue
            source=Path(parent)/filename;target=destination/source.relative_to(root);target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(source,target)
    run('Source secret scan',[os.getenv('GITLEAKS','gitleaks'),'dir','--redact','--no-banner','--report-format','json','--report-path',str(reports/'gitleaks.json'),temp],root,'gitleaks-log.txt')
if (root/'.git').exists():
    run('Git history secret scan',[os.getenv('GITLEAKS','gitleaks'),'git','--redact','--no-banner','--report-format','json','--report-path',str(reports/'git-history.json'),'.'],root,'git-history-log.txt')
else:
    (reports/'git-history-log.txt').write_text('No Git repository exists in this project; no history to scan or rewrite.\n')
if failed:raise SystemExit('Review required: '+', '.join(failed))
