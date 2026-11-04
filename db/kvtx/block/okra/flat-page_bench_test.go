//go:build !goscript

package kvtx_block_okra

import (
	"runtime"
	"testing"
	"time"
)

// benchFlatPages is the resident set size of the GC benchmarks.
const benchFlatPages = 20000

// BenchmarkFlatPageDecode measures one decode of an encoded page and a lookup in it.
func BenchmarkFlatPageDecode(b *testing.B) {
	// Encode once, then decode into the old and the flat form.
	page := newFlatSamplePage()
	data, _ := page.MarshalVT()
	key := page.Entries[20].Key
	b.Run("page", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			p := &Page{}
			if err := p.UnmarshalDecodedBlock(data); err != nil {
				b.Fatal(err)
			}
			_ = p.Entries[20].Key
		}
	})
	b.Run("flat", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			p := &flatPage{}
			if err := p.UnmarshalDecodedBlock(data); err != nil {
				b.Fatal(err)
			}
			_ = p.searchEntry(key)
		}
	})
}

// BenchmarkFlatPageLookup measures a key search in an already decoded page.
func BenchmarkFlatPageLookup(b *testing.B) {
	// Decode once into each form, then search repeatedly.
	page := newFlatSamplePage()
	data, _ := page.MarshalVT()
	old := &Page{}
	flat := &flatPage{}
	if err := old.UnmarshalDecodedBlock(data); err != nil {
		b.Fatal(err)
	}
	if err := flat.UnmarshalDecodedBlock(data); err != nil {
		b.Fatal(err)
	}
	key := page.Entries[20].Key
	b.Run("page", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			old.searchEntry(key)
		}
	})
	b.Run("flat", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			flat.searchEntry(key)
		}
	})
}

// BenchmarkResidentGC measures a full collection with many resident pages.
func BenchmarkResidentGC(b *testing.B) {
	data, _ := newFlatSamplePage().MarshalVT()
	run := func(b *testing.B, resident func(n int) any) {
		keep := resident(benchFlatPages)
		runtime.GC()
		var gcs0, gcs1 runtime.MemStats
		runtime.ReadMemStats(&gcs0)
		start := time.Now()
		for range b.N {
			runtime.GC()
		}
		elapsed := time.Since(start)
		runtime.ReadMemStats(&gcs1)
		runtime.KeepAlive(keep)
		b.ReportMetric(float64(elapsed.Nanoseconds())/float64(b.N)/1e6, "ms/gc")
		b.ReportMetric(float64(gcs1.HeapAlloc)/1e6, "heapMB")
	}
	b.Run("page", func(b *testing.B) {
		run(b, func(n int) any {
			out := make([]*Page, n)
			for i := range out {
				out[i] = &Page{}
				if err := out[i].UnmarshalDecodedBlock(data); err != nil {
					b.Fatal(err)
				}
			}
			return out
		})
	})
	b.Run("flat", func(b *testing.B) {
		run(b, func(n int) any {
			out := make([]*flatPage, n)
			for i := range out {
				out[i] = &flatPage{}
				if err := out[i].UnmarshalDecodedBlock(data); err != nil {
					b.Fatal(err)
				}
			}
			return out
		})
	})
}
