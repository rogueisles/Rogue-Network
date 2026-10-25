"""Start the checksum-pinned Rogue Network mainnet release."""
import hashlib,os,re,subprocess,sys
from pathlib import Path
ROOT=Path('/opt/rgn');BINARY_SHA='338ada631c9c01aa5343503852de94faf7d43ff13840e3a315c3cbd5e4dc5783';CONFIG_SHA='87c66819311b112adce09855f9c2c38490fb32a193ea35ca7c466892319aefd8'
def settings(env):
 data=Path(env.get('RGN_DATADIR','/var/lib/rgn'))
 if not data.is_absolute():raise ValueError('RGN_DATADIR must be absolute')
 wallet=env.get('WALLET','').strip()
 if wallet and (not re.fullmatch(r'0x[0-9a-fA-F]{40}',wallet) or int(wallet[2:],16)==0):raise ValueError('WALLET must be a nonzero public RGN address; no private key is used')
 def number(key,default,low,high):
  raw=env.get(key,str(default))
  if not re.fullmatch('[0-9]+',raw) or not low<=int(raw)<=high:raise ValueError(key+' is outside the supported range')
  return str(int(raw))
 return data,wallet,number('RGN_CACHE_MB',512,128,4096),number('RGN_MAX_PEERS',25,1,200),number('RGN_P2P_PORT',30303,1024,65535),number('RGN_RPC_PORT',18545,1024,65535)
def command(env):
 data,wallet,cache,peers,p2p,rpc=settings(env)
 a=[str(ROOT/'egem'),'--config',str(ROOT/'rgn-node.toml'),'--datadir',str(data),'--networkid','70088','--syncmode','full','--gcmode','full','--cache',cache,'--maxpeers',peers,'--port',p2p,'--gasprice','1','--rpc','--rpcaddr','0.0.0.0','--rpcport',rpc,'--rpcvhosts','localhost,127.0.0.1,rgn-node,rgn','--rpcapi','eth,net,web3','--ipcpath','egem.ipc','--nousb']
 if wallet:a+=['--etherbase',wallet]
 return a
def verify_payload():
 for p,h in ((ROOT/'egem',BINARY_SHA),(ROOT/'RGN-mainnet-v4.1.0-H1121800.json',CONFIG_SHA)):
  if p.is_symlink() or hashlib.sha256(p.read_bytes()).hexdigest()!=h:raise ValueError('Bundled release payload checksum differs')
def main():
 verify_payload()
 if len(sys.argv)>1:
  if sys.argv[1:] not in (['version'],['--help'],['help']):raise ValueError('Use the documented environment settings, or the version/help command')
  os.execv(str(ROOT/'egem'),[str(ROOT/'egem'),*sys.argv[1:]])
 args=command(os.environ);data,wallet,*_=settings(os.environ)
 if data.is_symlink():raise ValueError('Datadir cannot be a symbolic link')
 data.mkdir(mode=0o700,parents=True,exist_ok=True)
 if not os.access(data,os.W_OK):raise ValueError('Datadir must be writable by container UID10001; see the bind-mount instructions')
 print('Rogue Network 4.1.0; chain 70088; FORGE V5 activates at block 1,121,800.',flush=True)
 print('Checking shared mainnet genesis/configuration in the persistent datadir.',flush=True)
 subprocess.run([str(ROOT/'egem'),'--datadir',str(data),'--networkid','70088','--nousb','init',str(ROOT/'RGN-mainnet-v4.1.0-H1121800.json')],check=True,timeout=180)
 if wallet:print('WALLET selects the RGN work beneficiary. A compatible FORGE adapter and native backend are required for solo mining; this node does not start mining by itself.',flush=True)
 else:print('Starting regular RGN node without mining.',flush=True)
 os.execv(args[0],args)
if __name__=='__main__':
 try:main()
 except (ValueError,OSError,subprocess.SubprocessError) as e:print('RGN startup stopped: '+str(e),file=sys.stderr);sys.exit(2)
