package forge

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/params"
	"github.com/TeamEGEM/go-egem/rlp"
)

func EncodeMultiProof(p *MultiProof) ([]byte, error) {
	if p == nil || len(p.MinerExtra) > 32 {
		return nil, errors.New("invalid multi proof")
	}
	b, e := rlp.EncodeToBytes(p)
	if e != nil {
		return nil, e
	}
	version := p.Template.Version
	if version != 3 && version != 4 {
		return nil, errors.New("unsupported multi proof version")
	}
	out := append([]byte{'O', 'G', 'G', 'F', version}, b...)
	if len(out) > MaxProofSize {
		return nil, errors.New("multi proof too large")
	}
	return out, nil
}
func multiPayout(c *params.ForgeConfig, id uint32) common.Hash {
	switch id {
	case 98:
		return c.DOGEPayoutScriptHash
	case 63:
		return c.PepecoinPayoutScriptHash
	case 50:
		return c.DingocoinPayoutScriptHash
	case 16:
		return c.BellscoinPayoutScriptHash
	}
	return common.Hash{}
}
func canonicalMultiRoot(leaves map[uint32]common.Hash, nonce uint32) (common.Hash, uint32, error) {
	var size uint32 = 2
	var indices map[uint32]uint32
	for ; size <= 32; size <<= 1 {
		indices = map[uint32]uint32{}
		used := map[uint32]bool{}
		for id := range leaves {
			i := ExpectedChainIndex(nonce, id, size)
			if used[i] {
				break
			}
			used[i] = true
			indices[id] = i
		}
		if len(indices) == len(leaves) {
			break
		}
	}
	if size > 32 {
		return common.Hash{}, 0, errors.New("unsupported auxiliary collision")
	}
	level := make([]common.Hash, size)
	for id, hash := range leaves {
		level[indices[id]] = hash
	}
	for len(level) > 1 {
		next := make([]common.Hash, len(level)/2)
		for i := range next {
			next[i] = doubleSHA256(append(append([]byte{}, level[2*i][:]...), level[2*i+1][:]...))
		}
		level = next
	}
	return level[0], size, nil
}
func verifyMulti(c *params.ForgeConfig, h *types.Header, p *MultiProof) error {
	if err := verifyMultiBindings(c, h, p); err != nil {
		return err
	}
	raw, err := ScryptHash(p.ParentHeader)
	if err != nil {
		return err
	}
	target, err := TargetForConfig(c, h.Difficulty)
	if err != nil {
		return err
	}
	if scryptHashValue(raw).Cmp(target) > 0 {
		return errors.New("invalid FORGE Scrypt proof-of-work")
	}
	return nil
}
func verifyMultiBindings(c *params.ForgeConfig, h *types.Header, p *MultiProof) error {
	if !c.RequireMulti || !c.AllowAuxTree || p == nil {
		return errors.New("multi-chain proof not enabled")
	}
	if err := MultiTemplateValid(p.Template); err != nil {
		return err
	}
	t := p.Template
	if c.RewardSchedule == params.RogueRewardSchedule && t.Version != 4 {
		return errors.New("activated proof requires a mined V4 certificate")
	}
	if c.RewardSchedule == params.RogueRewardSchedule && t.Mask & ^uint8(3) != 0 {
		return errors.New("RGN mainnet proof contains a disabled external chain")
	}
	if c.RequireDGBOrDOGE && !RevenueAnchorPresent(t.Mask) {
		return errors.New("OGG issuance requires DGB or DOGE shared work")
	}
	if len(p.ParentHeader) != 80 || len(p.Aux) != len(t.Aux) || len(p.ParentMerkleBranch) > 20 || len(p.MinerExtra) > 32 {
		return errors.New("invalid multi proof dimensions")
	}
	work, err := WorkHash(h)
	if err != nil {
		return err
	}
	if err := ValidateMultiSignatures(t, work, p.Signatures, c.TemplateSigners, c.TemplateQuorum, h.Time.Uint64()); err != nil {
		return err
	}
	if t.Expires-t.ValidFrom > c.TemplateMaxValidity {
		return errors.New("template exceeds chain validity limit")
	}
	parent, err := parseCoinbase(p.ParentCoinbase)
	if err != nil {
		return err
	}
	if err := verifyCoinbaseMerkle(p.ParentHeader, parent, p.ParentMerkleBranch, 0); err != nil {
		return err
	}
	// Only the signed extranonce hole may differ from the independently
	// validated parent coinbase. It must lie inside scriptSig, after the
	// complete merged-mining commitment, never inside a treasury output.
	off, end := int(t.ExtraOffset), int(t.ExtraOffset+t.ExtraSize)
	magic := bytes.Index(parent.ScriptSig, mergedMiningMagic)
	if len(p.ParentCoinbase) < 42 || p.ParentCoinbase[4] != 1 || int(p.ParentCoinbase[41]) != len(parent.ScriptSig) || off < 42 || end > 42+len(parent.ScriptSig) || end > len(p.ParentCoinbase) || magic < 0 || off < 42+magic+44 {
		return errors.New("invalid parent extranonce authorization")
	}
	normalized := append([]byte{}, p.ParentCoinbase...)
	certificateIndex := -1
	if t.Version == 4 {
		certificateOffset, index, err := certificatePayloadOffset(parent)
		if err != nil {
			return err
		}
		if certificateOffset < end || certificateOffset+32 > len(normalized) {
			return errors.New("certificate commitment overlaps the extranonce or transaction boundary")
		}
		digest, err := MultiCertificateDigest(t, work, p.Signatures)
		if err != nil || !bytes.Equal(normalized[certificateOffset:certificateOffset+32], digest[:]) {
			return errors.New("certificate differs from mined parent commitment")
		}
		for i := certificateOffset; i < certificateOffset+32; i++ {
			normalized[i] = 0
		}
		certificateIndex = index
	}
	for i := off; i < end; i++ {
		normalized[i] = 0
	}
	branch, _ := rlp.EncodeToBytes(p.ParentMerkleBranch)
	if crypto.Keccak256Hash(normalized) != t.ParentCoinbaseHash || crypto.Keccak256Hash(branch) != t.ParentBranchHash {
		return errors.New("parent body differs from signed validated template")
	}
	parentTime := binary.LittleEndian.Uint32(p.ParentHeader[68:72])
	if binary.LittleEndian.Uint32(p.ParentHeader[:4]) != t.ParentVersion {
		return errors.New("parent version differs from signed native template")
	}
	if uint64(parentTime) > t.Expires {
		return errors.New("parent work time exceeds signed validity")
	}
	if t.Mask&1 != 0 {
		prev, e := bitcoinHeaderPrevHashDisplay(p.ParentHeader)
		if e != nil || prev != t.DGBPrevHash || binary.LittleEndian.Uint32(p.ParentHeader[72:76]) != t.DGBBits || parentTime < t.ParentMinTime {
			return errors.New("DGB parent differs from signed template")
		}
		if err := verifyPayouts(parent, c.LTCPayoutScriptHash); err != nil {
			return fmt.Errorf("DGB treasury: %v", err)
		}
	} else {
		if !bytes.Equal(p.ParentHeader[4:36], make([]byte, 32)) || binary.LittleEndian.Uint32(p.ParentHeader[:4]) != 1 || binary.LittleEndian.Uint32(p.ParentHeader[72:76]) != 0x1d00ffff || len(p.ParentMerkleBranch) != 0 {
			return errors.New("invalid synthetic Scrypt carrier")
		}
		wantOutputs := 1
		if t.Version == 4 {
			wantOutputs = 2
		}
		if len(parent.Outputs) != wantOutputs || (t.Version == 4 && certificateIndex != 1) || parent.Outputs[0].Value != 0 || !bytes.Equal(parent.Outputs[0].Script, []byte{0x6a}) {
			return errors.New("synthetic carrier cannot claim native revenue")
		}
	}
	leaves := map[uint32]common.Hash{c.AuxChainID: work}
	for i, a := range p.Aux {
		ref := t.Aux[i]
		if a.ChainID != ref.ChainID || len(a.MerkleBranch) > 20 {
			return errors.New("auxiliary evidence order/size mismatch")
		}
		if err := verifyDogeVersion(a.Header, a.ChainID); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(p.ParentHeader[:4])>>16 == a.ChainID {
			return errors.New("parent and child chain IDs collide")
		}
		raw, display, e := bitcoinHeaderHashDisplay(a.Header)
		if e != nil || display != ref.Hash {
			return errors.New("auxiliary header differs from signed native candidate")
		}
		cb, e := parseCoinbase(a.Coinbase)
		if e != nil {
			return e
		}
		required := multiPayout(c, a.ChainID)
		if required == (common.Hash{}) {
			return errors.New("unconfigured auxiliary treasury")
		}
		if e := verifyPayouts(cb, required); e != nil {
			return fmt.Errorf("auxiliary %d treasury: %v", a.ChainID, e)
		}
		if e := verifyCoinbaseMerkle(a.Header, cb, a.MerkleBranch, 0); e != nil {
			return e
		}
		leaves[a.ChainID] = raw
	}
	root, size, nonce, err := parseMergedMiningCommitment(parent.ScriptSig)
	if err != nil {
		return err
	}
	// Rebuild the entire canonical tree, including zero empty slots. An
	// undeclared extra chain with an unverified payout cannot hide in a branch.
	expected, wantSize, err := canonicalMultiRoot(leaves, nonce)
	if err != nil {
		return err
	}
	if root != expected || size != wantSize {
		return errors.New("noncanonical or hidden auxiliary commitment")
	}

	return nil
}
