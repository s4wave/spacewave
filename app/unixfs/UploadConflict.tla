---- MODULE UploadConflict ----
\* Files and folders dropped or picked into a folder, uploaded one commit
\* per item, and the question asked when a dropped name is already taken.
\*
\* A drop first lists its destination: a dropped name that holds a file asks
\* at once, showing the version a one byte read of that file returned, and a
\* name that holds a folder or link asks with no version; neither sends a
\* byte. Every other name is sent as missing. The store checks each file's
\* expectation inside its commit, so a name another writer took between the
\* listing and the commit refuses that send: with a version when it holds a
\* file, without one when it holds a folder or link. That race is the only
\* way a file streams before it asks.
\*
\* A dropped folder whose name holds a folder asks once whether to merge
\* into it; what the drop holds inside it waits on that answer, and its
\* files then ask as files do. Contents are the files dropped inside a
\* config's one folder: they wait while it asks, arrive as files once it is
\* answered, and end with it on Skip. Cancelling a folder's row cancels it
\* and everything inside it, whatever state each is in.
\* A dropped folder whose name holds a file or link offers only Keep both
\* and Skip; one sent as missing that another writer's file beat fails at
\* the store, which refuses a folder over a file, and offers Retry.
\*
\* The question names one item and its folder. Replace resends a file
\* expecting the version shown; Merge sends a folder into the folder of its
\* name; Keep both sends the item under the first free "name (n).ext", as
\* missing; Skip leaves the name alone. With several names of one drop
\* asking the same question, an answer can apply to all of them. A Replace the store refuses with a newer
\* version asks again showing it; one refused because the name no longer
\* holds a file is resent as missing, which writes it when the name is empty
\* and asks again when a folder or link took it. A failed send keeps its
\* answer and offers Retry. Cancel all stops every file still sending,
\* failed or asking. Each drop asks in turn: a second drop's names wait
\* until the first drop's questions are answered.

People == {"person"}

\* The dropped names, by drop. Drop2 arrives while Drop1 may be asking.
\* Folders are the dropped names that are folders; the rest are files.
\* Starts is what a name may hold before the drop, and Writes what another
\* writer may leave it holding.
CONSTANTS Drop1, Drop2, Folders, Contents, Starts, Writes
Names == Drop1 \cup Drop2
\* Top is the folder the Contents were dropped in, and Inside(f) what f holds.
Top == CHOOSE f \in Folders : TRUE
Inside(f) == IF f = Top THEN Contents ELSE {}

\* What a name holds in the folder: nothing, another writer's file at version
\* "a" or "b", the dropped file once written ("m"), or a folder or link.
Files == {"a", "b", "m"}
Holds == {"none", "dir"} \cup Files

Answers == {"replace", "merge", "keep", "skip"}
AllOf == [a \in Answers |->
           CASE a = "replace" -> "replaceall"
             [] a = "merge" -> "mergeall"
             [] a = "keep" -> "keepall"
             [] a = "skip" -> "skipall"]
Buttons == Answers \cup {AllOf[a] : a \in Answers}
             \cup {"retry", "cancel", "cancelfolder"}

VARIABLES
  disk,     \* what each name holds in the folder
  item,     \* each name's upload: out, waiting, sending, asking, failed, done,
            \* skipped, cancelled
  expect,   \* what a send expects the name to hold: missing, a version,
            \* merge into a folder, or keep
  shown,    \* the version a question shows, or "dir" for a folder or link
  answer,   \* the person's last answer: none, merge, keep, skip, or the
            \* version Replace named
  raced,    \* another writer changed the name since its drop
  clobbered, \* history: a write replaced a file without a Replace naming it
  merged,   \* history: a folder merged into one held at the drop, unasked
  wasted    \* history: a file streamed into a refusal no other writer caused
vars == <<disk, item, expect, shown, answer, raced, clobbered, merged, wasted>>

