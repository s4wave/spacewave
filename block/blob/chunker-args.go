package blob

import "bytes"

// ApplyArgs merges another arguments object into ChunkerArgs.
func (c *ChunkerArgs) ApplyArgs(other *ChunkerArgs) {
	if c == nil || other == nil {
		return
	}

	chunkerType := c.GetChunkerType()
	if oct := other.GetChunkerType(); oct != 0 && oct != chunkerType {
		// chunker type changed, clear args
		if chunkerType != 0 {
			c.Reset()
		}
		c.ChunkerType = oct
		chunkerType = oct
	}

	switch chunkerType {
	case ChunkerType_ChunkerType_DEFAULT:
		fallthrough
	case ChunkerType_ChunkerType_JC:
		if c.JcArgs == nil {
			c.JcArgs = &JcArgs{}
		}
		c.JcArgs.ApplyArgs(other.GetJcArgs())
	case ChunkerType_ChunkerType_RABIN:
		if c.RabinArgs == nil {
			c.RabinArgs = &RabinArgs{}
		}
		c.RabinArgs.ApplyArgs(other.GetRabinArgs())
	}
}

// ApplyArgs merges another arguments object into RabinArgs.
func (c *RabinArgs) ApplyArgs(other *RabinArgs) {
	if c == nil || other == nil {
		return
	}

	if opol := other.GetPol(); opol != 0 {
		c.Pol = opol
	}
	if minSize := other.GetChunkingMinSize(); minSize != 0 {
		c.ChunkingMinSize = minSize
	}
	if maxSize := other.GetChunkingMaxSize(); maxSize != 0 {
		c.ChunkingMinSize = maxSize
	}
}

// ApplyArgs merges another arguments object into JcArgs.
func (c *JcArgs) ApplyArgs(other *JcArgs) {
	if c == nil || other == nil {
		return
	}

	if okey := other.GetKey(); len(okey) != 0 {
		c.Key = bytes.Clone(okey)
	}
	if minSize := other.GetChunkingMinSize(); minSize != 0 {
		c.ChunkingMinSize = minSize
	}
	if targetSize := other.GetChunkingTargetSize(); targetSize != 0 {
		c.ChunkingTargetSize = targetSize
	}
	if maxSize := other.GetChunkingMaxSize(); maxSize != 0 {
		c.ChunkingMaxSize = maxSize
	}
}
