package forge

import (
	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
	"math"
	"math/big"
	"testing"
)

func registryTargetConfig() *params.ChainConfig {
	return &params.ChainConfig{ForgeBlock: big.NewInt(1), Forge: &params.ForgeConfig{RegistryBlock: big.NewInt(100), RegistryTargetBlockTimeMillis: 12500, TargetBlockTime: 12, InitialDifficulty: big.NewInt(1000000), MinimumDifficulty: big.NewInt(1), RetargetV2: true}}
}

func TestRegistryTargetPreservesEveryPreActivationSpacing(t *testing.T) {
	config := registryTargetConfig()
	parent := &types.Header{Number: big.NewInt(98), Time: big.NewInt(1000000), Difficulty: big.NewInt(1000000)}
	for _, gap := range []uint64{1, 2, 11, 12, 13, 14, 60, 120, 600, 7200, math.MaxUint64 - 1000000} {
		got := CalcDifficulty(config, 1000000+gap, parent)
		want := calcRetargetV2(config.Forge, 1000000+gap, parent)
		if got.Cmp(want) != 0 {
			t.Fatal("historical retarget changed", gap, got, want)
		}
	}
	if RegistryTargetMillis(config, big.NewInt(99)) != 12000 || RegistryTargetMillis(config, big.NewInt(100)) != 12500 {
		t.Fatal("target activation boundary differs")
	}
}

func TestRegistryHalfSecondTargetHasNoIntegerDeadBand(t *testing.T) {
	config := registryTargetConfig()
	parent := &types.Header{Number: big.NewInt(99), Time: big.NewInt(1000000), Difficulty: big.NewInt(1000000000)}
	fast := CalcDifficulty(config, 1000012, parent)
	slow := CalcDifficulty(config, 1000013, parent)
	if fast.Cmp(parent.Difficulty) <= 0 || slow.Cmp(parent.Difficulty) >= 0 {
		t.Fatal("12 and13 seconds do not straddle12.5", fast, slow)
	}
	for gap := uint64(1); gap <= 300; gap++ {
		got := CalcDifficulty(config, 1000000+gap, parent)
		reference := 1e9 * math.Pow(2, (12.5-float64(gap))/120)
		numeric, _ := new(big.Float).SetInt(got).Float64()
		// Retain the measured 0.015% fixed-point polynomial precision bound.
		if math.Abs(numeric-reference) > reference*0.00015+2 {
			t.Fatal("fractional exponent differs from numerical reference", gap, numeric, reference)
		}
	}
	for gap := uint64(1); gap <= 1000; gap++ {
		got := CalcDifficulty(config, 1000000+gap, parent)
		if got.Sign() <= 0 || got.Cmp(config.Forge.MinimumDifficulty) < 0 || got.Cmp(diffOneTarget) > 0 {
			t.Fatal("retarget escaped canonical bounds", gap, got)
		}
	}
	if got := CalcDifficulty(config, math.MaxUint64, parent); got.Cmp(config.Forge.MinimumDifficulty) != 0 {
		t.Fatal("huge outage did not reach bounded floor", got)
	}
}

func TestRegistryExponentPolynomialPrecisionBound(t *testing.T) {
	for frac := uint64(0); frac < 65536; frac++ {
		approximation := float64(65536+((195766423245049*frac+971821376*frac*frac+5127*frac*frac*frac+(1<<47))>>48)) / 65536
		exact := math.Pow(2, float64(frac)/65536)
		if math.Abs(approximation-exact)/exact > 0.00015 {
			t.Fatal("relative polynomial precision exceeds bound", frac, approximation, exact)
		}
	}
}

func TestRegistryPrivateTargetUsesFractionalDamping(t *testing.T) {
	config := registryTargetConfig()
	config.Forge.PrivateCPUTest = true
	config.Forge.RetargetV2 = false
	config.Forge.MinimumDifficulty = big.NewInt(65536)
	parent := &types.Header{Number: big.NewInt(99), Time: big.NewInt(1000000), Difficulty: big.NewInt(1000000)}
	if CalcDifficulty(config, 1000012, parent).Cmp(parent.Difficulty) <= 0 || CalcDifficulty(config, 1000013, parent).Cmp(parent.Difficulty) >= 0 {
		t.Fatal("private fractional damping is insensitive to12/13 second spacing")
	}
	parent.Number = big.NewInt(98)
	if got, want := CalcDifficulty(config, 1000012, parent), calcCPUDifficulty(config.Forge, 1000012, parent); got.Cmp(want) != 0 {
		t.Fatal("historical private target changed")
	}
}
