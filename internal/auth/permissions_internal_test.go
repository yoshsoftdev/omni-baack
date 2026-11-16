package auth

import (
	"reflect"
	"testing"
)

// TEAM-04: Me lists only granted codes, sorted, and never nil (the UI hides actions on it).
func TestGrantedPermissions(t *testing.T) {
	s := &Service{}
	if got := s.grantedPermissions("agent"); got == nil || len(got) != 0 {
		t.Fatalf("without a source = %#v, want empty slice", got)
	}
	s.SetPermissionSource(func(role string) map[string]bool {
		return map[string]bool{"contact.view": true, "conversation.reply": true, "contact.delete": false}
	})
	want := []string{"contact.view", "conversation.reply"}
	if got := s.grantedPermissions("agent"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