DropOf(n) == IF n \in Drop1 THEN Drop1 ELSE Drop2
Asking(d) == {n \in d : item[n] = "asking"}
\* The question shows the first drop with a name asking, and its first name.
QDrop == IF Asking(Drop1) # {} THEN Drop1 ELSE Drop2
Asked == Asking(QDrop) # {}
Current == CHOOSE n \in Asking(QDrop) : TRUE
\* The answers a question offers, from the item's kind and what holds its
\* name: Replace for a file over a file, Merge for a folder over a folder.
Offers(n) ==
  {"keep", "skip"}
    \cup (IF n \notin Folders /\ shown[n] \in Files THEN {"replace"} ELSE {})
    \cup (IF n \in Folders /\ shown[n] = "dir" THEN {"merge"} ELSE {})
\* The names of the question's drop asking the same question.
Alike == {n \in Asking(QDrop) : Offers(n) = Offers(Current)}
Others == Alike \ {Current}
Busy == {n \in Names : item[n] \in {"waiting", "sending", "asking", "failed"}}

Shown(p) ==
  (IF Asked THEN Offers(Current) ELSE {})
    \cup (IF Asked /\ Others # {} THEN {AllOf[a] : a \in Offers(Current)}
          ELSE {})
    \cup (IF \E n \in Names : item[n] = "failed" THEN {"retry"} ELSE {})
    \cup (IF Busy # {} THEN {"cancel"} ELSE {})
    \cup (IF Folders \cap Busy # {} THEN {"cancelfolder"} ELSE {})

\* What the store can do with an answer for the asking name n: Replace a
\* file it shows, Merge a folder into a folder.
Works(n, a) ==
  \/ a \in {"keep", "skip"}
  \/ a = "replace" /\ n \notin Folders /\ shown[n] \in Files
  \/ a = "merge" /\ n \in Folders /\ shown[n] = "dir"

\* What the manager accepts.
Allowed(p, b) ==
  \/ \E a \in Answers : b = a /\ Asked /\ Works(Current, a)
  \/ \E a \in Answers : b = AllOf[a] /\ Asked /\ Others # {}
                         /\ \A n \in Alike : Works(n, a)
  \/ b = "retry" /\ \E n \in Names : item[n] = "failed"
  \/ b = "cancel" /\ Busy # {}
  \/ b = "cancelfolder" /\ Folders \cap Busy # {}

\* What the screen says with nothing to press.
Hint(p) ==
  CASE \E n \in Names : item[n] = "out" -> "Waits for a drop"
    [] OTHER -> "Lists each upload as complete or skipped"

\* The preflight: a taken name asks, any other is sent as missing. A
\* folder's shown is "dir" over a folder, and a file's version over a file.
Arrive(n) ==
  IF disk[n] = "none" THEN <<"sending", "missing", "none">>
  ELSE <<"asking", "none", IF disk[n] \in Files THEN disk[n] ELSE "dir">>
\* A content waits while its folder asks, and otherwise arrives with it.
Start(n) ==
  IF n \in Contents /\ Arrive(Top)[1] = "asking" THEN <<"waiting", "none", "none">>
  ELSE Arrive(n)

Drop(d) ==
  /\ d # {}
  /\ \A n \in d : item[n] = "out"
  /\ d = Drop1 \/ \A n \in Drop1 : item[n] # "out"
  /\ item' = [n \in Names |-> IF n \in d THEN Start(n)[1] ELSE item[n]]
  /\ expect' = [n \in Names |-> IF n \in d THEN Start(n)[2] ELSE expect[n]]
  /\ shown' = [n \in Names |-> IF n \in d THEN Start(n)[3] ELSE shown[n]]
  /\ UNCHANGED <<disk, answer, raced, clobbered, merged, wasted>>

\* Another writer creates, changes, replaces or removes a name.
OtherWrite(n) ==
  /\ \E h \in Writes \ {disk[n]} : disk' = [disk EXCEPT ![n] = h]
  /\ raced' = [raced EXCEPT ![n] = TRUE]
  /\ UNCHANGED <<item, expect, shown, answer, clobbered, merged, wasted>>

\* The store's check inside the commit. A folder is created, or merges into
\* the folder of its name, and never lands on a file.
Accepts(n) ==
  \/ expect[n] = "keep"
  \/ expect[n] = "missing" /\ disk[n] = "none"
  \/ expect[n] \in Files /\ disk[n] = expect[n]
  \/ n \in Folders /\ expect[n] \in {"missing", "merge"}
     /\ disk[n] \in {"none", "dir"}

Land(n) ==
  /\ item[n] = "sending"
  /\ Accepts(n)
  /\ clobbered' = (clobbered \/ (expect[n] # "keep" /\ disk[n] \in Files
                                 /\ answer[n] # disk[n]))
  /\ merged' = (merged \/ (n \in Folders /\ disk[n] = "dir" /\ ~raced[n]
                           /\ answer[n] # "merge" /\ expect[n] # "keep"))
  /\ disk' = CASE expect[n] = "keep" -> disk
              [] n \in Folders -> [disk EXCEPT ![n] = "dir"]
              [] OTHER -> [disk EXCEPT ![n] = "m"]
  /\ item' = [item EXCEPT ![n] = "done"]
  /\ expect' = [expect EXCEPT ![n] = "none"]
  /\ UNCHANGED <<shown, answer, raced, wasted>>

\* The store refuses a folder over a file as an error.
Refuse(n) ==
  /\ item[n] = "sending"
  /\ n \notin Folders
  /\ ~Accepts(n)
  /\ wasted' = (wasted \/ (expect[n] = "missing" /\ ~raced[n]))
  /\ IF disk[n] \in Files \/ expect[n] = "missing"
       THEN /\ item' = [item EXCEPT ![n] = "asking"]
            /\ expect' = [expect EXCEPT ![n] = "none"]
            /\ shown' = [shown EXCEPT ![n] =
                          IF disk[n] \in Files THEN disk[n] ELSE "dir"]
       \* A Replace whose file is gone: no version came back; send as missing.
       ELSE /\ expect' = [expect EXCEPT ![n] = "missing"]
            /\ UNCHANGED <<item, shown>>
  /\ UNCHANGED <<disk, answer, raced, clobbered, merged>>

Fail(n) ==
  /\ item[n] = "sending"
  /\ item' = [item EXCEPT ![n] = "failed"]
  /\ UNCHANGED <<disk, expect, shown, answer, raced, clobbered, merged, wasted>>

Retry(p) ==
  /\ "retry" \in Shown(p)
  /\ \E n \in Names :
       /\ item[n] = "failed"
       /\ item' = [item EXCEPT ![n] = "sending"]
  /\ UNCHANGED <<disk, expect, shown, answer, raced, clobbered, merged, wasted>>

\* An answer to the asking names ns. What waits inside an answered folder
\* arrives as files do, or is skipped with it.
Freed(ns, n) == n \in Contents /\ item[n] = "waiting" /\ Top \in ns
Answer(ns, a) ==
  /\ item' = [n \in Names |->
               IF Freed(ns, n) THEN (IF a = "skip" THEN "skipped" ELSE Arrive(n)[1])
               ELSE IF n \notin ns THEN item[n]
               ELSE IF a = "skip" THEN "skipped" ELSE "sending"]
  /\ expect' = [n \in Names |->
                 IF Freed(ns, n) THEN (IF a = "skip" THEN "none" ELSE Arrive(n)[2])
                 ELSE IF n \notin ns THEN expect[n]
                 ELSE IF a = "replace" THEN shown[n]
                 ELSE IF a \in {"keep", "merge"} THEN a ELSE "none"]
  /\ answer' = [n \in Names |->
                 IF Freed(ns, n) THEN (IF a = "skip" THEN "skip" ELSE answer[n])
                 ELSE IF n \notin ns THEN answer[n]
                 ELSE IF a = "replace" THEN shown[n] ELSE a]
  /\ shown' = [n \in Names |->
                 IF Freed(ns, n) THEN (IF a = "skip" THEN "none" ELSE Arrive(n)[3])
                 ELSE IF n \in ns THEN "none" ELSE shown[n]]
  /\ UNCHANGED <<disk, raced, clobbered, merged, wasted>>

