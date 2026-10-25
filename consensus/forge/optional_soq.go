package forge

// validSOQOutputs recognizes a native ML-DSA witness-v1 payout without pinning
// an operator's wallet in consensus. Quorum signers additionally validate the
// exact native template, full reward and configured pool payout script.
func validSOQOutputs(cb *parsedCoinbase) bool {
	if cb == nil {
		return false
	}
	paid := false
	for _, out := range cb.Outputs {
		if out.Value == 0 {
			continue
		}
		if len(out.Script) != 34 || out.Script[0] != 0x51 || out.Script[1] != 0x20 {
			return false
		}
		paid = true
	}
	return paid
}
