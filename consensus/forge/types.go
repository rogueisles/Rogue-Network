package forge

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rlp"
)

const (
	ProofVersion       = uint8(1)
	TemplateVersion    = uint8(1)
	MaxProofSize       = 16 * 1024
	MaxMinerExtra      = 32
	MaxTemplateSigners = 16
)

var (
	proofMagic = []byte{'O', 'G', 'G', 'F'}
	ErrNoProof = errors.New("forge auxpow proof not present")
)

// ExternalTemplate is a quorum-signed snapshot of the external Scrypt chains.
// Consensus validates the submitted parent/candidate against this snapshot.
type ExternalTemplate struct {
	Version     uint8
	LTCPrevHash common.Hash
	LTCBits     uint32
	DogeAuxHash common.Hash
	ValidFrom   uint64
	Expires     uint64
}

// AuxPowProof contains all data needed to independently verify the Litecoin
// parent commitment and the embedded Dogecoin candidate. ParentCoinbase and
// DogeCoinbase are base (non-witness) transaction serializations because their
// txids are used by the Merkle proofs.
type AuxPowProofLegacy struct {
	MinerExtra         []byte
	ParentHeader       []byte
	ParentCoinbase     []byte
	ParentMerkleBranch []common.Hash
	ParentMerkleIndex  uint32
	DogeHeader         []byte
	DogeCoinbase       []byte
	DogeMerkleBranch   []common.Hash
	DogeMerkleIndex    uint32
	Template           ExternalTemplate
	Signatures         [][]byte
}

type AuxPowProofV2 struct {
	MinerExtra         []byte
	ParentHeader       []byte
	ParentCoinbase     []byte
	ParentMerkleBranch []common.Hash
	ParentMerkleIndex  uint32
	DogeHeader         []byte
	DogeCoinbase       []byte
	DogeMerkleBranch   []common.Hash
	DogeMerkleIndex    uint32
	Template           ExternalTemplate
	Signatures         [][]byte
	OGGChainBranch     []common.Hash
	DOGEChainBranch    []common.Hash
}

type AuxPowProof struct {
	Multi              *MultiProof
	MinerExtra         []byte
	ParentHeader       []byte
	ParentCoinbase     []byte
	ParentMerkleBranch []common.Hash
	ParentMerkleIndex  uint32
	DogeHeader         []byte
	DogeCoinbase       []byte
	DogeMerkleBranch   []common.Hash
	DogeMerkleIndex    uint32
	Template           ExternalTemplate
	Signatures         [][]byte
	OGGChainBranch     []common.Hash
	DOGEChainBranch    []common.Hash
}

type templateSigningPayload struct {
	Domain   []byte
	Template ExternalTemplate
}

// TemplateDigest is the domain-separated digest signed by authorised template
// signers. It is stable across implementations because the payload is RLP.
func TemplateDigest(t ExternalTemplate) common.Hash {
	blob, _ := rlp.EncodeToBytes(templateSigningPayload{
		Domain:   []byte("OGG-FORGE-TEMPLATE-V1"),
		Template: t,
	})
	return crypto.Keccak256Hash(blob)
}

// EncodeProof returns the bounded, versioned Header.Extra envelope.
func EncodeProof(p *AuxPowProof) ([]byte, error) {
	if p != nil && p.Multi != nil {
		return EncodeMultiProof(p.Multi)
	}
	if p == nil {
		return nil, errors.New("nil forge auxpow proof")
	}
	if len(p.MinerExtra) > MaxMinerExtra {
		return nil, fmt.Errorf("forge miner extra too long: %d > %d", len(p.MinerExtra), MaxMinerExtra)
	}
	var wire interface{} = legacyAuxPowProof(p)
	version := uint8(1)
	if len(p.OGGChainBranch) > 0 || len(p.DOGEChainBranch) > 0 {
		wire = asAuxPowProofV2(p)
		version = 2
	}
	payload, err := rlp.EncodeToBytes(wire)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(proofMagic)+1+len(payload))
	out = append(out, proofMagic...)
	out = append(out, version)
	out = append(out, payload...)
	if len(out) > MaxProofSize {
		return nil, fmt.Errorf("forge auxpow proof too large: %d > %d", len(out), MaxProofSize)
	}
	return out, nil
}

