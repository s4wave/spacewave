---- MODULE UnixFSTextFileViewer ----
\* A text file in a pane: read, then edited in a draft and saved back.
\*
\* The read is a one-shot read of the file, run again by Retry or a new file
\* handle; it is waiting, failed or shown. Nothing watches the file, so another
\* writer's commit shows at such a read or at a Save press. A failed read drops
\* what it read, so with no draft the failure takes the body and Retry reads
\* again.
\*
\* Edit opens a draft from the text shown, only on a file the person may write
\* and that was read whole as UTF-8 text. The draft belongs to the viewer: a
\* read that runs again, fails, or brings another writer's text never replaces
\* or hides it.
\* Only Discard edits and a save that lands end it.
\*
\* The draft remembers the version it started from. When a read brings a
\* different version, another writer saved while the person was editing: plain
\* Save leaves, and the person picks Save over theirs, Show theirs or Discard
\* edits, which takes their text. The viewer's reducer refuses a plain save in
\* that state whatever is pressed. A plain save first reads the file again; a
\* version other than the draft's base stops it unwritten and lands as a read,
\* in the same changed state; a file removed since is recreated from the
\* draft. A save that fails keeps the draft and offers the
\* save again; a landed save shows its text at once.

People == {"person"}

\* Versions name the file's contents in storage; "none" is no read yet, and
\* "gone" is a file another writer removed.
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
  checked,   \* the plain save in flight read the file and may write
  found,     \* the version the read at the press found
  writable,  \* the person may write the file
  whole,     \* the read holds the whole file, not a preview
  dropped    \* history: a draft ended other than by Discard or a landed save
vars == <<read, disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
          over, checked, found, writable, whole, dropped>>

Editing == mode = "editing"
Drafted == mode \in {"editing", "saving"}
\* Another writer's version arrived after the draft began.
Changed == Drafted /\ seen # base

\* SaveAccepted is the reducer's guard on a plain save.
SaveAccepted == Editing /\ dirty /\ seen = base

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
  \/ b = "save" /\ SaveAccepted
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
    [] mode = "reading" /\ read = "shown" /\ ~whole -> "Says why it cannot be edited"
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
  /\ checked = FALSE
  /\ found = "none"
  /\ writable \in BOOLEAN
  /\ whole \in BOOLEAN
  /\ dropped = FALSE

\* The read lands with the version in storage; a removed file only fails.
Arrive ==
  /\ read = "waiting"
  /\ disk # "gone"
  /\ read' = "shown"
  /\ seen' = disk
  /\ UNCHANGED <<disk, mode, base, dirty, saveFailed, theirs, overwrite, over,
                 checked, found, writable, whole>>
  /\ Keep
\* The read fails; a draft stays as it was.
Fail ==
  /\ read = "waiting"
  /\ read' = "failed"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, checked, found, writable, whole>>
  /\ Keep
\* Retry or a new file handle reads the file again.
Reread ==
  /\ read = "shown"
  /\ read' = "waiting"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, checked, found, writable, whole>>
  /\ Keep
\* Another writer saves or removes the file.
OtherWrite ==
  /\ disk' \in (Versions \cup {"gone"}) \ {disk}
  /\ UNCHANGED <<read, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, checked, found, writable, whole>>
  /\ Keep
Retry(p) ==
  /\ "retry" \in Shown(p)
  /\ read' = "waiting"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, checked, found, writable, whole>>
  /\ Keep
Edit(p) ==
  /\ "edit" \in Shown(p)
  /\ mode' = "editing"
  /\ base' = seen
  /\ dirty' = FALSE
  /\ UNCHANGED <<read, disk, seen, saveFailed, theirs, overwrite, over,
                 checked, found, writable, whole>>
  /\ Keep
Type(p) ==
  /\ Editing
  /\ ~theirs
  /\ dirty' = TRUE
  /\ UNCHANGED <<read, disk, seen, mode, base, saveFailed, theirs, overwrite,
                 over, checked, found, writable, whole>>
  /\ Keep
