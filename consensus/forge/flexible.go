package forge

import (
	"errors"
	"math/big"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/consensus"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rlp"
)

// FlexibleProof uses a new wire envelope. Legacy MultiProof RLP remains intact.
type FlexibleProof struct {
	Multi    MultiProof
	Registry RegistryEnvelope
}

func EncodeFlexibleProof(p *FlexibleProof) ([]byte, error) {
	if p == nil || p.Multi.Template.Version != 5 || len(p.Multi.MinerExtra) > MaxMinerExtra {
		return nil, errors.New("invalid flexible proof")
	}
	if err := FlexibleTemplateValid(p.Multi.Template); err != nil {
		return nil, err
	}
	b, err := rlp.EncodeToBytes(p)
	if err != nil {
		return nil, err
	}
	out := append([]byte{'O', 'G', 'G', 'F', 5}, b...)
	if len(out) > MaxProofSize {
		return nil, errors.New("flexible proof exceeds 16 KiB limit")
	}
	return out, nil
}

func FlexibleTemplateValid(t MultiTemplate) error {
	if t.Version != 5 || t.Mask == 0 || t.Mask & ^uint8(35) != 0 || t.ValidFrom == 0 || t.Expires <= t.ValidFrom || t.Expires-t.ValidFrom > 300 {
		return errors.New("invalid V5 core participation or validity")
	}
	if t.ParentCoinbaseHash == (common.Hash{}) || t.ParentBranchHash == (common.Hash{}) || t.ExtraSize < 1 || t.ExtraSize > 24 || t.ExtraOffset < 42 || t.ExtraOffset > 142 {
		return errors.New("missing V5 parent body authorization")
	}
	count := len(t.Aux)
	if t.Mask&1 != 0 {
		count++
		if t.DGBPrevHash == (common.Hash{}) || t.DGBBits == 0 || t.ParentMinTime == 0 {
			return errors.New("missing V5 DGB parent")
		}
	} else if t.DGBPrevHash != (common.Hash{}) || t.DGBBits != 0 || t.ParentMinTime != 0 {
		return errors.New("inactive V5 DGB parent fields")
	}
	if count == 0 || count > RegistryMaxMembers {
		return errors.New("V5 native participation exceeds limit")
	}
	mask := t.Mask & 1
	var previous uint32
	for _, aux := range t.Aux {
		if aux.ChainID == 0 || aux.ChainID > 65535 || aux.ChainID == 20295 || aux.ChainID <= previous || aux.Hash == (common.Hash{}) {
			return errors.New("invalid V5 auxiliary identity/order")
		}
		if aux.ChainID == 98 {
			mask |= 2
		}
		if aux.ChainID == SOQAuxChainID {
			mask |= ExternalSOQ
		}
		previous = aux.ChainID
	}
	if mask != t.Mask {
		return errors.New("V5 core mask differs from included native work")
	}
	return nil
}

func FlexibleSigningWork(work common.Hash, envelope RegistryEnvelope) (common.Hash, error) {
	if work == (common.Hash{}) {
		return common.Hash{}, errors.New("zero V5 RGN work")
	}
	registry, err := RegistryEnvelopeHash(&envelope)
	if err != nil {
		return common.Hash{}, err
	}
	b, err := rlp.EncodeToBytes(struct {
		Domain   []byte
		Work     common.Hash
		Registry common.Hash
	}{[]byte("RGN-FORGE-REGISTRY-WORK-V5"), work, registry})
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(b), nil
}

func registryBootstrap(config *params.ChainConfig) (RegistrySnapshot, error) {
	var snapshot RegistrySnapshot
	if config == nil || config.Forge == nil || len(config.Forge.RegistryBootstrap) == 0 || len(config.Forge.RegistryBootstrap) > 8192 {
		return snapshot, errors.New("missing or oversized registry bootstrap")
	}
	err := rlp.DecodeBytes(config.Forge.RegistryBootstrap, &snapshot)
	return snapshot, err
}