// DecodeProof decodes a FORGE envelope. ErrNoProof is returned for ordinary
// pre-submission miner extra; malformed FORGE-prefixed envelopes fail closed.
func DecodeProof(extra []byte) (*AuxPowProof, error) {
	if len(extra) < len(proofMagic) || !bytes.Equal(extra[:len(proofMagic)], proofMagic) {
		return nil, ErrNoProof
	}
	if len(extra) > MaxProofSize {
		return nil, fmt.Errorf("forge auxpow proof too large: %d > %d", len(extra), MaxProofSize)
	}
	if len(extra) < len(proofMagic)+1 {
		return nil, errors.New("truncated forge proof envelope")
	}
	if extra[len(proofMagic)] == 3 || extra[len(proofMagic)] == 4 {
		var m MultiProof
		if err := rlp.DecodeBytes(extra[len(proofMagic)+1:], &m); err != nil {
			return nil, err
		}
		if len(m.MinerExtra) > 32 {
			return nil, errors.New("multi miner extra too long")
		}
		if m.Template.Version != extra[len(proofMagic)] {
			return nil, errors.New("multi proof envelope version mismatch")
		}
		return &AuxPowProof{Multi: &m, MinerExtra: m.MinerExtra, ParentHeader: m.ParentHeader, ParentCoinbase: m.ParentCoinbase, ParentMerkleBranch: m.ParentMerkleBranch, Template: ExternalTemplate{ValidFrom: m.Template.ValidFrom, Expires: m.Template.Expires}}, nil
	}
	if extra[len(proofMagic)] != 1 && extra[len(proofMagic)] != 2 {
		return nil, fmt.Errorf("unsupported forge proof version %d", extra[len(proofMagic)])
	}
	var p AuxPowProof
	if extra[len(proofMagic)] == 1 {
		var old AuxPowProofLegacy
		if err := rlp.DecodeBytes(extra[len(proofMagic)+1:], &old); err != nil {
			return nil, err
		}
		p = *upgradeAuxPowProof(&old)
	} else {
		var v2 AuxPowProofV2
		if err := rlp.DecodeBytes(extra[len(proofMagic)+1:], &v2); err != nil {
			return nil, err
		}
		p = *fromAuxPowProofV2(&v2)
	}

	if len(p.MinerExtra) > MaxMinerExtra {
		return nil, fmt.Errorf("forge miner extra too long: %d > %d", len(p.MinerExtra), MaxMinerExtra)
	}
	return &p, nil
}

// WorkHash is the OGG AuxPoW leaf. Attaching the AuxPoW envelope must not alter
// it: if a proof is present, Header.Extra is replaced by proof.MinerExtra before
// hashing. The final Ethereum-style block hash still commits the full proof.
func WorkHash(header *types.Header) (common.Hash, error) {
	if header == nil {
		return common.Hash{}, errors.New("nil header")
	}
	cpy := *header
	if p, err := DecodeProof(header.Extra); err == nil {
		cpy.Extra = append([]byte(nil), p.MinerExtra...)
	} else if err != ErrNoProof {
		return common.Hash{}, err
	} else if len(header.Extra) > MaxMinerExtra {
		return common.Hash{}, fmt.Errorf("forge miner extra too long: %d > %d", len(header.Extra), MaxMinerExtra)
	}
	return cpy.Hash(), nil
}

