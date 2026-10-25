package forge

import (
	"bytes"
	"errors"

	"github.com/TeamEGEM/go-egem/common"
	"github.com/TeamEGEM/go-egem/crypto"
	"github.com/TeamEGEM/go-egem/rlp"
)

const certificateTag = "RGNCERT4"

// MultiCertificateDigest commits authorization bytes to the mined parent.
// The template's parent body commitment normalizes only this payload and the
// separately bounded extranonce, avoiding a circular signing dependency.
func MultiCertificateDigest(t MultiTemplate, work common.Hash, signatures [][]byte) (common.Hash, error) {
	if (t.Version != 4 && t.Version != 5) || work == (common.Hash{}) {
		return common.Hash{}, errors.New("invalid certificate domain")
	}
	if err := MultiTemplateValid(t); err != nil {
		return common.Hash{}, err
	}
	payload := struct {
		Domain     []byte
		Template   MultiTemplate
		WorkHash   common.Hash
		Signatures [][]byte
	}{[]byte("RGN-FORGE-CERTIFICATE-V4"), t, work, signatures}
	if t.Version == 5 {
		payload.Domain = []byte("RGN-FORGE-CERTIFICATE-V5")
	}
	blob, err := rlp.EncodeToBytes(payload)
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(blob), nil
}

func certificatePayloadOffset(parent *parsedCoinbase) (int, int, error) {
	return certificatePayloadOffsetVersion(parent, 4)
}
func certificatePayloadOffsetVersion(parent *parsedCoinbase, version uint8) (int, int, error) {
	tag := certificateTag
	if version == 5 {
		tag = "RGNCERT5"
	}

	offset, index := -1, -1
	for i, output := range parent.Outputs {
		script := output.Script
		if len(script) < 10 || !bytes.Equal(script[2:10], []byte(tag)) {
			continue
		}
		if offset >= 0 || output.Value != 0 || len(script) != 42 || script[0] != 0x6a || script[1] != 40 {
			return 0, 0, errors.New("ambiguous or malformed mined certificate commitment")
		}
		offset, index = output.ScriptOffset+10, i
	}
	if offset < 0 {
		return 0, 0, errors.New("missing mined certificate commitment")
	}
	return offset, index, nil
}
