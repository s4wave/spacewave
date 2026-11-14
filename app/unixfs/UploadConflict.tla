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
\* into it; its files then ask as files do. A folder is sent alone, and what
\* the drop holds inside it waits until the folder commits: while it asks,
\* while it is created, and while its send has failed. Contents are the
\* files dropped inside a config's one folder: they arrive once it lands, as
\* missing in a folder the drop created and from the listing in one it
\* merged into, and end with it on Skip. Cancelling a folder's row cancels it
\* and everything inside it, whatever state each is in; a folder that
\* already landed stays, with whatever of its contents landed.
\* A dropped folder whose name holds a file or link offers only Keep both
\* and Skip. A new folder is sent expecting nothing at its name, so the
\* store refuses it when another writer took that name since the listing,
\* and it asks like a file. Merge sends a folder with no expectation; the
\* store refuses a folder over a file as an error, which offers Retry.
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
  item,     \* each name's upload: out, waiting (a content whose folder has
            \* not landed), sending, asking, failed, done, skipped, cancelled
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
\* A content waits until its folder lands.
Start(n) ==
  IF n \in Contents THEN <<"waiting", "none", "none">>
  ELSE Arrive(n)
\* A content arrives when its folder f lands: as missing in a folder the drop
\* created, and from the listing in a folder it merged into.
Opened(f, n) == n \in Contents /\ f \in Folders /\ f = Top /\ item[n] = "waiting"
Into(f, n) == IF expect[f] = "merge" THEN Arrive(n) ELSE <<"sending", "missing", "none">>
\* A Keep both folder is new, so nothing is inside it yet.
Fresh(f) == IF f \in Folders /\ expect[f] = "keep" THEN Inside(f) ELSE {}

Drop(d) ==
  /\ d # {}
  /\ \A n \in d : item[n] = "out"
  /\ d = Drop1 \/ \A n \in Drop1 : item[n] # "out"
  /\ item' = [n \in Names |-> IF n \in d THEN Start(n)[1] ELSE item[n]]
  /\ expect' = [n \in Names |-> IF n \in d THEN Start(n)[2] ELSE expect[n]]
  /\ shown' = [n \in Names |-> IF n \in d THEN Start(n)[3] ELSE shown[n]]
  /\ UNCHANGED <<disk, answer, raced, clobbered, merged, wasted>>

\* Another writer creates, changes, replaces or removes a name; a folder
\* that stops being one takes what it held with it.
Emptied(n, h) == IF n \in Folders /\ h # "dir" THEN Inside(n) ELSE {}
OtherWrite(n) ==
  /\ \E h \in Writes \ {disk[n]} :
       disk' = [m \in Names |-> IF m = n THEN h
                 ELSE IF m \in Emptied(n, h) THEN "none" ELSE disk[m]]
  /\ raced' = [raced EXCEPT ![n] = TRUE]
  /\ UNCHANGED <<item, expect, shown, answer, clobbered, merged, wasted>>

\* The store's check inside the commit. A folder sent as missing is created
\* only where nothing is; one sent to merge is created, or merges into the
\* folder of its name; neither lands on a file.
Accepts(n) ==
  \/ expect[n] = "keep"
  \/ expect[n] = "missing" /\ disk[n] = "none"
  \/ expect[n] \in Files /\ disk[n] = expect[n]
  \/ n \in Folders /\ expect[n] = "merge" /\ disk[n] \in {"none", "dir"}

Land(n) ==
  /\ item[n] = "sending"
  /\ Accepts(n)
  /\ clobbered' = (clobbered \/ (expect[n] # "keep" /\ disk[n] \in Files
                                 /\ answer[n] # disk[n]))
  /\ merged' = (merged \/ (n \in Folders /\ disk[n] = "dir"
                           /\ answer[n] # "merge" /\ expect[n] # "keep"))
  /\ disk' = CASE expect[n] = "keep" ->
                [m \in Names |-> IF m \in Fresh(n) THEN "none" ELSE disk[m]]
              [] n \in Folders -> [disk EXCEPT ![n] = "dir"]
              [] OTHER -> [disk EXCEPT ![n] = "m"]
  /\ item' = [m \in Names |-> IF m = n THEN "done"
                ELSE IF Opened(n, m) THEN Into(n, m)[1] ELSE item[m]]
  /\ expect' = [m \in Names |-> IF m = n THEN "none"
                  ELSE IF Opened(n, m) THEN Into(n, m)[2] ELSE expect[m]]
  /\ shown' = [m \in Names |-> IF Opened(n, m) THEN Into(n, m)[3] ELSE shown[m]]
  /\ UNCHANGED <<answer, raced, wasted>>

\* The store refuses a folder sent to merge over a file as an error.
Refuse(n) ==
  /\ item[n] = "sending"
  /\ n \notin Folders \/ expect[n] = "missing"
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

\* An answer to the asking names ns. What waits inside a skipped folder is
\* skipped with it; inside any other answered folder it waits on the send.
Dropped(ns, a, n) ==
  a = "skip" /\ n \in Contents /\ item[n] = "waiting" /\ Top \in ns
Answer(ns, a) ==
  /\ item' = [n \in Names |->
               IF Dropped(ns, a, n) THEN "skipped"
               ELSE IF n \notin ns THEN item[n]
               ELSE IF a = "skip" THEN "skipped" ELSE "sending"]
  /\ expect' = [n \in Names |->
                 IF n \notin ns THEN expect[n]
                 ELSE IF a = "replace" THEN shown[n]
                 ELSE IF a \in {"keep", "merge"} THEN a ELSE "none"]
  /\ answer' = [n \in Names |->
                 IF Dropped(ns, a, n) THEN "skip"
                 ELSE IF n \notin ns THEN answer[n]
                 ELSE IF a = "replace" THEN shown[n] ELSE a]
  /\ shown' = [n \in Names |-> IF n \in ns THEN "none" ELSE shown[n]]
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

\* Cancelling a folder's row cancels it and everything inside it, found
\* from the drop's tree rather than by path: a Keep both folder (answer
\* "keep") is sent under its new name while what waits inside it still
\* carries the dropped one until the folder lands.
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
\* No dropped folder merges into a folder at its name unless the person
\* pressed Merge, including one another writer made after the listing. The
\* spec models one level of folders: a subfolder another writer makes
\* inside a folder this drop created, after that folder commits, is merged
\* into without asking, by design, since the drop's subfolders are sent with
\* no expectation.
NoMergeWithoutAnswer == ~merged
\* What a folder holds waits only while the folder asks, is sent, or failed.
NothingWaitsOnAnEndedFolder ==
  \A n \in Contents :
    item[n] = "waiting" => item[Top] \in {"asking", "sending", "failed"}
\* Nothing inside a folder is sent before the folder lands.
NothingSentBeforeItsFolder ==
  \A n \in Contents :
    item[n] \in {"sending", "asking", "failed", "done"} => item[Top] = "done"

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
