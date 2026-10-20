package forge

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/TeamEGEM/go-egem/common"
)

var mergedMiningMagic = []byte{0xfa, 0xbe, 0x6d, 0x6d}

type txOutput struct {
	Value        uint64
	Script       []byte
	ScriptOffset int
}

type parsedCoinbase struct {
	ScriptSig []byte
	Outputs   []txOutput
	TxID      common.Hash // raw double-SHA256 digest bytes (Bitcoin internal order)
}

func doubleSHA256(b []byte) common.Hash {
	one := sha256.Sum256(b)
	two := sha256.Sum256(one[:])
	return common.BytesToHash(two[:])
}

func reverse32(b []byte) common.Hash {
	var out [32]byte
	for i := 0; i < 32; i++ {
		out[i] = b[31-i]
	}
	return common.BytesToHash(out[:])
}

// ScriptHash is the consensus commitment used for external payout scripts.
func ScriptHash(script []byte) common.Hash {
	h := sha256.Sum256(script)
	return common.BytesToHash(h[:])
}

func readVarInt(b []byte, off *int) (uint64, error) {
	if *off >= len(b) {
		return 0, errors.New("truncated bitcoin varint")
	}
	p := b[*off]
	*off++
	switch p {
	case 0xfd:
		if *off+2 > len(b) {
			return 0, errors.New("truncated bitcoin varint16")
		}
		v := uint64(binary.LittleEndian.Uint16(b[*off : *off+2]))
		*off += 2
		if v < 0xfd {
			return 0, errors.New("non-canonical bitcoin varint16")
		}
		return v, nil
	case 0xfe:
		if *off+4 > len(b) {
			return 0, errors.New("truncated bitcoin varint32")
		}
		v := uint64(binary.LittleEndian.Uint32(b[*off : *off+4]))
		*off += 4
		if v <= 0xffff {
			return 0, errors.New("non-canonical bitcoin varint32")
		}
		return v, nil
	case 0xff:
		if *off+8 > len(b) {
			return 0, errors.New("truncated bitcoin varint64")
		}
		v := binary.LittleEndian.Uint64(b[*off : *off+8])
		*off += 8
		if v <= 0xffffffff {
			return 0, errors.New("non-canonical bitcoin varint64")
		}
		return v, nil
	default:
		return uint64(p), nil
	}
}

func parseCoinbase(tx []byte) (*parsedCoinbase, error) {
	if len(tx) < 4+1+36+1+4+1+4 {
		return nil, errors.New("truncated coinbase transaction")
	}
	off := 4
	// Consensus proof carries base/non-witness serialization so txid is exact.
	if tx[off] == 0 {
		return nil, errors.New("witness serialization not allowed in forge coinbase proof")
	}
	inCount, err := readVarInt(tx, &off)
	if err != nil {
		return nil, err
	}
	if inCount != 1 {
		return nil, fmt.Errorf("coinbase must have one input, got %d", inCount)
	}
	if off+36 > len(tx) {
		return nil, errors.New("truncated coinbase input")
	}
	prevHash := tx[off : off+32]
	prevIndex := binary.LittleEndian.Uint32(tx[off+32 : off+36])
	off += 36
	if !bytes.Equal(prevHash, make([]byte, 32)) || prevIndex != 0xffffffff {
		return nil, errors.New("transaction is not a coinbase")
	}
	scriptLen, err := readVarInt(tx, &off)
	if err != nil {
		return nil, err
	}
	if scriptLen > 10000 || off+int(scriptLen)+4 > len(tx) {
		return nil, errors.New("invalid coinbase script length")
	}
	scriptSig := append([]byte(nil), tx[off:off+int(scriptLen)]...)
	off += int(scriptLen) + 4 // script + sequence
	outCount, err := readVarInt(tx, &off)
	if err != nil {
		return nil, err
	}
	if outCount == 0 || outCount > 10000 {
		return nil, errors.New("invalid coinbase output count")
	}
	outs := make([]txOutput, 0, outCount)
	for i := uint64(0); i < outCount; i++ {
		if off+8 > len(tx) {
			return nil, errors.New("truncated coinbase output")
		}
		value := binary.LittleEndian.Uint64(tx[off : off+8])
		off += 8
		n, err := readVarInt(tx, &off)
		if err != nil {
			return nil, err
		}
		if n > 10000 || off+int(n) > len(tx) {
			return nil, errors.New("invalid coinbase output script length")
		}
		scriptOffset := off
		script := append([]byte(nil), tx[off:off+int(n)]...)
		off += int(n)
		outs = append(outs, txOutput{Value: value, Script: script, ScriptOffset: scriptOffset})
	}
	if off+4 != len(tx) {
		return nil, errors.New("unexpected trailing coinbase bytes")
	}
	return &parsedCoinbase{ScriptSig: scriptSig, Outputs: outs, TxID: doubleSHA256(tx)}, nil
}

func verifyPayouts(cb *parsedCoinbase, required common.Hash) error {
	positive := 0
	for _, out := range cb.Outputs {
		if out.Value > 0 {
			positive++
			if ScriptHash(out.Script) != required {
				return errors.New("external coinbase diverts positive-value payout")
			}
			continue
		}
		if len(out.Script) == 0 || out.Script[0] != 0x6a { // OP_RETURN
			return errors.New("zero-value external coinbase output must be OP_RETURN")
		}
	}
	if positive == 0 {
		return errors.New("external coinbase has no positive FORGE payout")
	}
	return nil
}

