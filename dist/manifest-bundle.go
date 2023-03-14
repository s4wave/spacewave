package bldr_dist

import "github.com/aperturerobotics/hydra/block"

// NewDistManifestBundleSubBlockCtor returns the sub-block constructor.
func NewDistManifestBundleSubBlockCtor(r **DistManifestBundle) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if v != nil || !create {
			return v
		}
		v = &DistManifestBundle{}
		*r = v
		return v
	}
}
