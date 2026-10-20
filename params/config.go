// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"fmt"
	"math/big"

	"github.com/TeamEGEM/go-egem/common"
)

var (
	MainnetGenesisHash = common.HexToHash("0xdfc3e94e54007bbabaed932e689ef41d2f109fe28486b90a981221e4fd9a4149") // Genesis hash associated with the default chain parameters.
	TestnetGenesisHash = common.HexToHash("0x42b6a7d113fe1f4acc35da7d8f542d5d19794047433726451c25925ef2783dcd") // Genesis hash associated with the alternate chain parameters.
)

// 33661
// 33666
var (
	// Default chain parameters for the predefined genesis block.
	MainnetChainConfig = &ChainConfig{
		ChainId:             big.NewInt(1987),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.HexToHash("0xdfc3e94e54007bbabaed932e689ef41d2f109fe28486b90a981221e4fd9a4149"),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		Ethash:              new(EthashConfig),
	}

	// Chain parameters for the predefined Ropsten genesis block.
	TestnetChainConfig = &ChainConfig{
		ChainId:             big.NewInt(87766),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.HexToHash("0x42b6a7d113fe1f4acc35da7d8f542d5d19794047433726451c25925ef2783dcd"),
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      nil,
		ConstantinopleBlock: nil,
		Ethash:              new(EthashConfig),
	}

	// RinkebyChainConfig contains the chain parameters to run a node on the Rinkeby test network.
	RinkebyChainConfig = &ChainConfig{
		ChainId:             big.NewInt(4),
		HomesteadBlock:      big.NewInt(1),
		DAOForkBlock:        nil,
		DAOForkSupport:      true,
		EIP150Block:         big.NewInt(2),
		EIP150Hash:          common.HexToHash("0x9b095b36c15eaf13044373aef8ee0bd3a382a5abb92e402afa44b8249c3a90e9"),
		EIP155Block:         big.NewInt(3),
		EIP158Block:         big.NewInt(3),
		ByzantiumBlock:      big.NewInt(1035301),
		ConstantinopleBlock: nil,
		Clique: &CliqueConfig{
			Period: 15,
			Epoch:  30000,
		},
	}

	// Convenience configuration for an isolated ProgPoW chain.
	TestnetProgPoWChainConfig = &ChainConfig{
		ChainId:             big.NewInt(70088),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.Hash{},
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: nil,
		Ethash:              nil,
		Clique:              nil,
		ProgPoW:             &ProgPoWConfig{Period: 10},
	}

	// ForgeDevChainConfig is a convenience preset for FORGE development/tests.
	// Activation requires an explicit configuration; these defaults do not enable it.
	ForgeDevChainConfig = &ChainConfig{
		ChainId:             big.NewInt(70088),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        nil,
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.Hash{},
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: nil,
		Ethash:              nil,
		Clique:              nil,
		ProgPoW:             &ProgPoWConfig{Period: 10},
		ForgeBlock:          big.NewInt(0),
		Forge: &ForgeConfig{
			AuxChainID:           0x4F47, // "OG" â€” distinct from Dogecoin 0x0062
			DogeAuxChainID:       0x0062,
			TargetBlockTime:      12,
			PreForgeGrossIssued:  new(big.Int).Mul(big.NewInt(700000000), big.NewInt(1000000000000000000)),
			InitialDifficulty:    big.NewInt(1),
			MinimumDifficulty:    big.NewInt(1),
			TemplateSigners:      []common.Address{common.HexToAddress("0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf")}, // dev key 1
			TemplateQuorum:       1,
			LTCPayoutScriptHash:  common.HexToHash("0x4ae81572f06e1b88fd5ced7a1a000945432e83e1551e6f721ee9c00b8cc33260"), // SHA256(OP_TRUE)
			DOGEPayoutScriptHash: common.HexToHash("0x8c2574892063f995fdf756bce07f46c1a5193e54cd52837ed91e32008ccf41ac"), // SHA256(OP_2)
			TemplateMaxValidity:  300,
		},
	}

	// AllEthashProtocolChanges contains every protocol change (EIPs) introduced
	// and accepted by the Ethereum core developers into the Ethash consensus.
	//
	// This configuration is intentionally not using keyed fields to force anyone
	// adding flags to the config to also have to set these fields.
	AllEthashProtocolChanges = &ChainConfig{big.NewInt(1337), big.NewInt(0), nil, false, big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, new(EthashConfig), nil, nil, nil, nil}

	// AllCliqueProtocolChanges contains every protocol change (EIPs) introduced
	// and accepted by the Ethereum core developers into the Clique consensus.
	//
	// This configuration is intentionally not using keyed fields to force anyone
	// adding flags to the config to also have to set these fields.
	AllCliqueProtocolChanges = &ChainConfig{big.NewInt(1337), big.NewInt(0), nil, false, big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, nil, &CliqueConfig{Period: 0, Epoch: 30000}, nil, nil, nil}

	TestChainConfig = &ChainConfig{big.NewInt(1), big.NewInt(0), nil, false, big.NewInt(0), common.Hash{}, big.NewInt(0), big.NewInt(0), big.NewInt(0), nil, new(EthashConfig), nil, nil, nil, nil}
	TestRules       = TestChainConfig.Rules(new(big.Int))
)

