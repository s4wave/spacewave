package main

import "github.com/s4wave/spacewave/db/sqlitefree"

// pendingByte is SQLite's PENDING_BYTE. The page holding it is the lock-byte
// page, which SQLite never uses and never puts on the freelist.
const pendingByte = sqlitefree.PendingByte

// lockByteOffset is where crosscheck and scan place the lock-byte page; a
// test moves it into a small file.
var lockByteOffset int64 = pendingByte

// lockBytePage is the page crosscheck and scan treat as the lock-byte page.
func lockBytePage(h header) uint32 { return uint32(lockByteOffset/h.PageSize + 1) } //nolint:gosec // the lock-byte page number is under 2^22.

// The header, freelist walk and punch plan are sqlitefree's, which the sqlite
// kvtx store's online punch shares.
type (
	// header is the part of the database header the punch needs.
	header = sqlitefree.Header
	// bitset is one bit per page number.
	bitset = sqlitefree.Bitset
	// walk is one pass over the freelist trunk chain.
	walk = sqlitefree.Walk
	// pageRange is count pages starting at page start.
	pageRange = sqlitefree.Range
	// plan is what a punch releases.
	plan = sqlitefree.Plan
)

// The shared header, walk and plan functions under the names this command
// and its tests use.
var (
	// readHeader reads and checks the 100 byte header of page 1.
	readHeader = sqlitefree.ReadHeader
	// newBitset returns a bitset for pages 0 to n.
	newBitset = sqlitefree.NewBitset
	// walkFreelist walks the trunk chain, strictly or tolerantly.
	walkFreelist = sqlitefree.WalkFreelist
	// makePlan keeps back the head trunks' leaves and ranges the rest.
	makePlan = sqlitefree.MakePlan
	// strictPlan walks strictly and refuses a count unlike the header's.
	strictPlan = sqlitefree.StrictPlan
)
