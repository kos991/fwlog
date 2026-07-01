package main

import "testing"

func TestServicePortLabelUsesCommonNames(t *testing.T) {
	tests := []struct {
		port int
		want string
	}{
		{80, "80:http"},
		{443, "443:https"},
		{53, "53:dns"},
		{22, "22:ssh"},
		{12345, "12345"},
	}

	for _, test := range tests {
		if got := servicePortLabel(test.port); got != test.want {
			t.Fatalf("servicePortLabel(%d) = %q, want %q", test.port, got, test.want)
		}
	}
}
