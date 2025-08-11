/*
 * Copyright (c) 2024 Gilles Chehade <gilles@poolp.org>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

// Package jc implements the JC (Jump Condition) content-defined chunking algorithm.
//
// This file is from https://raw.githubusercontent.com/PlakarKorp/go-cdc-chunkers/423839/chunkers/jc/jc.go and is licensed under the ISC license, shown above.
//
// It has been modified to have a simplified API.
package jc

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"

	"github.com/zeebo/blake3"
)

var (
	ErrNormalSize = errors.New("NormalSize is required and must be 64B <= NormalSize <= 1GB")
	ErrMinSize    = errors.New("MinSize is required and must be 64B <= MinSize <= 1GB && MinSize < NormalSize")
	ErrMaxSize    = errors.New("MaxSize is required and must be 64B <= MaxSize <= 1GB && MaxSize > NormalSize")
)

func generateSpacedMask(oneCount int, totalBits int) uint64 {
	if oneCount >= totalBits {
		return 0xFFFFFFFFFFFFFFFF
	}
	if oneCount <= 0 {
		return 0
	}

	step := totalBits / oneCount
	var mask uint64 = 0
	for i := range oneCount {
		pos := totalBits - 1 - i*step
		if pos >= 0 {
			mask |= 1 << pos
		}
	}
	return mask
}

func embedMask(maskC uint64) uint64 {
	if maskC == 0 {
		return 0
	}
	// Unset the least significant 1-bit in maskC
	return maskC & (maskC - 1)
}

// nonceBlake3Context is the blake3 nonce constant.
// don't change this
const nonceBlake3Context = "aperturerobotics/hydra 2025-08-10 jc nonce v1."

// gLen is the length of G
const gLen = 256

// defaultG is the default value for G pre-computed and used if len(key) == 0
// returns [256]uint64 as a []uint64
var defaultG = sync.OnceValue(func() []uint64 {
	out := make([]uint64, gLen)
	hashKeyForG([]byte("default G value for jc cdc"), out)
	return out
})

// hashKeyForG hashes a key for a G value.
func hashKeyForG(key []byte, out []uint64) {
	digestBytes := make([]byte, 8*gLen)
	blake3.DeriveKey(nonceBlake3Context, key, digestBytes)

	for i := range gLen {
		offset := i * 8
		if i >= len(out) {
			break
		}
		out[i] = binary.LittleEndian.Uint64(digestBytes[offset : offset+8])
	}
}

type JC struct {
	G          [gLen]uint64
	maskC      uint64
	maskJ      uint64
	jumpLength int

	minSize, maxSize, normalSize int
}

// NewJC constructs the JC with the default parameters.
func NewJC() (*JC, error) {
	var minSize = 2 * 1024
	var maxSize = 64 * 1024
	var normalSize = 8 * 1024

	return NewWithOptions(minSize, maxSize, normalSize, nil)
}

// NewWithOptions constructs the JC with the given options.
func NewWithOptions(minSize, maxSize, normalSize int, key []byte) (*JC, error) {
	// validate parameters
	if normalSize == 0 || normalSize < 64 || normalSize > 1024*1024*1024 {
		return nil, ErrNormalSize
	}
	if minSize < 64 || minSize > 1024*1024*1024 || minSize >= normalSize {
		return nil, ErrMinSize
	}
	if maxSize < 64 || maxSize > 1024*1024*1024 || maxSize <= normalSize {
		return nil, ErrMaxSize
	}

	c := &JC{minSize: minSize, maxSize: maxSize, normalSize: normalSize}
	bits := uint64(math.Log2(float64(normalSize)))

	cOnes := bits - 1
	jOnes := cOnes - 1
	numerator := 1 << (cOnes + jOnes)
	denominator := (1 << cOnes) - (1 << jOnes)
	c.jumpLength = numerator / denominator

	c.maskC = generateSpacedMask(int(cOnes), 64)
	c.maskJ = embedMask(c.maskC)

	// the key can be len(0) here and this will still be acceptable.
	if len(key) == 0 {
		copy(c.G[:], defaultG())
	} else {
		hashKeyForG(key, c.G[:])
	}

	return c, nil
}

// Algorithm implements the JC algorithm.
func (c *JC) Algorithm(data []byte, n int) int {
	switch {
	case n <= c.normalSize:
		return n
	case n >= c.maxSize:
		n = c.maxSize
	}

	fp := uint64(0)
	i := c.minSize

	for i < n {
		fp = (fp << 1) + c.G[data[i]]
		if (fp & c.maskJ) == 0 {
			if (fp & c.maskC) == 0 {
				return i
			}
			fp = 0
			i = i + c.jumpLength
		} else {
			i++
		}
	}

	return min(i, n)
}
