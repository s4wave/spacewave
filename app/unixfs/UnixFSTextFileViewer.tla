---- MODULE UnixFSTextFileViewer ----
\* A text file in a pane: read, then edited in a draft and saved back.
\*
\* The read is a one-shot read of the file, run again by Retry or a new file
\* handle; it is waiting, failed or shown. Nothing watches the file, so another
\* writer's commit shows at such a read or when the store refuses a Save. A
\* failed read drops
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
\* that state whatever is pressed. A plain save expects the draft's base, and
\* the store checks it inside the commit: a version other than the base stops
\* it unwritten, and the read after the refusal lands as a read, in the same
\* changed state, with Save over theirs offered while the viewer reads their
\* version back; a file removed since is recreated from the draft only while
\* nothing holds the path. A save that fails keeps the draft and offers the
\* save again. A landed save returns the version it wrote: the viewer shows the
\* saved text at that version, and the next plain Save expects it. When the
\* read after a refusal fails, Read again runs it once more.
\* UnixFSTextFileSave.tla models these saves with versions, text, previews
\* and the stale handle in more detail than a chart holds.

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
  writable,  \* the person may write the file
  whole,     \* the read holds the whole file, not a preview
  refused,   \* the store refused a plain save on the draft's base
  blind,     \* history: a plain save started over another writer's version
  dropped,   \* history: a draft ended other than by Discard or a landed save
  clobbered  \* history: a plain save replaced a version other than its base
vars == <<read, disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
          writable, whole, refused, blind, dropped, clobbered>>

Editing == mode = "editing"
Drafted == mode \in {"editing", "saving"}
\* A read brought a version other than the draft's.
ChangedRead == Drafted /\ seen \in Versions /\ seen # base
\* Another writer's version is under the draft: read, or only refused.
Changed == ChangedRead \/ (Drafted /\ refused)
\* The viewer waits for a read of the version that refused a plain save.
Awaiting == Drafted /\ refused /\ ~ChangedRead

\* SaveAccepted is the reducer's guard on a plain save.
SaveAccepted == Editing /\ dirty /\ ~Changed

\* Land takes a read of the version in storage. A read of the base the store
\* refused is from before the refusal and changes nothing; this chart draws
\* any other version as another writer's, and UnixFSTextFileSave.tla decides
\* by the text whether it is.
Land ==
  IF Drafted /\ refused /\ disk = base
    THEN UNCHANGED <<seen, base, refused>>
    ELSE /\ seen' = disk
         /\ UNCHANGED base
         /\ refused' = (refused /\ disk # base)

\* Keep records a draft that ends by any step but Discard or a landed save.
Keep ==
  /\ dropped' = (dropped \/ (Drafted /\ mode' = "reading"))
  /\ UNCHANGED <<blind, clobbered>>

\* Edit opens a draft of a whole read, or of a landed save.
CanEdit == mode = "reading" /\ writable /\ whole /\ read = "shown"

Shown(p) ==
  (IF CanEdit THEN {"edit"} ELSE {})
    \cup (IF SaveAccepted THEN {"save"} ELSE {})
    \cup (IF Editing /\ dirty /\ Changed THEN {"saveover"} ELSE {})
    \cup (IF Editing THEN {"discard"} ELSE {})
    \cup (IF read = "failed" THEN {"retry"} ELSE {})
    \cup (IF Editing /\ ChangedRead /\ ~theirs THEN {"theirs"} ELSE {})
    \cup (IF Editing /\ theirs THEN {"mine"} ELSE {})

Allowed(p, b) ==
  \/ b = "edit" /\ CanEdit
  \/ b = "save" /\ SaveAccepted
  \/ b = "saveover" /\ Editing /\ dirty /\ Changed
  \/ b = "discard" /\ Editing
  \/ b = "retry" /\ read = "failed"
  \/ b = "theirs" /\ Editing /\ ChangedRead /\ ~theirs
  \/ b = "mine" /\ Editing /\ theirs

\* What a state tells the person beside its presses.
Hint(p) ==
  CASE read = "waiting" /\ ~Drafted -> "Names the file it is reading"
    [] mode = "saving" -> "Saving"
    [] Awaiting /\ read = "waiting" -> "Says it is reading their version"
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
  /\ blind = FALSE
  \* A file that is read-only or a preview offers no Edit; one reason at a
  \* time draws each.
  /\ writable \in BOOLEAN
  /\ whole \in BOOLEAN
  /\ writable \/ whole
  /\ refused = FALSE
  /\ dropped = FALSE
  /\ clobbered = FALSE

\* The read lands with the version in storage; a removed file only fails.
Arrive ==
  /\ read = "waiting"
  /\ disk # "gone"
  /\ read' = "shown"
  /\ Land
  /\ UNCHANGED <<disk, mode, dirty, saveFailed, theirs, overwrite,
                 writable, whole>>
  /\ Keep
\* The read fails; a draft stays as it was.
Fail ==
  /\ read = "waiting"
  /\ read' = "failed"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 writable, whole, refused>>
  /\ Keep
\* Retry or a new file handle reads the file again.
Reread ==
  /\ read = "shown"
  /\ read' = "waiting"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 writable, whole, refused>>
  /\ Keep
