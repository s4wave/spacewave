package space_backup_test

import "time"

// writeLatency keeps a bounded 100-microsecond histogram and the exact maximum.
// Its final bucket includes slower samples; p99 then reports the exact maximum.
type writeLatency struct {
	// buckets count observations, capped at ten seconds.
	buckets [100001]uint64
	// count is the number of observed writes.
	count uint64
	// longest is the exact longest write, including overflow samples.
	longest time.Duration
}

// add observes one write without allocating or retaining a per-write record.
func (l *writeLatency) add(elapsed time.Duration) {
	l.count++
	l.longest = max(l.longest, elapsed)
	l.buckets[min(int(elapsed/(100*time.Microsecond)), len(l.buckets)-1)]++
}

// p99 returns the inclusive upper bucket edge for at least 99% of observations.
func (l *writeLatency) p99() time.Duration {
	var count uint64
	for index, samples := range l.buckets {
		count += samples
		if count*100 >= l.count*99 {
			if index == len(l.buckets)-1 {
				return l.longest
			}
			return time.Duration(index+1) * 100 * time.Microsecond
		}
	}
	return l.longest
}
