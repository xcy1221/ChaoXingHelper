#!/usr/bin/env python3
"""Root-only device administration. Private material stays outside web roots."""
import argparse
import datetime
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import time

ROOT = Path('/home/docker/services/caddy')
BACKEND = Path('/home/docker/services/chaoxing-helper/data/account')
PRIVATE = ROOT / 'aigc-access/private'
PUBLIC = ROOT / 'aigc-access/public'
IP = '221.216.94.213'
PORT = 18443


def run(args, data=None):
    result = subprocess.run(args, input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        raise RuntimeError(result.stderr.decode(errors='replace')[-2000:])
    return result.stdout


def atomic(path, content, mode=0o600, uid=None):
    tmp = path.with_name(path.name + '.tmp')
    tmp.write_bytes(content.encode() if isinstance(content, str) else content)
    tmp.chmod(mode)
    if uid is not None:
        os.chown(tmp, uid, uid)
    os.replace(tmp, path)


def accounts():
    p = PRIVATE / 'accounts.json'
    return json.loads(p.read_text()) if p.exists() else {}


def initialize():
    if os.geteuid() != 0:
        raise RuntimeError('Run with sudo.')
    os.umask(0o077)
    for p in [PRIVATE, PUBLIC, PUBLIC / 'clients']:
        p.mkdir(parents=True, exist_ok=True)
    PUBLIC.chmod(0o755)
    (PUBLIC / 'clients').chmod(0o755)
    key = PRIVATE / 'client-ca.key'
    cert = PUBLIC / 'client-ca.crt'
    if not key.exists():
        run(['openssl', 'req', '-x509', '-newkey', 'rsa:3072', '-noenc', '-sha256', '-days', '1825',
             '-subj', '/CN=ChaoXing Helper Device CA', '-addext', 'basicConstraints=critical,CA:TRUE',
             '-addext', 'keyUsage=critical,keyCertSign,cRLSign', '-addext', 'extendedKeyUsage=clientAuth', '-keyout', str(key), '-out', str(cert)])
        cert.chmod(0o644)
    if not cert.exists():
        raise RuntimeError('Client CA certificate missing; do not replace its key.')
    BACKEND.mkdir(parents=True, exist_ok=True)
    token = BACKEND / 'public-gateway-token'
    if not token.exists():
        atomic(token, secrets.token_urlsafe(48), uid=65532)
    if not (PRIVATE / 'accounts.json').exists():
        atomic(PRIVATE / 'accounts.json', '{}\n')


def save(users):
    users = {name: {'expires': u['expires']} for name, u in users.items()}
    atomic(PRIVATE / 'accounts.json', json.dumps(users, indent=2)+'\n')
    active = {name: u['expires'] for name, u in users.items() if u['expires'] > time.time()}
    atomic(BACKEND / 'public-users.json', json.dumps(active)+'\n', uid=65532)


def grant(name, csr, days):
    users = accounts()
    if name in users:
        raise RuntimeError('Account already exists; revoke it before replacing its device.')
    if not 1 <= days <= 365:
        raise RuntimeError('Certificate lifetime must be 1..365 days.')
    run(['openssl', 'req', '-in', str(csr), '-verify', '-noout'])
    package = PRIVATE / ('packages/' + name)
    package.mkdir(parents=True, exist_ok=True)
    ext = package / 'client.ext'
    atomic(ext, 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\n')
    cert = PUBLIC / 'clients' / (name + '.pem')
    run(['openssl', 'x509', '-req', '-in', str(csr), '-CA', str(PUBLIC/'client-ca.crt'),
         '-CAkey', str(PRIVATE/'client-ca.key'), '-set_serial', str(secrets.randbits(128)),
         '-days', str(days), '-sha256', '-subj', '/CN='+name, '-extfile', str(ext), '-out', str(cert)])
    cert.chmod(0o644)
    expires = int(time.time()) + days*86400
    users[name] = {'expires': expires}
    save(users)
    atomic(package / 'device.crt', cert.read_bytes())
    root_cert = ROOT / 'data/caddy/pki/authorities/local/root.crt'
    atomic(package / 'server-root.crt', root_cert.read_bytes())
    atomic(package / 'client-ca.crt', (PUBLIC/'client-ca.crt').read_bytes())
    (package/'credentials.txt').unlink(missing_ok=True)
    atomic(package/'connection.txt', f'URL=https://{IP}:{PORT}/\nDEVICE={name}\nEXPIRES={expires}\n')
    print(f'Created {name}; private delivery package: {package}')


def render(reload=False):
    users = accounts()
    save(users)
    (ROOT/'data/aigc-logs').mkdir(mode=0o700, exist_ok=True)
    for package in (PRIVATE/'packages').glob('*'):
        if package.is_dir():
            (package/'credentials.txt').unlink(missing_ok=True)
            if package.name in users:
                atomic(package/'connection.txt', f'URL=https://{IP}:{PORT}/\nDEVICE={package.name}\nEXPIRES={users[package.name]["expires"]}\n')
    token = (BACKEND/'public-gateway-token').read_text().strip()
    device_map = '\n'.join('            CN='+name+' '+name for name, u in users.items() if u['expires'] > time.time())
    lan = f'''{{
    default_sni 192.168.1.71
}}
https://192.168.1.71 {{
    tls internal
    route {{
        @outside not remote_ip 192.168.1.0/24 127.0.0.1 ::1
        respond @outside "LAN only" 403
        redir /server-readme /server-readme/ 308
        redir /chaoxing /chaoxing/ 308
        handle_path /server-readme/* {{
            reverse_proxy server-readme:8080
        }}
        handle_path /chaoxing/* {{
            reverse_proxy chaoxing-helper:8080 {{
                header_up X-Aigc-Gateway {token}
                header_up X-Aigc-Scope internal
                header_up X-Aigc-User ""
                header_up X-Aigc-Client-IP {{http.request.remote.host}}
            }}
        }}
        handle {{
            reverse_proxy openclaw-gateway:18789
        }}
    }}
}}
'''
    external = f'''
https://{IP}:{PORT} {{
    log aigc_external {{
        output file /data/aigc-logs/access.jsonl {{
            mode 0600
            roll_size 5MiB
            roll_keep 3
            roll_keep_for 720h
        }}
        format json
    }}
    tls internal {{
        client_auth {{
            mode require_and_verify
            trust_pool file /etc/caddy/aigc-access/client-ca.crt
            verifier leaf folder /etc/caddy/aigc-access/clients
        }}
    }}
    route {{
        map {{http.request.tls.client.subject}} {{aigc_device_user}} {{
{device_map}
            default ""
        }}
        log_append device {{aigc_device_user}}
        @unknownDevice expression `{{aigc_device_user}} == ""`
        respond @unknownDevice "Device not authorized" 403
        @private path /records* /api/records* /api/access-records* /server-readme* /chaoxing* /admin*
        respond @private "Not found" 404
        reverse_proxy chaoxing-helper:8080 {{
            header_up X-Aigc-Gateway {token}
            header_up X-Aigc-Scope external
            header_up X-Aigc-User {{aigc_device_user}}
            header_up X-Aigc-Client-IP {{http.request.remote.host}}
        }}
    }}
}}
'''
    text = lan + external
    config = json.loads(run(['docker','exec','-i','caddy','caddy','adapt','--config','/dev/stdin','--adapter','caddyfile'], text.encode()))
    found = 0
    for server in config['apps']['http']['servers'].values():
        if ':'+str(PORT) in server['listen']:
            found += 1
            policies = [p for p in server['tls_connection_policies'] if p.get('client_authentication')]
            if len(policies) != 1:
                raise RuntimeError('Public listener must require client authentication.')
            policy = policies[0]
            policy.pop('match', None)
            policy['default_sni'] = IP
            server['tls_connection_policies'] = [policy]
            # Numeric IP browsers omit SNI; this listener has a single host and backend.
            server['strict_sni_host'] = False
    if found != 1:
        raise RuntimeError('Unexpected public listener configuration.')
    candidate = ROOT / 'config/managed.next.json'
    atomic(candidate, json.dumps(config))
    run(['docker','run','--rm','--network','none','--security-opt','label=disable',
         '-v',str(PUBLIC)+':/etc/caddy/aigc-access:ro', '-v',str(ROOT/'config')+':/config:ro',
         '-v',str(ROOT/'data')+':/data:ro', '-v',str(ROOT/'data/aigc-logs')+':/data/aigc-logs:rw','-e','XDG_DATA_HOME=/data','-e','XDG_CONFIG_HOME=/config',
         'caddy:2-alpine','caddy','validate','--config','/config/managed.next.json'])
    atomic(ROOT/'Caddyfile', text)
    os.replace(candidate, ROOT/'config/managed.json')
    if reload:
        run(['docker','exec','caddy','caddy','reload','--config','/config/managed.json','--force'])
    print('Rendered HTTPS configuration.')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    sub = p.add_subparsers(dest='command', required=True)
    sub.add_parser('init')
    g = sub.add_parser('grant'); g.add_argument('user'); g.add_argument('csr', type=Path); g.add_argument('--days',type=int,default=90)
    r = sub.add_parser('revoke'); r.add_argument('user')
    sub.add_parser('list')
    r = sub.add_parser('render'); r.add_argument('--reload',action='store_true')
    a = p.parse_args()
    initialize()
    if hasattr(a, 'user') and not re.fullmatch(r'[A-Za-z0-9_-]{1,40}', a.user):
        p.error('Invalid user name.')
    if a.command == 'grant':
        grant(a.user, a.csr, a.days)
    elif a.command == 'revoke':
        users = accounts(); users.pop(a.user, None)
        save(users)
        (PUBLIC/'clients'/ (a.user+'.pem')).unlink(missing_ok=True)
        render(reload=True)
        print('Revoked '+a.user)
    elif a.command == 'render':
        render(a.reload)
    elif a.command == 'list':
        for name, u in accounts().items():
            print(name, datetime.datetime.fromtimestamp(u['expires'],datetime.timezone.utc).isoformat())


if __name__ == '__main__':
    main()
