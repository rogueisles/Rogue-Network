package eth

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"testing"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/common/hexutil"
	"github.com/TeamEGEM/go-egem/consensus/ethash"
	"github.com/TeamEGEM/go-egem/consensus/oggreward"
	"github.com/TeamEGEM/go-egem/core"
	"github.com/TeamEGEM/go-egem/core/state"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/core/vm"
	"github.com/TeamEGEM/go-egem/ethdb"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rpc"
)

func TestExplorerRewardRPCPreservesFieldsAndHeightSelection(t *testing.T) {
	blob, err := ioutil.ReadFile("../genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	var g core.Genesis
	if err := json.Unmarshal(blob, &g); err != nil {
		t.Fatal(err)
	}
	cfg := g.Config
	cfg.ForgeBlock = big.NewInt(params.RogueActivationHeight)
	cfg.Forge = integrationForgeConfig().Forge
	cfg.Forge.RewardSchedule = params.RogueRewardSchedule
	cfg.Forge.PreForgeGrossIssued, _ = new(big.Int).SetString(params.RoguePreForgeGrossIssuedWei, 10)
	cfg.Forge.AllowAuxTree, cfg.Forge.RequireMulti, cfg.Forge.RequireDGBOrDOGE, cfg.Forge.RetargetV2 = true, true, true, true
	db, err := ethdb.NewMemDatabase()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	genesis := g.MustCommit(db)
	chain, err := core.NewBlockChain(db, nil, cfg, ethash.NewFaker(), vm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()
	e := &Ethereum{blockchain: chain}
	e.ApiBackend = &EthApiBackend{eth: e}
	server := rpc.NewServer()
	defer server.Stop()
	if err := server.RegisterName("eth", &PublicMinerAPI{e: e}); err != nil {
		t.Fatal(err)
	}
	client := rpc.DialInProc(server)
	defer client.Close()
	oldStake := common.HexToAddress("0xCd442d7AC675D6c637a960e10913e341508C6672")
	oldTribe := common.HexToAddress("0xfeaD066Caa900F210B19B9df14aBc38B46a15e66")
	stake := common.HexToAddress("0xa47008c59f729756bEc7d01f6FE71328A242d0c4")
	tribe := common.HexToAddress("0x085CF5da09842FA3BA01068CC02c156198b1b114")
	treasury := common.HexToAddress("0x85ea896411EdFE9dD7fa6F4F5FaA19D2D81cdA5E")
	p2e := common.HexToAddress("0x03D50e192Bd14ec9f536DF00Db7Ae6723Bdb8BcD")
	for _, height := range []uint64{20999, 21000, 21001, 1071952, 1100099, 1100100, 1100110, 1319250, 13580767, 33098927, 33098928} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			miner := common.BigToAddress(new(big.Int).SetUint64(height))
			// Database fixtures exercise RPC lookup without claiming chain import.
			block := types.NewBlockWithHeader(&types.Header{Number: new(big.Int).SetUint64(height), Difficulty: big.NewInt(1), Coinbase: miner, Root: genesis.Root()})
			if err := core.WriteBlock(db, block); err != nil {
				t.Fatal(err)
			}
			if err := core.WriteCanonicalHash(db, block.Hash(), height); err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := client.Call(&fields, "eth_getBlockReward", fmt.Sprintf("0x%x", height)); err != nil {
				t.Fatal(err)
			}
			for alias, canonical := range map[string]string{"totalReward": "total", "minerReward": "miner", "stakingReward": "staking", "tribeReward": "tribe", "maintenanceReward": "maintenance", "p2eReward": "p2e", "treasuryReward": "treasury"} {
				if len(fields[alias]) == 0 || string(fields[alias]) != string(fields[canonical]) {
					t.Fatalf("incompatible %s alias: %s / %s", alias, fields[alias], fields[canonical])
				}
			}
			blob, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			var got OGGBlockReward
			if err := json.Unmarshal(blob, &got); err != nil {
				t.Fatal(err)
			}
			if uint64(got.BlockNumber) != height || got.BlockHash == nil || *got.BlockHash != block.Hash() || got.MinerAddress != miner || got.Pending {
				t.Fatal("reward not bound to requested block", string(blob))
			}
			wantStake, wantTribe, wantMaint, wantP2E, wantTreasury := stake, tribe, treasury, common.Address{}, common.Address{}
			if height < 21000 {
				wantStake, wantTribe = oldStake, oldTribe
			}
			if height >= uint64(params.RogueActivationHeight) {
				wantTribe, wantMaint, wantP2E, wantTreasury = common.Address{}, common.Address{}, p2e, treasury
			}
			if got.StakingAddress != wantStake || got.TribeAddress != wantTribe || got.MaintenanceAddress != wantMaint || got.P2EAddress != wantP2E || got.TreasuryAddress != wantTreasury {
				t.Fatal("incorrect destination selection", string(blob))
			}
			if got.SpecialIssuanceAddress != nil {
				want := treasury
				if height == 21001 {
					want = tribe
				}
				if *got.SpecialIssuanceAddress != want || (*big.Int)(got.SpecialIssuance).Sign() <= 0 {
					t.Fatal("incorrect special issuance recipient", string(blob))
				}
			} else if (*big.Int)(got.SpecialIssuance).Sign() != 0 {
				t.Fatal("special issuance has no recipient", string(blob))
			}
			st, err := state.New(common.Hash{}, state.NewDatabase(db))
			if err != nil {
				t.Fatal(err)
			}
			oggreward.AccumulateRewards(cfg, st, block.Header())
			reported := make(map[common.Address]*big.Int)
			add := func(address common.Address, amount *hexutil.Big) {
				if reported[address] == nil {
					reported[address] = new(big.Int)
				}
				reported[address].Add(reported[address], (*big.Int)(amount))
			}
			add(got.MinerAddress, got.MinerReward)
			add(got.StakingAddress, got.StakingReward)
			add(got.TribeAddress, got.TribeReward)
			add(got.MaintenanceAddress, got.MaintenanceReward)
			add(got.P2EAddress, got.P2EReward)
			add(got.TreasuryAddress, got.TreasuryReward)
			if got.SpecialIssuanceAddress != nil {
				add(*got.SpecialIssuanceAddress, got.SpecialIssuance)
			}
			for address, amount := range reported {
				if st.GetBalance(address).Cmp(amount) != 0 {
					t.Fatalf("RPC differs from state allocation at %s: %s / %s", address.Hex(), amount, st.GetBalance(address))
				}
			}
		})
	}
	for _, tag := range []string{"earliest", "latest"} {
		var got OGGBlockReward
		if err := client.Call(&got, "eth_getBlockReward", tag); err != nil || got.BlockHash == nil || *got.BlockHash != genesis.Hash() {
			t.Fatalf("block tag %q: %v", tag, err)
		}
	}
	var missing *OGGBlockReward
	if err := client.Call(&missing, "eth_getBlockReward", "0x1234567"); err != nil || missing != nil {
		t.Fatal("unknown block must return null", err, missing)
	}
	if err := client.Call(&missing, "eth_getBlockReward", "invalid"); err == nil {
		t.Fatal("invalid block selector accepted")
	}
	replacement := types.NewBlockWithHeader(&types.Header{
		Number: big.NewInt(params.RogueActivationHeight), Difficulty: big.NewInt(1), Root: genesis.Root(),
		Coinbase: common.HexToAddress("0xbeef"), GasUsed: 21000,
	})
	if err := core.WriteBlock(db, replacement); err != nil {
		t.Fatal(err)
	}
	if err := core.WriteCanonicalHash(db, replacement.Hash(), replacement.NumberU64()); err != nil {
		t.Fatal(err)
	}
	var replaced OGGBlockReward
	if err := client.Call(&replaced, "eth_getBlockReward", fmt.Sprintf("0x%x", params.RogueActivationHeight)); err != nil {
		t.Fatal(err)
	}
	if replaced.BlockHash == nil || *replaced.BlockHash != replacement.Hash() || replaced.MinerAddress != replacement.Coinbase() || (*big.Int)(replaced.Total).String() != "1100000000000000000000" {
		t.Fatal("canonical replacement did not update the reward recipient, or gas usage changed subsidy")
	}
}
