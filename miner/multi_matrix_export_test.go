package miner

import (
	"encoding/hex"
	"encoding/json"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/rlp"
	"math/big"
	"os"
	"testing"
	"time"
)

func TestExportMultiMatrixWork(t *testing.T) {
	path := os.Getenv("FORGE_MULTI_MATRIX_ISSUED")
	if path == "" {
		t.Skip("private matrix lab only")
	}
	cfg := remoteForgeConfig()
	cfg.ChainId = big.NewInt(7008902)
	cfg.Forge.PrivateCPUTest = true
	cfg.Forge.AllowAuxTree = true
	cfg.Forge.MinimumDifficulty = big.NewInt(65536)
	cfg.Forge.InitialDifficulty = big.NewInt(65536)
	cfg.Forge.TemplateSigners = []common.Address{common.HexToAddress("0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf")}
	engine, e := forge.New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	h := &types.Header{ParentHash: common.HexToHash("0x1234"), Number: big.NewInt(100), Difficulty: big.NewInt(65536), Time: big.NewInt(time.Now().Unix()), GasLimit: 8000000, UncleHash: types.EmptyUncleHash}
	agent := NewRemoteAgent(&forgeRemoteTestChain{cfg: cfg}, engine)
	agent.currentWork = &Work{Block: types.NewBlockWithHeader(h)}
	work, e := agent.CreateAuxWork(nil)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := rlp.EncodeToBytes(agent.auxWork[work.Hash].block.Header())
	if e != nil {
		t.Fatal(e)
	}
	out := map[string]interface{}{"headerRLP": hex.EncodeToString(raw), "workHash": work.Hash.Hex(), "timestamp": work.Timestamp, "chainId": 7008902, "privateCpuTest": true}
	data, _ := json.Marshal(out)
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
}
