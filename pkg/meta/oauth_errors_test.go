package meta_test

import (
	"testing"

	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

func TestClassifyOAuthError(t *testing.T) {
	if metapkg.ClassifyOAuthError(metapkg.NewAPIError(400, "authorization code has expired")) != metapkg.OAuthErrorExpired {
		t.Fatal("expected expired")
	}
	if metapkg.ClassifyOAuthError(metapkg.NewAPIError(400, "invalid authorization code")) != metapkg.OAuthErrorInvalid {
		t.Fatal("expected invalid")
	}
}
