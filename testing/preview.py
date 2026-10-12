#!/usr/bin/env python3
"""Manage only this project's protected ngrok UAT preview."""
from pathlib import Path
import subprocess,json,os,signal,time,re,sys,urllib.request
root=Path(__file__).resolve().parent;p=root/'private';os.umask(0o077)
compose=['docker','compose','--env-file',str(p/'environment.env'),'-f',str(root/'compose.yaml')]
process=p/'ngrok-process.json'
service='com.eurosuits.uat-preview'
# Remove only this ERP service before an explicit restart or stop.
subprocess.run(['launchctl','remove',service],capture_output=True)
if process.exists():
 pid=json.loads(process.read_text())['pid'];command=subprocess.run(['ps','-p',str(pid),'-o','command='],capture_output=True,text=True).stdout.strip()
 if command.startswith('/opt/homebrew/bin/ngrok http http://127.0.0.1:8083'):os.kill(pid,signal.SIGTERM)
 process.unlink();time.sleep(.5)
if len(sys.argv)>1 and sys.argv[1]=='stop':
 subprocess.run(compose+['stop','gateway-online','api-online'],check=True);print('Online preview stopped; local test workspace and data retained.');sys.exit()
subprocess.run(compose+['up','-d','postgres','redis','api-local','web','gateway-local'],check=True)
# Do not interfere with an ngrok agent belonging to another task.
try:
 urllib.request.urlopen('http://127.0.0.1:4040/api/tunnels',timeout=1)
except Exception:pass
else:raise RuntimeError('Another ngrok agent is already using port 4040. No changes made to it.')
# launchd owns the process so it outlives the command/chat session.
subprocess.run(['launchctl','submit','-l',service,'-o',str(p/'ngrok-service.log'),'-e',str(p/'ngrok-service-error.log'),'--','/opt/homebrew/bin/ngrok','http','http://127.0.0.1:8083','--inspect=false','--log=stdout','--log-format=json'],check=True)
status=subprocess.run(['launchctl','list',service],capture_output=True,text=True,check=True).stdout
match=re.search(r'"PID" = (\d+)',status)
process.write_text(json.dumps({'pid':int(match.group(1)) if match else 0,'service':service}))
url=None
for _ in range(100):
 try:
  data=json.load(urllib.request.urlopen('http://127.0.0.1:4040/api/tunnels',timeout=1))
  url=next((t['public_url'] for t in data['tunnels'] if t['config']['addr']=='http://127.0.0.1:8083' and t['public_url'].startswith('https://')),None)
 except Exception:pass
 if url:break
 time.sleep(.3)
if not url:raise RuntimeError('No public preview URL available; inspect testing/private/ngrok-service.log')
e=p/'environment.env';e.write_text(re.sub(r'^PUBLIC_ORIGIN=.*\n?','',e.read_text(),flags=re.M).rstrip()+'\nPUBLIC_ORIGIN='+url+'\n')
subprocess.run(compose+['up','-d','api-online','gateway-online'],check=True)
subprocess.run(compose+['exec','-T','gateway-online','nginx','-s','reload'],check=True)
subprocess.run([sys.executable,str(root/'report.py')],check=True)
print('Preview URL: '+url)