// ChainConfig is the core config which determines the blockchain settings.
//
// ChainConfig is stored in the database on a per block basis. This means
// that any network, identified by its genesis block, can have its own
// set of configuration options.
type ChainConfig struct {
	ChainId *big.Int `json:"chainId"` // Chain id identifies the current chain and is used for replay protection

	HomesteadBlock *big.Int `json:"homesteadBlock,omitempty"` // Homestead switch block (nil = no fork, 0 = already homestead)

	DAOForkBlock   *big.Int `json:"daoForkBlock,omitempty"`   // TheDAO hard-fork switch block (nil = no fork)
	DAOForkSupport bool     `json:"daoForkSupport,omitempty"` // Whether the nodes supports or opposes the DAO hard-fork

	// EIP150 implements the Gas price changes (https://github.com/ethereum/EIPs/issues/150)
	EIP150Block *big.Int    `json:"eip150Block,omitempty"` // EIP150 HF block (nil = no fork)
	EIP150Hash  common.Hash `json:"eip150Hash,omitempty"`  // EIP150 HF hash (needed for header only clients as only gas pricing changed)

	EIP155Block *big.Int `json:"eip155Block,omitempty"` // EIP155 HF block
	EIP158Block *big.Int `json:"eip158Block,omitempty"` // EIP158 HF block

	ByzantiumBlock      *big.Int `json:"byzantiumBlock,omitempty"`      // Byzantium switch block (nil = no fork, 0 = already on byzantium)
	ConstantinopleBlock *big.Int `json:"constantinopleBlock,omitempty"` // Constantinople switch block (nil = no fork, 0 = already activated)

	// Various consensus engines
	Ethash  *EthashConfig  `json:"ethash,omitempty"`
	Clique  *CliqueConfig  `json:"clique,omitempty"`
	ProgPoW *ProgPoWConfig `json:"progpow,omitempty"`

	// ForgeBlock activates the FORGE consensus rules. A nil value means FORGE is disabled.
	ForgeBlock *big.Int     `json:"forgeBlock,omitempty"`
	Forge      *ForgeConfig `json:"forge,omitempty"`
}

// EthashConfig is the consensus engine configs for proof-of-work based sealing.
type EthashConfig struct{}

// String implements the stringer interface, returning the consensus engine details.
func (c *EthashConfig) String() string {
	return "ethash"
}

