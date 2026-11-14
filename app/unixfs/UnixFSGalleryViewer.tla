---- MODULE UnixFSGalleryViewer ----
\* What the gallery draws while it watches a folder's tree for images.
\*
\* The watch runs on the folder tree's root handle and yields the whole index
\* of the images under the folder, again on each change. Until its first
\* index, a wait or a failure takes the body. A watch that fails after an
\* index reopens after a backoff and keeps the tiles it had, still pressable,
\* with a line saying it reads again; it never reports that failure, however
\* long it lasts. A root handle that fails or reloads drops the index: its
\* failure takes the body with the handle's Retry, its reload the wait. An
\* index with no image says so. Two notes can sit above the tiles: the index
\* stopped at its limit, and some folders could not be read. A tile opens its
\* image only with a preview to show, which needs both a session and a Space
\* to read it through. Moving to another folder starts a new watch from
\* waiting, so the last folder's tiles never show under the new one.

People == {"person"}

\* The folders the person can move between.
Paths == {"a", "b"}

\* open shows a tile's image; retry reads again after a failed first read.
Buttons == {"open", "retry"}

VARIABLES
  path,      \* the folder the pane shows
  watch,     \* the watch: waiting, watching, failed or reconnecting
  indexPath, \* the folder of the index the watch holds, or none
  found,     \* the index holds at least one image
  truncated, \* the index stopped at its limit
  issues,    \* some folders could not be read
  session,   \* the pane has a session to read previews through
  space      \* the pane is in a Space to read previews from
vars == <<path, watch, indexPath, found, truncated, issues, session, space>>

\* An index arrived and the watch still holds it.
Kept == indexPath # "none"

\* A tile has a preview to show and open.
Previews == session /\ space

\* The body: the tiles once an index with an image arrived, the empty row
\* once an index without one did, else the wait or the failure.
Body ==
  CASE Kept /\ found -> "tiles"
    [] Kept -> "empty"
    [] watch = "failed" -> "failed"
    [] OTHER -> "waiting"

\* The line above an index kept while the watch reopens.
Rereading == Kept /\ watch = "reconnecting"

Shown(p) ==
  (IF Body = "tiles" /\ Previews THEN {"open"} ELSE {})
    \cup (IF Body = "failed" THEN {"retry"} ELSE {})

Allowed(p, b) ==
  \/ b = "open" /\ Kept /\ found /\ Previews
  \/ b = "retry" /\ watch = "failed" /\ ~Kept

Hint(p) ==
  CASE Body = "waiting" -> "Names the folder it reads"
    [] Body = "empty" -> "Says no images are under this path"
    [] Body = "tiles" /\ ~Previews -> "Says previews need a session"
    [] OTHER -> ""

Init ==
  /\ path = "a"
  /\ watch = "waiting"
  /\ indexPath = "none"
  /\ found = FALSE
  /\ truncated = FALSE
  /\ issues = FALSE
  /\ session = TRUE
  /\ space = TRUE

\* The watch yields the whole index of the folder it watches.
Yield ==
  /\ watch \in {"waiting", "watching", "reconnecting"}
  /\ watch' = "watching"
  /\ indexPath' = path
  /\ found' \in BOOLEAN
  /\ truncated' \in BOOLEAN
  /\ issues' \in BOOLEAN
  /\ UNCHANGED <<path, session, space>>
\* The watch fails before its first index and reports it.
Fail ==
  /\ watch = "waiting"
  /\ ~Kept
  /\ watch' = "failed"
  /\ UNCHANGED <<path, indexPath, found, truncated, issues, session, space>>
\* The root handle fails, before or after an index; the watch drops what it
\* held and reports the handle's failure.
RootFail ==
  /\ watch' = "failed"
  /\ indexPath' = "none"
  /\ found' = FALSE
  /\ truncated' = FALSE
  /\ issues' = FALSE
  /\ UNCHANGED <<path, session, space>>
\* The root handle reloads, as on a lost connection; the watch drops what it
\* held and waits for the new handle.
RootReload ==
  /\ watch' = "waiting"
  /\ indexPath' = "none"
  /\ found' = FALSE
  /\ truncated' = FALSE
  /\ issues' = FALSE
  /\ UNCHANGED <<path, session, space>>
\* The watch fails after an index and reopens, keeping the index. It reopens
\* for as long as it fails and never reports the failure.
Drop ==
  /\ watch = "watching"
  /\ watch' = "reconnecting"
  /\ UNCHANGED <<path, indexPath, found, truncated, issues, session, space>>
\* The pane loses its session, as a detached pane does.
Detach ==
  /\ session
  /\ session' = FALSE
  /\ UNCHANGED <<path, watch, indexPath, found, truncated, issues, space>>
\* The pane is outside a Space, as a folder opened from a session alone is.
LeaveSpace ==
  /\ space
  /\ space' = FALSE
  /\ UNCHANGED <<path, watch, indexPath, found, truncated, issues, session>>
\* The person moves to another folder; a new watch starts from waiting.
Navigate(p) ==
  /\ \E to \in Paths \ {path} : path' = to
  /\ watch' = "waiting"
  /\ indexPath' = "none"
  /\ found' = FALSE
  /\ truncated' = FALSE
  /\ issues' = FALSE
  /\ UNCHANGED <<session, space>>
Retry(p) ==
  /\ "retry" \in Shown(p)
  /\ watch' = "waiting"
  /\ UNCHANGED <<path, indexPath, found, truncated, issues, session, space>>
\* The person opens a tile's image over the gallery.
Open(p) ==
  /\ "open" \in Shown(p)
  /\ UNCHANGED vars

Next ==
  \/ Yield
  \/ Fail
  \/ Drop
  \/ RootFail
  \/ RootReload
  \/ Detach
  \/ LeaveSpace
  \/ \E p \in People : Retry(p) \/ Open(p) \/ Navigate(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* Tiles found stay drawn, and pressable with previews, while the watch
\* reopens.
FoundStaysPressable == Kept /\ found /\ Previews => "open" \in Shown("person")
\* A failure takes the body only with no index held.
FailureOnlyBeforeIndex == Body = "failed" => ~Kept
\* The tiles drawn are the current folder's.
TilesOfCurrentPath == Kept => indexPath = path
TypeOK ==
  /\ path \in Paths
  /\ watch \in {"waiting", "watching", "failed", "reconnecting"}
  /\ indexPath \in Paths \cup {"none"}
  /\ found \in BOOLEAN
  /\ truncated \in BOOLEAN
  /\ issues \in BOOLEAN
  /\ session \in BOOLEAN
  /\ space \in BOOLEAN
====
