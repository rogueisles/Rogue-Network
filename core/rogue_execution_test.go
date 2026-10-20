package core

import (
	"math/big"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/consensus/progpow"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/core/vm"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/params"
)

type rogueExecutionChain struct{ config *params.ChainConfig }

func (c *rogueExecutionChain) Config() *params.ChainConfig                 { return c.config }
func (c *rogueExecutionChain) CurrentHeader() *types.Header                { return nil }
func (c *rogueExecutionChain) GetHeader(common.Hash, uint64) *types.Header { return nil }
func (c *rogueExecutionChain) GetHeaderByNumber(uint64) *types.Header      { return nil }
func (c *rogueExecutionChain) GetHeaderByHash(common.Hash) *types.Header   { return nil }
func (c *rogueExecutionChain) GetBlock(common.Hash, uint64) *types.Block   { return nil }

// This tests signed EVM transfers and state-root validation across H-1/H/H+1.
// Sealing is outside this test: the legacy engine uses its explicit fake mode.
func TestRogueSignedTransfersAndWrongRewardStateRejected(t *testing.T) {
	pre, _ := new(big.Int).SetString(params.RoguePreForgeGrossIssuedWei, 10)
	cfg := &params.ChainConfig{ChainId: big.NewInt(70088), HomesteadBlock: big.NewInt(0), EIP150Block: big.NewInt(0), EIP155Block: big.NewInt(0), EIP158Block: big.NewInt(0), ByzantiumBlock: big.NewInt(0), ConstantinopleBlock: big.NewInt(0), ProgPoW: &params.ProgPoWConfig{Period: 10}, ForgeBlock: big.NewInt(1100100), Forge: &params.ForgeConfig{
		RewardSchedule: params.RogueRewardSchedule, PreForgeGrossIssued: pre, RequireDGBOrDOGE: true, RequireMulti: true, AllowAuxTree: true, RetargetV2: true,
		AuxChainID: 20295, DogeAuxChainID: 98, TargetBlockTime: 12, InitialDifficulty: big.NewInt(1), MinimumDifficulty: big.NewInt(1), TemplateSigners: []common.Address{common.HexToAddress("0xcafe")}, TemplateQuorum: 1,
		LTCPayoutScriptHash: common.HexToHash("0x1"), DOGEPayoutScriptHash: common.HexToHash("0x2"), TemplateMaxValidity: 300,
	}}
	legacyCfg := *cfg
	legacyCfg.ForgeBlock = nil
	legacyCfg.Forge = nil
	legacy := progpow.NewFaker()
	engine, err := forge.NewHybrid(cfg, legacy)
	if err != nil {
		t.Fatal(err)
	}
	chain := &rogueExecutionChain{cfg}
	oldChain := &rogueExecutionChain{&legacyCfg}
	db, _ := ethdb.NewMemDatabase()
	defer db.Close()
	st, err := state.New(common.Hash{}, state.NewDatabase(db))
	if err != nil {
		t.Fatal(err)
	}
	key, _ := crypto.GenerateKey()
	sender := crypto.PubkeyToAddress(key.PublicKey)
	recipient := common.HexToAddress("0xface")
	miner := common.HexToAddress("0xbeef")
	st.AddBalance(sender, new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil))
	for i, n := range []int64{1100099, 1100100, 1100101} {
		parentState := st.Copy()
		header := &types.Header{Number: big.NewInt(n), Time: big.NewInt(1700000000 + 12*int64(i)), Difficulty: big.NewInt(1), GasLimit: 8000000, Coinbase: miner, UncleHash: types.EmptyUncleHash}
		tx := types.NewTransaction(uint64(i), recipient, big.NewInt(100), 21000, big.NewInt(1), nil)
		tx, err = types.SignTx(tx, types.NewEIP155Signer(big.NewInt(70088)), key)
		if err != nil {
			t.Fatal(err)
		}
		process := func(config *params.ChainConfig, s *state.StateDB) (*types.Header, types.Receipts, uint64) {
			h := types.CopyHeader(header)
			used := uint64(0)
			s.Prepare(tx.Hash(), common.Hash{}, 0)
			receipt, _, e := ApplyTransaction(config, nil, &miner, new(GasPool).AddGas(h.GasLimit), s, h, tx, &used, vm.Config{})
			if e != nil {
				t.Fatal(e)
			}
			if receipt.Status != 1 || used != 21000 {
				t.Fatal("transfer did not execute", n)
			}
			h.GasUsed = used
			return h, types.Receipts{receipt}, used
		}
		h, receipts, used := process(cfg, st)
		block, e := engine.Finalize(chain, h, st, []*types.Transaction{tx}, nil, receipts)
		if e != nil {
			t.Fatal(e)
		}
		if e = NewBlockValidator(cfg, nil, engine).ValidateState(block, nil, st, receipts, used); e != nil {
			t.Fatal(e)
		}
		if st.GetBalance(recipient).Cmp(big.NewInt(100*int64(i+1))) != 0 || st.GetNonce(sender) != uint64(i+1) {
			t.Fatal("balances or nonce failed across fork")
		}
		oldHeader, oldReceipts, oldUsed := process(&legacyCfg, parentState)
		oldBlock, e := legacy.Finalize(oldChain, oldHeader, parentState, []*types.Transaction{tx}, nil, oldReceipts)
		if e != nil {
			t.Fatal(e)
		}
		if n < 1100100 {
			if oldBlock.Root() != block.Root() {
				t.Fatal("pre-fork transaction/state behavior changed")
			}
		} else {
			if oldBlock.Root() == block.Root() {
				t.Fatal("new reward state unexpectedly matches old rules")
			}
			if e = NewBlockValidator(cfg, nil, engine).ValidateState(oldBlock, nil, st, receipts, used); e == nil {
				t.Fatal("old reward state root accepted after activation")
			}
			if e = NewBlockValidator(&legacyCfg, nil, legacy).ValidateState(block, nil, parentState, oldReceipts, oldUsed); e == nil {
				t.Fatal("new reward state root accepted under old rules")
			}
		}
	}
}