// ProgPoWConfig is the consensus engine config for ProgPoW proof-of-work.
type ProgPoWConfig struct {
	// Period is blocks per random-program epoch (default 10, matches progpowPeriod).
	Period uint64 `json:"period"`
}

// String implements the stringer interface.
func (c *ProgPoWConfig) String() string { return "progpow" }

// ForgeConfig contains FORGE consensus parameters. The activation height lives in
// ChainConfig.ForgeBlock so a binary can contain FORGE support while keeping it
// disabled until an explicit activation height is configured.
type ForgeConfig struct {
	// RewardSchedule selects an immutable issuance schedule explicitly. An empty
	// value preserves the existing reward rules and allocation percentages.
	RewardSchedule            string      `json:"rewardSchedule,omitempty"`
	RequireDGBOrDOGE          bool        `json:"requireDgbOrDogeWork,omitempty"`
	RequireMulti              bool        `json:"requireMultiExternalWork,omitempty"`
	PepecoinPayoutScriptHash  common.Hash `json:"pepecoinPayoutScriptHash,omitempty"`
	DingocoinPayoutScriptHash common.Hash `json:"dingocoinPayoutScriptHash,omitempty"`
	BellscoinPayoutScriptHash common.Hash `json:"bellscoinPayoutScriptHash,omitempty"`

	AllowAuxTree bool `json:"allowAuxTree,omitempty"`
	// PrivateCPUTest selects the isolated chain 7008902 difficulty domain.
	// Never enable on production or an existing chain.
	PrivateCPUTest bool `json:"privateCpuTest,omitempty"`

	// RetargetV2 opts a new FORGE deployment into the experimental relative
	// exponential 12-second retarget. It does not change the Scrypt work domain.
	// Like other FORGE parameters, it is immutable after ForgeBlock.
	RetargetV2 bool `json:"retargetV2,omitempty"`

	// AuxChainID identifies OGG in the merged-mining tree. It is intentionally
	// separate from the EVM chain ID.
	AuxChainID uint32 `json:"auxChainId"`

	// DogeAuxChainID is the Dogecoin AuxPoW chain ID used when committing OGG
	// and DOGE into the same Scrypt parent coinbase.
	DogeAuxChainID uint32 `json:"dogeAuxChainId"`

	// TargetBlockTime is the OGG target spacing, in seconds, used by the FORGE
	// difficulty algorithm.
	TargetBlockTime uint64 `json:"targetBlockTime"`

	// PreForgeGrossIssued freezes the exact amount of native OGG created before
	// ForgeBlock. It includes genesis, every pre-FORGE normal block reward, and
	// historical one-time native issuance. Burns never reduce this value.
	PreForgeGrossIssued *big.Int `json:"preForgeGrossIssued,omitempty"`

	// InitialDifficulty is the explicit Scrypt difficulty used by the first
	// FORGE block. The post-fork difficulty domain is intentionally independent
	// from the historical ProgPoW difficulty domain.
	// With RetargetV2 it is the first-block reference at 12 seconds since the
	// parent; earlier/later candidate timestamps adjust it exponentially.
	InitialDifficulty *big.Int `json:"initialDifficulty,omitempty"`

	// MinimumDifficulty is the hard floor for the FORGE Scrypt difficulty.
	MinimumDifficulty *big.Int `json:"minimumDifficulty,omitempty"`

	// TemplateSigners contains the addresses authorised to attest the external
	// Litecoin/Dogecoin template snapshot committed by a FORGE AuxPoW proof.
	TemplateSigners []common.Address `json:"templateSigners,omitempty"`

	// TemplateQuorum is the number of distinct authorised signatures required
	// on an external template.
	TemplateQuorum uint64 `json:"templateQuorum,omitempty"`

	// LTCPayoutScriptHash and DOGEPayoutScriptHash are SHA-256(scriptPubKey)
	// commitments. Every positive-value output in the corresponding external
	// coinbase must pay the committed script; zero-value outputs may only be
	// provably-unspendable OP_RETURN commitments.
	LTCPayoutScriptHash  common.Hash `json:"ltcPayoutScriptHash,omitempty"`
	DOGEPayoutScriptHash common.Hash `json:"dogePayoutScriptHash,omitempty"`

	// TemplateMaxValidity bounds the signed external-template validity window in
	// seconds. Header timestamps are checked against the signed window, keeping
	// consensus deterministic and independent of a node's wall clock.
	TemplateMaxValidity uint64 `json:"templateMaxValidity,omitempty"`
}

