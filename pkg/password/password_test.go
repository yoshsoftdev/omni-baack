package password

import "testing"

func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("password123")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if !Verify(hash, "password123") {
		t.Fatal("expected password to verify")
	}
	if Verify(hash, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
}
