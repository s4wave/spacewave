# Journal.tla counterexamples for the hold-until-free rule

These are the two failures that decide the `HoldToFree` and `ChildRipe`
switches, written out one action per line. Block names follow the spec's
`Kids`: R1 -> C -> {E, F} -> G, R2 -> C2 -> F, and R3 -> C. Each block starts
with one holder. Writer `a` is the only writer. Each step names the action and
the state it leaves.

The checker gives each failure's length, 17 and 11 states, but no trace. The
steps below were worked by hand from the spec's actions to that length, so
the checker's own trace may order independent steps differently.

## Today's rule: NoLiveListsFreed fails in 17 states

Config `testdata/Journal-revive.cfg`, with `HoldToFree = FALSE` and `Grace = 1`.

1. Init: main names R1. R1, C, E, F and G are stored, and each has one holder.
2. Observe(a): a reads main's head, R1.
3. Commit(a, R2): a puts R2 and C2 and moves main R1 -> R2. The record holds
   F (for C2) and drops main's holder from R1.
4. Tick: the record ages to Grace.
5. Net(1): R1 nets to zero holders. F has two holders.
6. Cascade(R1): R1 is Dead and its release of C begins.
7. Release(C): C reaches zero.
8. Cascade(C): C is Dead and its release of E and F begins.
9. Release(E): E reaches zero.
10. Release(F): F has one holder, from C2.
11. Cascade(E): E is Dead, with no children. Nothing is due.
12. Delete(C): C is freed. R1 is still stored and its list still names C.
13. Observe(a): a reads main's head, R2.
14. HoldStored(a, R1): main moves R2 -> R1 without a put. R1 is stored, so
    the move lands. The record holds R1 and sets the hold mark.
15. Tick: the record ages to Grace.
16. Net(1): R1 gains a holder and leaves zero, but it stays released (in rel),
    so it is Holding. R2 nets to zero.
17. Cascade(R1): the re-hold makes R1 Live while its list names the freed C.
    NoLiveListsFreed fails, and main's reads of C fail.

Under `HoldToFree = TRUE` (`Journal-revive-hold.cfg`), step 6 does not happen.
C keeps R1's holder until R1 itself is freed, so step 14's move finds the
whole tree stored. That config passes.

## A third candidate, the variant with ChildRipe: NoFreedRead fails in 11 states

Config `testdata/Journal-lease-childripe.cfg`, with `HoldToFree = TRUE`,
`ChildRipe = TRUE` and `Grace = 2`. `ChildRipe` frees a child at its parent's
ripe point instead of the journal's end at the child's own zeroing.

1. Init: main names R1. R1, C, E, F and G are stored, and each has one holder.
2. Observe(a): a reads main's head, R1.
3. Commit(a, R2): a puts R2 and C2 and moves main R1 -> R2.
4. Tick: the record ages once.
5. Tick: the record ages to Grace.
6. Net(1): R1 nets to zero holders, with ripe 0. Under the rule C keeps R1's
   holder, so C is not at zero.
7. Observe(a): a reads main's head, R2.
8. StageWrite(a, C): a puts C and E again and moves the stage root `stg` onto
   C. C is not at zero, so the hold mark stays unset. The record's holder for C
   has not netted.
9. Delete(R1): R1 is freed, and its release of C begins.
10. Release(C): C loses R1's holder and reaches zero. Its only other holder
    sits in the record from step 8. ChildRipe gives C ripe 0.
11. Delete(C): C is freed while `stg` names it. NoFreedRead fails.

The same length fails with main instead of the stage. At step 8, Commit(a, R3)
puts R3 over C, puts C again, and moves main to R3.

Without ChildRipe, step 10 gives C the journal's end as its ripe sequence. C
then waits for step 8's record to net, and that record gives C its holder. On
master, netting does this in `writeNode`: a block whose holders fall to zero
from a nonzero count takes `GetNextSeq()` as its ripe sequence. Children that a
release moves go through the same path.
