---- MODULE UnixFSTextFileSave ----
\* The text file viewer's saves against the store's version check, in more
\* detail than UnixFSTextFileViewer.tla can chart: versions with their text,
\* whole and preview reads, and the file's own handle still reading the
\* version a landed save replaced.
\*
\* The read is a one-shot read of the file through the file's own handle, run
\* again by Retry or a new file handle; it is waiting, failed or shown. Nothing
\* watches the file, so another writer's commit shows at such a read or when
\* the store refuses a Save. A read is whole, or a preview: cut at the read
\* limit, decoded with replacements, or with line endings the editor cannot
\* write back; a preview's text is not the file's. A whole read's text keeps
\* its byte order mark and holds its lines in LF; a draft that moves onto a
\* read takes that read's line endings, so two versions that differ only in
\* line endings hold the same text. A failed read drops what it read, so with
\* no draft the failure takes the body and Retry reads again.
\*
\* Edit opens a draft from the text shown, only on a file the person may write
\* and that was read whole as UTF-8 text. The draft belongs to the viewer: a
\* read that runs again, fails, or brings another writer's text never replaces
\* or hides it. Only Discard edits and a save that lands end it.
\*
\* The draft remembers the version it started from. A whole read with the
\* draft's text at another version moves the draft onto it. Any other read at
\* a different version is another writer's: plain Save leaves, and the person
\* picks Save over theirs, Show theirs or Discard edits. A plain save expects
\* the draft's version, and the store checks it inside the commit: another
\* version stops it unwritten, and the viewer reads their version back through
\* the root, offering Save over theirs meanwhile and Read again when that read
\* fails; a file removed since is recreated from the draft only while nothing
\* holds the path. The read back takes the version on disk when it starts and
\* lands it later, so a save over theirs can land between; that save ends the
\* read, and its result never lands.
\*
\* A landed save returns the version it wrote. The viewer shows the saved text
\* at that version, and the next draft starts from it with no read between.
\* The file's own handle can still read the version the save replaced; such a
\* read is from before the save and is not taken. The model lets the lagging
\* handle answer only until the viewer takes a read after the save.

People == {"person"}

\* Versions name the file's contents in storage, each with its text: "a" and
\* "c" hold the same text, as after a change to only the file's permissions.
\* "none" is no read yet, and "gone" a file another writer removed.
Versions == {"a", "b", "c"}
Texts == {"x", "y"}
Text(v) == IF v = "b" THEN "y" ELSE "x"

Buttons == {"edit", "save", "saveover", "discard", "retry", "theirs", "mine",
            "readagain"}

VARIABLES
  read,      \* the read: waiting, failed or shown
  disk,      \* the version in storage
  seen,      \* the version the viewer knows: a read's, or a landed save's
  mode,      \* reading, editing or saving
  base,      \* the version the draft started from
  dirty,     \* the draft differs from its base
  saveFailed, \* the last save failed
  theirs,    \* the body shows the other writer's text in place of the draft
  overwrite, \* the save in flight was Save over theirs
  over,      \* the version seen when the save in flight was pressed
  writable,  \* the person may write the file
  whole,     \* the last read held the whole file, not a preview
  refused,   \* the store refused a plain save on the draft's version
  back,      \* the read after a refusal: idle, reading or failed
  backAt,    \* the version the read back took, or "none" before it takes one
  replaced,  \* the versions the last landed save replaced, until a read is taken
  lag,       \* the version the file's own handle may still read, or "none"
  origin,    \* history: the text the draft started from
  dropped,   \* history: a draft ended other than by Discard or a landed save
  clobbered, \* history: a plain save replaced text the draft did not start from
  reverted,  \* history: a read from before a landed save was taken
  early      \* history: the read back in flight was taken before a landed save
vars == <<read, disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
          over, writable, whole, refused, back, backAt, replaced, lag, origin,
          dropped, clobbered, reverted, early>>

Editing == mode = "editing"
Drafted == mode \in {"editing", "saving"}
\* BaseText is the text the draft stands on.
BaseText == Text(base)
\* A read brought a version other than the draft's.
ChangedRead == Drafted /\ seen \in Versions /\ seen # base
\* Another writer's version is under the draft: read, or only refused.
Changed == ChangedRead \/ (Drafted /\ refused)
\* The viewer waits for a read of the version that refused a plain save.
Awaiting == Drafted /\ refused /\ ~ChangedRead
\* A read at v is from before what the viewer knows.
Stale(v) ==
  \/ v \in replaced
  \/ Drafted /\ refused /\ v = base

