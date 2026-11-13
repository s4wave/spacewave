//go:build !js && !wasip1 && !windows

package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// planMagic starts a plan file, before its gzip stream ends; planVersion is
// the layout of what follows it.
const (
	planMagic   = "swr-plan\n"
	planVersion = 3
)

// planMaxField and planMaxCount bound a field's bytes and a list's length in
// a plan file, so a corrupt file fails instead of allocating without bound.
const (
	planMaxField = 1 << 20
	planMaxCount = 1 << 28
	// planMaxParts bounds the input parts, far past planInputs' seven.
	planMaxParts = 64
)

// Reasons a --packs id is dropped from the plan.
const (
	dropNoWants   = "no-wants"
	dropNoEntry   = "no-pack-entry"
	dropMinWanted = "min-wanted"
	dropMaxPacks  = "max-packs-cut"
)

// dropReasons lists the drop reasons in the plan line's order.
var dropReasons = []string{dropNoWants, dropNoEntry, dropMinWanted, dropMaxPacks}

// droppedListName is the file in OutDir an in-process plan writes its
// dropped --packs ids to, as the log names no pack id.
const droppedListName = "plan-dropped.tsv"

// droppedPack is a --packs id the plan does not fetch, why, and the wanted
// refs it held when dropped.
type droppedPack struct {
	id     string
	reason string
	wants  int
}

// planCounts are the result counters planning adds. Absent and Ambiguous are
// also in the ledger, which a run with the plan counts at its start; the
// others are not, and a run with the plan adds them.
type planCounts struct {
	Absent, Deferred, SharedNum, NoExtent, Ambiguous int
}

// planFile is a written plan: the packs to fetch in order with every wanted
// ref, so a run needs neither the ref list nor the index view to plan.
//
// The file is planMagic and then a gzip stream of uvarints and
// length-prefixed byte strings:
//
//	version, inputs (32 bytes), then the input parts: count, then per part:
//	       name, sha256 (32 bytes)
//	counts: absent, deferred, shared-num, no-extent, ambiguous
//	packs: count, then per pack: id, size (0 for no entry), wants count,
//	       then per want: ref (BlockRef protobuf), extent (0 or 1), and
//	       off and size when extent is 1
//	multi: count, then per key: block key, candidate packs
//	dropped: count, then per id: id, reason, wants
//	totals: packs, wanted keys, bytes
//
// A want's block key is the key of its ref's hash, as the ref list gives it.
type planFile struct {
	// Inputs is the sha256 of planInputs; Parts its named parts, so a run
	// with the plan can name the inputs that differ.
	Inputs [32]byte
	Parts  []inputPart
	Counts planCounts
	// Packs are the packs in fetch order.
	Packs []*packWork
	// Multi is rescuer.multi after planning.
	Multi map[string]int
	// Dropped are the --packs ids not fetched, by id.
	Dropped []droppedPack
	// Wanted and Bytes are the plan's distinct wanted keys and planned
	// bytes; see planTotals.
	Wanted int
	Bytes  uint64
}

// planFlags returns the flags that shape a plan, as one line.
func planFlags(o *options, canSize bool) string {
	return fmt.Sprintf("min-wanted=%d order=%s order-overhead=%d max-packs=%d ranges=%t overhead=%d retry-failed=%t can-size=%t",
		o.MinWanted, o.Order, o.OrderOverhead, o.MaxPacks, o.Plan.Ranges, o.Plan.Overhead, o.RetryFailed, canSize)
}

// Names of a plan's input parts, in planInputs' order. A refusal names
// the parts that differ, never their values, so it holds no digit.
const (
	partFlags      = "flags"
	partRefs       = "refs"
	partPacks      = "packs"
	partWholePacks = "whole-packs"
	partIndex      = "index"
	partIndexTable = "index-table"
	partStoreID    = "store-id"
)

// inputPart is one named input of a plan and the sha256 of its bytes.
type inputPart struct {
	name string
	sum  [32]byte
}

