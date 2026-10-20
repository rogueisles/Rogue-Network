package forge

import (
	"errors"
	"math/big"

	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
)

// HeaderWork preserves accumulated historical difficulty and converts activated
// Scrypt targets to expected hash work. Seal validation remains mandatory.
func HeaderWork(config *params.ChainConfig, header *types.Header) (*big.Int, error) {
	if header == nil || header.Number == nil || header.Number.Sign() < 0 || header.Difficulty == nil || header.Difficulty.Sign() <= 0 {
		return nil, errors.New("invalid header work parameters")
	}
	if config == nil || !config.IsRogue(header.Number) {
		return new(big.Int).Set(header.Difficulty), nil
	}
	if config.Forge.PrivateCPUTest {
		return nil, errors.New("activated work requires the canonical Scrypt target")
	}
	target, err := TargetForDifficulty(header.Difficulty)
	if err != nil || target.Sign() <= 0 {
		return nil, errors.New("invalid activated Scrypt target")
	}
	// A hash is valid at or below target, giving target+1 successful values.
	return new(big.Int).Div(new(big.Int).Lsh(big.NewInt(1), 256), new(big.Int).Add(target, big.NewInt(1))), nil
}
