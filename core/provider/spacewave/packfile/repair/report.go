package repair

import packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"

// Report summarizes a pack metadata audit or repair run.
type Report struct {
	PacksScanned                 int
	PacksChanged                 int
	PackBlockCountTotal          uint64
	PackBlockCountMin            uint64
	PackBlockCountMax            uint64
	BeforeMaxFalsePositiveRate   float64
	AfterMaxFalsePositiveRate    float64
	Findings                     []*Finding
	UpdatedEntries               []*packfile.PackfileEntry
	VerifiedIndexedBlockCountSum uint64
}

func (r *Report) addBlockCount(count uint64) {
	r.PackBlockCountTotal += count
	if r.PacksScanned == 1 || count < r.PackBlockCountMin {
		r.PackBlockCountMin = count
	}
	if count > r.PackBlockCountMax {
		r.PackBlockCountMax = count
	}
}
