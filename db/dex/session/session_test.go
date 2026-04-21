package dex_session

import (
	"net"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// newTestPair creates a bidirectional pair of DexSessions using net.Pipe.
func newTestPair() (*DexSession, *DexSession) {
	c1, c2 := net.Pipe()
	s1 := NewDexSession(c1, 0, 0)
	s2 := NewDexSession(c2, 0, 0)
	return s1, s2
}

// buildTestData creates test data of the given size and a matching BlockRef.
func buildTestData(t *testing.T, size int) ([]byte, *block.BlockRef) {
	t.Helper()
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return data, ref
}

func TestSendReceiveBlock(t *testing.T) {
	s1, s2 := newTestPair()
	defer s1.Close()
	defer s2.Close()

	data, ref := buildTestData(t, 3500)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s1.SendBlock(1, ref, data)
	}()

	requestID, gotRef, gotData, err := s2.ReceiveBlock(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if requestID != 1 {
		t.Fatalf("expected request ID 1, got %d", requestID)
	}
	if !gotRef.EqualVT(ref) {
		t.Fatal("received ref does not match sent ref")
	}
	if len(gotData) != len(data) {
		t.Fatalf("expected data length %d, got %d", len(data), len(gotData))
	}
	for i := range data {
		if gotData[i] != data[i] {
			t.Fatalf("data mismatch at index %d", i)
			break
		}
	}

	if err := <-errCh; err != nil {
		t.Fatal(err.Error())
	}
}

func TestSendReceiveSmallBlock(t *testing.T) {
	s1, s2 := newTestPair()
	defer s1.Close()
	defer s2.Close()

	data, ref := buildTestData(t, 100)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s1.SendBlock(2, ref, data)
	}()

	requestID, gotRef, gotData, err := s2.ReceiveBlock(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if requestID != 2 {
		t.Fatalf("expected request ID 2, got %d", requestID)
	}
	if !gotRef.EqualVT(ref) {
		t.Fatal("received ref does not match sent ref")
	}
	if len(gotData) != len(data) {
		t.Fatalf("expected data length %d, got %d", len(data), len(gotData))
	}

	if err := <-errCh; err != nil {
		t.Fatal(err.Error())
	}
}

func TestMaxBlockSizeRejection(t *testing.T) {
	s1, s2 := newTestPair()
	defer s1.Close()
	defer s2.Close()

	data, ref := buildTestData(t, 5000)

	errCh := make(chan error, 1)
	go func() {
		// Send init with a large total_size, then close.
		errCh <- s1.SendInit(3, ref, 20000000)
	}()

	// Use a small max block size so the init is rejected.
	_, _, _, err := s2.ReceiveBlock(1000)
	if err == nil {
		t.Fatal("expected error for oversized block")
	}
	_ = data

	if sendErr := <-errCh; sendErr != nil {
		t.Fatal(sendErr.Error())
	}
}

func TestOversizeDataRejection(t *testing.T) {
	s1, s2 := newTestPair()
	defer s1.Close()
	defer s2.Close()

	data, ref := buildTestData(t, 200)

	errCh := make(chan error, 1)
	go func() {
		// Lie about total_size being 100 but send 200 bytes of data.
		if err := s1.SendInit(4, ref, 100); err != nil {
			errCh <- err
			return
		}
		// Send all data in one chunk.
		errCh <- s1.SendChunk(4, data, true)
	}()

	_, _, _, err := s2.ReceiveBlock(0)
	if err == nil {
		t.Fatal("expected error for data exceeding declared size")
	}

	<-errCh
}

func TestHashMismatch(t *testing.T) {
	s1, s2 := newTestPair()
	defer s1.Close()
	defer s2.Close()

	data, ref := buildTestData(t, 500)

	// Corrupt the data after building the ref.
	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	corrupted[0] ^= 0xFF

	errCh := make(chan error, 1)
	go func() {
		// Send init with the correct ref but wrong data.
		if err := s1.SendInit(5, ref, uint64(len(corrupted))); err != nil {
			errCh <- err
			return
		}
		errCh <- s1.SendChunk(5, corrupted, true)
	}()

	_, _, _, err := s2.ReceiveBlock(0)
	if err == nil {
		t.Fatal("expected hash mismatch error")
	}

	<-errCh
}

func TestCancel(t *testing.T) {
	s1, s2 := newTestPair()
	defer s1.Close()
	defer s2.Close()

	_, ref := buildTestData(t, 100)

	errCh := make(chan error, 1)
	go func() {
		if err := s1.SendInit(6, ref, 100); err != nil {
			errCh <- err
			return
		}
		errCh <- s1.SendCancel(6)
	}()

	// Read the init.
	msg, err := s2.ReadMessage()
	if err != nil {
		t.Fatal(err.Error())
	}
	if msg.GetRef() == nil {
		t.Fatal("expected init message with ref")
	}

	// Read the cancel.
	msg, err = s2.ReadMessage()
	if err != nil {
		t.Fatal(err.Error())
	}
	if !msg.GetCancel() {
		t.Fatal("expected cancel message")
	}

	if sendErr := <-errCh; sendErr != nil {
		t.Fatal(sendErr.Error())
	}
}

func TestMultipleBlocks(t *testing.T) {
	s1, s2 := newTestPair()
	defer s1.Close()
	defer s2.Close()

	data1, ref1 := buildTestData(t, 1500)
	data2, ref2 := buildTestData(t, 2000)
	// Ensure data2 is different from data1.
	for i := range data2 {
		data2[i] = byte((i + 128) % 256)
	}
	var err error
	ref2, err = block.BuildBlockRef(data2, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	errCh := make(chan error, 1)
	go func() {
		if err := s1.SendBlock(10, ref1, data1); err != nil {
			errCh <- err
			return
		}
		errCh <- s1.SendBlock(11, ref2, data2)
	}()

	// Receive first block.
	reqID1, gotRef1, gotData1, err := s2.ReceiveBlock(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if reqID1 != 10 {
		t.Fatalf("expected request ID 10, got %d", reqID1)
	}
	if !gotRef1.EqualVT(ref1) {
		t.Fatal("first block ref mismatch")
	}
	if len(gotData1) != len(data1) {
		t.Fatalf("first block data length mismatch: expected %d, got %d", len(data1), len(gotData1))
	}

	// Receive second block.
	reqID2, gotRef2, gotData2, err := s2.ReceiveBlock(0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if reqID2 != 11 {
		t.Fatalf("expected request ID 11, got %d", reqID2)
	}
	if !gotRef2.EqualVT(ref2) {
		t.Fatal("second block ref mismatch")
	}
	if len(gotData2) != len(data2) {
		t.Fatalf("second block data length mismatch: expected %d, got %d", len(data2), len(gotData2))
	}

	if sendErr := <-errCh; sendErr != nil {
		t.Fatal(sendErr.Error())
	}
}
