---
title: World Writes and Reads
section: objects
order: 2
summary: Write World operations that replay alike on every replica, rehearse them before the writer slot, check them in a test, and look objects up through an index object.
---

A Space World has one writer at a time, and every replica replays the same
operations to reach the same World. Four rules keep writes fast and replicas
in agreement, and each has a helper or a lint that checks it.

## An operation carries its inputs

A replica that replays your operation runs its `ApplyWorldOp` again, later and
on another machine. Anything the apply reads from outside the World, such as
the clock, a random source or the network, comes out differently there, and
the replica's World no longer matches yours. So the operation carries every
input: the caller sets the timestamp and any generated identity when it builds
the operation, and the apply reads only the operation and the World.

```go
// NewSetTitleOp builds the operation, taking the time from the caller.
func NewSetTitleOp(objKey, title string, ts time.Time) *SetTitleOp {
	return &SetTitleOp{
		ObjectKey: objKey,
		Title:     title,
		Timestamp: timestamp.New(ts),
	}
}

// ApplyWorldOp reads only the operation and the World.
func (o *SetTitleOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	_, _, err = world.AccessWorldObject(ctx, ws, o.GetObjectKey(), true, func(bcs *block.Cursor) error {
		return writeTitle(ctx, bcs, o.GetTitle(), o.GetTimestamp())
	})
	return false, err
}
```

Work outside the World, such as fetching a file, belongs before the operation:
fetch it, write its blocks, and give the operation their refs.

## Rehearse before the writer slot

Every block the apply reads from disk while it holds the writer slot delays
every other write. `world.ApplyRehearsed` applies the operation first to a
rehearsal, a fork of the newest World that takes no lock and never commits,
which loads the blocks the real apply reads. Then it applies the operation
under the slot, where those blocks are already in memory:

```go
_, _, err := world.ApplyRehearsed(ctx, ws, op, sender)
```

A rehearsal that rejects the operation, such as one naming a missing object,
returns that rejection without taking the slot. The locked apply checks the
operation again on the head it starts from, and its result is the one that
counts. Because a rehearsed operation applies twice, use `ApplyRehearsed` only
for operations whose apply has no effect outside the World.

## Check an operation in a test

`world_opcheck.Check` runs an operation through validation, a rehearsal, the
locked apply and a replay of its decoded form on a second replica a moment
later. It fails when the runs disagree, when the replayed World differs from
the applied one, or when the apply calls the network through the HTTP
transport, which fails every call while the check runs:

```go
func TestSetTitleConforms(t *testing.T) {
	ts := time.Unix(1_800_000_000, 0)
	world_opcheck.Check(t, world_opcheck.Case{
		Setup: func(ctx context.Context, ws world.WorldState) error {
			_, _, err := ws.ApplyWorldOp(ctx, NewCreateNoteOp("note/a", ts), "")
			return err
		},
		Op:       NewSetTitleOp("note/a", "Groceries", ts),
		LookupOp: lookupNoteOp,
	})
}
```

Run it for each operation type you register. An apply that reads the clock
fails it on the World root, and one that calls the network fails it on the
call.

## Look objects up through an index object

A graph lookup is indexed by its subject or its object. A lookup that fixes
only the predicate scans every quad with that predicate in the World, so it
slows down as the World grows. Instead, link each member to an index object
and look up by the index object:

```go
// Link the note to the notes index when the operation creates it.
err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("notes", "<note>", noteKey, ""))

// List the notes by the index object.
quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("notes", "<note>", "", ""), 0)
```

Object types work this way already: `world_types.ListObjectsWithType` reads
the quads that point at the type's object. The repository lint's `quadscan`
check flags a quad that fixes only the predicate outside tests, whether you
build it with `world.NewGraphQuad` directly or through a helper that forwards
its arguments.
