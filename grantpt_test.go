package main

import "testing"

func TestGrantPTBadFD(t *testing.T) {
	err := GrantPT(^uintptr(0))
	if err == nil {
		t.Error("GrantPT of invalid fd succeeded")
	}
}
