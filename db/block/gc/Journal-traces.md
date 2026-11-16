# Journal.tla counterexamples for the hold-until-free rule

These are the two failures that decide the `HoldToFree` and `ChildRipe`
switches, as the checker reported them, one action per line. Block names
follow the spec's `Kids`: R1 -> C -> {E, F} -> G, R2 -> C2 -> F, and R3 -> C.
Each block starts with one holder. Writer `a` is the only writer. Each step
names the action and the state it leaves.

## Today's rule: NoLiveListsFreed fails in 17 states

Config `testdata/Journal-revive.cfg`, with `HoldToFree = FALSE` and `Grace = 1`.

1. Init: main names R1. R1, C, E, F and G are stored.
2. Observe(a): a reads main's head, R1.
3. Commit(a, R2): a puts R2 and C2 and moves main R1 -> R2. The record holds
   F for C2 and drops main's holder from R1.
4. Observe(a): a reads main's head, R2.
5. Commit(a, R1): a puts R1, C and E again and moves main R2 -> R1. The record
   holds R1 and drops main's holder from R2.
6. Observe(a): a reads main's head, R1.
7. Tick: both records age to Grace.
8. Net: both records net. R2 reaches zero, and F has two holders.
9. Cascade(R2): R2 is Dead, and its release of C2 begins.
10. Release(C2): C2 reaches zero.
11. Cascade(C2): C2 is Dead, and its release of F begins.
12. Release(F): F has one holder, from C.
13. Delete(C2): C2 is freed. R2 is still stored, and its list still names C2.
14. HoldStored(a, R2): main moves R1 -> R2 without a put. R2 is stored, so
    the move lands. The record holds R2 and sets the hold mark.
15. Tick: the record ages to Grace.
16. Net: R2 gains a holder and leaves zero. It is still released, so it is
    Holding. R1 reaches zero.
17. Cascade(R2): the re-hold makes R2 Live, but its list names the freed C2.
    NoLiveListsFreed fails, and main's reads of C2 fail.

Under `HoldToFree = TRUE` (`Journal-revive-hold.cfg`), step 9 does not happen.
C2 keeps R2's holder until R2 itself is freed, so step 14's move finds the
whole tree stored. That config passes.

## A third candidate, the variant with ChildRipe: NoFreedRead fails in 11 states

Config `testdata/Journal-lease-childripe.cfg`, with `HoldToFree = TRUE`,
`ChildRipe = TRUE`, `TailCheck = FALSE` and `Grace = 2`. ChildRipe frees a
child at its parent's ripe point. The variant without it gives the child the
journal's end at the child's own zeroing. The tail check alone also keeps the
child, since the unnetted root move names it.

1. Init: main names R1. R1, C, E, F and G are stored.
2. Observe(a): a reads main's head, R1.
3. Commit(a, R2): a puts R2 and C2 and moves main R1 -> R2.
4. Observe(a): a reads main's head, R2.
5. Tick: the record ages once.
6. Tick: the record ages to Grace.
7. Net: R1 reaches zero, with ripe 0. Under the rule C keeps R1's holder, so C
   is not at zero.
8. Commit(a, R3): a puts R3 and puts C and E again, and moves main R2 -> R3.
   C is not at zero, so the hold mark stays unset. The record's holder for C,
   from R3, has not netted.
9. Delete(R1): R1 is freed, and its release of C begins.
10. Release(C): C loses R1's holder and reaches zero. ChildRipe gives C ripe 0.
11. Delete(C): C is freed while main reaches it through R3. NoFreedRead fails.

Without ChildRipe, step 10 gives C the journal's end as its ripe sequence. C
then waits until step 8's record nets, and that record gives C its holder from
R3. Netting on master does the same. In `writeNode`, a block whose holders
fall to zero from a nonzero count takes `GetNextSeq()` as its ripe sequence,
and the children a release moves are written through the same path.
