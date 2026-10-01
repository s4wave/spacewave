package kvtx

import (
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/s4wave/spacewave/db/block"
)

// TestPublicationCloseJoinsEveryPhysicalGroup holds the second group after the
// first completion broadcast and verifies that Close still owns the drain.
func TestPublicationCloseJoinsEveryPhysicalGroup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Hold the first physical acquisition while two groups enter the writer.
		v, store := newPublicationTestVolume(t)
		first := store.arm(t)
		receipts := []*block.PublicationReceipt{submitPublication(t, v, publicationFor(t, "0", "", "done"))}
		<-first.started
		for i := 1; i <= publicationGroupLimit; i++ {
			receipts = append(receipts, submitPublication(t, v, publicationFor(t, fmt.Sprint(i), "", "done")))
		}
		second := store.arm(t)

		// Begin shutdown while the writer cannot yet complete either group.
		closed := make(chan error, 1)
		go func() { closed <- v.Close() }()
		synctest.Wait()

		// Finish only the first group, leaving the second physical acquisition blocked.
		first.unblock()
		<-second.started
		synctest.Wait()
		select {
		case err := <-closed:
			t.Errorf("Close returned before the second group completed: %v", err)
		default:
		}

		// Release the final group and require every admitted publication to finish.
		second.unblock()
		for _, receipt := range receipts {
			if err := awaitPublication(t, receipt); err != nil {
				t.Fatal(err)
			}
		}
		if err := v.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

// TestPublicationFenceKeepsItsAdmissionBoundary admits a second group after a
// fence starts and verifies that the later group cannot extend that fence.
func TestPublicationFenceKeepsItsAdmissionBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Fill the first group before the fence captures its accepted prefix.
		v, store := newPublicationTestVolume(t)
		first := store.arm(t)
		submitPublication(t, v, publicationFor(t, "0", "", "done"))
		<-first.started
		for i := 1; i < publicationGroupLimit; i++ {
			submitPublication(t, v, publicationFor(t, fmt.Sprint(i), "", "done"))
		}
		synced := make(chan error, 1)
		go func() {
			_, err := v.Sync(t.Context())
			synced <- err
		}()
		synctest.Wait()

		// Admit later work, then finish only the prefix the fence already observed.
		second := store.arm(t)
		later := submitPublication(t, v, publicationFor(t, "later", "", "done"))
		first.unblock()
		<-second.started
		synctest.Wait()
		select {
		case err := <-synced:
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Error("Sync waited for a publication admitted after its fence")
		}

		// Release the unrelated group and join all test-owned operations.
		second.unblock()
		if err := awaitPublication(t, later); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
	})
}
