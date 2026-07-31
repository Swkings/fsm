package util

import (
	"testing"
)

func TestPtr(t *testing.T) {
	var elem int64 = 100

	elemPtr := Ptr(elem)

	t.Logf("ptr %v %v", elem, elemPtr)
}

func TestPtrTrim(t *testing.T) {
	var elem int64 = 100
	elemPtr := &elem

	elemPtrTrim := PtrTrim(elemPtr)

	t.Logf("ptr %v %v", elemPtr, elemPtrTrim)
}
