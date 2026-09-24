package sobject_sync

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/util/routine"
)

// leanSyncJoinCases observes real routine cancellation, blocked body exit and joined return.
func leanSyncJoinCases(t *testing.T, count, cut int) []leanSyncCase {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	workers := make([]*routine.RoutineContainer, count)
	gates := make([]chan struct{}, count)
	canceled := make([]chan struct{}, count)
	var stopped, exited atomic.Int32
	for i := range workers {
		workers[i] = routine.NewRoutineContainer()
		gates[i], canceled[i] = make(chan struct{}), make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-gates[i]:
			default:
				close(gates[i])
			}
			if exited, _ := workers[i].SetRoutine(nil); exited != nil {
				<-exited
			}
		})
		started := make(chan struct{})
		workers[i].SetRoutine(func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			stopped.Add(1)
			close(canceled[i])
			<-gates[i]
			exited.Add(1)
			return ctx.Err()
		})
		workers[i].SetContext(ctx, false)
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("join fixture worker did not start")
		}
		if i < cut {
			close(gates[i])
		}
	}
	joined := make(chan struct{})
	go func() {
		joinSyncWorkers(workers...)
		close(joined)
	}()
	defer func() {
		for i := cut; i < count; i++ {
			select {
			case <-gates[i]:
			default:
				close(gates[i])
			}
		}
		cancel()
		<-joined
	}()

	project := func(done bool, ready int) leanSyncCase {
		var a fastjson.Arena
		input, expected, result, observations := a.NewObject(), a.NewObject(), a.NewObject(), a.NewArray()
		for i := range workers {
			value := a.NewObject()
			value.Set("channel", a.NewTrue())
			value.Set("exited", leanSyncBool(&a, i < ready))
			observations.SetArrayItem(i, value)
		}
		input.Set("op", a.NewString("joinSyncWorkers"))
		input.Set("workers", observations)
		result.Set("stopped", a.NewNumberInt(int(stopped.Load())))
		result.Set("joined", a.NewNumberInt(int(exited.Load())))
		result.Set("done", leanSyncBool(&a, done))
		expected.Set("joining", result)
		return leanSyncCase{name: "join workers " + strconv.Itoa(count) + " ready " + strconv.Itoa(ready),
			request: input.MarshalTo(nil), expected: expected.MarshalTo(nil)}
	}
	var cases []leanSyncCase
	if cut < count {
		select {
		case <-canceled[cut]:
		case <-ctx.Done():
			t.Fatal("join did not reach the blocked worker")
		}
		select {
		case <-joined:
			t.Fatal("join returned while a worker body was still blocked")
		default:
		}
		cases = append(cases, project(false, cut))
		for i := cut; i < count; i++ {
			close(gates[i])
		}
	}
	select {
	case <-joined:
	case <-ctx.Done():
		t.Fatal("join did not finish after every body was released")
	}
	cases = append(cases, project(true, count))
	return cases
}

// TestLeanSyncShutdownConformance checks every partial join boundary and the empty worker list.
func TestLeanSyncShutdownConformance(t *testing.T) {
	oracle := leanSyncOracle(t)
	var cases []leanSyncCase
	for cut := range 5 {
		cases = append(cases, leanSyncJoinCases(t, 4, cut)...)
	}
	cases = append(cases, leanSyncJoinCases(t, 0, 0)...)
	checkLeanSync(t, oracle, cases)
}

// FuzzLeanSyncShutdown varies worker cardinality and the first unavailable body acknowledgment.
func FuzzLeanSyncShutdown(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(19))
	f.Fuzz(func(t *testing.T, seed uint64) {
		count := int(seed % 5)
		cut := int(seed / 5 % uint64(count+1))
		checkLeanSync(t, leanSyncOracle(t), leanSyncJoinCases(t, count, cut))
	})
}
