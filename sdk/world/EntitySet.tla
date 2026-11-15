---- MODULE EntitySet ----
\* watchKeyedSet in entity-set.ts follows a set of World objects: the members
\* a link joins to an origin, and each member's body. A full read lists the
\* members and reads every body at one revision R, then follows the link
\* pattern and each member's key on the Engine's shared change stream. Each
\* wake routes the queued events, reads again at one new revision only what
\* they touched, and emits the set at that revision.
\*
\* The World commits one change at a time: a link added or removed, or a body
\* rewritten. The server sends a change only for names it follows when it
\* passes the change, so a member linked after the stream opened is followed
\* only from the ack of its delta, whose seqno L may be past the revision the
\* member was read at; the ack rule reads it again. A gap the stream cannot
\* fill collapses the queue into one unknown change, which reads the whole set
\* again. A failed stream drops the subscription; the retry starts over.
\*
\* A full read with no subscription opens a stream at R that follows its
\* names at once, or joins one another subscriber opened. A joined stream
\* may pass commits after R before it acks the set's names, the link
\* pattern L among them, so the ack relists when it is past the listing.
\*
\* States of the follower: none (no value), atR (the set at R is emitted),
\* taking (routing a burst), gap (a full read is owed), failed.
\*
\* EmittedAtR: every member or body the emitted set holds that differs from
\* the World at R is owed a read by an event queued or still to be sent.
\* QuiescentMatchesWorld: once the stream has sent and the follower has taken
\* every change and ack, the set is the World as it is now.
EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS Keys, MaxSeq

\* Each switch is one rule of watchKeyedSet; the mutants turn one off.
AckRereads == TRUE
UnknownRereads == TRUE
SkipAtReadAt == TRUE
RelistOnLink == TRUE
RelistOnFollowed == TRUE

\* L names the link pattern among the names a stream follows.
L == "L"

VARIABLES
    wseq, hist, log,
    subscribed, sent, srvFollow, srvPending, inbox, gaps, fails,
    cstate, members, rows, readAt, listedAt, R,
    unfollowed, clientPending, dirty, relist, listUnfollowed

world == <<wseq, hist, log>>
server == <<subscribed, sent, srvFollow, srvPending, inbox, gaps, fails>>
client == <<cstate, members, rows, readAt, listedAt, R,
            unfollowed, clientPending, dirty, relist, listUnfollowed>>
vars == <<world, server, client>>

NoKey == CHOOSE k \in Keys : TRUE
Event(kind, key, seq, keys) ==
    [kind |-> kind, key |-> key, seq |-> seq, keys |-> keys]

Init ==
    /\ wseq = 0
    /\ \E m0 \in SUBSET Keys :
         hist = [s \in 0..MaxSeq |-> [mem |-> m0, body |-> [k \in Keys |-> 0]]]
    /\ log = [s \in 1..MaxSeq |-> [kind |-> "none", key |-> NoKey]]
    /\ subscribed = FALSE /\ sent = 0 /\ srvFollow = {} /\ srvPending = {}
    /\ inbox = <<>> /\ gaps = 0 /\ fails = 0
    /\ cstate = "none" /\ members = {} /\ rows = [k \in Keys |-> 0]
    /\ readAt = [k \in Keys |-> 0] /\ listedAt = 0 /\ R = 0
    /\ unfollowed = {} /\ clientPending = {} /\ dirty = {} /\ relist = FALSE
    /\ listUnfollowed = FALSE

\* ---- World ----

\* Commit writes one change: a link toggled or a body rewritten. Later
\* entries of hist track the newest World, so hist[s] is the World at s.
Commit(k, kind) ==
    /\ wseq < MaxSeq
    /\ LET cur == hist[wseq]
           next == IF kind = "link"
                   THEN [cur EXCEPT !.mem = IF k \in cur.mem
                                            THEN cur.mem \ {k}
                                            ELSE cur.mem \cup {k}]
                   ELSE [cur EXCEPT !.body[k] = wseq + 1]
       IN hist' = [s \in 0..MaxSeq |-> IF s > wseq THEN next ELSE hist[s]]
    /\ log' = [log EXCEPT ![wseq + 1] = [kind |-> kind, key |-> k]]
    /\ wseq' = wseq + 1
    /\ UNCHANGED <<server, client>>

\* ---- Server ----

