package forge

import (
	"errors"
	"math/big"

	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
)

var diffOneTarget = mustHexBig("00000000ffff0000000000000000000000000000000000000000000000000000")

func mustHexBig(s string) *big.Int {
	v, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("invalid FORGE consensus integer")
	}
	return v
}

// TargetForDifficulty maps Scrypt difficulty 1 to Bitcoin/Litecoin's canonical
// 0x1d00ffff target and scales inversely for higher integer difficulty.
func TargetForDifficulty(diff *big.Int) (*big.Int, error) {
	if diff == nil || diff.Sign() <= 0 {
		return nil, errors.New("invalid FORGE difficulty")
	}
	return new(big.Int).Div(new(big.Int).Set(diffOneTarget), diff), nil
}

// TargetForConfig preserves the legacy target unless the isolated CPU network
// explicitly opts in. One CPU difficulty unit is 2^-32 canonical difficulty;
// the 256-bit numerator remains below 2^256. Header difficulty and total chain
// work use this same fixed domain throughout the private chain.
func TargetForConfig(c *params.ForgeConfig, diff *big.Int) (*big.Int, error) {
	if c == nil || !c.PrivateCPUTest {
		return TargetForDifficulty(diff)
	}
	if err := validateConfig(c); err != nil {
		return nil, err
	}
	if diff == nil || diff.Sign() <= 0 || diff.Cmp(c.MinimumDifficulty) < 0 {
		return nil, errors.New("invalid private CPU difficulty")
	}
	target := new(big.Int).Lsh(new(big.Int).Set(diffOneTarget), 32)
	target.Div(target, diff)
	if target.Sign() == 0 {
		return nil, errors.New("zero private CPU target")
	}
	return target, nil
}

// calcCPUDifficulty retains the fractional 1/8 damping until the final division.
// This avoids the legacy 5..19-second rounding dead band. It is test-only.
func calcCPUDifficulty(c *params.ForgeConfig, timestamp uint64, parent *types.Header) *big.Int {
	actual := uint64(1)
	if timestamp > parent.Time.Uint64() {
		actual = timestamp - parent.Time.Uint64()
	}
	// At >=60 seconds the 150% upper spacing clamp already applies.
	if actual > 60 {
		actual = 60
	}
	span8 := int64(84 + actual) // 8*12 + (actual-12)
	if span8 < 72 {
		span8 = 72
	}
	if span8 > 144 {
		span8 = 144
	}
	d := new(big.Int).Mul(parent.Difficulty, big.NewInt(96))
	d.Div(d, big.NewInt(span8))
	if d.Cmp(c.MinimumDifficulty) < 0 {
		d.Set(c.MinimumDifficulty)
	}
	return d
}

