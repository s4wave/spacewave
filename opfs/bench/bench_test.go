//go:build js

package bench

import (
	"runtime"
	"sort"
	"sync"
	"testing"
)

type actorMetrics struct {
	avgBatch float64
	maxBatch int
	p95Batch int
	batches  int
}

type memoryMetrics struct {
	baseHeapSys   uint64
	baseHeapInuse uint64
	maxHeapSys    uint64
	maxHeapInuse  uint64
	maxHeapAlloc  uint64
	finalHeapSys  uint64
	finalHeapInuse uint64
	finalHeapAlloc uint64
}

func TestWriteActorCoalescing(t *testing.T) {
	const (
		bursts    = 64
		burstSize = 32
	)

	for _, tc := range []struct {
		name  string
		yield bool
	}{
		{name: "no-yield", yield: false},
		{name: "gosched", yield: true},
	} {
		m := runActorScenario(bursts, burstSize, tc.yield)
		t.Logf(
			"actor policy=%s bursts=%d burstSize=%d avgBatch=%.2f p95Batch=%d maxBatch=%d",
			tc.name,
			bursts,
			burstSize,
			m.avgBatch,
			m.p95Batch,
			m.maxBatch,
		)
	}
}

func TestWasmMemoryGrowth(t *testing.T) {
	const iterations = 24
	for _, size := range []int{
		2 << 20,
		4 << 20,
		8 << 20,
	} {
		fresh := runMemoryScenario(size, iterations, false)
		reuse := runMemoryScenario(size, iterations, true)
		t.Logf(
			"memory sizeMiB=%d mode=fresh baseHeapSys=%d maxHeapSys=%d finalHeapSys=%d maxHeapAlloc=%d finalHeapAlloc=%d",
			size>>20,
			fresh.baseHeapSys,
			fresh.maxHeapSys,
			fresh.finalHeapSys,
			fresh.maxHeapAlloc,
			fresh.finalHeapAlloc,
		)
		t.Logf(
			"memory sizeMiB=%d mode=reuse baseHeapSys=%d maxHeapSys=%d finalHeapSys=%d maxHeapAlloc=%d finalHeapAlloc=%d",
			size>>20,
			reuse.baseHeapSys,
			reuse.maxHeapSys,
			reuse.finalHeapSys,
			reuse.maxHeapAlloc,
			reuse.finalHeapAlloc,
		)
	}
}

func runActorScenario(bursts, burstSize int, yield bool) actorMetrics {
	reqCh := make(chan struct{}, bursts*burstSize)
	batchCh := make(chan int, bursts)
	doneCh := make(chan actorMetrics, 1)
	go func() {
		var sizes []int
		for {
			_, ok := <-reqCh
			if !ok {
				break
			}
			batchSize := 1
			if yield {
				runtime.Gosched()
			}
			for {
				select {
				case _, ok := <-reqCh:
					if !ok {
						doneCh <- summarizeActorSizes(sizes, batchSize)
						return
					}
					batchSize++
				default:
					sizes = append(sizes, batchSize)
					batchCh <- batchSize
					goto nextBatch
				}
			}
		nextBatch:
		}
		doneCh <- summarizeActorSizes(sizes, 0)
	}()

	for burst := 0; burst < bursts; burst++ {
		var wg sync.WaitGroup
		wg.Add(burstSize)
		for i := 0; i < burstSize; i++ {
			go func() {
				defer wg.Done()
				reqCh <- struct{}{}
			}()
		}
		wg.Wait()
		<-batchCh
	}
	close(reqCh)
	return <-doneCh
}

func summarizeActorSizes(sizes []int, tail int) actorMetrics {
	if tail > 0 {
		sizes = append(sizes, tail)
	}
	if len(sizes) == 0 {
		return actorMetrics{}
	}
	sorted := append([]int(nil), sizes...)
	sort.Ints(sorted)
	sum := 0
	for _, size := range sizes {
		sum += size
	}
	return actorMetrics{
		avgBatch: float64(sum) / float64(len(sizes)),
		maxBatch: sorted[len(sorted)-1],
		p95Batch: sorted[(len(sorted)-1)*95/100],
		batches:  len(sizes),
	}
}

func runMemoryScenario(size, iterations int, reuse bool) memoryMetrics {
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	metrics := memoryMetrics{
		baseHeapSys:   before.HeapSys,
		baseHeapInuse: before.HeapInuse,
		maxHeapSys:    before.HeapSys,
		maxHeapInuse:  before.HeapInuse,
		maxHeapAlloc:  before.HeapAlloc,
	}

	var scratch []byte
	for i := 0; i < iterations; i++ {
		var buf []byte
		if reuse {
			if cap(scratch) < size {
				scratch = make([]byte, size)
			}
			buf = scratch[:size]
		} else {
			buf = make([]byte, size)
		}
		touchPages(buf, byte(i))
		runtime.GC()
		var current runtime.MemStats
		runtime.ReadMemStats(&current)
		if current.HeapSys > metrics.maxHeapSys {
			metrics.maxHeapSys = current.HeapSys
		}
		if current.HeapInuse > metrics.maxHeapInuse {
			metrics.maxHeapInuse = current.HeapInuse
		}
		if current.HeapAlloc > metrics.maxHeapAlloc {
			metrics.maxHeapAlloc = current.HeapAlloc
		}
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	metrics.finalHeapSys = after.HeapSys
	metrics.finalHeapInuse = after.HeapInuse
	metrics.finalHeapAlloc = after.HeapAlloc
	return metrics
}

func touchPages(buf []byte, seed byte) {
	for i := 0; i < len(buf); i += 4096 {
		buf[i] = seed + byte(i/4096)
	}
	if len(buf) > 0 {
		buf[len(buf)-1] = seed
	}
}