func merkleRoot(leaf common.Hash, branch []common.Hash, index uint32) common.Hash {
	h := leaf
	for _, sibling := range branch {
		pair := make([]byte, 0, 64)
		if index&1 != 0 {
			pair = append(pair, sibling.Bytes()...)
			pair = append(pair, h.Bytes()...)
		} else {
			pair = append(pair, h.Bytes()...)
			pair = append(pair, sibling.Bytes()...)
		}
		h = doubleSHA256(pair)
		index >>= 1
	}
	return h
}

func verifyCoinbaseMerkle(header []byte, cb *parsedCoinbase, branch []common.Hash, index uint32) error {
	if len(header) != 80 {
		return fmt.Errorf("invalid parent header length %d", len(header))
	}
	root := merkleRoot(cb.TxID, branch, index)
	if !bytes.Equal(root.Bytes(), header[36:68]) {
		return errors.New("coinbase merkle proof does not match header")
	}
	return nil
}

// ExpectedChainIndex is the standard AuxPoW LCG chain-slot function used by
// Namecoin/Dogecoin-style merged mining.
func ExpectedChainIndex(auxNonce, chainID, merkleSize uint32) uint32 {
	rand := auxNonce
	rand = rand*1103515245 + 12345
	rand += chainID
	rand = rand*1103515245 + 12345
	return rand % merkleSize
}

// BuildAuxRoot constructs the fixed two-leaf OGG+DOGE merged-mining root.
// dogeLeaf must be the raw double-SHA256 candidate digest (not display-reversed).
func BuildAuxRoot(oggLeaf, dogeLeaf common.Hash, auxNonce uint32, oggID, dogeID uint32) (common.Hash, uint32, uint32, error) {
	const size uint32 = 2
	oi := ExpectedChainIndex(auxNonce, oggID, size)
	di := ExpectedChainIndex(auxNonce, dogeID, size)
	if oi == di {
		return common.Hash{}, oi, di, errors.New("OGG and DOGE AuxPoW slots collide")
	}
	leaves := [2]common.Hash{}
	leaves[oi] = oggLeaf
	leaves[di] = dogeLeaf
	pair := append(append(make([]byte, 0, 64), leaves[0].Bytes()...), leaves[1].Bytes()...)
	return doubleSHA256(pair), oi, di, nil
}

func parseMergedMiningCommitment(script []byte) (common.Hash, uint32, uint32, error) {
	first := bytes.Index(script, mergedMiningMagic)
	if first < 0 {
		return common.Hash{}, 0, 0, errors.New("missing merged-mining commitment")
	}
	if bytes.Index(script[first+len(mergedMiningMagic):], mergedMiningMagic) >= 0 {
		return common.Hash{}, 0, 0, errors.New("multiple merged-mining commitments")
	}
	p := first + len(mergedMiningMagic)
	if p+32+4+4 > len(script) {
		return common.Hash{}, 0, 0, errors.New("truncated merged-mining commitment")
	}
	// Namecoin/Dogecoin AuxPoW stores the chain-Merkle root in display byte
	// order in the coinbase script (the internal uint256 bytes are reversed
	// before the search/commitment check).
	root := reverse32(script[p : p+32])
	p += 32
	size := binary.LittleEndian.Uint32(script[p : p+4])
	p += 4
	nonce := binary.LittleEndian.Uint32(script[p : p+4])
	return root, size, nonce, nil
}

func verifyMergedMiningCommitment(cb *parsedCoinbase, oggLeaf, dogeLeaf common.Hash, oggID, dogeID uint32) error {
	committed, size, nonce, err := parseMergedMiningCommitment(cb.ScriptSig)
	if err != nil {
		return err
	}
	if size != 2 {
		return fmt.Errorf("invalid merged-mining tree size %d", size)
	}
	expected, _, _, err := BuildAuxRoot(oggLeaf, dogeLeaf, nonce, oggID, dogeID)
	if err != nil {
		return err
	}
	if committed != expected {
		return errors.New("merged-mining root mismatch")
	}
	return nil
}

func bitcoinHeaderPrevHashDisplay(header []byte) (common.Hash, error) {
	if len(header) != 80 {
		return common.Hash{}, errors.New("invalid bitcoin-family header length")
	}
	return reverse32(header[4:36]), nil
}

func bitcoinHeaderBits(header []byte) (uint32, error) {
	if len(header) != 80 {
		return 0, errors.New("invalid bitcoin-family header length")
	}
	return binary.LittleEndian.Uint32(header[72:76]), nil
}

func bitcoinHeaderHashDisplay(header []byte) (common.Hash, common.Hash, error) {
	if len(header) != 80 {
		return common.Hash{}, common.Hash{}, errors.New("invalid bitcoin-family header length")
	}
	raw := doubleSHA256(header)
	return raw, reverse32(raw.Bytes()), nil
}

func verifyDogeVersion(header []byte, chainID uint32) error {
	if len(header) != 80 {
		return errors.New("invalid DOGE header length")
	}
	v := binary.LittleEndian.Uint32(header[:4])
	if v&(1<<8) == 0 {
		return errors.New("DOGE AuxPoW flag missing")
	}
	if v>>16 != chainID {
		return fmt.Errorf("invalid DOGE AuxPoW chain ID: have %#x want %#x", v>>16, chainID)
	}
	return nil
}