// CalcDifficulty implements a 12-second DigiShield-style single-block retarget.
// The observed spacing is damped by 1/8 and bounded to 75%-150% of target
// spacing, preventing abrupt swings while reacting every block.
func CalcDifficulty(config *params.ChainConfig, timestamp uint64, parent *types.Header) *big.Int {
	if config == nil || config.Forge == nil || parent == nil {
		return new(big.Int)
	}
	fc := config.Forge
	next := new(big.Int).Add(parent.Number, big.NewInt(1))
	if config.IsForgeRegistry(next) {
		reference := parent
		if config.ForgeBlock != nil && next.Cmp(config.ForgeBlock) == 0 {
			initial := *parent
			initial.Difficulty = fc.InitialDifficulty
			reference = &initial
		}
		if fc.PrivateCPUTest || !fc.RetargetV2 {
			return calcRegistryDampedDifficulty(fc, timestamp, reference)
		}
		return calcRegistryRetarget(fc, timestamp, reference)
	}
	if config.ForgeBlock != nil && next.Cmp(config.ForgeBlock) == 0 {
		if fc.RetargetV2 {
			// The configured initial value is a reference at 12 seconds, not
			// a fixed first-block barrier. Allow overdue activation work to
			// ease too, without importing pre-FORGE difficulty units.
			initialParent := *parent
			initialParent.Difficulty = fc.InitialDifficulty
			return calcRetargetV2(fc, timestamp, &initialParent)
		}
		return copyBig(fc.InitialDifficulty)
	}
	if parent.Difficulty == nil || parent.Difficulty.Sign() <= 0 || fc.TargetBlockTime == 0 {
		return copyBig(fc.InitialDifficulty)
	}
	if fc.PrivateCPUTest {
		return calcCPUDifficulty(fc, timestamp, parent)
	}
	if fc.RetargetV2 {
		return calcRetargetV2(fc, timestamp, parent)
	}
	target := int64(fc.TargetBlockTime)
	actual := int64(1)
	if timestamp > parent.Time.Uint64() {
		delta := timestamp - parent.Time.Uint64()
		if delta > uint64(^uint64(0)>>1) {
			delta = uint64(^uint64(0) >> 1)
		}
		actual = int64(delta)
	}
	modulated := target + (actual-target)/8
	minSpan := target - target/4
	maxSpan := target + target/2
	if minSpan < 1 {
		minSpan = 1
	}
	if modulated < minSpan {
		modulated = minSpan
	}
	if modulated > maxSpan {
		modulated = maxSpan
	}

	diff := new(big.Int).Mul(parent.Difficulty, big.NewInt(target))
	diff.Div(diff, big.NewInt(modulated))
	if fc.MinimumDifficulty != nil && diff.Cmp(fc.MinimumDifficulty) < 0 {
		diff.Set(fc.MinimumDifficulty)
	}
	return diff
}

// calcRetargetV2 is an experimental relative exponential retarget, not BCH
// ASERT consensus. Its half-life is 120 seconds: D_next ~= D_parent *
// 2^((12 - candidate_spacing)/120). Unlike the legacy clamp, work can ease
// further while a block is overdue. Repeated ideal spacings leave D unchanged.
// Integer low-difficulty steps and canonical minimum remain explicit limits.
//
// Only the deterministic 16-bit 2^x approximation is reused from the CC0 ASERT
// specification (freetrader, Toomim, Culianu, Lundeberg; 2020-08-12):
// https://upgradespecs.bitcoincashnode.org/2020-11-15-asert/
// This uses a relative parent, candidate time, integer DIFFICULTY (not compact
// target), a 120s half-life and directional rounding. It is not an audited or
// bit-compatible implementation of aserti3-2d.
func calcRetargetV2(c *params.ForgeConfig, timestamp uint64, parent *types.Header) *big.Int {
	actual := uint64(1)
	if timestamp > parent.Time.Uint64() {
		actual = timestamp - parent.Time.Uint64()
	}
	// Avoid overflowing signed exponent arithmetic or allocating giant shifts.
	// Past this bound the scaled parent is below one, even allowing polynomial
	// approximation and nearest-integer rounding; the configured floor wins.
	if actual > 12+120*uint64(parent.Difficulty.BitLen()+2) {
		return copyBig(c.MinimumDifficulty)
	}
	exponent := (int64(12) - int64(actual)) * 65536 / 120 // Go truncates toward zero.
	shifts := exponent >> 16                              // arithmetic floor for negative exponents
	frac := uint64(exponent - shifts*65536)
	fractional := (195766423245049*frac + 971821376*frac*frac + 5127*frac*frac*frac + (1 << 47)) >> 48
	factor := uint64(65536) + fractional
	numerator := new(big.Int).Mul(parent.Difficulty, new(big.Int).SetUint64(factor))
	denominator := new(big.Int).Lsh(big.NewInt(1), 16)
	if shifts < 0 {
		denominator.Lsh(denominator, uint(-shifts))
	} else {
		numerator.Lsh(numerator, uint(shifts))
	}
	numerator.Add(numerator, new(big.Int).Rsh(new(big.Int).Set(denominator), 1))
	diff := numerator.Div(numerator, denominator)
	if diff.Cmp(parent.Difficulty) == 0 {
		if actual < 12 {
			diff.Add(diff, big.NewInt(1))
		} else if actual > 12 {
			diff.Sub(diff, big.NewInt(1))
		}
	}
	if diff.Cmp(c.MinimumDifficulty) < 0 {
		diff.Set(c.MinimumDifficulty)
	}
	// Keep a nonzero canonical Scrypt target, even at the numeric upper bound.
	if diff.Cmp(diffOneTarget) > 0 {
		diff.Set(diffOneTarget)
	}
	return diff
}

