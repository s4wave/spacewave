---- MODULE UnixFSGalleryViewer ----
\* What the gallery draws while it watches a folder's tree for images.
\*
\* The watch yields the whole index of the images under the folder, again on
\* each change. Until its first index, a wait or a failure takes the body.
\* Once an index arrived it stays: a watch that fails later reopens and keeps
\* the tiles it had, still pressable, with a line saying it reads again. An
\* index with no image says so. Two notes can sit above the tiles: the index
\* stopped at its limit, and some folders could not be read. A tile opens its
\* image only while the pane has a session to read the preview through.

People == {"person"}

\* open shows a tile's image; retry reads again after a failed first read.
Buttons == {"open", "retry"}

VARIABLES
  watch,     \* the watch: waiting, watching, failed or reconnecting
  kept,      \* an index arrived and the watch still holds it
  found,     \* the index holds at least one image
  truncated, \* the index stopped at its limit
  issues,    \* some folders could not be read
  session    \* the pane has a session to read previews through
vars == <<watch, kept, found, truncated, issues, session>>

\* The body: the tiles once an index with an image arrived, the empty row
\* once an index without one did, else the wait or the failure.
Body ==
  CASE kept /\ found -> "tiles"
    [] kept -> "empty"
    [] watch = "failed" -> "failed"
    [] OTHER -> "waiting"

\* The line above an index kept while the watch reopens.
Rereading == kept /\ watch = "reconnecting"

Shown(p) ==
  (IF Body = "tiles" /\ session THEN {"open"} ELSE {})
    \cup (IF Body = "failed" THEN {"retry"} ELSE {})

Allowed(p, b) ==
  \/ b = "open" /\ kept /\ found /\ session
  \/ b = "retry" /\ watch = "failed" /\ ~kept

Hint(p) ==
  CASE Body = "waiting" -> "Names the folder it reads"
    [] Body = "empty" -> "Says no images are under this path"
    [] Body = "tiles" /\ ~session -> "Says previews need a session"
    [] OTHER -> ""

Init ==
  /\ watch = "waiting"
  /\ kept = FALSE
  /\ found = FALSE
  /\ truncated = FALSE
  /\ issues = FALSE
  /\ session = TRUE

\* The watch yields the whole index.
Yield ==
  /\ watch \in {"waiting", "watching", "reconnecting"}
  /\ watch' = "watching"
  /\ kept' = TRUE
  /\ found' \in BOOLEAN
  /\ truncated' \in BOOLEAN
  /\ issues' \in BOOLEAN
  /\ UNCHANGED session
\* The watch fails before its first index and reports it.
Fail ==
  /\ watch = "waiting"
  /\ ~kept
  /\ watch' = "failed"
  /\ UNCHANGED <<kept, found, truncated, issues, session>>
\* The watch fails after an index and reopens, keeping the index.
Drop ==
  /\ watch = "watching"
  /\ watch' = "reconnecting"
  /\ UNCHANGED <<kept, found, truncated, issues, session>>
\* The pane loses its session, as in a fixture or a detached pane.
Detach ==
  /\ session
  /\ session' = FALSE
  /\ UNCHANGED <<watch, kept, found, truncated, issues>>
Retry(p) ==
  /\ "retry" \in Shown(p)
  /\ watch' = "waiting"
  /\ UNCHANGED <<kept, found, truncated, issues, session>>
\* The person opens a tile's image over the gallery.
Open(p) ==
  /\ "open" \in Shown(p)
  /\ UNCHANGED vars

Next ==
  \/ Yield
  \/ Fail
  \/ Drop
  \/ Detach
  \/ \E p \in People : Retry(p) \/ Open(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Tiles found stay drawn, and pressable with a session, through a failure.
FoundStaysPressable == kept /\ found /\ session => "open" \in Shown("person")
\* A failure takes the body only before the first index.
FailureOnlyBeforeIndex == Body = "failed" => ~kept
TypeOK ==
  /\ watch \in {"waiting", "watching", "failed", "reconnecting"}
  /\ kept \in BOOLEAN
  /\ found \in BOOLEAN
  /\ truncated \in BOOLEAN
  /\ issues \in BOOLEAN
  /\ session \in BOOLEAN
====
