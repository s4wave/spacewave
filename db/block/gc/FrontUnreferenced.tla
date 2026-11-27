---- MODULE FrontUnreferenced ----
\* While Open, the front keeps an entry for a block the store lacks, with
\* neither a list nor a hole record, for as long as a root or a stored list
\* that holds its children references it. Such a block's count includes
\* the implied holder its birth would take back, so once its last parent
\* releases, it reads one holder for good and the proof at zero never pops
\* it. One real holder kept after a loss reads the same, so the count
\* cannot tell the two apart.
\*
\* The collector settles such an entry with a reference scan
\* (front-unreferenced.go). It marks the entry's front key with a token of
\* its own, reads every root and every stored list once, in snapshots that
\* other writers interleave with, and pops the entry when the scan found no
\* holding list that references the block and its mark still stands.
\* Records.addFront writes the front key without a mark, so any push after
\* the mark refuses the pop, and so does the mark of another collector on
\* the same store. Each producer of such an entry pushes the block in the
\* transaction that makes a list hold it: the walk, when it verifies a list
\* that references the block, the seeding of a root or an unnetted down
\* page, the unpark of a released parent, and a readoption, which gives
\* every released list its holds back.
\*
\* The model follows one such block. Lists are the roots and the lists
\* that can reference it. A list in holds is stored and holds its children.
\* One in kept is parked and its park still holds them, and one in parked is
\* stored and its park released them. A list in neither is not stored, or
\* is an old list the walk did not verify yet. The count is not modeled: it
\* chooses which entries the collector marks, and safety does not rest on
\* it. Neither are the pop's gates on the adoption mark, an owed recount
\* and a freeable Volume, which are the front's own: the pop is safe here
\* without them.
\*
\* A list in claim holds by a writer's own claim: a Land stored it from
\* content, or put again a released parent and kept its marked list. Such a
\* Land pushes no absent child, with or without the scan, so the front
\* never promised an entry for it. The invariants below therefore speak of
\* the other holders, the old ones. A claimed parent the scan reads keeps
\* the entry. One that lands behind the scan's position does not, and the
\* state after that pop is the one a Land after the pop reaches.
\*
\* A second collector on the same store is modeled by its mark alone. Its
\* own pop obeys the same rules, so from here it only takes the entry away.
\* A block the store loses again after its birth is not modeled.

EXTENDS FiniteSets

CONSTANTS Lists, Skip
VARIABLE s

\* The lists that hold the block for the old generation.
Old(t) == (t.holds \cup t.kept) \ t.claim

TypeOK ==
  /\ s.holds \subseteq Lists
  /\ s.kept \subseteq Lists
  /\ s.parked \subseteq Lists
  /\ s.holds \cap s.kept = {}
  /\ (s.holds \cup s.kept) \cap s.parked = {}
  /\ s.claim \subseteq s.holds \cup s.kept
  /\ s.born \in BOOLEAN
  /\ s.front \in BOOLEAN
  /\ s.mark \in {"none", "mine", "other"}
  /\ s.pc \in {"idle", "scan", "done"}
  /\ s.seen \subseteq Lists
  /\ s.referenced \in BOOLEAN
  /\ s.unsafe \in BOOLEAN

Init ==
  s = [holds |-> {}, kept |-> {}, parked |-> {}, claim |-> {},
       born |-> FALSE, front |-> FALSE, mark |-> "none", pc |-> "idle",
       seen |-> {}, referenced |-> FALSE, unsafe |-> FALSE]

\* Records.addFront writes the front key with an empty value, which
\* deletes any mark.
Pushed(writer, t) ==
  IF Skip = writer THEN t
  ELSE [t EXCEPT !.front = TRUE,
                 !.mark = IF Skip = "push-mark" THEN @ ELSE "none"]

\* The walk verifies a list that references the block, or the seeding
\* reads a root or a down page that does.
Verify(l) ==
  /\ l \notin s.holds \cup s.kept \cup s.parked
  /\ s' = Pushed("verify", [s EXCEPT !.holds = @ \cup {l}])

\* A Land stores a parent from content, or puts a released parent again
\* and keeps its marked list. It pushes no absent child.
Land(l) ==
  /\ l \notin s.holds \cup s.kept
  /\ s' = [s EXCEPT !.parked = @ \ {l}, !.holds = @ \cup {l},
                    !.claim = @ \cup {l}]