// RegistryTargetMillis is height-gated; historical headers retain the original
// integer-second target and retarget implementation byte for byte.
func RegistryTargetMillis(config *params.ChainConfig, number *big.Int) uint64 {
	if config == nil || !config.IsForge(number) {
		return 0
	}
	if config.IsForgeRegistry(number) {
		return config.Forge.RegistryTargetBlockTimeMillis
	}
	if config != nil && config.Forge != nil {
		return config.Forge.TargetBlockTime * 1000
	}
	return 0
}

func calcRegistryDampedDifficulty(c *params.ForgeConfig, timestamp uint64, parent *types.Header) *big.Int {
	actual := uint64(1)
	if timestamp > parent.Time.Uint64() {
		actual = timestamp - parent.Time.Uint64()
	}
	if actual > 60 {
		actual = 60
	}
	target := c.RegistryTargetBlockTimeMillis
	span8 := 7*target + actual*1000
	if span8 < 6*target {
		span8 = 6 * target
	}
	if span8 > 12*target {
		span8 = 12 * target
	}
	diff := new(big.Int).Mul(parent.Difficulty, new(big.Int).SetUint64(8*target))
	diff.Div(diff, new(big.Int).SetUint64(span8))
	if diff.Cmp(c.MinimumDifficulty) < 0 {
		diff.Set(c.MinimumDifficulty)
	}
	if !c.PrivateCPUTest && diff.Cmp(diffOneTarget) > 0 {
		diff.Set(diffOneTarget)
	}
	return diff
}

// The existing relative exponential algorithm and 120-second half-life are
// retained. Only the target term becomes 12500 milliseconds after activation.
func calcRegistryRetarget(c *params.ForgeConfig, timestamp uint64, parent *types.Header) *big.Int {
	actual := uint64(1)
	if timestamp > parent.Time.Uint64() {
		actual = timestamp - parent.Time.Uint64()
	}
	// Avoid overflowing signed exponent arithmetic or allocating giant shifts.
	// Past this bound the scaled parent is below one, even allowing polynomial
	// approximation and nearest-integer rounding; the configured floor wins.
	if actual > 13+120*uint64(parent.Difficulty.BitLen()+2) {
		return copyBig(c.MinimumDifficulty)
	}
	exponent := (int64(c.RegistryTargetBlockTimeMillis) - int64(actual)*1000) * 65536 / 120000 // Go truncates toward zero.
	shifts := exponent >> 16                                                                   // arithmetic floor for negative exponents
	frac := uint64(exponent - shifts*65536)
	fractional := (195766423245049*frac + 971821376*frac*frac + 5127*frac*frac*frac + (1 << 47)) >> 48
	factor := uint64(65536) + fractional
	numerator := new(big.Int).Mul(parent.Difficulty, new(big.Int).SetUint64(factor))
	denominator := new(big.Int).Lsh(big.NewInt(1), 16)
	if shifts < 0 {
		denominator.Lsh(denominator, uint(-shifts))
	} else {
		numerator.Lsh(numerator, uint(shifts))
	}
	numerator.Add(numerator, new(big.Int).Rsh(new(big.Int).Set(denominator), 1))
	diff := numerator.Div(numerator, denominator)
	if diff.Cmp(parent.Difficulty) == 0 {
		if actual*1000 < c.RegistryTargetBlockTimeMillis {
			diff.Add(diff, big.NewInt(1))
		} else if actual*1000 > c.RegistryTargetBlockTimeMillis {
			diff.Sub(diff, big.NewInt(1))
		}
	}
	if diff.Cmp(c.MinimumDifficulty) < 0 {
		diff.Set(c.MinimumDifficulty)
	}
	// Keep a nonzero canonical Scrypt target, even at the numeric upper bound.
	if diff.Cmp(diffOneTarget) > 0 {
		diff.Set(diffOneTarget)
	}
	return diff
}