func validateRegistryConfig(c *params.ChainConfig) error {
	cfg := c.Forge
	if cfg.RegistryBlock == nil {
		if len(cfg.RegistryBootstrap) != 0 || cfg.RegistryMinDelay != 0 || cfg.RegistryMinOperationInterval != 0 || cfg.RegistryTargetBlockTimeMillis != 0 {
			return errors.New("disabled registry carries bootstrap parameters")
		}
		return nil
	}
	if cfg.RegistryBlock.Sign() <= 0 || cfg.RegistryBlock.BitLen() > 64 || !cfg.RequireMulti || !cfg.AllowAuxTree || !cfg.RequireDGBOrDOGE {
		return errors.New("invalid registry activation or native-source policy")
	}
	if c.ValidateRogueSchedule() {
		if cfg.RegistryBlock.Cmp(c.ForgeBlock) <= 0 || cfg.RegistryMinDelay < 720 || cfg.RegistryMinOperationInterval < 64 {
			return errors.New("mainnet registry requires a future height and bounded administrative delay")
		}
	} else if c.ChainId == nil || (c.ChainId.Cmp(big.NewInt(7008902)) != 0 && c.ChainId.Cmp(big.NewInt(7008914)) != 0) || cfg.RegistryBlock.Cmp(c.ForgeBlock) < 0 {
		return errors.New("registry staging requires an isolated allowed test chain")
	}
	if cfg.RegistryTargetBlockTimeMillis != 12500 {
		return errors.New("registry upgrade requires the explicit 12500 ms target")
	}
	if cfg.RegistryMinDelay == 0 || cfg.RegistryMinDelay > 1000000 || cfg.RegistryMinOperationInterval == 0 || cfg.RegistryMinOperationInterval > 1000000 {
		return errors.New("invalid registry delay/operation bound")
	}
	snapshot, err := registryBootstrap(c)
	if err != nil {
		return err
	}
	ctx := RegistryContext{ChainID: c.ChainId, Genesis: common.Hash{1}, WorkSigners: cfg.TemplateSigners, MinDelay: cfg.RegistryMinDelay, MinOperationInterval: cfg.RegistryMinOperationInterval}
	if err = ValidateRegistrySnapshot(snapshot, ctx); err != nil {
		return err
	}
	if snapshot.Admin.Sequence != 0 || snapshot.Admin.Epoch != 1 || snapshot.Admin.LastOperationHeight != 0 || snapshot.Admin.NextHeight != 0 {
		return errors.New("bootstrap registry has administrative history")
	}
	if len(snapshot.Coins) != 3 {
		return errors.New("bootstrap registry must contain exactly the three core sources")
	}
	for _, coin := range snapshot.Coins {
		if coin.EnabledFrom > cfg.RegistryBlock.Uint64() || coin.NextTreasuryHeight != 0 {
			return errors.New("bootstrap core source is unavailable or already rotating")
		}
	}
	if snapshot.Coins[0].Treasury != cfg.LTCPayoutScriptHash || snapshot.Coins[1].Treasury != cfg.DOGEPayoutScriptHash {
		return errors.New("bootstrap changes the existing DGB/DOGE treasury")
	}
	if c.ValidateRogueSchedule() {
		expected := []common.Hash{
			common.HexToHash("7497ea1b465eb39f1c8f507bc877078fe016d6fcb6dfad3a64c98dcc6e1e8496"),
			common.HexToHash("1a91e3dace36e2be3bf030a65679fe821aa1d6ef92e7c9902eb318182c355691"),
			common.HexToHash("0d828600816cbd7c23789660b53f90cb6ec7ff85540698e13845eb2d2f0486a8"),
		}
		for i, coin := range snapshot.Coins {
			if coin.Genesis != expected[i] {
				return errors.New("bootstrap native mainnet identity differs")
			}
		}
	}
	return nil
}

// RegistryParentContext uses the exact parent hash. It has no mutable registry
// database dependency and works with a header-only batch overlay and side forks.
func RegistryParentContext(config *params.ChainConfig, chain consensus.ChainReader, parent *types.Header) (common.Hash, uint64, RegistryContext, error) {
	var ctx RegistryContext
	if config == nil || config.Forge == nil || chain == nil || parent == nil || parent.Number == nil {
		return common.Hash{}, 0, ctx, errors.New("missing registry parent context")
	}
	genesis := chain.GetHeaderByNumber(0)
	if genesis == nil {
		return common.Hash{}, 0, ctx, errors.New("registry network genesis unavailable")
	}
	ctx = RegistryContext{ChainID: new(big.Int).Set(config.ChainId), Genesis: genesis.Hash(), WorkSigners: append([]common.Address(nil), config.Forge.TemplateSigners...), MinDelay: config.Forge.RegistryMinDelay, MinOperationInterval: config.Forge.RegistryMinOperationInterval}
	if config.IsForgeRegistry(parent.Number) {
		p, err := DecodeProof(parent.Extra)
		if err != nil || p.Flexible == nil {
			return common.Hash{}, 0, ctx, errors.New("registry parent lacks V5 proof")
		}
		ctx.ParentCheckpoint = p.Flexible.Registry.Checkpoint
		return p.Flexible.Registry.Root, p.Flexible.Registry.Checkpoint, ctx, nil
	}
	snapshot, err := registryBootstrap(config)
	if err != nil {
		return common.Hash{}, 0, ctx, err
	}
	root, err := RegistrySnapshotRoot(snapshot)
	return root, 0, ctx, err
}

