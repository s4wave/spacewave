package linkedlist

import "sync"

// LinkedList implements a pointer-linked list with a head and tail.
//
// The empty value is a valid empty linked list.
type LinkedList[T any] struct {
	// mtx guards below fields
	mtx sync.RWMutex
	// head is the current head elem
	// least-recently-added
	head *linkedListElem[T]
	// tail is the current tail item
	// most-recently-added
	tail *linkedListElem[T]
}

// linkedListElem is an elem in the linked list.
type linkedListElem[T any] struct {
	// next is the next element in the list
	next *linkedListElem[T]
	// val is the value
	val T
}

// NewLinkedList constructs a new LinkedList.
func NewLinkedList[T any](elems ...T) *LinkedList[T] {
	ll := &LinkedList[T]{}
	for _, elem := range elems {
		ll.pushElem(elem)
	}
	return ll
}

// Push pushes a value to the end of the linked list.
func (l *LinkedList[T]) Push(val T) {
	l.mtx.Lock()
	l.pushElem(val)
	l.mtx.Unlock()
}

// PushFront pushes a value to the front of the linked list.
// It will be returned next for Pop or Peek.
func (l *LinkedList[T]) PushFront(val T) {
	l.mtx.Lock()
	elem := &linkedListElem[T]{val: val}
	if l.head != nil {
		elem.next = l.head
	} else {
		l.tail = elem
	}
	l.head = elem
	l.mtx.Unlock()
}

// Peek peeks the head of the linked list.
func (l *LinkedList[T]) Peek() (T, bool) {
	l.mtx.Lock()
	var val T
	exists := l.head != nil
	if exists {
		val = l.head.val
	}
	l.mtx.Unlock()
	return val, exists
}

// PeekTail peeks the tail of the linked list.
func (l *LinkedList[T]) PeekTail() (T, bool) {
	l.mtx.Lock()
	var val T
	exists := l.tail != nil
	if exists {
		val = l.tail.val
	}
	l.mtx.Unlock()
	return val, exists
}

// Pop dequeues the head of the linked list.
func (l *LinkedList[T]) Pop() (T, bool) {
	l.mtx.Lock()
	var val T
	exists := l.head != nil
	if exists {
		val = l.head.val
		if l.head.next != nil {
			l.head = l.head.next
		} else {
			l.head = nil
			l.tail = nil
		}
	}
	l.mtx.Unlock()
	return val, exists
}

// pushElem pushes an element to the list while mtx is locked.
func (l *LinkedList[T]) pushElem(val T) {
	elem := &linkedListElem[T]{val: val}
	if l.tail == nil {
		l.head = elem
	} else {
		l.tail.next = elem
	}
	l.tail = elem
}
