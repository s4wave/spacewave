---- MODULE UploadConflict ----
\* Files dropped or picked into a folder, uploaded one commit per file, and
\* the question asked when a dropped name is already taken.
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
\* The question names one file and its folder. Replace resends the file
\* expecting the version shown; Keep both sends it under the first free
\* "name (n).ext", as missing; Skip leaves the name alone. With several names
\* of one drop asking, each answer can apply to all of them; Replace applies
\* only to those holding a file. A Replace the store refuses with a newer
\* version asks again showing it; one refused because the name no longer
\* holds a file is resent as missing, which writes it when the name is empty
\* and asks again when a folder or link took it. A failed send keeps its
\* answer and offers Retry. Cancel all stops every file still sending,
\* failed or asking. Each drop asks in turn: a second drop's names wait
\* until the first drop's questions are answered.

People == {"person"}

\* The dropped names, by drop. Drop2 arrives while Drop1 may be asking.
\* Starts is what a name may hold before the drop, and Writes what another
\* writer may leave it holding.
CONSTANTS Drop1, Drop2, Starts, Writes
Names == Drop1 \cup Drop2

\* What a name holds in the folder: nothing, another writer's file at version
\* "a" or "b", the dropped file once written ("m"), or a folder or link.
Files == {"a", "b", "m"}
Holds == {"none", "dir"} \cup Files

Buttons == {"replace", "keep", "skip", "replaceall", "keepall", "skipall",
            "retry", "cancel"}

VARIABLES
  disk,     \* what each name holds in the folder
  item,     \* each name's upload: out, sending, asking, failed, done, skipped, cancelled
  expect,   \* what a send expects the name to hold: missing, a version, or keep
  shown,    \* the version a question shows, or "dir" for a folder or link
  answer,   \* the person's last answer: none, keep, skip, or the version Replace named
  raced,    \* another writer changed the name since its drop
  clobbered, \* history: a write replaced a file without a Replace naming it
  wasted    \* history: a file streamed into a refusal no other writer caused
vars == <<disk, item, expect, shown, answer, raced, clobbered, wasted>>

DropOf(n) == IF n \in Drop1 THEN Drop1 ELSE Drop2
Asking(d) == {n \in d : item[n] = "asking"}
\* The question shows the first drop with a name asking, and its first name.
QDrop == IF Asking(Drop1) # {} THEN Drop1 ELSE Drop2
Asked == Asking(QDrop) # {}
Current == CHOOSE n \in Asking(QDrop) : TRUE
Others == Asking(QDrop) \ {Current}
Versioned(n) == shown[n] \in Files
Busy == {n \in Names : item[n] \in {"sending", "asking", "failed"}}

Shown(p) ==
  (IF Asked THEN {"keep", "skip"} ELSE {})
    \cup (IF Asked /\ Versioned(Current) THEN {"replace"} ELSE {})
    \cup (IF Asked /\ Others # {} THEN {"keepall", "skipall"} ELSE {})
    \cup (IF Asked /\ Others # {} /\ Versioned(Current)
          THEN {"replaceall"} ELSE {})
    \cup (IF \E n \in Names : item[n] = "failed" THEN {"retry"} ELSE {})
    \cup (IF Busy # {} THEN {"cancel"} ELSE {})

\* What the manager accepts.
Allowed(p, b) ==
  \/ b \in {"keep", "skip"} /\ Asked
  \/ b = "replace" /\ Asked /\ Versioned(Current)
  \/ b \in {"keepall", "skipall"} /\ Asked /\ Others # {}
  \/ b = "replaceall" /\ Asked /\ Versioned(Current)
  \/ b = "retry" /\ \E n \in Names : item[n] = "failed"
  \/ b = "cancel" /\ Busy # {}

\* What the screen says with nothing to press.
Hint(p) ==
  CASE \E n \in Names : item[n] = "out" -> "Waits for a drop"
    [] OTHER -> "Lists each upload as complete or skipped"

\* The preflight: a taken name asks, any other is sent as missing.
Arrive(n) ==
  IF disk[n] = "none" THEN <<"sending", "missing", "none">>
  ELSE <<"asking", "none", IF disk[n] \in Files THEN disk[n] ELSE "dir">>

Drop(d) ==
  /\ d # {}
  /\ \A n \in d : item[n] = "out"
  /\ d = Drop1 \/ \A n \in Drop1 : item[n] # "out"
  /\ item' = [n \in Names |-> IF n \in d THEN Arrive(n)[1] ELSE item[n]]
  /\ expect' = [n \in Names |-> IF n \in d THEN Arrive(n)[2] ELSE expect[n]]
  /\ shown' = [n \in Names |-> IF n \in d THEN Arrive(n)[3] ELSE shown[n]]
  /\ UNCHANGED <<disk, answer, raced, clobbered, wasted>>

\* Another writer creates, changes, replaces or removes a name.
OtherWrite(n) ==
  /\ \E h \in Writes \ {disk[n]} : disk' = [disk EXCEPT ![n] = h]
  /\ raced' = [raced EXCEPT ![n] = TRUE]
  /\ UNCHANGED <<item, expect, shown, answer, clobbered, wasted>>

\* The store's check inside the commit.
Accepts(n) ==
  \/ expect[n] = "keep"
  \/ expect[n] = "missing" /\ disk[n] = "none"
  \/ expect[n] \in Files /\ disk[n] = expect[n]

Land(n) ==
  /\ item[n] = "sending"
  /\ Accepts(n)
  /\ clobbered' = (clobbered \/ (expect[n] # "keep" /\ disk[n] \in Files
                                 /\ answer[n] # disk[n]))
  /\ disk' = IF expect[n] = "keep" THEN disk ELSE [disk EXCEPT ![n] = "m"]
  /\ item' = [item EXCEPT ![n] = "done"]
  /\ expect' = [expect EXCEPT ![n] = "none"]
  /\ UNCHANGED <<shown, answer, raced, wasted>>

Refuse(n) ==
  /\ item[n] = "sending"
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
  /\ UNCHANGED <<disk, answer, raced, clobbered>>

Fail(n) ==
  /\ item[n] = "sending"
  /\ item' = [item EXCEPT ![n] = "failed"]
  /\ UNCHANGED <<disk, expect, shown, answer, raced, clobbered, wasted>>

Retry(p) ==
  /\ "retry" \in Shown(p)
  /\ \E n \in Names :
       /\ item[n] = "failed"
       /\ item' = [item EXCEPT ![n] = "sending"]
  /\ UNCHANGED <<disk, expect, shown, answer, raced, clobbered, wasted>>

\* An answer to the asking names ns.
Answer(ns, a) ==
  /\ item' = [n \in Names |->
               IF n \notin ns THEN item[n]
               ELSE IF a = "skip" THEN "skipped" ELSE "sending"]
  /\ expect' = [n \in Names |->
                 IF n \notin ns THEN expect[n]
                 ELSE IF a = "replace" THEN shown[n]
                 ELSE IF a = "keep" THEN "keep" ELSE "none"]
  /\ answer' = [n \in Names |->
                 IF n \notin ns THEN answer[n]
                 ELSE IF a = "replace" THEN shown[n] ELSE a]
  /\ shown' = [n \in Names |-> IF n \in ns THEN "none" ELSE shown[n]]
  /\ UNCHANGED <<disk, raced, clobbered, wasted>>