func (f *Forge) verifyFlexible(chain consensus.ChainReader, h *types.Header, seal bool) error {
	if chain == nil || h == nil || h.Number == nil || !h.Number.IsUint64() || h.Number.Sign() <= 0 || h.Time == nil || !h.Time.IsUint64() || h.Time.Sign() <= 0 {
		return errors.New("V5 requires a known exact parent and valid time")
	}
	parent := chain.GetHeader(h.ParentHash, h.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	p, err := DecodeProof(h.Extra)
	if err != nil {
		return err
	}
	if p.Flexible == nil {
		return errors.New("registry activation requires V5 proofs")
	}
	root, _, ctx, err := RegistryParentContext(f.config, chain, parent)
	if err != nil {
		return err
	}
	view, err := ValidateRegistryEnvelope(root, h.Number.Uint64(), &p.Flexible.Registry, ctx)
	if err != nil {
		return err
	}
	if err = verifyFlexibleBindings(f.config.Forge, h, p.Flexible, *view); err != nil {
		return err
	}
	if seal {
		return f.verifyFlexiblePow(h)
	}
	return nil
}

func (f *Forge) verifyFlexiblePow(h *types.Header) error {
	p, err := DecodeProof(h.Extra)
	if err != nil || p.Flexible == nil {
		return errors.New("missing V5 proof")
	}
	raw, err := ScryptHash(p.Flexible.Multi.ParentHeader)
	if err != nil {
		return err
	}
	target, err := TargetForConfig(f.config.Forge, h.Difficulty)
	if err != nil {
		return err
	}
	if scryptHashValue(raw).Cmp(target) > 0 {
		return errors.New("invalid FORGE Scrypt proof-of-work")
	}
	return nil
}

func registryPayouts(h *types.Header, p *FlexibleProof, view RegistryView) (map[uint32]common.Hash, error) {
	want := make(map[uint32]bool)
	if p.Multi.Template.Mask&1 != 0 {
		want[0] = true
	}
	for _, aux := range p.Multi.Template.Aux {
		want[aux.ChainID] = true
	}
	if len(want) != len(view.Coins) {
		return nil, errors.New("V5 registry membership does not match native work")
	}
	payouts := make(map[uint32]common.Hash, len(want))
	for _, coin := range view.Coins {
		if !want[coin.ChainID] {
			return nil, errors.New("V5 registry includes unused native source")
		}
		treasury, err := RegistryTreasury(coin, h.Number.Uint64())
		if err != nil {
			return nil, err
		}
		payouts[coin.ChainID] = treasury
	}
	return payouts, nil
}

// RegistrySnapshotAtParent serves operators from the checkpoint included in
// headers. The root comparison protects against a concurrent canonical reorg.
func RegistrySnapshotAtParent(config *params.ChainConfig, chain consensus.ChainReader, parent *types.Header) (RegistrySnapshot, RegistryContext, common.Hash, uint64, error) {
	root, checkpoint, ctx, err := RegistryParentContext(config, chain, parent)
	if err != nil {
		return RegistrySnapshot{}, ctx, root, checkpoint, err
	}
	if checkpoint == 0 {
		snapshot, e := registryBootstrap(config)
		return snapshot, ctx, root, checkpoint, e
	}
	operationHeader := chain.GetHeaderByNumber(checkpoint)
	if operationHeader == nil {
		return RegistrySnapshot{}, ctx, root, checkpoint, errors.New("registry checkpoint unavailable on this chain view")
	}
	p, e := DecodeProof(operationHeader.Extra)
	if e != nil || p.Flexible == nil || len(p.Flexible.Registry.Operations) != 1 {
		return RegistrySnapshot{}, ctx, root, checkpoint, errors.New("invalid registry checkpoint proof")
	}
	op := p.Flexible.Registry.Operations[0]
	snapshot, e := RegistryApplyChange(op.Before, checkpoint, op.Change, ctx)
	if e != nil {
		return snapshot, ctx, root, checkpoint, e
	}
	got, e := RegistrySnapshotRoot(snapshot)
	if e != nil || got != root {
		return snapshot, ctx, root, checkpoint, errors.New("registry checkpoint differs from exact parent root; retry after reorg")
	}
	return snapshot, ctx, root, checkpoint, nil
}
