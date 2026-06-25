package message

import (
	"testing"
)

func TestShouldApplyStatus_Monotonic(t *testing.T) {
	sent := StatusSent
	delivered := StatusDelivered
	read := StatusRead

	cases := []struct {
		current *Status
		next    Status
		want    bool
	}{
		{nil, StatusSent, true},
		{&sent, StatusDelivered, true},
		{&delivered, StatusRead, true},
		{&read, StatusDelivered, false},
		{&delivered, StatusSent, false},
		{&sent, StatusSent, false},
	}
	for _, tc := range cases {
		if got := shouldApplyStatus(tc.current, tc.next); got != tc.want {
			t.Fatalf("current=%v next=%v got=%v want=%v", tc.current, tc.next, got, tc.want)
		}
	}
}