Press(p, b) ==
  /\ b \in Shown(p)
  /\ CASE b = "replace" -> Answer({Current}, "replace")
       [] b = "keep" -> Answer({Current}, "keep")
       [] b = "skip" -> Answer({Current}, "skip")
       [] b = "replaceall" ->
            Answer({n \in Asking(QDrop) : Versioned(n)}, "replace")
       [] b = "keepall" -> Answer(Asking(QDrop), "keep")
       [] b = "skipall" -> Answer(Asking(QDrop), "skip")

CancelAll(p) ==
  /\ "cancel" \in Shown(p)
  /\ item' = [n \in Names |-> IF n \in Busy THEN "cancelled" ELSE item[n]]
  /\ expect' = [n \in Names |-> IF n \in Busy THEN "none" ELSE expect[n]]
  /\ shown' = [n \in Names |-> IF n \in Busy THEN "none" ELSE shown[n]]
  /\ UNCHANGED <<disk, answer, raced, clobbered, wasted>>

Init ==
  /\ disk \in [Names -> Starts]
  /\ item = [n \in Names |-> "out"]
  /\ expect = [n \in Names |-> "none"]
  /\ shown = [n \in Names |-> "none"]
  /\ answer = [n \in Names |-> "none"]
  /\ raced = [n \in Names |-> FALSE]
  /\ clobbered = FALSE
  /\ wasted = FALSE

Next ==
  \/ Drop(Drop1) \/ Drop(Drop2)
  \/ \E n \in Names : OtherWrite(n) \/ Land(n) \/ Refuse(n) \/ Fail(n)
  \/ \E p \in People :
       Retry(p) \/ CancelAll(p)
       \/ \E b \in {"replace", "keep", "skip", "replaceall", "keepall",
                    "skipall"} : Press(p, b)

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
\* A file waiting on the person is always on the question.
AskingIsAsked == \A n \in Names : item[n] = "asking" => Asked
\* Only a question about a file offers Replace.
ReplaceOnlyFiles == "replace" \in Shown("person") => shown[Current] \in Files
\* A second drop's names wait until the first drop's questions are answered.
OneDropAsks == Asking(Drop1) # {} => QDrop = Drop1

TypeOK ==
  /\ disk \in [Names -> Holds]
  /\ item \in [Names -> {"out", "sending", "asking", "failed", "done",
                         "skipped", "cancelled"}]
  /\ expect \in [Names -> {"none", "missing", "keep"} \cup Files]
  /\ shown \in [Names -> {"none", "dir"} \cup Files]
  /\ answer \in [Names -> {"none", "keep", "skip"} \cup Files]
  /\ raced \in [Names -> BOOLEAN]
  /\ clobbered \in BOOLEAN
  /\ wasted \in BOOLEAN
====