func (c *ForgeConfig) String() string { return "forge" }

// CliqueConfig is the consensus engine configs for proof-of-authority based sealing.
type CliqueConfig struct {
	Period uint64 `json:"period"` // Number of seconds between blocks to enforce
	Epoch  uint64 `json:"epoch"`  // Epoch length to reset votes and checkpoint
}

// String implements the stringer interface, returning the consensus engine details.
func (c *CliqueConfig) String() string {
	return "clique"
}

// String implements the fmt.Stringer interface.
func (c *ChainConfig) String() string {
	var engine string
	switch {
	case c.ProgPoW != nil && c.Forge != nil:
		engine = c.ProgPoW.String() + "->" + c.Forge.String()
	case c.ProgPoW != nil:
		engine = c.ProgPoW.String()
	case c.Forge != nil:
		engine = c.Forge.String()
	case c.Clique != nil:
		engine = c.Clique.String()
	case c.Ethash != nil:
		engine = c.Ethash.String()
	default:
		engine = "unknown"
	}
	return fmt.Sprintf("{ChainID: %v Engine: %s EGEM Geth}", c.ChainId, engine)
}

// IsHomestead returns whether num is either equal to the homestead block or greater.
func (c *ChainConfig) IsHomestead(num *big.Int) bool {
	return isForked(c.HomesteadBlock, num)
}

// IsDAO returns whether num is either equal to the DAO fork block or greater.
func (c *ChainConfig) IsDAOFork(num *big.Int) bool {
	return isForked(c.DAOForkBlock, num)
}

func (c *ChainConfig) IsEIP150(num *big.Int) bool {
	return isForked(c.EIP150Block, num)
}

func (c *ChainConfig) IsEIP155(num *big.Int) bool {
	return isForked(c.EIP155Block, num)
}

func (c *ChainConfig) IsEIP158(num *big.Int) bool {
	return isForked(c.EIP158Block, num)
}

func (c *ChainConfig) IsByzantium(num *big.Int) bool {
	return isForked(c.ByzantiumBlock, num)
}

func (c *ChainConfig) IsConstantinople(num *big.Int) bool {
	return isForked(c.ConstantinopleBlock, num)
}

// IsForge returns whether FORGE consensus rules are active at num.
func (c *ChainConfig) IsForge(num *big.Int) bool {
	return c.Forge != nil && isForked(c.ForgeBlock, num)
}

// GasTable returns the gas table corresponding to the current phase (homestead or homestead reprice).
//
// The returned GasTable's fields shouldn't, under any circumstances, be changed.
func (c *ChainConfig) GasTable(num *big.Int) GasTable {
	if num == nil {
		return GasTableHomestead
	}
	switch {
	case c.IsEIP158(num):
		return GasTableEIP158
	case c.IsEIP150(num):
		return GasTableEIP150
	default:
		return GasTableHomestead
	}
}

// CheckCompatible checks whether scheduled fork transitions have been imported
// with a mismatching chain configuration.
func (c *ChainConfig) CheckCompatible(newcfg *ChainConfig, height uint64) *ConfigCompatError {
	bhead := new(big.Int).SetUint64(height)

	// Iterate checkCompatible to find the lowest conflict.
	var lasterr *ConfigCompatError
	for {
		err := c.checkCompatible(newcfg, bhead)
		if err == nil || (lasterr != nil && err.RewindTo == lasterr.RewindTo) {
			break
		}
		lasterr = err
		bhead.SetUint64(err.RewindTo)
	}
	return lasterr
}

