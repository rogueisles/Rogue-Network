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
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rlp"
)

type forgeRemoteTestChain struct {
	cfg *params.ChainConfig
}

func TestRemoteAgentIssuedTimestampIsStableAndBound(t *testing.T) {
	cfg := remoteForgeConfig()
	engine, _ := forge.New(cfg)
	agent := NewRemoteAgent(&forgeRemoteTestChain{cfg: cfg}, engine)
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1), Time: big.NewInt(1000)}
	agent.currentWork = &Work{Block: types.NewBlockWithHeader(header)}
	first, err := agent.CreateAuxWork(nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := agent.CreateAuxWork(nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Timestamp != 1000 || again.Timestamp != first.Timestamp || again.Hash != first.Hash {
		t.Fatal("reissuing work changed header time")
	}
	header.Time = big.NewInt(1001)
	agent.currentWork = &Work{Block: types.NewBlockWithHeader(header)}
	changed, err := agent.CreateAuxWork(nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Hash == first.Hash || changed.Timestamp != 1001 || agent.auxWork[first.Hash].block.Time().Uint64() != 1000 {
		t.Fatal("time not bound to exact issued work")
	}
	for _, bad := range []*big.Int{big.NewInt(0), big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 65)} {
		header.Time = bad
		agent.currentWork = &Work{Block: types.NewBlockWithHeader(header)}
		if _, err := agent.CreateAuxWork(nil); err == nil {
			t.Fatal("invalid timestamp issued")
		}
	}
}

// Export real issued work for the pool/node interoperability script. This is
// test-only and leaves all configured consensus parameters unchanged.
func TestExportForgePoolWork(t *testing.T) {
	path := os.Getenv("FORGE_ISSUED_FIXTURE")
	if path == "" {
		t.Skip("run pool/scripts/test-forge-node.sh")
	}
	cfg := remoteForgeConfig()
	engine, _ := forge.New(cfg)
	agent := NewRemoteAgent(&forgeRemoteTestChain{cfg: cfg}, engine)
	header := &types.Header{ParentHash: common.HexToHash("0x1234"), Number: big.NewInt(100), Difficulty: big.NewInt(1),
		Time: big.NewInt(1700000000), GasLimit: 8000000, UncleHash: types.EmptyUncleHash}
	agent.currentWork = &Work{Block: types.NewBlockWithHeader(header)}
	work, err := agent.CreateAuxWork(nil)
	if err != nil {
		t.Fatal(err)
	}
	issued := agent.auxWork[work.Hash].block.Header()
	blob, err := rlp.EncodeToBytes(issued)
	if err != nil {
		t.Fatal(err)
	}
	fixture := map[string]interface{}{"headerRLP": hex.EncodeToString(blob), "workHash": work.Hash.Hex(), "timestamp": work.Timestamp, "templateMaxValidity": cfg.Forge.TemplateMaxValidity}
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func (c *forgeRemoteTestChain) Config() *params.ChainConfig                 { return c.cfg }
func (c *forgeRemoteTestChain) CurrentHeader() *types.Header                { return nil }
func (c *forgeRemoteTestChain) GetHeader(common.Hash, uint64) *types.Header { return nil }
func (c *forgeRemoteTestChain) GetHeaderByNumber(uint64) *types.Header      { return nil }
func (c *forgeRemoteTestChain) GetHeaderByHash(common.Hash) *types.Header   { return nil }
func (c *forgeRemoteTestChain) GetBlock(common.Hash, uint64) *types.Block   { return nil }

func remoteForgeConfig() *params.ChainConfig {
	return &params.ChainConfig{
		ChainId: big.NewInt(70088), ForgeBlock: big.NewInt(1),
		Forge: &params.ForgeConfig{
			AuxChainID: 0x4F47, DogeAuxChainID: 0x0062, TargetBlockTime: 12,
			PreForgeGrossIssued: big.NewInt(1), InitialDifficulty: big.NewInt(4), MinimumDifficulty: big.NewInt(1),
			TemplateSigners: []common.Address{common.HexToAddress("0x1")}, TemplateQuorum: 1,
			LTCPayoutScriptHash: common.HexToHash("0x1"), DOGEPayoutScriptHash: common.HexToHash("0x2"), TemplateMaxValidity: 300,
		},
	}
}

func TestRemoteAgentForgeWorkAndLegacyRejection(t *testing.T) {
	cfg := remoteForgeConfig()
	post, err := forge.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	chain := &forgeRemoteTestChain{cfg: cfg}
	agent := NewRemoteAgent(chain, post)
	h := &types.Header{
		ParentHash: common.HexToHash("0xab"), Number: big.NewInt(1), Difficulty: big.NewInt(4),
		GasLimit: 8000000, Time: big.NewInt(1000), UncleHash: types.EmptyUncleHash,
	}
	block := types.NewBlockWithHeader(h)
	agent.currentWork = &Work{Block: block}

	aux, err := agent.CreateAuxWork(nil)
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := forge.WorkHash(block.Header())
	if err != nil {
		t.Fatal(err)
	}
	if aux.Hash != wantHash || aux.Height != 1 || aux.PreviousBlockHash != h.ParentHash || aux.Timestamp != 1000 {
		t.Fatalf("unexpected aux work: %+v", aux)
	}
	wantTarget, _ := forge.TargetForDifficulty(big.NewInt(4))
	if aux.Target.Cmp(wantTarget) != 0 {
		t.Fatalf("target = %v want %v", aux.Target, wantTarget)
	}
	if _, err := agent.GetWork(); err == nil {
		t.Fatal("legacy GetWork remained enabled after FORGE activation")
	}
	if agent.SubmitAuxWork(aux.Hash, []byte("not-a-proof")) {
		t.Fatal("malformed FORGE proof accepted")
	}
	if agent.SubmitAuxWork(common.HexToHash("0xdead"), nil) {
		t.Fatal("unknown FORGE work accepted")
	}
}

func TestRemoteAgentRejectsOversizedMinerExtra(t *testing.T) {
	cfg := remoteForgeConfig()
	post, _ := forge.New(cfg)
	agent := NewRemoteAgent(&forgeRemoteTestChain{cfg: cfg}, post)
	agent.currentWork = &Work{Block: types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1)})}
	if _, err := agent.CreateAuxWork(make([]byte, forge.MaxMinerExtra+1)); err == nil {
		t.Fatal("oversized miner extra accepted")
	}
}

