//go:build !js

package storebench

import (
	"bytes"
	"strings"
	"time"

	"github.com/google/pprof/profile"
	"github.com/pkg/errors"
)

// cpuPath names a code path by substrings of its function names. A sample
// counts toward the path when any frame of its stack holds one of them.
type cpuPath struct {
	// name names the path in the report.
	name string
	// funcs are the function name substrings.
	funcs []string
}

// cpuPaths are the code paths the report attributes CPU to.
var cpuPaths = []cpuPath{
	{name: "load-write", funcs: []string{"storebench.(*load).commitWrite"}},
	{name: "world-commit", funcs: []string{"world/block.(*EngineTx).Commit", "space/world.(*spaceTurn).Admit"}},
	{name: "s4db-commit", funcs: []string{"s4db.(*writer).commit"}},
	{name: "watch-view", funcs: []string{"world.WatchViewWith"}},
	{name: "read-set-touched", funcs: []string{"world.(*ReadSet).Touched"}},
	{name: "decode", funcs: []string{".UnmarshalVT"}},
	{name: "okra", funcs: []string{"/okra."}},
	{name: "collect", funcs: []string{"gc.(*Collector).Collect"}},
	{name: "gc", funcs: []string{"runtime.gcBgMarkWorker", "runtime.gcAssistAlloc"}},
}

// attributeCPU parses a CPU profile and returns the share of its samples on
// each path, after a total row.
func attributeCPU(data []byte) ([]*CpuShare, error) {
	// Parse the profile and find its CPU time sample.
	prof, err := profile.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "parse cpu profile")
	}
	vi := -1
	for i, st := range prof.SampleType {
		if st.Type == "cpu" {
			vi = i
		}
	}
	if vi < 0 {
		return nil, errors.New("cpu profile has no cpu sample type")
	}

	// Sum the CPU of every sample and of the samples on each path.
	var total int64
	paths := make([]int64, len(cpuPaths))
	for _, s := range prof.Sample {
		v := s.Value[vi]
		total += v
		for i, p := range cpuPaths {
			if sampleOnPath(s, p) {
				paths[i] += v
			}
		}
	}

	// Report the total and each path's share of it.
	out := []*CpuShare{{Name: "total", Fraction: 1, CpuMs: uint64(time.Duration(total) / time.Millisecond)}} // #nosec G115 -- sample totals are positive.
	for i, p := range cpuPaths {
		share := &CpuShare{Name: p.name, CpuMs: uint64(time.Duration(paths[i]) / time.Millisecond)} // #nosec G115 -- sample totals are positive.
		if total > 0 {
			share.Fraction = float64(paths[i]) / float64(total)
		}
		out = append(out, share)
	}
	return out, nil
}

// sampleOnPath reports whether a frame of the sample's stack is on the path.
func sampleOnPath(s *profile.Sample, p cpuPath) bool {
	for _, loc := range s.Location {
		for _, line := range loc.Line {
			if line.Function == nil {
				continue
			}
			for _, f := range p.funcs {
				if strings.Contains(line.Function.Name, f) {
					return true
				}
			}
		}
	}
	return false
}
