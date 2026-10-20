// rogue-config prepares a candidate chain configuration OFFLINE. It never
// connects to an RPC server, opens a node database, or activates a fork.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/core"
	"github.com/TeamEGEM/go-egem/params"
)

var mainnetGenesis = common.HexToHash("0xdc4d38c0ef41d3fdf50642fdf5fc99e221eadd09e73bbe616724f55a136270ab")

func prepare(genesisJSON, proofJSON []byte) ([]byte, error) {
	var g core.Genesis
	if err := json.Unmarshal(genesisJSON, &g); err != nil {
		return nil, err
	}
	if !g.Config.HasRogueHistoricalRules() || g.Config.Forge != nil || g.Config.ForgeBlock != nil {
		return nil, errors.New("expected existing unforked OGG mainnet genesis configuration")
	}
	if g.ToBlock(nil).Hash() != mainnetGenesis {
		return nil, errors.New("mainnet genesis hash mismatch; no candidate prepared")
	}
	var f params.ForgeConfig
	if err := json.Unmarshal(proofJSON, &f); err != nil {
		return nil, err
	}
	// Economic parameters are compiled rules. Operators supply only the public
	// native proof deployment parameters, not adjustable issuance amounts.
	f.RewardSchedule = params.RogueRewardSchedule
	f.PreForgeGrossIssued, _ = new(big.Int).SetString(params.RoguePreForgeGrossIssuedWei, 10)
	f.RequireDGBOrDOGE = true
	f.RequireMulti = true
	f.AllowAuxTree = true
	f.RetargetV2 = true
	f.TargetBlockTime = 12
	for _, signer := range f.TemplateSigners {
		if signer == common.HexToAddress("0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf") {
			return nil, errors.New("public development template signer must not be used for mainnet")
		}
	}
	old := g.Config
	cloneBytes, err := json.Marshal(old)
	if err != nil {
		return nil, err
	}
	var cfg params.ChainConfig
	if err = json.Unmarshal(cloneBytes, &cfg); err != nil {
		return nil, err
	}
	cfg.ForgeBlock = big.NewInt(params.RogueActivationHeight)
	cfg.Forge = &f
	if compat := old.CheckCompatible(&cfg, uint64(params.RogueActivationHeight-1)); compat != nil {
		return nil, compat
	}
	if _, err = forge.New(&cfg); err != nil {
		return nil, err
	}
	g.Config = &cfg
	if g.ToBlock(nil).Hash() != mainnetGenesis {
		return nil, errors.New("candidate would alter genesis")
	}
	// Keep all supplied genesis fields and comments verbatim in structure,
	// replacing only config; allocation and existing historical rules survive.
	var original map[string]json.RawMessage
	if err = json.Unmarshal(genesisJSON, &original); err != nil {
		return nil, err
	}
	original["config"], err = json.Marshal(&cfg)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(original, "", "  ")
}

func main() {
	genesis := flag.String("genesis", "", "existing exported mainnet genesis JSON")
	proof := flag.String("proof-config", "", "reviewed PUBLIC proof settings; never private keys")
	output := flag.String("out", "", "new candidate file; existing files are never overwritten")
	flag.Parse()
	if *genesis == "" || *proof == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "Required: -genesis existing.json -proof-config public.json -out candidate.json. This is not a deployment tool.")
		os.Exit(2)
	}
	read := func(p string) []byte {
		b, e := ioutil.ReadFile(p)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return b
	}
	result, err := prepare(read(*genesis), read(*proof))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_, writeErr := f.Write(append(result, '\n'))
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		fmt.Fprintln(os.Stderr, "candidate write failed", writeErr, closeErr)
		os.Exit(1)
	}
	fmt.Println("Candidate prepared for review only. Existing genesis and chain ID preserved. No node configuration or database changed.")
}
