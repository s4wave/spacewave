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
	"io"
	"math"
	"sync"

	"github.com/aperturerobotics/util/scrub"
	"github.com/zeebo/blake3"
)

var (
	ErrTargetSize = errors.New("TargetSize is required and must be 64B <= TargetSize <= 1GB")
	ErrMinSize    = errors.New("MinSize is required and must be 64B <= MinSize <= 1GB && MinSize < TargetSize")
	ErrMaxSize    = errors.New("MaxSize is required and must be 64B <= MaxSize <= 1GB && MaxSize > TargetSize")
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

// Default chunking parameters
const (
	DefaultMinSize    uint64 = 2 * 1024  // 2KB
	DefaultMaxSize    uint64 = 64 * 1024 // 64KB
	DefaultTargetSize uint64 = 8 * 1024  // 8KB
)

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

// Chunk represents a chunk of data with its position and data.
type Chunk struct {
	// Start is the absolute position in the stream where this chunk begins
	Start uint64
	// Length is the length of the chunk
	Length int
	// Data contains the actual chunk data
	Data []byte
}

type JC struct {
	G          [gLen]uint64
	maskC      uint64
	maskJ      uint64
	jumpLength int

	minSize, maxSize, targetSize int
}

// Chunker wraps JC to provide streaming chunking functionality.
type Chunker struct {
	jc     *JC
	reader io.Reader
	buf    []byte // sliding window buffer
	bufLen int    // current valid data length in buf
	bufPos int    // current read position in buf
	pos    uint64 // absolute position in stream
	eof    bool
}

// NewJC constructs the JC with the default parameters.
func NewJC() (*JC, error) {
	return NewWithOptions(DefaultMinSize, DefaultMaxSize, DefaultTargetSize, nil)
}

// NewWithOptions constructs the JC with the given options.
func NewWithOptions(minSize, maxSize, targetSize uint64, key []byte) (*JC, error) {
	// validate parameters
	if targetSize == 0 || targetSize < 64 || targetSize > 1024*1024*1024 {
		return nil, ErrTargetSize
	}
	if minSize < 64 || minSize > 1024*1024*1024 || minSize >= targetSize {
		return nil, ErrMinSize
	}
	if maxSize < 64 || maxSize > 1024*1024*1024 || maxSize <= targetSize {
		return nil, ErrMaxSize
	}

	c := &JC{minSize: int(minSize), maxSize: int(maxSize), targetSize: int(targetSize)}
	bits := uint64(math.Log2(float64(targetSize)))

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

// NewChunker creates a new streaming chunker with the default parameters.
func NewChunker(reader io.Reader) (*Chunker, error) {
	jc, err := NewJC()
	if err != nil {
		return nil, err
	}
	return &Chunker{
		jc:     jc,
		reader: reader,
		buf:    make([]byte, jc.maxSize+jc.minSize), // just enough for sliding window
	}, nil
}

// NewChunkerWithOptions creates a new streaming chunker with the given options.
func NewChunkerWithOptions(reader io.Reader, minSize, maxSize, targetSize uint64, key []byte) (*Chunker, error) {
	jc, err := NewWithOptions(minSize, maxSize, targetSize, key)
	if err != nil {
		return nil, err
	}
	return &Chunker{
		jc:     jc,
		reader: reader,
		buf:    make([]byte, jc.maxSize+jc.minSize), // just enough for sliding window
	}, nil
}

// Algorithm implements the JC algorithm as a top-level function.
func Algorithm(data []byte, n int, G []uint64, maskC, maskJ uint64, jumpLength, minSize, maxSize, targetSize int) int {
	switch {
	case n <= targetSize:
		return n
	case n >= maxSize:
		n = maxSize
	}

	fp := uint64(0)
	i := minSize

	for i < n {
		fp = (fp << 1) + G[data[i]]
		if (fp & maskJ) == 0 {
			if (fp & maskC) == 0 {
				return i
			}
			fp = 0
			i = i + jumpLength
		} else {
			i++
		}
	}

	return min(i, n)
}

// Algorithm implements the JC algorithm.
func (c *JC) Algorithm(data []byte, n int) int {
	return Algorithm(data, n, c.G[:], c.maskC, c.maskJ, c.jumpLength, c.minSize, c.maxSize, c.targetSize)
}

// Reset clears the internal buffers and resets the chunker state to release memory.
// The chunker can be reused after calling Reset with the same reader and options.
func (c *Chunker) Reset() {
	if c.buf != nil {
		scrub.Scrub(c.buf)
	}
	c.bufLen = 0
	c.bufPos = 0
	c.pos = 0
	c.eof = false
}

// Next returns the position and length of the next chunk of data. If an error
// occurs while reading, the error is returned. Afterwards, the state of the
// current chunk is undefined. When the last chunk has been returned, all
// subsequent calls yield an io.EOF error.
func (c *Chunker) Next(data []byte) (Chunk, error) {
	availableData := c.bufLen - c.bufPos
	if c.eof && availableData == 0 {
		return Chunk{}, io.EOF
	}

	// Keep reading until we have enough data for chunk boundary detection or reach EOF
	for !c.eof {
		// If we have enough data to potentially find a boundary, try that first
		if availableData >= c.jc.minSize {
			// Check if we can find a chunk boundary
			chunkSize := c.jc.Algorithm(c.buf[c.bufPos:c.bufLen], availableData)

			// If we found a boundary before maxSize, use it
			if chunkSize < availableData || availableData >= c.jc.maxSize {
				break
			}
		}

		// Compact buffer if needed - only compact when we've consumed a significant portion
		if c.bufPos > len(c.buf)/4 && c.bufLen > c.bufPos {
			copy(c.buf, c.buf[c.bufPos:c.bufLen])
			c.bufLen -= c.bufPos
			c.bufPos = 0
		} else if c.bufPos > 0 && c.bufLen == c.bufPos {
			// All data consumed, reset positions
			c.bufLen = 0
			c.bufPos = 0
		}

		// Read more data into buffer
		if c.bufLen < len(c.buf) {
			n, err := c.reader.Read(c.buf[c.bufLen:])
			if n > 0 {
				c.bufLen += n
				availableData = c.bufLen - c.bufPos
			}
			if err != nil {
				if err == io.EOF {
					c.eof = true
				} else {
					return Chunk{}, err
				}
			}
		} else {
			// Buffer is full but we still need more data - this shouldn't happen
			// with proper maxSize, but handle gracefully
			break
		}
	}

	// If no data left, return EOF
	if availableData == 0 {
		return Chunk{}, io.EOF
	}

	// Find chunk boundary using JC algorithm
	chunkSize := c.jc.Algorithm(c.buf[c.bufPos:c.bufLen], availableData)

	// Extract chunk data - avoid allocation when possible
	var chunkData []byte
	if data != nil && len(data) >= chunkSize {
		// Use provided buffer - copy data into it
		chunkData = data[:chunkSize]
		copy(chunkData, c.buf[c.bufPos:c.bufPos+chunkSize])
	} else {
		// Return a slice of our internal buffer - caller must not modify
		chunkData = c.buf[c.bufPos : c.bufPos+chunkSize]
	}

	// Create chunk
	chunk := Chunk{
		Start:  c.pos,
		Length: chunkSize,
		Data:   chunkData,
	}

	// Update state
	c.bufPos += chunkSize
	c.pos += uint64(chunkSize)

	return chunk, nil
}
