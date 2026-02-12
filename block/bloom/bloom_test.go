package bloom

import (
	"fmt"
	"testing"

	boom "github.com/bits-and-blooms/bloom/v3"
)

// TestBloom performs a end to end test of the bloom block.
func TestBloom(t *testing.T) {
	n := uint(512)
	fpRate := float64(0.1)
	k := uint(4)

	bl := boom.NewWithEstimates(n, fpRate)
	var datas [][]byte
	for i := range 1000 {
		msgData := fmt.Appendf(nil, "hello world #%v", i)
		bl.Add(msgData)
		datas = append(
			datas,
			msgData,
		)
	}

	checkFilter := func(b *boom.BloomFilter) {
		if blk := b.K(); blk != k {
			t.Fatalf("expected k %v but got %v", k, blk)
		}
		for i, data := range datas {
			if !b.Test(data) {
				t.Fatalf("index %d: Test returned false", i)
			}
		}
	}
	checkFilter(bl)

	dataBlk := NewBloom(bl)
	md, err := dataBlk.MarshalBlock()
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("bloom filter size %v encoded to %v bytes", n, len(md))

	outBlk := NewBloomBlock().(*BloomFilter)
	err = outBlk.UnmarshalBlock(md)
	if err != nil {
		t.Fatal(err.Error())
	}

	bf := outBlk.ToBloomFilter()
	checkFilter(bf)
}

// TestBloomConsistent is a set of checks to ensure that optimalM and
// optimalK calculations are the same between releases.
func TestBloomConsistent(t *testing.T) {
	check := func(n uint, fpRate float64, expectedK, expectedM uint) {
		m, k := boom.EstimateParameters(n, fpRate)
		t.Logf(
			"OptimalM(n = %v, fpRate = %v) => %v (%v bytes)\n",
			n,
			fpRate,
			m,
			uint(float32(m)/8),
		)

		t.Logf("OptimalK(%v) => %v", fpRate, k)
		if k != expectedK {
			t.Fatalf("expected k %v but got %v", expectedK, k)
		}
		if m != expectedM {
			t.Fatalf("expected m %v but got %v", expectedM, m)
		}
	}

	// n=64, 10%, k=4, 38 bytes
	check(64, 0.1, 4, 307)
	// n=128, 10%, k=4, 76 bytes
	check(128, 0.1, 4, 614)
	// n=256, 10%, k=4, 153 bytes
	check(256, 0.1, 4, 1227)
	// n=512, 10%, k=4, 306 bytes
	check(512, 0.1, 4, 2454)
	// n=2048, 10%, k=4, 1227 bytes
	check(2048, 0.1, 4, 9816)
	// n=4096, 10%, k=4, 2453 bytes
	check(4096, 0.1, 4, 19631)
	// n=100,000, 10%, k=4, 59,906 bytes
	check(100000, 0.1, 4, 479253)
	// n=1,000,000, 40%, k=4, 238392 bytes
	check(1000000, 0.4, 2, 1907140)
}