\* ServerSend passes the next commit, sending it when it names a followed
\* name: a link needs the pattern L; a body needs its key.
ServerSend ==
    /\ subscribed /\ sent < wseq
    /\ LET s == sent + 1
           c == log[s]
       IN inbox' = IF (c.kind = "link" /\ L \in srvFollow)
                      \/ (c.kind = "body" /\ c.key \in srvFollow)
                   THEN Append(inbox, Event(c.kind, c.key, s, {}))
                   ELSE inbox
    /\ sent' = sent + 1
    /\ UNCHANGED <<world, client, subscribed, srvFollow, srvPending, gaps, fails>>

\* ServerAck applies the pending delta and acks it at the seqno it passed.
ServerAck ==
    /\ subscribed /\ srvPending # {}
    /\ srvFollow' = srvFollow \cup srvPending
    /\ srvPending' = {}
    /\ inbox' = Append(inbox, Event("followed", NoKey, sent, srvPending))
    /\ UNCHANGED <<world, client, subscribed, sent, gaps, fails>>

\* ServerGap collapses the queue and the rest of the commits into one
\* unknown change, applying a pending delta whose ack it drops.
ServerGap ==
    /\ subscribed /\ gaps < 1
    /\ srvFollow' = srvFollow \cup srvPending
    /\ srvPending' = {}
    /\ inbox' = <<Event("unknown", NoKey, wseq, srvPending)>>
    /\ sent' = wseq
    /\ gaps' = gaps + 1
    /\ UNCHANGED <<world, client, subscribed, fails>>

\* Fail ends the stream; the follower holds no subscription.
Fail ==
    /\ subscribed /\ fails < 1
    /\ subscribed' = FALSE /\ inbox' = <<>> /\ srvFollow' = {}
    /\ srvPending' = {} /\ fails' = fails + 1
    /\ cstate' = "failed"
    /\ UNCHANGED <<world, sent, gaps, members, rows, readAt, listedAt, R,
                   unfollowed, clientPending, dirty, relist, listUnfollowed>>

\* ---- Follower ----

\* Follow sends the delta for added and removed members. An added member
\* waits in unfollowed for the ack that says from which seqno it is sent.
Follow(added, removed) ==
    /\ srvPending' = (srvPending \cup added) \ removed
    /\ srvFollow' = srvFollow \ removed
    /\ unfollowed' = (unfollowed \cup added) \ removed
    /\ clientPending' = (clientPending \cup added) \ removed

\* FullRead lists and reads everything at one revision. Without a stream it
\* opens one from R that follows every member and L, or joins an open one
\* whose ack of those names is still to come; on a stream it sends the
\* delta.
FullRead ==
    /\ cstate \in {"none", "gap", "failed"}
    /\ LET mem == hist[wseq].mem
       IN /\ members' = mem
          /\ IF subscribed
             THEN /\ Follow(mem \ members, members \ mem)
                  /\ UNCHANGED <<subscribed, sent, inbox, listUnfollowed>>
             ELSE /\ subscribed' = TRUE /\ sent' = wseq /\ inbox' = <<>>
                  /\ \/ /\ srvFollow' = mem \cup {L} /\ srvPending' = {}
                        /\ unfollowed' = {} /\ clientPending' = {}
                        /\ listUnfollowed' = FALSE
                     \/ /\ srvFollow' = {} /\ srvPending' = mem \cup {L}
                        /\ unfollowed' = mem /\ clientPending' = mem \cup {L}
                        /\ listUnfollowed' = TRUE
    /\ rows' = hist[wseq].body
    /\ readAt' = [k \in Keys |-> wseq]
    /\ listedAt' = wseq /\ R' = wseq
    /\ dirty' = {} /\ relist' = FALSE
    /\ cstate' = "atR"
    /\ UNCHANGED <<world, gaps, fails>>

