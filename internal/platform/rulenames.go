package platform

import (
	_ "embed"
	"encoding/json"
)

// rulenames.json holds the Chinese names of rule and check codes. The
// console imports the same file, so API labels and the console agree.
//
//go:embed rulenames.json
var ruleNamesJSON []byte

var ruleNames = func() map[string]string {
	var file struct{ Reasons, Checks map[string]string }
	if err := json.Unmarshal(ruleNamesJSON, &file); err != nil {
		panic(err)
	}
	names := map[string]string{}
	for _, m := range []map[string]string{file.Checks, file.Reasons} {
		for code, name := range m {
			names[code] = name
		}
	}
	return names
}()

// labelHits fills each hit's display name.
func labelHits(hits []RuleHit) []RuleHit {
	out := make([]RuleHit, len(hits))
	for i, hit := range hits {
		hit.Label = ruleNames[hit.Code]
		out[i] = hit
	}
	return out
}
