package forge

import (
	"errors"
	"github.com/TeamEGEM/go-egem/common"
)

func branchRoot(leaf common.Hash, branch []common.Hash, index uint32) common.Hash {
	for _, sibling := range branch {
		var pair []byte
		if index&1 == 0 {
			pair = append(append([]byte{}, leaf.Bytes()...), sibling.Bytes()...)
		} else {
			pair = append(append([]byte{}, sibling.Bytes()...), leaf.Bytes()...)
		}
		leaf = doubleSHA256(pair)
		index >>= 1
	}
	return leaf
}

// Extra leaves are optional pool revenue. OGG still requires cryptographic
// membership and treasury outputs for its signed DGB and DOGE backing work.
// Arbitrary extra leaves never weaken those checks or count as proven revenue.
func verifyExpandedCommitment(cb *parsedCoinbase, ogg, doge common.Hash, oggID, dogeID uint32, oggBranch, dogeBranch []common.Hash) error {
	if len(oggBranch) == 0 || len(oggBranch) > 16 || len(dogeBranch) != len(oggBranch) {
		return errors.New("invalid expanded auxiliary branch depths")
	}
	committed, size, nonce, err := parseMergedMiningCommitment(cb.ScriptSig)
	if err != nil {
		return err
	}
	if size != uint32(1)<<uint(len(oggBranch)) {
		return errors.New("expanded auxiliary tree size mismatch")
	}
	oi := ExpectedChainIndex(nonce, oggID, size)
	di := ExpectedChainIndex(nonce, dogeID, size)
	if oi == di {
		return errors.New("OGG and DOGE auxiliary indices collide")
	}
	if branchRoot(ogg, oggBranch, oi) != committed || branchRoot(doge, dogeBranch, di) != committed {
		return errors.New("expanded OGG or DOGE membership proof mismatch")
	}
	return nil
}