\* Another writer saves or removes the file.
OtherWrite ==
  /\ disk' \in (Versions \cup {"gone"}) \ {disk}
  /\ UNCHANGED <<read, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 writable, whole, refused>>
  /\ Keep
Retry(p) ==
  /\ "retry" \in Shown(p)
  /\ read' = "waiting"
  /\ UNCHANGED <<disk, seen, mode, base, dirty, saveFailed, theirs, overwrite,
                 writable, whole, refused>>
  /\ Keep
Edit(p) ==
  /\ "edit" \in Shown(p)
  /\ mode' = "editing"
  /\ base' = seen
  /\ dirty' = FALSE
  /\ UNCHANGED <<read, disk, seen, saveFailed, theirs, overwrite,
                 writable, whole, refused>>
  /\ Keep
Type(p) ==
  /\ Editing
  /\ ~theirs
  /\ dirty' = TRUE
  /\ UNCHANGED <<read, disk, seen, mode, base, saveFailed, theirs, overwrite,
                 writable, whole, refused>>
  /\ Keep
\* A plain save is any dispatch the reducer accepts, shown or not. It expects
\* the draft's base.
Save(p) ==
  /\ SaveAccepted
  /\ mode' = "saving"
  /\ overwrite' = FALSE
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, base, dirty, writable, whole, refused>>
  /\ dropped' = dropped
  /\ blind' = (blind \/ Changed)
  /\ UNCHANGED clobbered
SaveOver(p) ==
  /\ "saveover" \in Shown(p)
  /\ mode' = "saving"
  /\ overwrite' = TRUE
  /\ saveFailed' = FALSE
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, base, dirty, writable, whole, refused>>
  /\ Keep
\* The store commits the save in one step with its check: Save over theirs
\* always, a plain save only over its base or, recreating the file, over
\* nothing. The viewer shows the saved text at the version the commit
\* returns.
SaveDone ==
  /\ mode = "saving"
  /\ overwrite \/ disk \in {base, "gone"}
  /\ clobbered' = (clobbered \/ (~overwrite /\ disk \notin {base, "gone"}))
  /\ disk' \in Versions \ {disk}
  /\ seen' = disk'
  /\ mode' = "reading"
  /\ base' = "none"
  /\ dirty' = FALSE
  /\ refused' = FALSE
  /\ overwrite' = FALSE
  /\ saveFailed' = FALSE
  /\ UNCHANGED <<read, theirs, writable, whole, blind, dropped>>
\* The store refuses a plain save over another writer's version; nothing is
\* written, Save over theirs is offered, and the viewer reads theirs back.
Refused ==
  /\ mode = "saving"
  /\ ~overwrite
  /\ disk \notin {base, "gone"}
  /\ mode' = "editing"
  /\ refused' = TRUE
  /\ read' = "waiting"
  /\ UNCHANGED <<disk, seen, base, dirty, saveFailed, theirs, overwrite,
                 writable, whole>>
  /\ Keep
\* The save fails; the draft stays and Save is offered again. Save over theirs
\* fails through the same reducer step, so the chart draws it once.
SaveFail ==
  /\ mode = "saving"
  /\ ~overwrite
  /\ mode' = "editing"
  /\ saveFailed' = TRUE
  /\ overwrite' = FALSE
  /\ UNCHANGED <<read, disk, seen, base, dirty, theirs, writable, whole,
                 refused>>
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
  /\ UNCHANGED <<read, disk, seen, overwrite, writable, whole, blind,
                 dropped, clobbered>>
ShowTheirs(p) ==
  /\ "theirs" \in Shown(p)
  /\ theirs' = TRUE
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, overwrite,
                 writable, whole, refused>>
  /\ Keep
BackToMine(p) ==
  /\ "mine" \in Shown(p)
  /\ theirs' = FALSE
  /\ UNCHANGED <<read, disk, seen, mode, base, dirty, saveFailed, overwrite,
                 writable, whole, refused>>
  /\ Keep

Next ==
  \/ Arrive
  \/ Fail
  \/ Reread
  \/ OtherWrite
  \/ SaveDone
  \/ Refused
  \/ SaveFail
  \/ \E p \in People :
       Retry(p) \/ Edit(p) \/ Type(p) \/ Save(p)
       \/ SaveOver(p) \/ Discard(p) \/ ShowTheirs(p) \/ BackToMine(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* A draft ends only by Discard edits or a save that lands.
DraftNeverDropped == ~dropped
\* No save writes over a version the draft did not start from unless the
\* person pressed Save over theirs.
NoBlindOverwrite == ~blind
\* A draft over a removed file can save: the store's refusal never takes the
\* removal for another writer's version, which would leave only Save over
\* theirs or Discard edits.
RemovedFileSaves == disk = "gone" => ~ENABLED Refused
\* A plain save replaces only the draft's base, or nothing; the store checks
\* that inside the commit, so no write lands between a check and the write.
NoWriteOverTheirs == ~clobbered
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
  /\ blind \in BOOLEAN
  /\ writable \in BOOLEAN
  /\ whole \in BOOLEAN
  /\ refused \in BOOLEAN
  /\ dropped \in BOOLEAN
  /\ clobbered \in BOOLEAN
\* Only a draft carries a base.
BaseOnlyDrafted == (base # "none") <=> Drafted
====