\* SaveAccepted is the reducer's guard on a plain save.
SaveAccepted == Editing /\ dirty /\ ~Changed

\* Keep records a draft that ends by any step but Discard or a landed save.
Keep ==
  /\ dropped' = (dropped \/ (Drafted /\ mode' = "reading"))
  /\ UNCHANGED clobbered

\* ReadText is the text a read of v shows: the file's when whole, else the
\* preview's t, which may equal any text.
ReadText(v, w, t) == IF w THEN Text(v) ELSE t

\* Land takes a read of v that is not stale: whole, or a preview showing t.
\* A whole read with the draft's text moves a draft no save holds onto v; a
\* save in flight keeps the version it expected, so its refusal names it.
\* A read taken is past the last save, so the versions it replaced stop being
\* stale.
Land(v, w, t) ==
  /\ seen' = v
  /\ whole' = w
  /\ replaced' = {}
  /\ IF Editing /\ w /\ ReadText(v, w, t) = BaseText
       THEN base' = v /\ refused' = FALSE
       ELSE UNCHANGED <<base, refused>>

Shown(p) ==
  (IF mode = "reading" /\ read = "shown" /\ writable /\ whole
     THEN {"edit"} ELSE {})
    \cup (IF SaveAccepted THEN {"save"} ELSE {})
    \cup (IF Editing /\ dirty /\ Changed THEN {"saveover"} ELSE {})
    \cup (IF Editing THEN {"discard"} ELSE {})
    \cup (IF read = "failed" THEN {"retry"} ELSE {})
    \cup (IF Editing /\ ChangedRead /\ ~theirs THEN {"theirs"} ELSE {})
    \cup (IF Editing /\ theirs THEN {"mine"} ELSE {})
    \cup (IF Awaiting /\ back = "failed" THEN {"readagain"} ELSE {})

Allowed(p, b) ==
  \/ b = "edit" /\ mode = "reading" /\ read = "shown" /\ writable /\ whole
  \/ b = "save" /\ SaveAccepted
  \/ b = "saveover" /\ Editing /\ dirty /\ Changed
  \/ b = "discard" /\ Editing
  \/ b = "retry" /\ read = "failed"
  \/ b = "theirs" /\ Editing /\ ChangedRead /\ ~theirs
  \/ b = "mine" /\ Editing /\ theirs
  \/ b = "readagain" /\ back = "failed"

\* What a state tells the person beside its presses.
Hint(p) ==
  CASE read = "waiting" /\ ~Drafted -> "Names the file it is reading"
    [] mode = "saving" -> "Saving"
    [] Awaiting /\ back = "reading" -> "Says it is reading their version"
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
  /\ writable \in BOOLEAN
  /\ whole = TRUE
  /\ refused = FALSE
  /\ back = "idle"
  /\ backAt = "none"
  /\ replaced = {}
  /\ lag = "none"
  /\ origin = "none"
  /\ dropped = FALSE
  /\ clobbered = FALSE
  /\ reverted = FALSE
  /\ early = FALSE