Press(p, b) ==
  /\ b \in Shown(p)
  /\ \E a \in Answers :
       \/ b = a /\ Answer({Current}, a)
       \/ b = AllOf[a] /\ Answer(Alike, a)

CancelAll(p) ==
  /\ "cancel" \in Shown(p)
  /\ item' = [n \in Names |-> IF n \in Busy THEN "cancelled" ELSE item[n]]
  /\ expect' = [n \in Names |-> IF n \in Busy THEN "none" ELSE expect[n]]
  /\ shown' = [n \in Names |-> IF n \in Busy THEN "none" ELSE shown[n]]
  /\ UNCHANGED <<disk, answer, raced, clobbered, merged, wasted>>

\* Cancelling a folder's row cancels it and everything inside it.
CancelFolder(p) ==
  /\ "cancelfolder" \in Shown(p)
  /\ \E f \in Folders \cap Busy :
       LET gone == ({f} \cup Inside(f)) \cap Busy
       IN /\ item' = [n \in Names |-> IF n \in gone THEN "cancelled" ELSE item[n]]
          /\ expect' = [n \in Names |-> IF n \in gone THEN "none" ELSE expect[n]]
          /\ shown' = [n \in Names |-> IF n \in gone THEN "none" ELSE shown[n]]
  /\ UNCHANGED <<disk, answer, raced, clobbered, merged, wasted>>

