---- MODULE UnixFSTextFileViewer ----
\* A text file in a pane: read, then edited in a draft and saved back.
\*
\* The read is a one-shot read of the file, run again when the file changes;
\* it is waiting, failed or shown. A failed read drops what it read, so with no
\* draft the failure takes the body and Retry reads again.
\*
\* Edit opens a draft from the text shown, only on a file the person may write
\* and that was read whole. The draft belongs to the viewer: a read that runs
\* again, fails, or brings another writer's text never replaces or hides it.
\* Only Discard edits and a save that lands end it.
\*
\* The draft remembers the version it started from. When a read brings a
\* different version, another writer saved while the person was editing: plain
\* Save leaves, and the person picks Save over theirs, Show theirs or Discard
\* edits, which takes their text. A save that fails keeps the draft and offers
\* the save again.

People == {"person"}

\* Versions name the file's contents in storage; "none" is no read yet.
Versions == {"a", "b"}

Buttons == {"edit", "save", "saveover", "discard", "retry", "theirs", "mine"}

VARIABLES
  read,      \* the read: waiting, failed or shown
  disk,      \* the version in storage
  seen,      \* the version the last read brought
  mode,      \* reading, editing or saving
  base,      \* the version the draft started from
  dirty,     \* the draft differs from its base
  saveFailed, \* the last save failed
  theirs,    \* the body shows the other writer's text in place of the draft
  overwrite, \* the save in flight was Save over theirs
  over,      \* the version seen when the save in flight was pressed
  writable,  \* the person may write the file
  whole,     \* the read holds the whole file, not a preview
  dropped    \* history: a draft ended other than by Discard or a landed save
vars == <<read, disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
          over, writable, whole, dropped>>

Editing == mode = "editing"
Drafted == mode \in {"editing", "saving"}
\* Another writer's version arrived after the draft began.
Changed == Drafted /\ seen # base

\* Keep records a draft that ends by any step but Discard or a landed save.
Keep == dropped' = (dropped \/ (Drafted /\ mode' = "reading"))

Shown(p) ==
  (IF mode = "reading" /\ read = "shown" /\ writable /\ whole
     THEN {"edit"} ELSE {})
    \cup (IF Editing /\ dirty /\ ~Changed THEN {"save"} ELSE {})
    \cup (IF Editing /\ dirty /\ Changed THEN {"saveover"} ELSE {})
    \cup (IF Editing THEN {"discard"} ELSE {})
    \cup (IF read = "failed" THEN {"retry"} ELSE {})
    \cup (IF Editing /\ Changed /\ ~theirs THEN {"theirs"} ELSE {})
    \cup (IF Editing /\ theirs THEN {"mine"} ELSE {})

Allowed(p, b) ==
  \/ b = "edit" /\ mode = "reading" /\ read = "shown" /\ writable /\ whole
  \/ b = "save" /\ Editing /\ dirty /\ seen = base
  \/ b = "saveover" /\ Editing /\ dirty /\ seen # base
  \/ b = "discard" /\ Editing
  \/ b = "retry" /\ read = "failed"
  \/ b = "theirs" /\ Editing /\ seen # base /\ ~theirs
  \/ b = "mine" /\ Editing /\ theirs

\* What a state tells the person beside its presses.
Hint(p) ==
  CASE read = "waiting" /\ ~Drafted -> "Names the file it is reading"
    [] mode = "saving" -> "Saving"
    [] mode = "reading" /\ read = "shown" /\ ~writable -> "Says why it is read-only"
    [] mode = "reading" /\ read = "shown" /\ ~whole -> "Says it is too large to edit"
    [] OTHER -> ""

Init ==
  /\ read = "waiting"
  /\ disk = "a"
  /\ seen = "none"
  /\ mode = "reading"
  /\ base = "none"
  /\ dirty = FALSE
  /\ saveFailed = FALSE
  /\ theirs = FALSE
  /\ overwrite = FALSE
  /\ over = "none"
  /\ writable \in BOOLEAN
  /\ whole \in BOOLEAN
  /\ dropped = FALSE

\* The read lands with the version in storage.
Arrive ==
  /\ read = "waiting"
  /\ read' = "shown"
  /\ seen' = disk
  /\ UNCHANGED <<disk, mode, base, dirty, saveFailed, theirs, overwrite, over,
                 writable, whole>>
  /\ Keep
\* The read fails; a draft stays as it was.
Fail ==
  /\ read = "waiting"
  /\ read' = "failed"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, writable, whole>>
  /\ Keep
