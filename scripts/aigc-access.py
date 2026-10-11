"""Administer the deployed AIGC entrance through the existing serverctl connection."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import secrets


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--serverctl-root',type=Path,default=Path(r'F:\xcy\projects\normal_project\home_server'))
    sub = p.add_subparsers(dest='command',required=True)
    grant = sub.add_parser('grant')
    grant.add_argument('user'); grant.add_argument('csr',type=Path)
    grant.add_argument('--days',type=int,default=90); grant.add_argument('--out',type=Path)
    revoke = sub.add_parser('revoke'); revoke.add_argument('user')
    sub.add_parser('list')
    a = p.parse_args()
    import re
    if hasattr(a,'user') and not re.fullmatch(r'[A-Za-z0-9_-]{1,40}',a.user):
        p.error('Invalid account name.')
    root = a.serverctl_root.resolve()
    spec = importlib.util.spec_from_file_location('serverctl',root/'scripts/serverctl.py')
    ctl = importlib.util.module_from_spec(spec); spec.loader.exec_module(ctl)
    manager = ctl.Manager(root=root)
    helper = '/home/docker/services/caddy/public-access.py'
    lock = root/'.private/operation.lock'
    fd = os.open(lock,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
    with os.fdopen(fd,'w') as stream:
        json.dump({'pid':os.getpid(),'operation':'aigc-access-'+a.command},stream)
    outcome = 'failed'
    try:
        manager.guard()
        if a.command == 'grant':
            csr = a.csr.resolve(strict=True)
            output = (a.out or Path(__file__).resolve().parent.parent/'.inspection/deliveries'/a.user).resolve()
            if (output/'device.crt').exists():
                raise RuntimeError('Delivery package already exists; choose another output directory.')
            stage = manager.staging+'/aigc-csr-'+secrets.token_hex(8)
            manager.remote.run(['install','-d','-m','700',stage])
            manager.remote.run(['python3','-c',f'import sys; from pathlib import Path; Path({stage+"/device.csr"!r}).write_bytes(sys.stdin.buffer.read())'],input_file=csr)
            manager.remote.run(['python3',helper,'grant',a.user,stage+'/device.csr','--days',str(a.days)])
            try:
                manager.remote.run(['python3',helper,'render','--reload'])
            except Exception:
                manager.remote.run(['python3',helper,'revoke',a.user],check=False)
                raise
            output.mkdir(parents=True,exist_ok=True)
            package = '/home/docker/services/caddy/aigc-access/private/packages/'+a.user
            for name in ['device.crt','server-root.crt','client-ca.crt','connection.txt']:
                manager.remote.run(['cat',package+'/'+name],output=output/name)
            print('Device enabled. Deliver this certificate package only to its owner:',output)
        else:
            args = ['python3',helper,a.command]
            if hasattr(a,'user'):args.append(a.user)
            print(manager.remote.run(args))
        outcome = 'success'
    finally:
        with (root/'state/actions.jsonl').open('a',encoding='utf-8') as stream:
            stream.write(json.dumps({'time':ctl.now(),'operation':'aigc-access-'+a.command,'result':outcome})+'\n')
        lock.unlink()


if __name__ == '__main__':
    main()
