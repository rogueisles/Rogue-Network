package miner

import (
 "encoding/hex"
 "encoding/json"
 "math/big"
 "os"
 "testing"
 "github.com/TeamEGEM/go-egem/common"
 "github.com/TeamEGEM/go-egem/consensus/forge"
 "github.com/TeamEGEM/go-egem/core/types"
 "github.com/TeamEGEM/go-egem/rlp"
)

func TestMultiV3IssuedWorkSubmission(t *testing.T){
 path:=os.Getenv("FORGE_MULTI_PROOF_FIXTURE");if path==""{t.Skip("native laboratory proof required")}
 var v struct{HeaderRLP,Proof string;Private bool `json:"privateCpuTest"`};b,e:=os.ReadFile(path);if e!=nil || json.Unmarshal(b,&v)!=nil || !v.Private{t.Fatal("invalid private fixture")}
 raw,_:=hex.DecodeString(v.HeaderRLP);var h types.Header;if e=rlp.DecodeBytes(raw,&h);e!=nil{t.Fatal(e)};proof,_:=hex.DecodeString(v.Proof)
 cfg:=remoteForgeConfig();cfg.ChainId=big.NewInt(7008902);cfg.Forge.PrivateCPUTest=true;cfg.Forge.AllowAuxTree=true;cfg.Forge.RequireMulti=true;cfg.Forge.MinimumDifficulty=big.NewInt(65536);cfg.Forge.InitialDifficulty=big.NewInt(65536)
 cfg.Forge.TemplateSigners=[]common.Address{common.HexToAddress("0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf")}
 pins:=[]*common.Hash{&cfg.Forge.LTCPayoutScriptHash,&cfg.Forge.DOGEPayoutScriptHash,&cfg.Forge.PepecoinPayoutScriptHash,&cfg.Forge.DingocoinPayoutScriptHash,&cfg.Forge.BellscoinPayoutScriptHash}
 scripts:=[]string{"0014706267a5255e3666936bcb9a777667599f387546","76a9144b1508ebe06723842994adf33bdba36b4eba44c488ac","76a914bb0ff0797074c03e9969359b605997b53c9eaa8488ac","76a914e306e2a1a240f634da707d31079f18431317d7af88ac","0014088a288ee425094ccd67779afd0c2016d010368c"}
 for i,s:=range scripts{raw,_:=hex.DecodeString(s);*pins[i]=forge.ScriptHash(raw)}
 engine,e:=forge.New(cfg);if e!=nil{t.Fatal(e)};agent:=NewRemoteAgent(&forgeRemoteTestChain{cfg:cfg},engine);agent.currentWork=&Work{Block:types.NewBlockWithHeader(&h)};results:=make(chan *Result,1);agent.SetReturnCh(results)
 work,e:=agent.CreateAuxWork(nil);if e!=nil{t.Fatal(e)}
 if agent.SubmitAuxWork(common.Hash{1},proof){t.Fatal("unknown work accepted")}
 bad:=append([]byte{},proof...);bad[len(bad)-1]^=1;if agent.SubmitAuxWork(work.Hash,bad){t.Fatal("corrupt proof accepted")}
 if !agent.SubmitAuxWork(work.Hash,proof){t.Fatal("valid native V3 proof did not reach miner result path")}
 select{case <-results:default:t.Fatal("missing miner result")}
 if agent.SubmitAuxWork(work.Hash,proof){t.Fatal("issued work replay accepted")}
 t.Log("native V3 solution accepted by actual issued-work submission path exactly once")
}
