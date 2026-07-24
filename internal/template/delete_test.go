package template

import (
	"testing"

	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

func TestIsMetaTemplateAlreadyGone(t *testing.T) {
	if !isMetaTemplateAlreadyGone(metapkg.NewAPIError(404, `{"error":{"message":"not found"}}`)) {
		t.Fatal("expected 404 to be treated as already gone")
	}
	if !isMetaTemplateAlreadyGone(metapkg.NewAPIError(400, `{"error":{"message":"Template does not exist"}}`)) {
		t.Fatal("expected does not exist body to be treated as already gone")
	}
	if isMetaTemplateAlreadyGone(metapkg.NewAPIError(500, `{"error":{"message":"server error"}}`)) {
		t.Fatal("expected 500 not to be treated as already gone")
	}
}