func (c *ChainConfig) checkCompatible(newcfg *ChainConfig, head *big.Int) *ConfigCompatError {
	rogue := func(cfg *ChainConfig) bool {
		return cfg.Forge != nil && cfg.Forge.RewardSchedule == RogueRewardSchedule
	}
	if (rogue(c) || rogue(newcfg)) && !historicalEngineEqual(c, newcfg) {
		return newCompatError("RGN historical consensus engine", big.NewInt(0), big.NewInt(0))
	}
	if isForkIncompatible(c.HomesteadBlock, newcfg.HomesteadBlock, head) {
		return newCompatError("Homestead fork block", c.HomesteadBlock, newcfg.HomesteadBlock)
	}
	if isForkIncompatible(c.DAOForkBlock, newcfg.DAOForkBlock, head) {
		return newCompatError("DAO fork block", c.DAOForkBlock, newcfg.DAOForkBlock)
	}
	if c.IsDAOFork(head) && c.DAOForkSupport != newcfg.DAOForkSupport {
		return newCompatError("DAO fork support flag", c.DAOForkBlock, newcfg.DAOForkBlock)
	}
	if isForkIncompatible(c.EIP150Block, newcfg.EIP150Block, head) {
		return newCompatError("EIP150 fork block", c.EIP150Block, newcfg.EIP150Block)
	}
	if isForkIncompatible(c.EIP155Block, newcfg.EIP155Block, head) {
		return newCompatError("EIP155 fork block", c.EIP155Block, newcfg.EIP155Block)
	}
	if isForkIncompatible(c.EIP158Block, newcfg.EIP158Block, head) {
		return newCompatError("EIP158 fork block", c.EIP158Block, newcfg.EIP158Block)
	}
	if c.IsEIP158(head) && !configNumEqual(c.ChainId, newcfg.ChainId) {
		return newCompatError("EIP158 chain ID", c.EIP158Block, newcfg.EIP158Block)
	}
	if isForkIncompatible(c.ByzantiumBlock, newcfg.ByzantiumBlock, head) {
		return newCompatError("Byzantium fork block", c.ByzantiumBlock, newcfg.ByzantiumBlock)
	}
	if isForkIncompatible(c.ConstantinopleBlock, newcfg.ConstantinopleBlock, head) {
		return newCompatError("Constantinople fork block", c.ConstantinopleBlock, newcfg.ConstantinopleBlock)
	}
	if isForkIncompatible(c.ForgeBlock, newcfg.ForgeBlock, head) {
		return newCompatError("FORGE fork block", c.ForgeBlock, newcfg.ForgeBlock)
	}
	// Once FORGE is active, its consensus parameters are immutable. Treat a
	// parameter mismatch like a fork-config mismatch and require a rewind to
	// immediately before the activation block.
	if c.IsForge(head) && !forgeConfigEqual(c.Forge, newcfg.Forge) {
		return newCompatError("FORGE parameters", c.ForgeBlock, newcfg.ForgeBlock)
	}
	return nil
}

func historicalEngineEqual(x, y *ChainConfig) bool {
	if (x.ProgPoW == nil) != (y.ProgPoW == nil) || (x.Ethash == nil) != (y.Ethash == nil) || (x.Clique == nil) != (y.Clique == nil) {
		return false
	}
	if x.ProgPoW != nil && *x.ProgPoW != *y.ProgPoW {
		return false
	}
	return x.Clique == nil || *x.Clique == *y.Clique
}

