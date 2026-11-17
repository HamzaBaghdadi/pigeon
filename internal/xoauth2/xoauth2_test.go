package xoauth2

import "testing"

func TestXOAuth2InitialResponse(t *testing.T) {
	mech, ir, err := NewClient("me@gmail.com", "test-access-token").Start()
	if err != nil || mech != "XOAUTH2" {
		t.Fatalf("Start = %s, %v", mech, err)
	}
	if want := "user=me@gmail.comauth=Bearer test-access-token"; string(ir) != want {
		t.Errorf("initial response = %q, want %q", ir, want)
	}
}
