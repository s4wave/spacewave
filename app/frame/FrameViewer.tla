---- MODULE FrameViewer ----
\* What the camera and display viewers draw from their three sources: the
\* object handle, the Device's presence, and the frame track. Both viewers
\* share this machine and one decoder; the display adds input.
\*
\* The handle opens first, and the Device watch and the track run only on an
\* open handle. A Device that goes offline is a wait, not a failure: the track
\* stops, the body says the Device is offline, and the track reconnects on its
\* own when the Device returns. A track that opens waits for its first
\* keyframe before it draws. A live track that stops delivering frames holds
\* its last picture and says it is paused until frames resume. Only a refusal
\* or a broken track is a failure, with a Retry that reopens the track.

CONSTANT Input \* the viewer sends pointer and keyboard input (the display)

People == {"person"}

Buttons == {"retryHandle", "retryTrack", "control"}

VARIABLES
  handle, \* the object handle: waiting, open or failed
  device, \* the Device's presence: none until the handle opens
  track   \* the frame track: none while there is nothing to connect to
vars == <<handle, device, track>>

Shown(p) ==
  (IF handle = "failed" THEN {"retryHandle"} ELSE {})
    \cup (IF track = "failed" THEN {"retryTrack"} ELSE {})
    \cup (IF Input /\ track = "live" THEN {"control"} ELSE {})

Allowed(p, b) ==
  \/ b = "retryHandle" /\ handle = "failed"
  \/ b = "retryTrack" /\ device = "online" /\ track = "failed"
  \/ b = "control" /\ Input /\ track = "live"

\* What a state with nothing to press tells the person.
Hint(p) ==
  CASE handle = "waiting" -> "Opening"
    [] handle = "open" /\ device = "offline" -> "Device offline; reconnects when it returns"
    [] track = "connecting" -> "Connecting to the Device"
    [] track = "starting" -> "Waiting for the first picture"
    [] track = "live" -> "The live picture"
    [] track = "stalled" -> "Paused: last picture held until frames resume"
    [] OTHER -> ""

Init ==
  /\ handle = "waiting"
  /\ device = "none"
  /\ track = "none"

\* The handle opens and the Device watch reports its first presence; an
\* online Device's track starts connecting.
HandleOpen ==
  /\ handle = "waiting"
  /\ handle' = "open"
  /\ device' \in {"offline", "online"}
  /\ track' = IF device' = "online" THEN "connecting" ELSE "none"
\* The handle fails, before or after it opened; what ran on it stops.
HandleFail ==
  /\ handle \in {"waiting", "open"}
  /\ handle' = "failed"
  /\ device' = "none"
  /\ track' = "none"
RetryHandle(p) ==
  /\ "retryHandle" \in Shown(p)
  /\ handle' = "waiting"
  /\ UNCHANGED <<device, track>>
\* The Device leaves, stopping the track, or returns, reconnecting it.
DeviceLeave ==
  /\ device = "online"
  /\ device' = "offline"
  /\ track' = "none"
  /\ UNCHANGED handle
DeviceReturn ==
  /\ handle = "open"
  /\ device = "offline"
  /\ device' = "online"
  /\ track' = "connecting"
  /\ UNCHANGED handle
\* The track opens, draws its first keyframe, stalls and resumes, or fails.
TrackStep ==
  /\ device = "online"
  /\ \/ track = "connecting" /\ track' \in {"starting", "failed"}
     \/ track = "starting" /\ track' \in {"live", "failed"}
     \/ track = "live" /\ track' \in {"stalled", "failed"}
     \/ track = "stalled" /\ track' \in {"live", "failed"}
  /\ UNCHANGED <<handle, device>>
RetryTrack(p) ==
  /\ "retryTrack" \in Shown(p)
  /\ track' = "connecting"
  /\ UNCHANGED <<handle, device>>
\* Input reaches a live display and changes nothing the viewer draws.
Control(p) ==
  /\ "control" \in Shown(p)
  /\ UNCHANGED vars

Next ==
  \/ HandleOpen
  \/ HandleFail
  \/ DeviceLeave
  \/ DeviceReturn
  \/ TrackStep
  \/ \E p \in People : RetryHandle(p) \/ RetryTrack(p) \/ Control(p)

S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* The Device watch and the track run only on an open handle.
SourcesNeedHandle == handle # "open" => device = "none" /\ track = "none"
\* No track runs to a Device that is not online.
NoTrackWhileOffline == device # "online" => track = "none"
\* Input goes only to a live picture, and never to a camera.
InputOnlyLive == \A p \in People :
  "control" \in Shown(p) => Input /\ track = "live"
TypeOK ==
  /\ handle \in {"waiting", "open", "failed"}
  /\ device \in {"none", "offline", "online"}
  /\ track \in {"none", "connecting", "starting", "live", "stalled", "failed"}
====
