"""Local read-only health: correct chain/configuration and process responsiveness."""
import json,os,socket,sys
from pathlib import Path
ROOT=Path('/opt/rgn')
def rpc(data,m,p):
 if m not in ('admin_nodeInfo','net_version','eth_getBlockByNumber','eth_syncing','net_peerCount','eth_mining','eth_coinbase'):raise ValueError('Read-only check required')
 with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as s:
  s.settimeout(3);s.connect(str(data/'egem.ipc'));s.sendall((json.dumps(dict(jsonrpc='2.0',id=1,method=m,params=p))+'\n').encode());raw=b''
  while len(raw)<1024*1024:
   b=s.recv(65536)
   if not b:raise ValueError('IPC closed')
   raw+=b
   if b'\n' in raw:
    v=json.loads(raw.split(b'\n',1)[0])
    if v.get('error') or v.get('id')!=1:raise ValueError('RPC check failed')
    return v['result']
  raise ValueError('Response too large')
def check(data,details=False):
 info=rpc(data,'admin_nodeInfo',[]);cfg=info['protocols']['eth']['config'];expected=json.loads((ROOT/'RGN-mainnet-v4.1.0-H1121800.json').read_text())['config']
 if cfg!=expected or rpc(data,'net_version',[])!='70088' or rpc(data,'eth_getBlockByNumber',['0x0',False])['hash']!='0xdc4d38c0ef41d3fdf50642fdf5fc99e221eadd09e73bbe616724f55a136270ab':raise ValueError('Chain/configuration differs')
 r=dict(identityAndForkConfigMatch=True)
 if details:
  head=rpc(data,'eth_getBlockByNumber',['latest',False]);r.update(nodeId=info['id'],height=int(head['number'],16),peers=int(rpc(data,'net_peerCount',[]),16),syncing=rpc(data,'eth_syncing',[]),mining=rpc(data,'eth_mining',[]))
  if os.environ.get('WALLET'):r['beneficiary']=rpc(data,'eth_coinbase',[])
 return r
if __name__=='__main__':
 try:
  r=check(Path(os.environ.get('RGN_DATADIR','/var/lib/rgn')),'--quiet' not in sys.argv)
  if '--quiet' not in sys.argv:print(json.dumps(r,indent=2))
 except Exception:print('RGN local identity/configuration check is not ready',file=sys.stderr);sys.exit(1)
