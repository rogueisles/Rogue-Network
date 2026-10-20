package main

import (
	"encoding/json"
	"github.com/TeamEGEM/go-egem/core"
	"io/ioutil"
	"testing"
)

func TestPreparePreservesGenesisAndHistoricalFields(t *testing.T) {
	g, err := ioutil.ReadFile("../../genesis.json")
	if err != nil {
		t.Fatal(err)
	}
	proof := []byte(`{"auxChainId":20295,"dogeAuxChainId":98,"initialDifficulty":1,"minimumDifficulty":1,"templateSigners":["0x000000000000000000000000000000000000cafe"],"templateQuorum":1,"ltcPayoutScriptHash":"0x1111111111111111111111111111111111111111111111111111111111111111","dogePayoutScriptHash":"0x2222222222222222222222222222222222222222222222222222222222222222","templateMaxValidity":300}`)
	result, err := prepare(g, proof)
	if err != nil {
		t.Fatal(err)
	}
	var before, after core.Genesis
	json.Unmarshal(g, &before)
	json.Unmarshal(result, &after)
	if before.ToBlock(nil).Hash() != mainnetGenesis || after.ToBlock(nil).Hash() != mainnetGenesis {
		t.Fatal("genesis changed")
	}
	if after.Config.ForgeBlock.Int64() != 1100100 || !after.Config.ValidateRogueSchedule() {
		t.Fatal("wrong fork config")
	}
	a, b := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	json.Unmarshal(g, &a)
	json.Unmarshal(result, &b)
	delete(a, "config")
	delete(b, "config")
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if string(ab) != string(bb) {
		t.Fatal("non-config genesis data modified")
	}
	if _, err := prepare(result, proof); err == nil {
		t.Fatal("already configured chain accepted")
	}
	var bad map[string]interface{}
	json.Unmarshal(g, &bad)
	bad["nonce"] = "0x43"
	badBytes, _ := json.Marshal(bad)
	if _, err := prepare(badBytes, proof); err == nil {
		t.Fatal("wrong genesis accepted")
	}
	var altered map[string]interface{}
	json.Unmarshal(g, &altered)
	altered["config"].(map[string]interface{})["eip158Block"] = 100
	alteredBytes, _ := json.Marshal(altered)
	if _, err := prepare(alteredBytes, proof); err == nil {
		t.Fatal("same genesis with altered historical rules accepted")
	}
}
