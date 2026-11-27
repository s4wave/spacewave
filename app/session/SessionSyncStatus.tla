---- MODULE SessionSyncStatus ----
\* The status popover observes independent sources. The model bounds transfer
\* data to idle, active and failed; counts and rates do not affect ownership.
\* Local accounts use peer replication progress; cloud accounts keep the cloud
\* summary even when their direct path fails. Opening the popover is read-only.
EXTENDS Naturals
CONSTANT Local
VARIABLES cloud, access, peer, replication, open, cloudSeen, peerSeen
vars == <<cloud, access, peer, replication, open, cloudSeen, peerSeen>>
TransferStates == {"synced", "active", "error"}
AccessStates == {"unknown", "pending", "ready", "dormant", "failed", "deleted", "unauthenticated"}
PeerStates == {"unknown", "disabled", "starting", "noPeers", "idle", "active", "fallback", "error"}
Summary == IF Local THEN replication ELSE cloud
Shown(p) == {"toggle"}
Allowed(p, a) == a = "toggle"
Hint(p) == ""
Buttons == {"toggle"}
People == {"person"}
S == INSTANCE Surface WITH Actions <- Buttons, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck
Init ==
  /\ cloud = "synced" /\ access = "unknown" /\ peer = "unknown"
  /\ replication = "synced" /\ open = FALSE
  /\ cloudSeen = cloud /\ peerSeen = peer
CloudChanged ==
  /\ ~Local
  /\ \E c \in TransferStates : cloud' = c /\ cloudSeen' = c
  /\ UNCHANGED <<access, peer, replication, open, peerSeen>>
AccessChanged ==
  /\ ~Local
  /\ access' \in AccessStates
  /\ UNCHANGED <<cloud, peer, replication, open, cloudSeen, peerSeen>>
PeerChanged ==
  /\ \E p \in PeerStates : peer' = p /\ peerSeen' = p
  /\ UNCHANGED <<cloud, access, replication, open, cloudSeen>>
ReplicationChanged ==
  /\ Local
  /\ replication' \in TransferStates
  /\ UNCHANGED <<cloud, access, peer, open, cloudSeen, peerSeen>>
Toggle == /\ open' = ~open /\ UNCHANGED <<cloud, access, peer, replication, cloudSeen, peerSeen>>
Next == CloudChanged \/ AccessChanged \/ PeerChanged \/ ReplicationChanged \/ Toggle
CloudOwnsProgress == cloud = cloudSeen
PeerOwnsFailure == peer = peerSeen
CloudSummaryIndependent == ~Local => Summary = cloudSeen
====