func forgeConfigEqual(x, y *ForgeConfig) bool {
	if x == nil || y == nil {
		return x == y
	}
	return x.RequireDGBOrDOGE == y.RequireDGBOrDOGE &&
		x.RewardSchedule == y.RewardSchedule &&
		x.RequireMulti == y.RequireMulti && x.AllowAuxTree == y.AllowAuxTree &&
		x.PepecoinPayoutScriptHash == y.PepecoinPayoutScriptHash &&
		x.DingocoinPayoutScriptHash == y.DingocoinPayoutScriptHash &&
		x.BellscoinPayoutScriptHash == y.BellscoinPayoutScriptHash &&
		x.PrivateCPUTest == y.PrivateCPUTest &&
		x.RetargetV2 == y.RetargetV2 &&
		x.AuxChainID == y.AuxChainID &&
		x.DogeAuxChainID == y.DogeAuxChainID &&
		x.TargetBlockTime == y.TargetBlockTime &&
		configNumEqual(x.PreForgeGrossIssued, y.PreForgeGrossIssued) &&
		configNumEqual(x.InitialDifficulty, y.InitialDifficulty) &&
		configNumEqual(x.MinimumDifficulty, y.MinimumDifficulty) &&
		addressSliceEqual(x.TemplateSigners, y.TemplateSigners) &&
		x.TemplateQuorum == y.TemplateQuorum &&
		x.LTCPayoutScriptHash == y.LTCPayoutScriptHash &&
		x.DOGEPayoutScriptHash == y.DOGEPayoutScriptHash &&
		x.TemplateMaxValidity == y.TemplateMaxValidity
}

func addressSliceEqual(x, y []common.Address) bool {
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// isForkIncompatible returns true if a fork scheduled at s1 cannot be rescheduled to
// block s2 because head is already past the fork.
func isForkIncompatible(s1, s2, head *big.Int) bool {
	return (isForked(s1, head) || isForked(s2, head)) && !configNumEqual(s1, s2)
}

// isForked returns whether a fork scheduled at block s is active at the given head block.
func isForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

func configNumEqual(x, y *big.Int) bool {
	if x == nil {
		return y == nil
	}
	if y == nil {
		return x == nil
	}
	return x.Cmp(y) == 0
}

// ConfigCompatError is raised if the locally-stored blockchain is initialised with a
// ChainConfig that would alter the past.
type ConfigCompatError struct {
	What string
	// block numbers of the stored and new configurations
	StoredConfig, NewConfig *big.Int
	// the block number to which the local chain must be rewound to correct the error
	RewindTo uint64
}

func newCompatError(what string, storedblock, newblock *big.Int) *ConfigCompatError {
	var rew *big.Int
	switch {
	case storedblock == nil:
		rew = newblock
	case newblock == nil || storedblock.Cmp(newblock) < 0:
		rew = storedblock
	default:
		rew = newblock
	}
	err := &ConfigCompatError{what, storedblock, newblock, 0}
	if rew != nil && rew.Sign() > 0 {
		err.RewindTo = rew.Uint64() - 1
	}
	return err
}

func (err *ConfigCompatError) Error() string {
	return fmt.Sprintf("mismatching %s in database (have %d, want %d, rewindto %d)", err.What, err.StoredConfig, err.NewConfig, err.RewindTo)
}

// Rules wraps ChainConfig and is merely syntatic sugar or can be used for functions
// that do not have or require information about the block.
//
// Rules is a one time interface meaning that it shouldn't be used in between transition
// phases.
type Rules struct {
	ChainId                                   *big.Int
	IsHomestead, IsEIP150, IsEIP155, IsEIP158 bool
	IsByzantium                               bool
}

func (c *ChainConfig) Rules(num *big.Int) Rules {
	chainId := c.ChainId
	if chainId == nil {
		chainId = new(big.Int)
	}
	return Rules{ChainId: new(big.Int).Set(chainId), IsHomestead: c.IsHomestead(num), IsEIP150: c.IsEIP150(num), IsEIP155: c.IsEIP155(num), IsEIP158: c.IsEIP158(num), IsByzantium: c.IsByzantium(num)}
}
