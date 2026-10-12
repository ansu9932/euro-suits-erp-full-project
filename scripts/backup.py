#!/usr/bin/env python3
"""Back up the Docker deployment's database and append-only document store locally."""
import pathlib,subprocess,datetime,hashlib,os
root=pathlib.Path(__file__).resolve().parents[1]
os.umask(0o077)
folder=root/'backups'/datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
folder.mkdir(parents=True)
for name,command in [
 ('database.dump',['docker','compose','exec','-T','postgres','pg_dump','-U','euroerp','-d','euroerp','-Fc']),
 ('documents.tar.gz',['docker','compose','exec','-T','api','tar','-czf','-','-C','/data/files','.'])]:
 with (folder/name).open('wb') as out: subprocess.run(command,cwd=root,stdout=out,check=True)
(folder/'SHA256SUMS').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in sorted(folder.iterdir()) if p.is_file()))
print(folder)