func validateConfig(c *params.ForgeConfig) error {
	if c == nil {
		return errors.New("missing FORGE config")
	}
	if c.RewardSchedule != "" && c.RewardSchedule != params.RogueRewardSchedule {
		return errors.New("unknown FORGE reward schedule")
	}
	rogue := c.RewardSchedule == params.RogueRewardSchedule
	if rogue && (c.PepecoinPayoutScriptHash != (common.Hash{}) || c.DingocoinPayoutScriptHash != (common.Hash{}) || c.BellscoinPayoutScriptHash != (common.Hash{})) {
		return errors.New("RGN mainnet enables only DGB and DOGE external chains")
	}
	if c.RequireDGBOrDOGE && (!c.RequireMulti || !c.AllowAuxTree) {
		return errors.New("DGB/DOGE anchor policy requires multi-chain proofs and auxiliary tree")
	}
	if c.AuxChainID == 0 || c.DogeAuxChainID == 0 || c.AuxChainID == c.DogeAuxChainID {
		return errors.New("invalid FORGE auxiliary chain IDs")
	}
	if c.TargetBlockTime == 0 {
		return errors.New("missing FORGE target block time")
	}
	if c.RetargetV2 && (c.PrivateCPUTest || c.TargetBlockTime != 12) {
		return errors.New("FORGE retarget v2 requires canonical work and 12-second target")
	}
	if c.InitialDifficulty == nil || c.InitialDifficulty.Sign() <= 0 {
		return errors.New("missing FORGE initial difficulty")
	}
	if c.MinimumDifficulty == nil || c.MinimumDifficulty.Sign() <= 0 {
		return errors.New("missing FORGE minimum difficulty")
	}
	if c.RetargetV2 && c.InitialDifficulty.Cmp(diffOneTarget) > 0 {
		return errors.New("FORGE initial difficulty exceeds canonical target range")
	}
	if c.InitialDifficulty.Cmp(c.MinimumDifficulty) < 0 {
		return errors.New("FORGE initial difficulty below minimum")
	}
	if c.PrivateCPUTest && (c.TargetBlockTime != 12 || c.MinimumDifficulty.Cmp(big.NewInt(65536)) < 0) {
		return errors.New("private CPU test requires 12 seconds and minimum difficulty >= 65536")
	}
	if c.PreForgeGrossIssued == nil || c.PreForgeGrossIssued.Sign() < 0 {
		return errors.New("missing FORGE pre-fork gross issuance snapshot")
	}
	if len(c.TemplateSigners) == 0 || c.TemplateQuorum == 0 || c.TemplateQuorum > uint64(len(c.TemplateSigners)) {
		return errors.New("invalid FORGE template signer quorum")
	}
	if rogue && len(c.TemplateSigners) > MaxTemplateSigners {
		return errors.New("template signer set exceeds the bounded certificate capacity")
	}
	seen := make(map[common.Address]struct{})
	for _, signer := range c.TemplateSigners {
		if signer == (common.Address{}) {
			return errors.New("zero FORGE template signer")
		}
		if _, ok := seen[signer]; ok {
			return errors.New("duplicate FORGE template signer")
		}
		seen[signer] = struct{}{}
	}
	if c.LTCPayoutScriptHash == (common.Hash{}) || c.DOGEPayoutScriptHash == (common.Hash{}) {
		return errors.New("missing FORGE external payout script commitment")
	}
	if c.RequireMulti && (!c.AllowAuxTree || c.AuxChainID != 20295 || c.DogeAuxChainID != 98 || (!rogue && (c.PepecoinPayoutScriptHash == (common.Hash{}) || c.DingocoinPayoutScriptHash == (common.Hash{}) || c.BellscoinPayoutScriptHash == (common.Hash{})))) {
		return errors.New("multi-chain consensus requires all five pinned treasury destinations")
	}
	if c.TemplateMaxValidity == 0 {
		return errors.New("missing FORGE template validity bound")
	}
	return nil
}

func verifyTemplate(c *params.ForgeConfig, t ExternalTemplate, signatures [][]byte, blockTime uint64) error {
	return verifyTemplateWithDigest(c, t, signatures, blockTime, TemplateDigest(t))
}

func verifyTemplateWithDigest(c *params.ForgeConfig, t ExternalTemplate, signatures [][]byte, blockTime uint64, digest common.Hash) error {
	if t.Version != TemplateVersion && t.Version != 0x23 {
		return fmt.Errorf("unsupported external template version %d", t.Version)
	}
	if t.Expires < t.ValidFrom || t.Expires-t.ValidFrom > c.TemplateMaxValidity {
		return errors.New("invalid external template validity window")
	}
	if blockTime < t.ValidFrom || blockTime > t.Expires {
		return errors.New("external template not valid at block timestamp")
	}
	allowed := make(map[common.Address]struct{}, len(c.TemplateSigners))
	for _, a := range c.TemplateSigners {
		allowed[a] = struct{}{}
	}
	valid := make(map[common.Address]struct{})
	for _, sig := range signatures {
		if len(sig) != 65 {
			continue
		}
		pub, err := crypto.SigToPub(digest.Bytes(), sig)
		if err != nil {
			continue
		}
		addr := crypto.PubkeyToAddress(*pub)
		if _, ok := allowed[addr]; !ok {
			continue
		}
		valid[addr] = struct{}{}
	}
	if uint64(len(valid)) < c.TemplateQuorum {
		return fmt.Errorf("insufficient external template signatures: have %d want %d", len(valid), c.TemplateQuorum)
	}
	return nil
}

