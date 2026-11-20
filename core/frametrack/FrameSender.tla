---- MODULE FrameSender ----
\* How a Device sends a live frame stream to one viewer, in the shape of Media
\* over QUIC (moq-lite): a track is a sequence of groups, each group starts at
\* a keyframe, and each group travels on its own reliable, ordered Bifrost
\* stream. Any prefix of a group decodes, so the sender gets loss tolerance
\* from where it cuts a group, not from acknowledgements.
\*
\* The sender queues at most Window frames in the open group. A new keyframe
\* opens a new group and resets the old group's stream, discarding its unsent
\* tail: the viewer wants the newest picture, which the keyframe carries. When
\* the queue is full the sender drops the frame and ends the group, so no gap
\* ever opens inside a group; it then sends nothing until the next keyframe.
\* A source that encodes (the test pattern, the display's GPU encoder, a Pi's
\* hardware encoder) is asked for one at once; a source that passes a
\* camera's own stream through waits for the camera's next keyframe. A JPEG
\* stream is all keyframes, so every JPEG frame is its own group.

EXTENDS Naturals, Sequences

CONSTANTS
  Window, \* frames the open group may queue unsent
  MaxSeq, \* frames the source makes in one model run
  Encoder \* the source can make a keyframe on request

VARIABLES
  frames,    \* the kind of each frame the source has made, in order
  queue,     \* the open group's frames not yet written, oldest first
  open,      \* a group is open and takes delta frames
  delivered, \* the frames the viewer received
  keyAsked   \* the sender asked the source for a keyframe
vars == <<frames, queue, open, delivered, keyAsked>>

Kinds == {"key", "delta"}

Init ==
  /\ frames = <<>>
  /\ queue = <<>>
  /\ open = FALSE
  /\ delivered = {}
  /\ keyAsked = FALSE

\* The source makes the next frame, a keyframe when one was asked for.
Produce(kind) ==
  /\ Len(frames) < MaxSeq
  /\ keyAsked => kind = "key"
  /\ frames' = Append(frames, kind)
  /\ UNCHANGED delivered
  /\ CASE kind = "key" ->
            \* A new group; the old group's unsent tail is reset.
            /\ queue' = <<Len(frames')>>
            /\ open' = TRUE
            /\ keyAsked' = FALSE
       [] open /\ Len(queue) < Window ->
            /\ queue' = Append(queue, Len(frames'))
            /\ UNCHANGED <<open, keyAsked>>
       [] OTHER ->
            \* No open group, or the viewer is behind: drop the frame, end
            \* the group after what is queued, and wait for a keyframe.
            /\ open' = FALSE
            /\ keyAsked' = Encoder
            /\ UNCHANGED queue

\* The stream writes the oldest queued frame and the viewer receives it.
Write ==
  /\ queue # <<>>
  /\ delivered' = delivered \cup {Head(queue)}
  /\ queue' = Tail(queue)
  /\ UNCHANGED <<frames, open, keyAsked>>

\* The model run ends once the source is done and the queue is written.
Done ==
  /\ Len(frames) = MaxSeq
  /\ queue = <<>>
  /\ UNCHANGED vars

Next == (\E k \in Kinds : Produce(k)) \/ Write \/ Done

\* The keyframe a frame n decodes from: the last one at or before n.
LastKey(n) ==
  CHOOSE k \in 1..n :
    /\ frames[k] = "key"
    /\ \A j \in (k + 1)..n : frames[j] # "key"

\* Every frame the viewer receives decodes: it follows a keyframe, and every
\* frame from that keyframe to it was delivered.
DecodeComplete ==
  \A n \in delivered :
    /\ \E k \in 1..n : frames[k] = "key"
    /\ \A m \in LastKey(n)..n : m \in delivered

\* The viewer is never more than the window behind the source.
QueueBounded == Len(queue) <= Window

TypeOK ==
  /\ frames \in Seq(Kinds)
  /\ queue \in Seq(1..MaxSeq)
  /\ open \in BOOLEAN
  /\ delivered \subseteq 1..MaxSeq
  /\ keyAsked \in BOOLEAN
====
