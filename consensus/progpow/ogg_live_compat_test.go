package progpow

import (
	"math/big"
	"testing"

	"github.com/TeamEGEM/go-egem/core/types"
	"github.com/TeamEGEM/go-egem/params"
)

func TestOGGLiveDifficultyRegression(t *testing.T) {
	// Real live-chain era-4 vector captured from OGG Node 2 compatibility work.
	parent := &types.Header{
		Number:     big.NewInt(816387),
		Time:       big.NewInt(1000),
		Difficulty: big.NewInt(64377844455),
	}
	got := CalcDifficulty(params.TestnetProgPoWChainConfig, 1003, parent)
	want := big.NewInt(72425075011)
	if got.Cmp(want) != 0 {
		t.Fatalf("live OGG difficulty mismatch: have %v want %v", got, want)
	}
}

func TestOGGDifficultyEraBoundaries(t *testing.T) {
	tests := []struct {
		parent uint64
		delta  uint64
		diff   int64
		want   int64
	}{
		{114, 3, 8000000, 10000000},  // +25%
		{115, 3, 10000000, 11000000}, // +10%
		{4099, 13, 10000000, 9000000},
		{4100, 3, 8000000, 9000000},    // +12.5%
		{20999, 12, 7000000, 6000000},  // -1/7
		{21000, 3, 8000000, 9000000},   // +12.5%
		{21000, 12, 10000000, 8000000}, // -20%
	}
	for _, tc := range tests {
		p := &types.Header{Number: new(big.Int).SetUint64(tc.parent), Time: big.NewInt(1000), Difficulty: big.NewInt(tc.diff)}
		got := CalcDifficulty(params.TestnetProgPoWChainConfig, 1000+tc.delta, p)
		if got.Cmp(big.NewInt(tc.want)) != 0 {
			t.Fatalf("parent=%d delta=%d: have %v want %d", tc.parent, tc.delta, got, tc.want)
		}
	}
}