func copyBig(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

func legacyAuxPowProof(p *AuxPowProof) *AuxPowProofLegacy {
	return &AuxPowProofLegacy{MinerExtra: p.MinerExtra, ParentHeader: p.ParentHeader, ParentCoinbase: p.ParentCoinbase, ParentMerkleBranch: p.ParentMerkleBranch, ParentMerkleIndex: p.ParentMerkleIndex, DogeHeader: p.DogeHeader, DogeCoinbase: p.DogeCoinbase, DogeMerkleBranch: p.DogeMerkleBranch, DogeMerkleIndex: p.DogeMerkleIndex, Template: p.Template, Signatures: p.Signatures}
}
func upgradeAuxPowProof(p *AuxPowProofLegacy) *AuxPowProof {
	return &AuxPowProof{MinerExtra: p.MinerExtra, ParentHeader: p.ParentHeader, ParentCoinbase: p.ParentCoinbase, ParentMerkleBranch: p.ParentMerkleBranch, ParentMerkleIndex: p.ParentMerkleIndex, DogeHeader: p.DogeHeader, DogeCoinbase: p.DogeCoinbase, DogeMerkleBranch: p.DogeMerkleBranch, DogeMerkleIndex: p.DogeMerkleIndex, Template: p.Template, Signatures: p.Signatures}
}

// V2 authorization is bound to the exact issued OGG header. The structure and
// domain match the separately implemented pool encoder, including field order.
func verifyTemplateBound(c *params.ForgeConfig, t ExternalTemplate, signatures [][]byte, blockTime uint64, work common.Hash) error {
	if t.Version == TemplateVersion {
		return verifyTemplate(c, t, signatures, blockTime)
	}
	if t.Version != 0x23 || work == (common.Hash{}) {
		return fmt.Errorf("unsupported pilot template or zero OGG work")
	}
	payload := struct {
		Domain   []byte
		Template ExternalTemplate
		WorkHash common.Hash
	}{[]byte("OGG-FORGE-TEMPLATE-V2-WORK"), t, work}
	blob, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return err
	}
	return verifyTemplateWithDigest(c, t, signatures, blockTime, crypto.Keccak256Hash(blob))
}

func asAuxPowProofV2(p *AuxPowProof) *AuxPowProofV2 {
	return &AuxPowProofV2{MinerExtra: p.MinerExtra, ParentHeader: p.ParentHeader, ParentCoinbase: p.ParentCoinbase, ParentMerkleBranch: p.ParentMerkleBranch, ParentMerkleIndex: p.ParentMerkleIndex, DogeHeader: p.DogeHeader, DogeCoinbase: p.DogeCoinbase, DogeMerkleBranch: p.DogeMerkleBranch, DogeMerkleIndex: p.DogeMerkleIndex, Template: p.Template, Signatures: p.Signatures, OGGChainBranch: p.OGGChainBranch, DOGEChainBranch: p.DOGEChainBranch}
}
func fromAuxPowProofV2(p *AuxPowProofV2) *AuxPowProof {
	return &AuxPowProof{MinerExtra: p.MinerExtra, ParentHeader: p.ParentHeader, ParentCoinbase: p.ParentCoinbase, ParentMerkleBranch: p.ParentMerkleBranch, ParentMerkleIndex: p.ParentMerkleIndex, DogeHeader: p.DogeHeader, DogeCoinbase: p.DogeCoinbase, DogeMerkleBranch: p.DogeMerkleBranch, DogeMerkleIndex: p.DogeMerkleIndex, Template: p.Template, Signatures: p.Signatures, OGGChainBranch: p.OGGChainBranch, DOGEChainBranch: p.DOGEChainBranch}
}
