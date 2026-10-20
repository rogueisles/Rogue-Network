package forge

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
	"golang.org/x/crypto/scrypt"
)

// ScryptHash computes Litecoin-compatible scrypt_1024_1_1_256 over an 80-byte
// parent header. The returned hash is raw/internal byte order.
func ScryptHash(header []byte) (common.Hash, error) {
	if len(header) != 80 {
		return common.Hash{}, fmt.Errorf("invalid Scrypt header length %d", len(header))
	}
	out, err := scrypt.Key(header, header, 1024, 1, 1, 32)
	if err != nil {
		return common.Hash{}, err
	}
	return common.BytesToHash(out), nil
}

func scryptHashValue(raw common.Hash) *big.Int {
	rev := make([]byte, 32)
	for i := range rev {
		rev[i] = raw[31-i]
	}
	return new(big.Int).SetBytes(rev)
}

func verifyAuxPow(config *params.ForgeConfig, header *types.Header, p *AuxPowProof) error {
	if err := validateConfig(config); err != nil {
		return err
	}
	if p == nil {
		return errors.New("missing FORGE AuxPoW proof")
	}
	if p.Multi != nil {
		return verifyMulti(config, header, p.Multi)
	}
	if config.RequireMulti {
		return errors.New("D14 requires a nonempty multi-chain treasury proof")
	}
	if len(p.ParentHeader) != 80 || len(p.DogeHeader) != 80 {
		return errors.New("invalid external header length")
	}
	if p.ParentMerkleIndex != 0 || p.DogeMerkleIndex != 0 {
		return errors.New("FORGE external coinbase must be Merkle index zero")
	}
	boundWork, err := WorkHash(header)
	if err != nil {
		return err
	}
	if err := verifyTemplateBound(config, p.Template, p.Signatures, header.Time.Uint64(), boundWork); err != nil {
		return err
	}

	ltcPrev, err := bitcoinHeaderPrevHashDisplay(p.ParentHeader)
	if err != nil {
		return err
	}
	if ltcPrev != p.Template.LTCPrevHash {
		return errors.New("Litecoin parent does not match signed previous hash")
	}
	bits, err := bitcoinHeaderBits(p.ParentHeader)
	if err != nil {
		return err
	}
	if bits != p.Template.LTCBits {
		return errors.New("Litecoin parent does not match signed nBits")
	}

	parentCB, err := parseCoinbase(p.ParentCoinbase)
	if err != nil {
		return fmt.Errorf("invalid Litecoin coinbase: %v", err)
	}
	if err := verifyPayouts(parentCB, config.LTCPayoutScriptHash); err != nil {
		return fmt.Errorf("invalid Litecoin payout: %v", err)
	}
	if err := verifyCoinbaseMerkle(p.ParentHeader, parentCB, p.ParentMerkleBranch, p.ParentMerkleIndex); err != nil {
		return fmt.Errorf("invalid Litecoin Merkle proof: %v", err)
	}

	if err := verifyDogeVersion(p.DogeHeader, config.DogeAuxChainID); err != nil {
		return err
	}
	dogeRaw, dogeDisplay, err := bitcoinHeaderHashDisplay(p.DogeHeader)
	if err != nil {
		return err
	}
	if dogeDisplay != p.Template.DogeAuxHash {
		return errors.New("DOGE candidate does not match signed template hash")
	}
	dogeCB, err := parseCoinbase(p.DogeCoinbase)
	if err != nil {
		return fmt.Errorf("invalid DOGE coinbase: %v", err)
	}
	if err := verifyPayouts(dogeCB, config.DOGEPayoutScriptHash); err != nil {
		return fmt.Errorf("invalid DOGE payout: %v", err)
	}
	if err := verifyCoinbaseMerkle(p.DogeHeader, dogeCB, p.DogeMerkleBranch, p.DogeMerkleIndex); err != nil {
		return fmt.Errorf("invalid DOGE Merkle proof: %v", err)
	}

	oggWork, err := WorkHash(header)
	if err != nil {
		return err
	}
	if len(p.OGGChainBranch) > 0 || len(p.DOGEChainBranch) > 0 {
		if !config.AllowAuxTree {
			return errors.New("expanded auxiliary tree is not enabled")
		}
		if err := verifyExpandedCommitment(parentCB, oggWork, dogeRaw, config.AuxChainID, config.DogeAuxChainID, p.OGGChainBranch, p.DOGEChainBranch); err != nil {
			return err
		}
	} else if err := verifyMergedMiningCommitment(parentCB, oggWork, dogeRaw, config.AuxChainID, config.DogeAuxChainID); err != nil {
		return err
	}

	scryptRaw, err := ScryptHash(p.ParentHeader)
	if err != nil {
		return err
	}
	target, err := TargetForConfig(config, header.Difficulty)
	if err != nil {
		return err
	}
	if scryptHashValue(scryptRaw).Cmp(target) > 0 {
		return errors.New("invalid FORGE Scrypt proof-of-work")
	}
	return nil
}
