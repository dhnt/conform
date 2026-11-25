package cert

import (
	"testing"

	"github.com/dhnt/conform/contract"
)

func TestLayerVerdictsFollowAssertionKind(t *testing.T) {
	assertions := []contract.Assertion{{Name: "F-01", Kind: "cli"}, {Name: "U-01", Kind: "ui"}}
	results := []contract.Result{{Name: "F-01", Status: contract.PASS}, {Name: "U-01", Status: contract.FAIL}}
	layers := LayerVerdicts(assertions, results)
	if layers[LayerCLI] != PASS || layers[LayerUI] != FAIL {
		t.Fatalf("layers: %+v", layers)
	}
	evidence := ResultsByAssertion(assertions, results, Fresh)
	if evidence[0].Layer != LayerCLI || evidence[1].Layer != LayerUI {
		t.Fatalf("evidence: %+v", evidence)
	}
}