\* The file changed or its handle reconnected, so it is read again.
Reread ==
  /\ read = "shown"
  /\ read' = "waiting"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, writable, whole>>
  /\ Keep
\* Another writer saves the file.
OtherWrite ==
  /\ disk' \in Versions \ {disk}
  /\ UNCHANGED <<read, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, writable, whole>>
  /\ Keep
Retry(p) ==
  /\ "retry" \in Shown(p)
  /\ read' = "waiting"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, writable, whole>>
  /\ Keep
Edit(p) ==
  /\ "edit" \in Shown(p)
  /\ mode' = "editing"
  /\ base' = seen
  /\ dirty' = FALSE
  /\ UNCHANGED <<read, disk, seen, saveFailed, theirs, overwrite, over,
                 writable, whole>>
  /\ Keep
Type(p) ==
  /\ Editing
  /\ ~theirs
  /\ dirty' = TRUE
  /\ UNCHANGED <<read, disk, seen, mode, base, saveFailed, theirs, overwrite,
                 over, writable, whole>>
  /\ Keep
Save(p) ==
  /\ "save" \in Shown(p)
  /\ mode' = "saving"
  /\ overwrite' = FALSE
  /\ over' = seen
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, base, dirty, writable, whole>>
  /\ Keep
SaveOver(p) ==
  /\ "saveover" \in Shown(p)
  /\ mode' = "saving"
  /\ overwrite' = TRUE
  /\ over' = seen
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, base, dirty, writable, whole>>
  /\ Keep
\* The save lands as a new version; the viewer reads it.
SaveDone ==
  /\ mode = "saving"
  /\ disk' \in Versions \ {disk}
  /\ mode' = "reading"
  /\ base' = "none"
  /\ dirty' = FALSE
  /\ overwrite' = FALSE
  /\ over' = "none"
  /\ read' = "waiting"
  /\ UNCHANGED <<seen, saveFailed, theirs, writable, whole, dropped>>
\* The save fails; the draft stays and Save is offered again.
SaveFail ==
  /\ mode = "saving"
  /\ mode' = "editing"
  /\ saveFailed' = TRUE
  /\ overwrite' = FALSE
  /\ over' = "none"
  /\ UNCHANGED <<read, disk, seen, base, dirty, theirs, writable, whole>>
  /\ Keep
\* Discard edits ends the draft; the body shows the file as last read.
Discard(p) ==
  /\ "discard" \in Shown(p)
  /\ mode' = "reading"
  /\ base' = "none"
  /\ dirty' = FALSE
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, overwrite, over, writable, whole, dropped>>
ShowTheirs(p) ==
  /\ "theirs" \in Shown(p)
  /\ theirs' = TRUE
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, overwrite,
                 over, writable, whole>>
  /\ Keep
BackToMine(p) ==
  /\ "mine" \in Shown(p)
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, overwrite,
                 over, writable, whole>>
  /\ Keep

Next ==
  \/ Arrive
  \/ Fail
  \/ Reread
  \/ OtherWrite
  \/ SaveDone
  \/ SaveFail
  \/ \E p \in People :
       Retry(p) \/ Edit(p) \/ Type(p) \/ Save(p) \/ SaveOver(p) \/ Discard(p)
       \/ ShowTheirs(p) \/ BackToMine(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* A draft ends only by Discard edits or a save that lands.
DraftNeverDropped == ~dropped
\* No save writes over a version the draft did not start from unless the
\* person pressed Save over theirs.
NoBlindOverwrite == mode = "saving" /\ ~overwrite => over = base
\* Edit shows only where the draft could be saved.
EditOnlyWhereWritable == "edit" \in Shown("person") => writable /\ whole
TypeOK ==
  /\ read \in {"waiting", "failed", "shown"}
  /\ disk \in Versions
  /\ seen \in Versions \cup {"none"}
  /\ mode \in {"reading", "editing", "saving"}
  /\ base \in Versions \cup {"none"}
  /\ dirty \in BOOLEAN
  /\ saveFailed \in BOOLEAN
  /\ theirs \in BOOLEAN
  /\ overwrite \in BOOLEAN
  /\ over \in Versions \cup {"none"}
  /\ writable \in BOOLEAN
  /\ whole \in BOOLEAN
  /\ dropped \in BOOLEAN
\* Only a draft carries a base.
BaseOnlyDrafted == (base # "none") <=> Drafted
====
