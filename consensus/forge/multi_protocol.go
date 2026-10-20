package forge

import (
	"errors"
	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/rlp"
	"math/big"
)

const MultiExternalMask uint8 = 31
const MultiDomain = "OGG-FORGE-MULTI-TEMPLATE-V3-WORK"

type MultiAuxRef struct {
	ChainID uint32
	Hash    common.Hash
}
type MultiTemplate struct {
	Version            uint8
	Mask               uint8
	DGBPrevHash        common.Hash
	DGBBits            uint32
	ParentMinTime      uint32
	ParentVersion      uint32
	ParentCoinbaseHash common.Hash
	ParentBranchHash   common.Hash
	ExtraOffset        uint32
	ExtraSize          uint32
	Aux                []MultiAuxRef
	ValidFrom          uint64
	Expires            uint64
}
type MultiAuxProof struct {
	ChainID      uint32
	Header       []byte
	Coinbase     []byte
	MerkleBranch []common.Hash
}
type MultiProof struct {
	MinerExtra         []byte
	ParentHeader       []byte
	ParentCoinbase     []byte
	ParentMerkleBranch []common.Hash
	Aux                []MultiAuxProof
	Template           MultiTemplate
	Signatures         [][]byte
}

func MultiChainBit(id uint32) uint8 {
	switch id {
	case 98:
		return 2
	case 63:
		return 4
	case 50:
		return 8
	case 16:
		return 16
	}
	return 0
}
func MultiTemplateValid(t MultiTemplate) error {
	if t.Version != 3 || t.Mask == 0 || t.Mask & ^MultiExternalMask != 0 || t.ValidFrom == 0 || t.Expires <= t.ValidFrom || t.Expires-t.ValidFrom > 300 {
		return errors.New("invalid multi-chain participation or validity")
	}
	if t.ParentCoinbaseHash == (common.Hash{}) || t.ParentBranchHash == (common.Hash{}) || t.ExtraSize < 1 || t.ExtraSize > 24 || t.ExtraOffset < 42 || t.ExtraOffset > 142 {
		return errors.New("missing parent body authorization")
	}
	if t.Mask&1 != 0 {
		if t.DGBPrevHash == (common.Hash{}) || t.DGBBits == 0 || t.ParentMinTime == 0 {
			return errors.New("missing native DGB parent")
		}
	} else if t.DGBPrevHash != (common.Hash{}) || t.DGBBits != 0 || t.ParentMinTime != 0 {
		return errors.New("inactive DGB carries signed parent fields")
	}
	mask := t.Mask & 1
	var prev uint32
	for _, a := range t.Aux {
		bit := MultiChainBit(a.ChainID)
		if bit == 0 || a.ChainID <= prev || a.Hash == (common.Hash{}) || mask&bit != 0 {
			return errors.New("invalid or duplicate auxiliary identity")
		}
		mask |= bit
		prev = a.ChainID
	}
	if mask != t.Mask {
		return errors.New("participation mask and auxiliary set differ")
	}
	return nil
}
func MultiDigest(t MultiTemplate, work common.Hash) (common.Hash, error) {
	if err := MultiTemplateValid(t); err != nil {
		return common.Hash{}, err
	}
	if work == (common.Hash{}) {
		return common.Hash{}, errors.New("zero OGG work")
	}
	payload := struct {
		Domain   []byte
		Template MultiTemplate
		WorkHash common.Hash
	}{[]byte(MultiDomain), t, work}
	b, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(b), nil
}
func ValidateMultiSignatures(t MultiTemplate, work common.Hash, sigs [][]byte, allowed []common.Address, quorum uint64, stamp uint64) error {
	digest, err := MultiDigest(t, work)
	if err != nil {
		return err
	}
	if t.ValidFrom != stamp || quorum == 0 || quorum > uint64(len(allowed)) {
		return errors.New("invalid multi-chain work timestamp or quorum")
	}
	authorized := map[common.Address]bool{}
	for _, a := range allowed {
		authorized[a] = true
	}
	seen := map[common.Address]bool{}
	for _, sig := range sigs {
		if len(sig) != 65 || !crypto.ValidateSignatureValues(sig[64], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:64]), true) {
			continue
		}
		pub, e := crypto.SigToPub(digest.Bytes(), sig)
		if e == nil {
			a := crypto.PubkeyToAddress(*pub)
			if authorized[a] {
				seen[a] = true
			}
		}
	}
	if uint64(len(seen)) < quorum {
		return errors.New("insufficient multi-chain template signatures")
	}
	return nil
}

// RevenueAnchorPresent requires supported DGB or DOGE participation. Bonus
// auxiliary chains cannot satisfy OGG issuance by themselves.
func RevenueAnchorPresent(mask uint8) bool {
	return mask & ^MultiExternalMask == 0 && mask&3 != 0
}