\* A plain save is any dispatch the reducer accepts, shown or not.
Save(p) ==
  /\ SaveAccepted
  /\ mode' = "saving"
  /\ overwrite' = FALSE
  /\ over' = seen
  /\ checked' = FALSE
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, base, dirty, found, writable, whole>>
  /\ Keep
\* The plain save reads the file. Its base, or no file at all, lets the write
\* go; another version stops it unwritten and lands as a read.
Check ==
  /\ mode = "saving"
  /\ ~overwrite
  /\ ~checked
  /\ IF disk \in {base, "gone"}
       THEN /\ checked' = TRUE
            /\ found' = disk
            /\ UNCHANGED <<mode, seen, over>>
       ELSE /\ mode' = "editing"
            /\ seen' = disk
            /\ over' = "none"
            /\ UNCHANGED <<checked, found>>
  /\ UNCHANGED <<read, disk, base, dirty, saveFailed, theirs, overwrite,
                 writable, whole>>
  /\ Keep
SaveOver(p) ==
  /\ "saveover" \in Shown(p)
  /\ mode' = "saving"
  /\ overwrite' = TRUE
  /\ over' = seen
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, base, dirty, checked, found, writable,
                 whole>>
  /\ Keep
\* The save lands as a new version and shows at once.
SaveDone ==
  /\ mode = "saving"
  /\ overwrite \/ checked
  /\ disk' \in Versions \ {disk}
  /\ seen' = disk'
  /\ mode' = "reading"
  /\ base' = "none"
  /\ dirty' = FALSE
  /\ overwrite' = FALSE
  /\ over' = "none"
  /\ checked' = FALSE
  /\ found' = "none"
  /\ UNCHANGED <<read, saveFailed, theirs, writable, whole, dropped>>
\* The save fails; the draft stays and Save is offered again.
SaveFail ==
  /\ mode = "saving"
  /\ mode' = "editing"
  /\ saveFailed' = TRUE
  /\ overwrite' = FALSE
  /\ over' = "none"
  /\ checked' = FALSE
  /\ found' = "none"
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
  /\ UNCHANGED <<read, disk, seen, overwrite, over, checked, found, writable,
                 whole, dropped>>
ShowTheirs(p) ==
  /\ "theirs" \in Shown(p)
  /\ theirs' = TRUE
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, overwrite,
                 over, checked, found, writable, whole>>
  /\ Keep
BackToMine(p) ==
  /\ "mine" \in Shown(p)
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, overwrite,
                 over, checked, found, writable, whole>>
  /\ Keep

Next ==
  \/ Arrive
  \/ Fail
  \/ Reread
  \/ OtherWrite
  \/ Check
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
\* A draft over a removed file can save: the read at the press never takes
\* the removal for another writer's version, which would leave only Save over
\* theirs or Discard edits.
RemovedFileSaves == Drafted => seen # "gone"
\* A plain save writes only after its read at the press found the draft's
\* base. Another writer between that read and the write is the storage
\* layer's to refuse.
NoWriteOverTheirs ==
  mode = "saving" /\ checked /\ ~overwrite => found \in {base, "gone"}
\* Edit shows only where the draft could be saved.
EditOnlyWhereWritable == "edit" \in Shown("person") => writable /\ whole
TypeOK ==
  /\ read \in {"waiting", "failed", "shown"}
  /\ disk \in Versions \cup {"gone"}
  /\ seen \in Versions \cup {"none"}
  /\ mode \in {"reading", "editing", "saving"}
  /\ base \in Versions \cup {"none"}
  /\ dirty \in BOOLEAN
  /\ saveFailed \in BOOLEAN
  /\ theirs \in BOOLEAN
  /\ overwrite \in BOOLEAN
  /\ over \in Versions \cup {"none"}
  /\ checked \in BOOLEAN
  /\ found \in Versions \cup {"none", "gone"}
  /\ writable \in BOOLEAN
  /\ whole \in BOOLEAN
  /\ dropped \in BOOLEAN
\* Only a draft carries a base.
BaseOnlyDrafted == (base # "none") <=> Drafted
====