\* The free parks a list at zero, and its park still holds its children.
Park(l) ==
  /\ l \in s.holds
  /\ s' = [s EXCEPT !.holds = @ \ {l}, !.kept = @ \cup {l}]

\* A park that kept its holds ends when the list gains a holder.
Unkeep(l) ==
  /\ l \in s.kept
  /\ s' = [s EXCEPT !.kept = @ \ {l}, !.holds = @ \cup {l}]

\* The park gives the list's children their downs.
Release(l) ==
  /\ l \in s.kept
  /\ s' = [s EXCEPT !.kept = @ \ {l}, !.parked = @ \cup {l},
                    !.claim = @ \ {l}]

\* A released park gains a holder: its list holds again.
Unpark(l) ==
  /\ l \in s.parked
  /\ s' = Pushed("unpark", [s EXCEPT !.parked = @ \ {l}, !.holds = @ \cup {l}])

\* A readoption deletes every parked record and counts every stored list.
Recount ==
  /\ s.parked # {}
  /\ s' = Pushed("recount", [s EXCEPT !.holds = @ \cup s.parked, !.parked = {}])

\* The sweep deletes a parked list.
Sweep(l) ==
  /\ l \in s.parked
  /\ s' = [s EXCEPT !.parked = @ \ {l}]

\* A put stores the block. Whether it pushes the block is not relied on.
Birth ==
  /\ ~s.born
  /\ s' = [s EXCEPT !.born = TRUE]

\* The walk decodes the stored block, which takes its entry off the front
\* and pushes its children.
Decode ==
  /\ s.born /\ s.front
  /\ s' = [s EXCEPT !.front = FALSE, !.mark = "none"]

\* The collector marks the entry with its token and starts a scan. A scan
\* a restart cut short leaves its mark, which the next scan writes again,
\* with its own token, before it reads.
Mark ==
  /\ s.pc = "idle" /\ s.front
  /\ s' = [s EXCEPT !.mark = "mine", !.pc = "scan", !.seen = {},
                    !.referenced = FALSE]

\* Another collector on the same store marks the entry with its token.
OtherMark ==
  /\ s.front
  /\ s' = [s EXCEPT !.mark = "other"]

\* One slice reads a root or list. A list holds unless its park released
\* its children.
Scan(l) ==
  /\ s.pc = "scan" /\ l \notin s.seen
  /\ s' = [s EXCEPT !.seen = @ \cup {l},
                    !.referenced = @ \/ l \in s.holds
                                     \/ (Skip # "scan-kept" /\ l \in s.kept)]

Finish ==
  /\ s.pc = "scan"
  /\ s.seen = Lists \/ Skip = "scan-short"
  /\ s' = [s EXCEPT !.pc = "done"]

\* The pop rechecks, in its transaction, the entry, its own mark and that
\* the block is still not stored.
Pop ==
  /\ s.pc = "done"
  /\ IF /\ ~s.referenced /\ s.front
        /\ \/ s.mark = "mine"
           \/ Skip = "pop-token" /\ s.mark = "other"
           \/ Skip = "pop-mark"
        /\ ~s.born \/ Skip = "pop-born"
     THEN s' = [s EXCEPT !.pc = "idle", !.front = FALSE, !.mark = "none",
                         !.unsafe = Old(s) # {} \/ s.born]
     ELSE s' = [s EXCEPT !.pc = "idle"]

\* A restart drops the scan the collector kept in memory.
Restart == s' = [s EXCEPT !.pc = "idle"]

Next ==
  \/ \E l \in Lists : \/ Verify(l) \/ Land(l) \/ Park(l) \/ Unkeep(l)
                      \/ Release(l) \/ Unpark(l) \/ Sweep(l) \/ Scan(l)
  \/ Recount \/ Birth \/ Decode \/ Mark \/ OtherMark \/ Finish \/ Pop
  \/ Restart

Spec == Init /\ [][Next]_s

\* An old holder references the block only while the block is on the
\* front, until the block is stored.
FrontComplete == (~s.born /\ Old(s) # {}) => s.front

\* No pop takes the entry while an old holder references the block, or
\* once the block is stored and the walk owes it a decode.
NoUnsafePop == ~s.unsafe

====
