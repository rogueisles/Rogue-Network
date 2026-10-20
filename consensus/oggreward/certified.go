package oggreward

import "math/big"

// certifiedGeometric encloses the exact rational power with outward-rounded
// 512-bit intervals. It returns only when both bounds floor to the same wei.
// This ports the deployed legacy optimization without approximating rewards.
func certifiedGeometric(initial, numerator, denominator, exponent *big.Int) (*big.Int, bool) {
	if exponent.Sign() == 0 {
		return new(big.Int).Set(initial), true
	}
	if exponent.Cmp(big.NewInt(1)) == 0 {
		return new(big.Int).Quo(new(big.Int).Mul(initial, numerator), denominator), true
	}
	const bits = 512
	scale := new(big.Int).Lsh(big.NewInt(1), bits)
	mask := new(big.Int).Sub(scale, big.NewInt(1))
	scaled := new(big.Int).Mul(numerator, scale)
	lo := new(big.Int).Quo(scaled, denominator)
	hi := new(big.Int).Quo(new(big.Int).Add(scaled, new(big.Int).Sub(denominator, big.NewInt(1))), denominator)
	resultLo, resultHi := new(big.Int).Set(scale), new(big.Int).Set(scale)
	mul := func(a, b, c, d *big.Int) (*big.Int, *big.Int) {
		lower := new(big.Int).Rsh(new(big.Int).Mul(a, c), bits)
		upper := new(big.Int).Mul(b, d)
		upper.Add(upper, mask).Rsh(upper, bits)
		return lower, upper
	}
	for n := new(big.Int).Set(exponent); n.Sign() > 0; {
		if n.Bit(0) == 1 {
			resultLo, resultHi = mul(resultLo, resultHi, lo, hi)
		}
		n.Rsh(n, 1)
		if n.Sign() > 0 {
			lo, hi = mul(lo, hi, lo, hi)
		}
	}
	resultLo.Mul(resultLo, initial).Rsh(resultLo, bits)
	resultHi.Mul(resultHi, initial).Rsh(resultHi, bits)
	if resultLo.Cmp(resultHi) == 0 {
		return resultLo, true
	}
	return nil, false
}