func TestRemoteAgentPrivateCPUTarget(t *testing.T) {
	cfg := remoteForgeConfig()
	cfg.ChainId = big.NewInt(7008902)
	cfg.Forge.PrivateCPUTest = true
	cfg.Forge.MinimumDifficulty = big.NewInt(65536)
	cfg.Forge.InitialDifficulty = big.NewInt(131072)
	post, err := forge.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	chain := &forgeRemoteTestChain{cfg: cfg}
	agent := NewRemoteAgent(chain, post)
	h := &types.Header{
		ParentHash: common.HexToHash("0xab"), Number: big.NewInt(1), Difficulty: big.NewInt(131072),
		GasLimit: 8000000, Time: big.NewInt(1000), UncleHash: types.EmptyUncleHash,
	}
	block := types.NewBlockWithHeader(h)
	agent.currentWork = &Work{Block: block}

	aux, err := agent.CreateAuxWork(nil)
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := forge.WorkHash(block.Header())
	if err != nil {
		t.Fatal(err)
	}
	if aux.Hash != wantHash || aux.Height != 1 || aux.PreviousBlockHash != h.ParentHash || aux.Timestamp != 1000 {
		t.Fatalf("unexpected aux work: %+v", aux)
	}
	wantTarget, _ := forge.TargetForConfig(cfg.Forge, big.NewInt(131072))
	if aux.Target.Cmp(wantTarget) != 0 {
		t.Fatalf("target = %v want %v", aux.Target, wantTarget)
	}
	if _, err := agent.GetWork(); err == nil {
		t.Fatal("legacy GetWork remained enabled after FORGE activation")
	}
	if agent.SubmitAuxWork(aux.Hash, []byte("not-a-proof")) {
		t.Fatal("malformed FORGE proof accepted")
	}
	if agent.SubmitAuxWork(common.HexToHash("0xdead"), nil) {
		t.Fatal("unknown FORGE work accepted")
	}
}