// planInputs returns the sha256 over what a plan is made from, and its named
// parts: the flags, the ref list's bytes, the --packs ids sorted, the
// --whole-packs ids in order, the index's identity, and the table and store
// id its rows are read by. index is the identity runIndexIdentity read, once
// a run. The ledger is not hashed: a run with the plan subtracts what it
// finished.
func planInputs(o *options, canSize bool, index string) ([32]byte, []inputPart, error) {
	// Hash the flags and the ref list's bytes.
	var sum [32]byte
	parts := []inputPart{{name: partFlags, sum: sha256.Sum256([]byte(planFlags(o, canSize)))}}
	refs, err := fileSum(o.RefList)
	if err != nil {
		return sum, nil, err
	}
	parts = append(parts, inputPart{name: partRefs, sum: refs})

	// Hash the pack ids that bound and extend the plan.
	h := sha256.New()
	if o.Packs == nil {
		_, _ = io.WriteString(h, "-\n")
	} else {
		ids := slices.Sorted(maps.Keys(o.Packs))
		_, _ = fmt.Fprintf(h, "%d\n", len(ids))
		writeLines(h, ids)
	}
	parts = append(parts, inputPart{name: partPacks, sum: sumOf(h)})
	h = sha256.New()
	_, _ = fmt.Fprintf(h, "%d\n", len(o.WholePacks))
	writeLines(h, o.WholePacks)
	parts = append(parts, inputPart{name: partWholePacks, sum: sumOf(h)})

	// Hash the index's identity, then the table and the store id.
	parts = append(parts,
		inputPart{name: partIndex, sum: sha256.Sum256([]byte(index))},
		inputPart{name: partIndexTable, sum: sha256.Sum256([]byte(o.IndexTable))},
		inputPart{name: partStoreID, sum: sha256.Sum256([]byte(o.StoreID))},
	)

	// Hash the parts.
	h = sha256.New()
	_, _ = fmt.Fprintf(h, "swr-plan-inputs %d\n", planVersion)
	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%s %x\n", p.name, p.sum)
	}
	return sumOf(h), parts, nil
}

