package linkedlist

import "testing"

// TestLinkedList tests the linked list.
func TestLinkedList(t *testing.T) {
	ll := NewLinkedList(0, 1, 2, 3, 4)
	v, ok := ll.Peek()
	if v != 0 || !ok {
		t.Fail()
	}
	v, ok = ll.PeekTail()
	if v != 4 || !ok {
		t.Fail()
	}
	ll.Push(5)
	v, ok = ll.PeekTail()
	if v != 5 || !ok {
		t.Fail()
	}
	v, ok = ll.Pop()
	if v != 0 || !ok {
		t.Fail()
	}
	ll.Push(6)
	v, ok = ll.Pop()
	if v != 1 || !ok {
		t.Fail()
	}
	v, ok = ll.Peek()
	if v != 2 || !ok {
		t.Fail()
	}
	v, ok = ll.PeekTail()
	if v != 6 || !ok {
		t.Fail()
	}
}
