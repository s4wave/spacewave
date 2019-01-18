package object

// Prefixer implements an object store prefixer.
type Prefixer struct {
	base   ObjectStore
	prefix string
}

// NewPrefixer constructs a new object store prefixer.
func NewPrefixer(base ObjectStore, prefix string) ObjectStore {
	return &Prefixer{prefix: prefix, base: base}
}

// GetObject gets an object by key.
func (p *Prefixer) GetObject(key string) (val []byte, found bool, err error) {
	return p.base.GetObject(p.prefix + key)
}

// SetObject sets an object by key.
func (p *Prefixer) SetObject(key string, val []byte) error {
	return p.base.SetObject(p.prefix+key, val)
}

// DeleteObject deletes an object by key.
func (p *Prefixer) DeleteObject(key string) error {
	return p.base.DeleteObject(p.prefix + key)
}

// ListKeys lists keys with a given prefix.
func (p *Prefixer) ListKeys(prefix string) ([]string, error) {
	keys, err := p.base.ListKeys(p.prefix + prefix)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		keys[i] = keys[i][len(p.prefix):]
	}
	return keys, nil
}

// _ is a type assertion
var _ ObjectStore = ((*Prefixer)(nil))
