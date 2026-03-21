package unixfs_block

import "sort"

// GetXattrValue returns the value of the named xattr, or nil if not found.
func (n *FSNode) GetXattrValue(name string) []byte {
	for _, xa := range n.GetXattrs() {
		if xa.GetName() == name {
			return xa.GetValue()
		}
	}
	return nil
}

// SetXattr sets or replaces an xattr. Maintains sorted order by name.
func (n *FSNode) SetXattr(name string, value []byte) {
	for i, xa := range n.Xattrs {
		if xa.GetName() == name {
			n.Xattrs[i].Value = value
			return
		}
	}
	n.Xattrs = append(n.Xattrs, &FSXattr{Name: name, Value: value})
	sort.Slice(n.Xattrs, func(i, j int) bool {
		return n.Xattrs[i].GetName() < n.Xattrs[j].GetName()
	})
}

// RemoveXattr removes the named xattr. Returns false if not found.
func (n *FSNode) RemoveXattr(name string) bool {
	for i, xa := range n.Xattrs {
		if xa.GetName() == name {
			n.Xattrs = append(n.Xattrs[:i], n.Xattrs[i+1:]...)
			return true
		}
	}
	return false
}

// ListXattrNames returns all xattr names in sorted order.
func (n *FSNode) ListXattrNames() []string {
	xattrs := n.GetXattrs()
	names := make([]string, 0, len(xattrs))
	for _, xa := range xattrs {
		names = append(names, xa.GetName())
	}
	return names
}
