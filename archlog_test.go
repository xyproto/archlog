package main

import (
	"testing"
)

func TestNickToInfo(t *testing.T) {
	found, ok := nickToNameAndEmailWithUrl("arodseth", TU_URL)
	if ok != nil {
		t.Fatal("Could not find nick")
	}
	// run with "go test -test.v" to see the test log
	t.Log(found)
}
