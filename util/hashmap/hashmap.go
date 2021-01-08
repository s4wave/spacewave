package hashmap

// Hashmap implements an in-memory []byte -> interface{} store.
//
// Concurrency safe.
type Hashmap interface {
	// Get looks up an item in the hash map.
	Get(key []byte) (interface{}, bool)
	// Set sets an item in the hash map.
	Set(key []byte, value interface{})
	// Remove deletes an item from the hash map.
	Remove(key []byte)
	// Exists checks if an item exists in the hash map.
	Exists(key []byte) bool
	// Iterate iterates over the hashmap.
	//
	// Iterator (might) not include items added during iteration.
	Iterate(cb func(key []byte, value interface{}) error) error
}

// NewHashmap constructs a new hash map of default type.
func NewHashmap() Hashmap {
	return NewCtrieMap()
}
