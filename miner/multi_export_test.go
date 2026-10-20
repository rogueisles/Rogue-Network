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
	"strings"
	"testing"
	"time"
)

func TestExportMultiLabWork(t *testing.T) {
	path := os.Getenv("FORGE_MULTI_ISSUED_FIXTURE")
	if path == "" {
		t.Skip("owned multi-chain lab only")
	}
	cfg := remoteForgeConfig()
	cfg.ChainId = big.NewInt(7008902)
	cfg.Forge.PrivateCPUTest = true
	cfg.Forge.AllowAuxTree = true
	cfg.Forge.MinimumDifficulty = big.NewInt(65536)
	cfg.Forge.InitialDifficulty = big.NewInt(65536)
	cfg.Forge.TemplateSigners = []common.Address{common.HexToAddress("0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf")}
	engine, err := forge.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	agent := NewRemoteAgent(&forgeRemoteTestChain{cfg: cfg}, engine)
	header := &types.Header{ParentHash: common.HexToHash("0x1234"), Number: big.NewInt(100), Difficulty: big.NewInt(65536), Time: big.NewInt(time.Now().Unix()), GasLimit: 8000000, UncleHash: types.EmptyUncleHash}
	agent.currentWork = &Work{Block: types.NewBlockWithHeader(header)}
	work, err := agent.CreateAuxWork(nil)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := rlp.EncodeToBytes(agent.auxWork[work.Hash].block.Header())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]interface{}{"headerRLP": hex.EncodeToString(blob), "workHash": work.Hash.Hex(), "timestamp": work.Timestamp,
		"target": "0x" + strings.Repeat("0", 64-len(work.Target.Text(16))) + work.Target.Text(16), "chainId": 7008902, "privateCpuTest": true}
	data, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
