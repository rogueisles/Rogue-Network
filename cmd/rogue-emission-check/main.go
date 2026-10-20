// Derive the exact sum of per-block integer rewards using enclosing intervals.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"time"
)

func integer(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("invalid integer")
	}
	return n
}

func main() {
	bits := flag.Uint("bits", 384, "outward-rounded interval precision")
	flag.Parse()
	if *bits < 128 || *bits > 2048 {
		panic("unsupported precision")
	}
	const count = uint64(12480667)
	numerator := integer("999999807871237324305429716870248633")
	denominator := integer("1000000000000000000000000000000000000")
	initial := integer("1100000000000000000000")
	tail := integer("100000000000000000000")
	budget := integer("7156658799291511116642406018")
	lo := new(big.Int).Lsh(new(big.Int).Set(initial), *bits)
	hi := new(big.Int).Set(lo)
	denMinusOne := new(big.Int).Sub(denominator, big.NewInt(1))
	lower, upper, sum := new(big.Int), new(big.Int), new(big.Int)
	last := new(big.Int)
	digest := sha256.New()
	var encoding [16]byte
	started := time.Now()
	for n := uint64(0); n < count; n++ {
		lower.Rsh(lo, *bits)
		upper.Rsh(hi, *bits)
		if lower.Cmp(upper) != 0 {
			panic(fmt.Sprintf("ambiguous reward at %d", n))
		}
		sum.Add(sum, lower)
		last.Set(lower)
		lower.FillBytes(encoding[:])
		digest.Write(encoding[:])
		lo.Mul(lo, numerator).Quo(lo, denominator)
		hi.Mul(hi, numerator).Add(hi, denMinusOne).Quo(hi, denominator)
	}
	lower.Rsh(lo, *bits)
	upper.Rsh(hi, *bits)
	if lower.Cmp(upper) != 0 || lower.Cmp(tail) > 0 || last.Cmp(tail) <= 0 {
		panic("tail threshold is not minimal")
	}
	remaining := new(big.Int).Sub(budget, sum)
	full, final := new(big.Int), new(big.Int)
	full.QuoRem(remaining, tail, final)
	finalOffset := count + full.Uint64()
	if final.Sign() == 0 {
		finalOffset--
	}
	report := map[string]interface{}{
		"method":       "Every exact rational reward is enclosed by outward-rounded integer intervals; both bounds floor to the same wei for every offset.",
		"intervalBits": *bits, "certifiedRewardCount": count, "normalBudgetWei": budget.String(),
		"decayNumerator": numerator.String(), "decayDenominator": denominator.String(),
		"exactPreTailIssuedWei": sum.String(), "perReward16ByteBigEndianSHA256": hex.EncodeToString(digest.Sum(nil)),
		"lastDecliningRewardWei": last.String(), "firstUnflooredCurveRewardWei": lower.String(),
		"tailStartOffset": count, "tailStartHeight": 1100100 + count,
		"tailRewardWei": tail.String(), "remainingTailAllowanceWei": remaining.String(),
		"fullTailRewardBlocks": full.String(), "finalPartialRewardWei": final.String(),
		"lastRewardOffset": finalOffset, "lastRewardHeight": 1100100 + finalOffset,
		"firstZeroRewardHeight": 1100101 + finalOffset, "elapsedSeconds": time.Since(started).Seconds(),
	}
	json.NewEncoder(os.Stdout).Encode(report)
}