Init ==
  /\ disk \in [Names -> Starts]
  \* A content's name holds something only inside a folder that exists.
  /\ \A n \in Contents : disk[Top] # "dir" => disk[n] = "none"
  /\ item = [n \in Names |-> "out"]
  /\ expect = [n \in Names |-> "none"]
  /\ shown = [n \in Names |-> "none"]
  /\ answer = [n \in Names |-> "none"]
  /\ raced = [n \in Names |-> FALSE]
  /\ clobbered = FALSE
  /\ merged = FALSE
  /\ wasted = FALSE

Next ==
  \/ Drop(Drop1) \/ Drop(Drop2)
  \/ \E n \in Names : OtherWrite(n) \/ Land(n) \/ Refuse(n) \/ Fail(n)
  \/ \E p \in People :
       Retry(p) \/ CancelAll(p) \/ CancelFolder(p)
       \/ \E b \in Buttons \ {"retry", "cancel", "cancelfolder"} : Press(p, b)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* No upload replaces a file unless the person pressed Replace on a question
\* showing that file's version.
NoReplaceWithoutAnswer == ~clobbered
\* A dropped file ends skipped only by the person's Skip.
NothingDiscarded == \A n \in Names : item[n] = "skipped" => answer[n] = "skip"
\* A name taken at the drop asks before any byte is sent.
AskBeforeSending == ~wasted
\* No dropped folder merges into a folder its name held at the drop unless
\* the person pressed Merge.
NoMergeWithoutAnswer == ~merged
\* What a folder holds waits only while the folder asks.
NothingWaitsOnAnEndedFolder ==
  \A n \in Contents : item[n] = "waiting" => item[Top] = "asking"

TypeOK ==
  /\ disk \in [Names -> Holds]
  /\ item \in [Names -> {"out", "waiting", "sending", "asking", "failed", "done",
                         "skipped", "cancelled"}]
  /\ expect \in [Names -> {"none", "missing", "merge", "keep"} \cup Files]
  /\ shown \in [Names -> {"none", "dir"} \cup Files]
  /\ answer \in [Names -> {"none", "merge", "keep", "skip"} \cup Files]
  /\ raced \in [Names -> BOOLEAN]
  /\ clobbered \in BOOLEAN
  /\ merged \in BOOLEAN
  /\ wasted \in BOOLEAN
====
