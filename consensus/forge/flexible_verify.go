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

func verifyFlexibleBindings(c *params.ForgeConfig, h *types.Header, proof *FlexibleProof, view RegistryView) error {
	p := &proof.Multi
	payouts, err := registryPayouts(h, proof, view)
	if err != nil {
		return err
	}
	if !c.RequireMulti || !c.AllowAuxTree || p == nil {
		return errors.New("multi-chain proof not enabled")
	}
	if err := MultiTemplateValid(p.Template); err != nil {
		return err
	}
	t := p.Template
	if t.Version != 5 || t.Mask & ^uint8(35) != 0 || t.Mask&35 == 0 {
		return errors.New("V5 requires an established native core source")
	}
	if len(p.ParentHeader) != 80 || len(p.Aux) != len(t.Aux) || len(p.ParentMerkleBranch) > 20 || len(p.MinerExtra) > 32 {
		return errors.New("invalid multi proof dimensions")
	}
	work, err := WorkHash(h)
	if err != nil {
		return err
	}
	boundWork, err := FlexibleSigningWork(work, proof.Registry)
	if err != nil {
		return err
	}
	if err := ValidateMultiSignatures(t, boundWork, p.Signatures, c.TemplateSigners, c.TemplateQuorum, h.Time.Uint64()); err != nil {
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
	if t.Version == 5 {
		certificateOffset, index, err := certificatePayloadOffsetVersion(parent, 5)
		if err != nil {
			return err
		}
		if certificateOffset < end || certificateOffset+32 > len(normalized) {
			return errors.New("certificate commitment overlaps the extranonce or transaction boundary")
		}
		digest, err := MultiCertificateDigest(t, boundWork, p.Signatures)
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
		if err := verifyPayouts(parent, payouts[0]); err != nil {
			return fmt.Errorf("DGB treasury: %v", err)
		}
	} else {
		if !bytes.Equal(p.ParentHeader[4:36], make([]byte, 32)) || binary.LittleEndian.Uint32(p.ParentHeader[:4]) != 1 || binary.LittleEndian.Uint32(p.ParentHeader[72:76]) != 0x1d00ffff || len(p.ParentMerkleBranch) != 0 {
			return errors.New("invalid synthetic Scrypt carrier")
		}
		wantOutputs := 1
		if t.Version == 5 {
			wantOutputs = 2
		}
		if len(parent.Outputs) != wantOutputs || (t.Version == 5 && certificateIndex != 1) || parent.Outputs[0].Value != 0 || !bytes.Equal(parent.Outputs[0].Script, []byte{0x6a}) {
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
		required, ok := payouts[a.ChainID]
		if !ok || required == (common.Hash{}) {
			return errors.New("unregistered V5 auxiliary treasury")
		}
		if e := verifyPayouts(cb, required); e != nil {
			return fmt.Errorf("auxiliary %d registry treasury: %v", a.ChainID, e)
		}
		if a.ChainID == SOQAuxChainID && !validSOQOutputs(cb) {
			return errors.New("invalid SOQ registered witness destination")
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
