---- MODULE DeviceDrive ----
\* What the file browser draws for a folder served live from a Device: the
\* object handle, the Device's presence, and the folder's root.
\*
\* The folder is a checkout-root node placed on the Device; its controller
\* serves the folder over Bifrost to Session peers of the account, and the
\* node's access setting decides on the Device whether writes are taken at
\* all. The browser lists and opens files once the root is ready. On a
\* read-write folder it offers Enable editing; editing tools appear only once
\* the person pressed it and the root is ready, and stay pressed across a
\* reconnect. A Device that goes offline is a wait that reconnects on its
\* own; a refused or broken root is a failure with Retry.

CONSTANT Writable \* the node's access setting is read-write

People == {"person"}

Buttons == {"retryHandle", "retryRoot", "open", "enableEditing", "edit"}

VARIABLES
  handle,  \* the object handle: waiting, open or failed
  device,  \* the Device's presence: none until the handle opens
  root,    \* the served folder's root: none while there is nothing to reach
  editing  \* the person pressed Enable editing in this viewer
vars == <<handle, device, root, editing>>

Ready == root = "ready"

Shown(p) ==
  (IF handle = "failed" THEN {"retryHandle"} ELSE {})
    \cup (IF root = "failed" THEN {"retryRoot"} ELSE {})
    \cup (IF Ready THEN {"open"} ELSE {})
    \cup (IF Ready /\ Writable /\ ~editing THEN {"enableEditing"} ELSE {})
    \cup (IF Ready /\ Writable /\ editing THEN {"edit"} ELSE {})

\* The Device accepts a write only on a read-write folder; the viewer offers
\* one only after Enable editing.
Allowed(p, b) ==
  \/ b = "retryHandle" /\ handle = "failed"
  \/ b = "retryRoot" /\ device = "online" /\ root = "failed"
  \/ b = "open" /\ Ready
  \/ b = "enableEditing" /\ Ready /\ Writable /\ ~editing
  \/ b = "edit" /\ Ready /\ Writable /\ editing

Hint(p) ==
  CASE handle = "waiting" -> "Opening"
    [] handle = "open" /\ device = "offline" -> "Device offline; reconnects when it returns"
    [] root = "connecting" -> "Connecting to the Device"
    [] OTHER -> ""

Init ==
  /\ handle = "waiting"
  /\ device = "none"
  /\ root = "none"
  /\ editing = FALSE

HandleOpen ==
  /\ handle = "waiting"
  /\ handle' = "open"
  /\ device' \in {"offline", "online"}
  /\ root' = IF device' = "online" THEN "connecting" ELSE "none"
  /\ UNCHANGED editing
\* The handle fails; what ran on it stops and the approval ends with it.
HandleFail ==
  /\ handle \in {"waiting", "open"}
  /\ handle' = "failed"
  /\ device' = "none"
  /\ root' = "none"
  /\ editing' = FALSE
RetryHandle(p) ==
  /\ "retryHandle" \in Shown(p)
  /\ handle' = "waiting"
  /\ UNCHANGED <<device, root, editing>>
DeviceLeave ==
  /\ device = "online"
  /\ device' = "offline"
  /\ root' = "none"
  /\ UNCHANGED <<handle, editing>>
DeviceReturn ==
  /\ handle = "open"
  /\ device = "offline"
  /\ device' = "online"
  /\ root' = "connecting"
  /\ UNCHANGED <<handle, editing>>
\* The root opens or is refused, and a ready root can break.
RootStep ==
  /\ device = "online"
  /\ \/ root = "connecting" /\ root' \in {"ready", "failed"}
     \/ root = "ready" /\ root' = "failed"
  /\ UNCHANGED <<handle, device, editing>>
RetryRoot(p) ==
  /\ "retryRoot" \in Shown(p)
  /\ root' = "connecting"
  /\ UNCHANGED <<handle, device, editing>>
EnableEditing(p) ==
  /\ "enableEditing" \in Shown(p)
  /\ editing' = TRUE
  /\ UNCHANGED <<handle, device, root>>
\* Opening a file and an edit change nothing this machine draws.
Use(p) ==
  /\ \E b \in {"open", "edit"} : b \in Shown(p)
  /\ UNCHANGED vars

Next ==
  \/ HandleOpen
  \/ HandleFail
  \/ DeviceLeave
  \/ DeviceReturn
  \/ RootStep
  \/ \E p \in People :
       RetryHandle(p) \/ RetryRoot(p) \/ EnableEditing(p) \/ Use(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

SourcesNeedHandle == handle # "open" => device = "none" /\ root = "none"
NoRootWhileOffline == device # "online" => root = "none"
\* Editing tools appear only on a ready read-write folder after the press.
EditOnlyApproved == \A p \in People :
  "edit" \in Shown(p) => Writable /\ editing /\ Ready
TypeOK ==
  /\ handle \in {"waiting", "open", "failed"}
  /\ device \in {"none", "offline", "online"}
  /\ root \in {"none", "connecting", "ready", "failed"}
  /\ editing \in BOOLEAN
====