\* The viewer's own read lands: the version in storage, or the one its handle
\* still holds until a read after the save is taken. A removed file only
\* fails. A stale read changes nothing the viewer knows.
Arrive ==
  /\ read = "waiting"
  /\ disk # "gone"
  /\ read' = "shown"
  /\ \E v \in {disk} \cup (IF lag # "none" /\ replaced # {} THEN {lag} ELSE {}),
        w \in BOOLEAN, t \in Texts :
       IF Stale(v)
         THEN UNCHANGED <<seen, whole, base, refused, replaced, reverted>>
         ELSE /\ Land(v, w, t)
              /\ reverted' = (reverted \/ (replaced # {} /\ v = lag /\ v # disk))
  /\ UNCHANGED <<disk, mode, dirty, saveFailed, theirs, overwrite, over,
                 writable, back, backAt, early, lag, origin>>
  /\ Keep
\* The read fails; a draft stays as it was.
Fail ==
  /\ read = "waiting"
  /\ read' = "failed"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, writable, whole, refused, back, backAt, early, replaced, lag, origin,
                 reverted>>
  /\ Keep
\* Retry or a new file handle reads the file again.
Reread ==
  /\ read = "shown"
  /\ read' = "waiting"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, writable, whole, refused, back, backAt, early, replaced, lag, origin,
                 reverted>>
  /\ Keep
\* Another writer saves or removes the file.
OtherWrite ==
  /\ disk' \in (Versions \cup {"gone"}) \ {disk}
  /\ UNCHANGED <<read, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, writable, whole, refused, back, backAt, early, replaced, lag, origin,
                 reverted>>
  /\ Keep
\* The file's own handle catches up with storage.
CatchUp ==
  /\ lag # "none"
  /\ lag' = "none"
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, theirs,
                 overwrite, over, writable, whole, refused, back, backAt, early, replaced,
                 origin, reverted>>
  /\ Keep
\* The read after a refusal takes the version on disk through the root, which
\* the upload checked against.
BackTake ==
  /\ back = "reading"
  /\ backAt = "none"
  /\ disk # "gone"
  /\ backAt' = disk
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, theirs,
                 overwrite, over, writable, whole, refused, back, replaced, lag,
                 origin, reverted, early>>
  /\ Keep
\* It lands later; a read still at the version the save expected fails with
\* Read again.
BackRead ==
  /\ back = "reading"
  /\ backAt # "none"
  /\ backAt' = "none"
  /\ \E w \in BOOLEAN, t \in Texts :
       IF Stale(backAt)
         THEN back' = "failed" /\ UNCHANGED <<seen, whole, base, refused, replaced, reverted>>
         ELSE /\ back' = "idle"
              /\ Land(backAt, w, t)
              /\ reverted' = (reverted \/ early)
  /\ UNCHANGED <<read, disk, mode, dirty, saveFailed, theirs, overwrite, over,
                 writable, lag, origin, early>>
  /\ Keep
BackFail ==
  /\ back = "reading"
  /\ back' = "failed"
  /\ backAt' = "none"
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, theirs,
                 overwrite, over, writable, whole, refused, replaced, lag,
                 origin, reverted, early>>
  /\ Keep
Retry(p) ==
  /\ "retry" \in Shown(p)
  /\ read' = "waiting"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 over, writable, whole, refused, back, backAt, early, replaced, lag, origin,
                 reverted>>
  /\ Keep
ReadAgain(p) ==
  /\ "readagain" \in Shown(p)
  /\ back' = "reading"
  /\ early' = FALSE
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, theirs,
                 overwrite, over, writable, whole, refused, backAt, replaced,
                 lag, origin, reverted>>
  /\ Keep
\* Edit drafts from what the viewer knows: a whole read, or a landed save.
Edit(p) ==
  /\ "edit" \in Shown(p)
  /\ mode' = "editing"
  /\ base' = seen
  /\ dirty' = FALSE
  /\ refused' = FALSE
  /\ origin' = Text(seen)
  /\ UNCHANGED <<read, disk, seen, saveFailed, theirs, overwrite, over,
                 writable, whole, back, backAt, early, replaced, lag, reverted>>
  /\ Keep
Type(p) ==
  /\ Editing
  /\ ~theirs
  /\ dirty' = TRUE
  /\ UNCHANGED <<read, disk, seen, mode, base, saveFailed, theirs, overwrite,
                 over, writable, whole, refused, back, backAt, early, replaced, lag, origin,
                 reverted>>
  /\ Keep
\* A plain save is any dispatch the reducer accepts, shown or not. It expects
\* the draft's version.
Save(p) ==
  /\ SaveAccepted
  /\ mode' = "saving"
  /\ overwrite' = FALSE
  /\ over' = seen
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, base, dirty, writable, whole, refused, back, backAt, early,
                 replaced, lag, origin, reverted>>
  /\ Keep
SaveOver(p) ==
  /\ "saveover" \in Shown(p)
  /\ mode' = "saving"
  /\ overwrite' = TRUE
  /\ over' = seen
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, base, dirty, writable, whole, refused, back, backAt, early,
                 replaced, lag, origin, reverted>>
  /\ Keep
\* The store commits the save in one step with its check: Save over theirs
\* always, a plain save only over its version or, recreating the file, over
\* nothing. The viewer shows the saved text at the version the commit
\* returns; the file's own handle may still read the version the save
\* replaced, which the viewer read through it. The save ends a read back in
\* flight, which took its version before the save.
SaveDone ==
  /\ mode = "saving"
  /\ overwrite \/ disk \in {base, "gone"}
  /\ clobbered' = (clobbered \/
                   (~overwrite /\ disk # "gone" /\ Text(disk) # origin))
  /\ disk' \in Versions \ {disk}
  /\ replaced' = {seen, base} \cap Versions
  /\ lag' = IF disk \in {seen, base} THEN disk ELSE "none"
  /\ seen' = disk'
  /\ early' = (backAt # "none")
  /\ back' = "idle"
  /\ backAt' = "none"
  /\ mode' = "reading"
  /\ base' = "none"
  /\ dirty' = FALSE
  /\ refused' = FALSE
  /\ overwrite' = FALSE
  /\ over' = "none"
  /\ UNCHANGED <<read, saveFailed, theirs, writable, whole, origin, dropped,
                 reverted>>
\* The store refuses a plain save over another writer's version; nothing is
\* written, and the viewer reads theirs back.
Refused ==
  /\ mode = "saving"
  /\ ~overwrite
  /\ disk \notin {base, "gone"}
  /\ mode' = "editing"
  /\ refused' = TRUE
  /\ back' = "reading"
  /\ backAt' = "none"
  /\ early' = FALSE
  /\ over' = "none"
  /\ UNCHANGED <<read, disk, seen, base, dirty, saveFailed, theirs, overwrite,
                 writable, whole, replaced, lag, origin, reverted>>
  /\ Keep
\* The save fails; the draft stays and Save is offered again.
SaveFail ==
  /\ mode = "saving"
  /\ mode' = "editing"
  /\ saveFailed' = TRUE
  /\ overwrite' = FALSE
  /\ over' = "none"
  /\ UNCHANGED <<read, disk, seen, base, dirty, theirs, writable, whole,
                 refused, back, backAt, early, replaced, lag, origin, reverted>>
  /\ Keep
\* Discard edits ends the draft; the body shows what the viewer knows.
Discard(p) ==
  /\ "discard" \in Shown(p)
  /\ mode' = "reading"
  /\ base' = "none"
  /\ dirty' = FALSE
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ refused' = FALSE
  /\ UNCHANGED <<read, disk, seen, overwrite, over, writable, whole, back, backAt, early,
                 replaced, lag, origin, dropped, clobbered, reverted>>
ShowTheirs(p) ==
  /\ "theirs" \in Shown(p)
  /\ theirs' = TRUE
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, overwrite,
                 over, writable, whole, refused, back, backAt, early, replaced, lag, origin,
                 reverted>>
  /\ Keep
BackToMine(p) ==
  /\ "mine" \in Shown(p)
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, overwrite,
                 over, writable, whole, refused, back, backAt, early, replaced, lag, origin,
                 reverted>>
  /\ Keep

Next ==
  \/ Arrive
  \/ Fail
  \/ Reread
  \/ OtherWrite
  \/ CatchUp
  \/ BackTake
  \/ BackRead
  \/ BackFail
  \/ SaveDone
  \/ Refused
  \/ SaveFail
  \/ \E p \in People :
       Retry(p) \/ ReadAgain(p) \/ Edit(p) \/ Type(p) \/ Save(p)
       \/ SaveOver(p) \/ Discard(p) \/ ShowTheirs(p) \/ BackToMine(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* A draft ends only by Discard edits or a save that lands.
DraftNeverDropped == ~dropped
\* No save writes over a version the draft did not start from unless the
\* person pressed Save over theirs.
NoBlindOverwrite == mode = "saving" /\ ~overwrite => over = base
\* A draft over a removed file can save: the store's refusal never takes the
\* removal for another writer's version, which would leave only Save over
\* theirs or Discard edits.
RemovedFileSaves == disk = "gone" => ~ENABLED Refused
\* A plain save replaces only the text the draft started from, or nothing:
\* the store checks the version inside the commit, and the draft moves only
\* onto a whole read of its own text.
NoWriteOverTheirs == ~clobbered
\* After a landed save the viewer never takes a read from before it for what
\* is on disk: the version it replaced, still on the file's own handle, or
\* their version, read back before a save over theirs landed.
SavedNeverReverts == ~reverted
\* After the store refuses a plain save, Save over theirs stands in for Save
\* until a read shows what refused it.
RefusalOffersOver == Editing /\ refused /\ dirty => "saveover" \in Shown("person")
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
  /\ writable \in BOOLEAN
  /\ whole \in BOOLEAN
  /\ refused \in BOOLEAN
  /\ back \in {"idle", "reading", "failed"}
  /\ backAt \in Versions \cup {"none"}
  /\ replaced \subseteq Versions
  /\ lag \in Versions \cup {"none"}
  /\ origin \in Texts \cup {"none"}
  /\ dropped \in BOOLEAN
  /\ clobbered \in BOOLEAN
  /\ reverted \in BOOLEAN
  /\ early \in BOOLEAN
\* Only a draft carries a base.
BaseOnlyDrafted == (base # "none") <=> Drafted
====
