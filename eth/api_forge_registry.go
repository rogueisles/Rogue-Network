package eth

import (
	"errors"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/common/hexutil"
	"github.com/TeamEGEM/go-egem/consensus/forge"
	"github.com/TeamEGEM/go-egem/rlp"
)

// ForgeRegistryState is bound to an exact RGN parent, not a mutable latest file.
type ForgeRegistryState struct {
	ParentHash           common.Hash      `json:"parentHash"`
	ParentHeight         hexutil.Uint64   `json:"parentHeight"`
	ActivationBlock      hexutil.Uint64   `json:"activationBlock"`
	Root                 common.Hash      `json:"root"`
	Checkpoint           hexutil.Uint64   `json:"checkpoint"`
	SnapshotRLP          hexutil.Bytes    `json:"snapshotRlp"`
	Genesis              common.Hash      `json:"genesis"`
	ChainID              *hexutil.Big     `json:"chainId"`
	WorkSigners          []common.Address `json:"workSigners"`
	MinDelay             hexutil.Uint64   `json:"minDelay"`
	MinOperationInterval hexutil.Uint64   `json:"minOperationInterval"`
}

func (api *PublicMinerAPI) ForgeRegistry(parentHash common.Hash) (*ForgeRegistryState, error) {
	chain := api.e.BlockChain()
	config := chain.Config()
	if config == nil || config.Forge == nil || config.Forge.RegistryBlock == nil {
		return nil, errors.New("FORGE registry is not scheduled")
	}
	parent := chain.GetHeaderByHash(parentHash)
	if parent == nil {
		return nil, errors.New("unknown FORGE registry parent")
	}
	snapshot, ctx, root, checkpoint, err := forge.RegistrySnapshotAtParent(config, chain, parent)
	if err != nil {
		return nil, err
	}
	raw, err := rlp.EncodeToBytes(snapshot)
	if err != nil {
		return nil, err
	}
	return &ForgeRegistryState{ParentHash: parent.Hash(), ParentHeight: hexutil.Uint64(parent.Number.Uint64()),
		ActivationBlock: hexutil.Uint64(config.Forge.RegistryBlock.Uint64()), Root: root, Checkpoint: hexutil.Uint64(checkpoint),
		SnapshotRLP: raw, Genesis: ctx.Genesis, ChainID: hexBigCopy(ctx.ChainID), WorkSigners: ctx.WorkSigners,
		MinDelay: hexutil.Uint64(ctx.MinDelay), MinOperationInterval: hexutil.Uint64(ctx.MinOperationInterval)}, nil
}