\* Take routes the next queued event into the wake.
Take ==
    /\ cstate \in {"atR", "taking"} /\ inbox # <<>>
    /\ LET e == Head(inbox) IN
       IF e.kind = "unknown"
       THEN IF UnknownRereads
            THEN /\ cstate' = "gap" /\ inbox' = <<>>
                 /\ clientPending' = clientPending \ e.keys
                 /\ UNCHANGED <<unfollowed, dirty, relist, listUnfollowed>>
            ELSE /\ cstate' = "taking" /\ inbox' = Tail(inbox)
                 /\ clientPending' = clientPending \ e.keys
                 /\ UNCHANGED <<unfollowed, dirty, relist, listUnfollowed>>
       ELSE
       /\ inbox' = Tail(inbox)
       /\ cstate' = "taking"
       /\ CASE e.kind = "followed" ->
                 LET pend == clientPending \ e.keys IN
                 /\ dirty' = IF AckRereads
                             THEN dirty \cup {k \in unfollowed :
                                                readAt[k] < e.seq}
                             ELSE dirty
                 /\ relist' = (relist \/ (RelistOnFollowed /\ listUnfollowed
                                          /\ e.seq > listedAt))
                 /\ clientPending' = pend
                 /\ unfollowed' = IF pend = {} THEN {} ELSE unfollowed
                 /\ listUnfollowed' = (listUnfollowed /\ pend # {})
            [] e.kind = "link" ->
                 /\ relist' = (relist \/ (RelistOnLink /\ e.seq > listedAt))
                 /\ UNCHANGED <<dirty, unfollowed, clientPending, listUnfollowed>>
            [] OTHER ->
                 /\ dirty' = IF e.key \in members
                                /\ e.seq > (IF SkipAtReadAt
                                            THEN readAt[e.key] ELSE R)
                             THEN dirty \cup {e.key}
                             ELSE dirty
                 /\ UNCHANGED <<relist, unfollowed, clientPending,
                                 listUnfollowed>>
    /\ UNCHANGED <<world, subscribed, sent, srvFollow, srvPending, gaps, fails,
                   members, rows, readAt, listedAt, R>>

\* Read ends the wake: on one revision it lists again if a link moved, reads
\* the touched and added members, and emits the set at that revision.
Read ==
    /\ cstate = "taking"
    /\ IF ~relist /\ dirty = {}
       THEN /\ cstate' = "atR"
            /\ UNCHANGED <<members, rows, readAt, listedAt, R, srvFollow,
                           srvPending, unfollowed, clientPending>>
            /\ UNCHANGED listUnfollowed
       ELSE LET mem == IF relist THEN hist[wseq].mem ELSE members
                added == mem \ members
                removed == members \ mem
                read == (dirty \cup added) \ removed
            IN /\ members' = mem
               /\ rows' = [k \in Keys |-> IF k \in read
                                         THEN hist[wseq].body[k] ELSE rows[k]]
               /\ readAt' = [k \in Keys |-> IF k \in read
                                           THEN wseq ELSE readAt[k]]
               /\ listedAt' = IF relist THEN wseq ELSE listedAt
               /\ R' = wseq
               /\ Follow(added, removed)
               /\ UNCHANGED listUnfollowed
               /\ cstate' = "atR"
    /\ dirty' = {} /\ relist' = FALSE
    /\ UNCHANGED <<world, subscribed, sent, inbox, gaps, fails>>

Next ==
    \/ \E k \in Keys, kind \in {"link", "body"} : Commit(k, kind)
    \/ ServerSend \/ ServerAck \/ ServerGap \/ Fail
    \/ FullRead \/ Take \/ Read

Spec == Init /\ [][Next]_vars

\* ---- Properties ----

Queued(P(_)) == \E i \in 1..Len(inbox) : P(inbox[i])

\* ToSend(kind, k) holds when a commit at or below R of kind on k is still
\* to be sent.
ToSend(kind, k) ==
    \E s \in (sent + 1)..R : log[s].kind = kind /\ (kind = "link" \/ log[s].key = k)

RelistOwed ==
    \/ Queued(LAMBDA e : e.kind \in {"link", "unknown"})
    \/ Queued(LAMBDA e : e.kind = "followed" /\ listUnfollowed
                         /\ e.seq > listedAt)
    \/ (L \in srvFollow /\ ToSend("link", NoKey))
    \/ L \in srvPending

BodyOwed(k) ==
    \/ Queued(LAMBDA e : e.kind = "unknown")
    \/ Queued(LAMBDA e : e.kind = "body" /\ e.key = k /\ e.seq > readAt[k])
    \/ Queued(LAMBDA e : e.kind = "followed" /\ k \in unfollowed
                         /\ readAt[k] < e.seq)
    \/ (k \in srvFollow /\ ToSend("body", k))
    \/ k \in srvPending

EmittedAtR ==
    cstate = "atR" =>
        /\ (members = hist[R].mem \/ RelistOwed)
        /\ \A k \in members : rows[k] = hist[R].body[k] \/ BodyOwed(k)

Quiescent ==
    /\ cstate = "atR" /\ subscribed /\ inbox = <<>>
    /\ sent = wseq /\ srvPending = {}

QuiescentMatchesWorld ==
    Quiescent =>
        /\ members = hist[wseq].mem
        /\ \A k \in members : rows[k] = hist[wseq].body[k]
=============================================================================