// sumOf returns h's sha256 as an array.
func sumOf(h hash.Hash) [32]byte {
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// partsDiffer returns the names of the parts that differ between a plan's
// and a run's, in the run's order, with any part only one of them has.
func partsDiffer(plan, now []inputPart) []string {
	// Index the plan's sums by part name.
	had := make(map[string][32]byte, len(plan))
	for _, p := range plan {
		had[p.name] = p.sum
	}

	// Name each of the run's parts the plan lacks or sums differently.
	var names []string
	seen := make(map[string]bool, len(now))
	for _, p := range now {
		seen[p.name] = true
		if sum, ok := had[p.name]; !ok || sum != p.sum {
			names = append(names, p.name)
		}
	}

	// Name each of the plan's parts the run lacks.
	for _, p := range plan {
		if !seen[p.name] {
			names = append(names, p.name)
		}
	}
	return names
}

// checkPlan reads the PlanIn plan and refuses it unless it was made from
// this run's inputs and its packs and dropped ids are the --packs set. It
// reads no index and dials nothing, so main runs it before the netrc read
// and the client, and the run again before it uses the plan. index is the
// run's index identity, the --index-id sidecar's. Its errors name what
// differs, never a value.
func checkPlan(o *options, canSize bool, index string) (*planFile, error) {
	// Read the plan and this run's inputs.
	pf, err := readPlanFile(o.PlanIn)
	if err != nil {
		return nil, err
	}
	inputs, parts, err := planInputs(o, canSize, index)
	if err != nil {
		return nil, err
	}

	// Name the parts that differ, the index apart.
	if names := partsDiffer(pf.Parts, parts); len(names) != 0 || inputs != pf.Inputs {
		if slices.Equal(names, []string{partIndex}) {
			return nil, errors.New("--index-id names another index than the plan was made from")
		}
		if len(names) == 0 {
			names = []string{"inputs"}
		}
		return nil, errors.Errorf("--plan-in was made from other inputs: %s differ", strings.Join(names, ", "))
	}
	if err := pf.checkPacks(o.Packs); err != nil {
		return nil, err
	}
	return pf, nil
}

// runIndexIdentity reads a run's index identity, once, before it plans: for
// a plan out the --index-id sidecar checked against the index file and its
// -wal, for a plan in the sidecar trusted, as the run reads no index, and
// otherwise the sidecar when there is one, else the index file's path, size
// and modification time and those of its -wal.
func runIndexIdentity(o *options) (string, error) {
	switch {
	case o.PlanOut != "":
		return checkIndexFiles(o.IndexPath, o.IndexID)
	case o.IndexID != "":
		return readIndexID(o.IndexID)
	default:
		return indexIdentity(o.IndexPath)
	}
}

// checkIndexFiles hashes the index file at path and its -wal, streaming,
// and returns the identity the --index-id sidecar at idPath names, refusing
// unless the files are what it names: a plan out holds the sidecar's
// identity, so it must be the index planned from. A missing -wal is one the
// sidecar names none of.
func checkIndexFiles(path, idPath string) (string, error) {
	// Read the sidecar and hash the files.
	if path == "" {
		return "", errors.New("--plan-out needs the index file to check --index-id against")
	}
	want, err := readIndexID(idPath)
	if err != nil {
		return "", err
	}
	db, err := fileSum(path)
	if err != nil {
		return "", err
	}
	wal := "-"
	walSum, err := fileSum(path + "-wal")
	switch {
	case err == nil:
		wal = hex.EncodeToString(walSum[:])
	case !os.IsNotExist(err):
		return "", err
	}

	// Name the file that differs.
	wantDB, wantWAL, _ := strings.Cut(want, "\n")
	if wantDB != "index-db "+hex.EncodeToString(db[:]) {
		return "", errors.New("--index-id does not match the index file")
	}
	if wantWAL != "index-wal "+wal+"\n" {
		return "", errors.New("--index-id does not match the index -wal")
	}
	return want, nil
}

// writeLines writes each string as a line.
func writeLines(h hash.Hash, lines []string) {
	for _, s := range lines {
		_, _ = io.WriteString(h, s+"\n")
	}
}

// fileSum returns the sha256 of a file's bytes.
func fileSum(path string) ([32]byte, error) {
	// Open the file.
	var sum [32]byte
	f, err := os.Open(path)
	if err != nil {
		return sum, err
	}
	defer f.Close()

	// Hash its bytes.
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return sum, errors.Wrap(err, path)
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// indexIdentity returns the index file's absolute path, size and
// modification time, and those of its -wal or "-" when it has none; "-" alone
// when there is no path.
func indexIdentity(path string) (string, error) {
	// Name no index when there is no path.
	if path == "" {
		return "index -\n", nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// Stat the file and its -wal.
	var out string
	for _, suffix := range []string{"", "-wal"} {
		info, err := os.Stat(abs + suffix)
		if suffix != "" && os.IsNotExist(err) {
			out += "index" + suffix + " -\n"
			continue
		}
		if err != nil {
			return "", err
		}
		out += fmt.Sprintf("index%s %s %d %d\n", suffix, abs, info.Size(), info.ModTime().UnixNano())
	}
	return out, nil
}

// indexIDMax bounds an --index-id sidecar's bytes.
const indexIDMax = 4 << 10

// readIndexID reads an --index-id sidecar and returns the index identity it
// gives, as "index-db <sha256>\nindex-wal <sha256 or ->\n".
//
// The sidecar is sha256sum's output for the index and its -wal, made where
// the index is, never by this command:
//
//	sha256sum view.db view.db-wal > view.db.id
//
// or for the index alone when it has no -wal, which reads as "-". Each
// non-blank line is a sha256 in hex, a space, and the file name, with
// sha256sum's "*" binary mark or leading "\" escape mark allowed. A name ending in
// -wal is the -wal's; the one other line is the index's. No other part of a
// name is read, so a byte-identical copy at another path, or a sidecar made
// on another host, gives the same identity. The sidecar is trusted: nothing
// here checks it against the files.
func readIndexID(path string) (string, error) {
	// Read the sidecar, bounded.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, indexIDMax+1))
	if err != nil {
		return "", errors.Wrap(err, path)
	}
	if len(raw) > indexIDMax {
		return "", errors.Errorf("--index-id %s: too long for a sidecar", path)
	}

	// Take the index's and the -wal's hash from their lines.
	db, wal := "", "-"
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		sum, name, _ := strings.Cut(strings.TrimPrefix(line, "\\"), " ")
		name = strings.TrimPrefix(strings.TrimPrefix(name, " "), "*")
		if _, err := hex.DecodeString(sum); err != nil || len(sum) != 2*sha256.Size || name == "" {
			return "", errors.Errorf("--index-id %s: a line is not \"<sha256> <file>\"", path)
		}
		sum = strings.ToLower(sum)
		switch {
		case strings.HasSuffix(name, "-wal") && wal == "-":
			wal = sum
		case !strings.HasSuffix(name, "-wal") && db == "":
			db = sum
		default:
			return "", errors.Errorf("--index-id %s: a second index or -wal line", path)
		}
	}
	if db == "" {
		return "", errors.Errorf("--index-id %s: no line for the index", path)
	}
	return "index-db " + db + "\nindex-wal " + wal + "\n", nil
}

// planTotals returns the distinct wanted keys of packs and their planned
// bytes, the cost choosePlan gives, request overhead included; a pack of
// unknown size, sized by SIZE when fetched, adds none.
func planTotals(packs []*packWork, o planOptions, multi map[string]int) (wanted int, bytes uint64) {
	seen := make(map[string]bool)
	for _, p := range packs {
		// Count each key once, a key of several candidates included.
		for _, w := range p.wants {
			if _, ok := multi[string(w.key)]; ok {
				if seen[string(w.key)] {
					continue
				}
				seen[string(w.key)] = true
			}
			wanted++
		}

		// Add the pack's planned cost.
		if _, cost, ok := choosePlan(p, o); ok {
			bytes += cost
		}
	}
	return wanted, bytes
}

// planLine returns the line printed before the first dial.
func planLine(packs []*packWork, o planOptions, multi map[string]int, dropped []droppedPack, inputs [32]byte) string {
	// Count the drops by reason.
	byReason := make(map[string]int, len(dropReasons))
	for _, d := range dropped {
		byReason[d.reason]++
	}

	// Format the totals.
	wanted, bytes := planTotals(packs, o, multi)
	return fmt.Sprintf("plan packs %d wanted %d bytes %d dropped %d (%s %d, %s %d, %s %d, %s %d) inputs %x\n",
		len(packs), wanted, bytes, len(dropped),
		dropNoWants, byReason[dropNoWants], dropNoEntry, byReason[dropNoEntry],
		dropMinWanted, byReason[dropMinWanted], dropMaxPacks, byReason[dropMaxPacks],
		inputs[:6])
}

// packsDiffer says how --packs differs from a plan's packs and dropped ids,
// by kind and with no count.
func packsDiffer(missing, extra bool) string {
	switch {
	case missing && extra:
		return "--packs differs from the plan: it names ids the plan neither planned nor dropped, and lacks plan ids"
	case missing:
		return "--packs differs from the plan: it names ids the plan neither planned nor dropped"
	default:
		return "--packs differs from the plan: it lacks plan ids"
	}
}

// checkPacks fails unless packs, when set, is the plan's packs and dropped
// ids together.
func (pf *planFile) checkPacks(packs map[string]bool) error {
	// Accept no --packs, which the inputs hash already matched.
	if packs == nil {
		return nil
	}

	// Compare the sets, naming no id.
	got := make(map[string]bool, len(pf.Packs)+len(pf.Dropped))
	for _, p := range pf.Packs {
		got[p.id] = true
	}
	for _, d := range pf.Dropped {
		got[d.id] = true
	}
	var missing, extra int
	for id := range packs {
		if !got[id] {
			missing++
		}
	}
	for id := range got {
		if !packs[id] {
			extra++
		}
	}
	if missing != 0 || extra != 0 {
		return errors.New(packsDiffer(missing != 0, extra != 0))
	}
	return nil
}

// writePlanFile writes the plan to path atomically: a temporary file in its
// directory, synced, renamed into place, and the directory synced.
func writePlanFile(path string, pf *planFile) error {
	// Write the plan to a temporary file.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	err = encodePlan(tmp, pf)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return errors.Wrap(err, "write plan "+path)
	}

	// Sync the directory so the rename is durable.
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	if cerr := d.Close(); err == nil {
		err = cerr
	}
	return err
}

// encodePlan writes the plan's bytes to w.
func encodePlan(w io.Writer, pf *planFile) error {
	// Write the magic, then the gzip stream through a buffer.
	if _, err := io.WriteString(w, planMagic); err != nil {
		return err
	}
	zw := gzip.NewWriter(w)
	bw := bufio.NewWriterSize(zw, 1<<20)
	pw := &planWriter{w: bw}

	// Write the header and the counts.
	pw.varint(planVersion)
	pw.raw(pf.Inputs[:])
	pw.count(len(pf.Parts))
	for _, p := range pf.Parts {
		pw.str(p.name)
		pw.raw(p.sum[:])
	}
	for _, n := range []int{pf.Counts.Absent, pf.Counts.Deferred, pf.Counts.SharedNum, pf.Counts.NoExtent, pf.Counts.Ambiguous} {
		pw.count(n)
	}

	// Write each pack with its wants.
	pw.count(len(pf.Packs))
	for _, p := range pf.Packs {
		pw.str(p.id)
		pw.varint(p.entry.GetSizeBytes())
		pw.count(len(p.wants))
		for _, w := range p.wants {
			ref, err := w.ref.MarshalVT()
			if err != nil {
				return err
			}
			pw.bytes(ref)
			if !w.extent {
				pw.varint(0)
				continue
			}
			pw.varint(1)
			pw.varint(w.off)
			pw.varint(uint64(w.size))
		}
	}

	// Write the shared-number keys, sorted, and the dropped ids.
	keys := slices.Sorted(maps.Keys(pf.Multi))
	pw.count(len(keys))
	for _, k := range keys {
		pw.str(k)
		pw.count(pf.Multi[k])
	}
	pw.count(len(pf.Dropped))
	for _, d := range pf.Dropped {
		pw.str(d.id)
		pw.str(d.reason)
		pw.count(d.wants)
	}

	// Write the totals and end the stream.
	pw.count(len(pf.Packs))
	pw.count(pf.Wanted)
	pw.varint(pf.Bytes)
	if pw.err != nil {
		return pw.err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return zw.Close()
}

// readPlanFile reads a plan written by writePlanFile.
func readPlanFile(path string) (*planFile, error) {
	// Open the file.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Decode the plan through a buffer.
	pf, err := decodePlan(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return nil, errors.Wrap(err, "read plan "+path)
	}
	return pf, nil
}

// decodePlan reads a plan's bytes, failing on a wrong magic or version, a
// field past its bounds, a torn stream or a bad gzip checksum.
func decodePlan(r io.Reader) (*planFile, error) {
	// Check the magic and open the gzip stream.
	magic := make([]byte, len(planMagic))
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != planMagic {
		return nil, errors.New("not a plan file")
	}
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, errTornPlan
	}
	defer zr.Close()
	pr := &planReader{r: bufio.NewReaderSize(zr, 1<<20)}

	// Read the header and the counts.
	if v := pr.varint(); pr.err == nil && v != planVersion {
		return nil, errors.New("plan format version is not this build's")
	}
	pf := &planFile{}
	pr.raw(pf.Inputs[:])
	n := pr.count()
	if n > planMaxParts {
		return nil, errors.New("plan holds too many input parts")
	}
	pf.Parts = make([]inputPart, n)
	for i := range pf.Parts {
		pf.Parts[i].name = string(pr.bytes())
		pr.raw(pf.Parts[i].sum[:])
	}
	pf.Counts = planCounts{Absent: pr.count(), Deferred: pr.count(), SharedNum: pr.count(), NoExtent: pr.count(), Ambiguous: pr.count()}

	// Read each pack with its wants, deriving each block key from its ref.
	n = pr.count()
	for i := 0; i < n && pr.err == nil; i++ {
		p := &packWork{id: string(pr.bytes())}
		if size := pr.varint(); size != 0 {
			p.entry = &packfile.PackfileEntry{Id: p.id, SizeBytes: size}
		}
		nw := pr.count()
		p.wants = make([]want, 0, min(nw, 1<<16))
		for j := 0; j < nw && pr.err == nil; j++ {
			raw := pr.bytes()
			if pr.err != nil {
				break
			}
			ref := &block.BlockRef{}
			if err := ref.UnmarshalVT(raw); err != nil || ref.GetHash() == nil {
				return nil, errors.New("plan holds a bad block ref")
			}
			w := want{key: packfile.BlockKey(ref.GetHash()), ref: ref}
			if pr.varint() == 1 {
				w.extent, w.off = true, pr.varint()
				size := pr.varint()
				if size > 1<<32-1 {
					return nil, errors.New("plan holds an extent past a pack's bound")
				}
				w.size = uint32(size) //nolint:gosec // bounded above.
			}
			p.wants = append(p.wants, w)
		}
		pf.Packs = append(pf.Packs, p)
	}

	// Read the shared-number keys and the dropped ids.
	nm := pr.count()
	pf.Multi = make(map[string]int, min(nm, 1<<16))
	for i := 0; i < nm && pr.err == nil; i++ {
		k := string(pr.bytes())
		pf.Multi[k] = pr.count()
	}
	nd := pr.count()
	for i := 0; i < nd && pr.err == nil; i++ {
		pf.Dropped = append(pf.Dropped, droppedPack{id: string(pr.bytes()), reason: string(pr.bytes()), wants: pr.count()})
	}

	// Read the totals, then the end of the stream, which checks its sum.
	packs := pr.count()
	pf.Wanted = pr.count()
	pf.Bytes = pr.varint()
	if pr.err != nil {
		return nil, pr.err
	}
	if packs != len(pf.Packs) {
		return nil, errors.New("plan totals do not match its packs")
	}
	if _, err := pr.r.ReadByte(); err != io.EOF {
		return nil, errTornPlan
	}
	return pf, nil
}

// errTornPlan is a plan file cut short or corrupt. It keeps no cause, as a
// decoder's cause can hold an offset, and a refusal holds no digit.
var errTornPlan = errors.New("torn or corrupt plan")

// planWriter writes a plan's fields, keeping the first error.
type planWriter struct {
	w   *bufio.Writer
	err error
	buf [binary.MaxVarintLen64]byte
}

// varint writes a uvarint.
func (pw *planWriter) varint(v uint64) {
	if pw.err != nil {
		return
	}
	n := binary.PutUvarint(pw.buf[:], v)
	_, pw.err = pw.w.Write(pw.buf[:n])
}

// count writes a count, never negative.
func (pw *planWriter) count(v int) {
	pw.varint(uint64(max(v, 0))) //nolint:gosec // not negative.
}

// raw writes bytes with no length.
func (pw *planWriter) raw(b []byte) {
	if pw.err != nil {
		return
	}
	_, pw.err = pw.w.Write(b)
}

// bytes writes a length and the bytes.
func (pw *planWriter) bytes(b []byte) {
	pw.count(len(b))
	pw.raw(b)
}

// str writes a string as bytes.
func (pw *planWriter) str(s string) {
	pw.count(len(s))
	if pw.err != nil {
		return
	}
	_, pw.err = pw.w.WriteString(s)
}

// planReader reads a plan's fields, keeping the first error; after one it
// returns zero values.
type planReader struct {
	r   *bufio.Reader
	err error
}

// varint reads a uvarint.
func (pr *planReader) varint() uint64 {
	if pr.err != nil {
		return 0
	}
	v, err := binary.ReadUvarint(pr.r)
	if err != nil {
		pr.err = errTornPlan
	}
	return v
}

// count reads a count of at most planMaxCount.
func (pr *planReader) count() int {
	v := pr.varint()
	if v > planMaxCount {
		pr.err = errors.New("plan count past its bound")
		return 0
	}
	return int(v) //nolint:gosec // bounded above.
}

// raw fills b.
func (pr *planReader) raw(b []byte) {
	if pr.err != nil {
		return
	}
	if _, err := io.ReadFull(pr.r, b); err != nil {
		pr.err = errTornPlan
	}
}

// bytes reads a length of at most planMaxField and the bytes.
func (pr *planReader) bytes() []byte {
	// Read the length and refuse one past the bound.
	n := pr.varint()
	if pr.err != nil {
		return nil
	}
	if n > planMaxField {
		pr.err = errors.New("plan field past its bound")
		return nil
	}

	// Read that many bytes.
	b := make([]byte, n)
	pr.raw(b)
	return b
}
