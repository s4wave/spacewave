----------------------------- MODULE GiteaTest -----------------------------
EXTENDS Naturals, TLC
\* Acceptance boundary exercised by TestGiteaQEMUFlowgraph; SQL protocol is modeled separately.
\* One issue and one Git commit are recorded independently, then a
\* bounded Forge build produces an artifact. There is no
\* cross-store transaction. ReplaceVM selects restart or replacement acceptance.
\* Stop is confirmed before a clean restart; this does not model power loss,
\* SQL dialects, Git fsync behavior, network authentication or runtime admission.
CONSTANTS ReplaceVM, PreserveBindings, RequireBuild
VARIABLES confirmed, phase, vm, sqlID, filesID, configID, issue, commit, recorded, built
vars == <<confirmed, phase, vm, sqlID, filesID, configID, issue, commit, recorded, built>>
Init == /\ confirmed = FALSE /\ phase = "off" /\ vm = 0 /\ sqlID = 1 /\ filesID = 1 /\ configID = 1
        /\ issue = FALSE /\ commit = FALSE /\ recorded = FALSE /\ built = FALSE
Start == /\ phase = "off" /\ vm = 0 /\ phase' = "booting" /\ vm' = 1
         /\ UNCHANGED <<confirmed, sqlID, filesID, configID, issue, commit, recorded, built>>
Attach == /\ phase = "booting" /\ phase' = "attached"
          /\ IF vm = 2 /\ ~PreserveBindings
                THEN /\ sqlID' = 2 /\ filesID' = 2 /\ configID' = 2 /\ issue' = FALSE /\ commit' = FALSE
                ELSE UNCHANGED <<confirmed, sqlID, filesID, configID, issue, commit>>
          /\ UNCHANGED <<confirmed, vm, recorded, built>>
Serve == /\ phase = "attached" /\ phase' = "running"
         /\ UNCHANGED <<confirmed, vm, sqlID, filesID, configID, issue, commit, recorded, built>>
BeginWork == /\ phase = "running" /\ ~recorded /\ phase' = "working"
             /\ UNCHANGED <<confirmed, vm, sqlID, filesID, configID, issue, commit, recorded, built>>
CreateIssue == /\ phase = "working" /\ ~issue /\ issue' = TRUE
               /\ UNCHANGED <<confirmed, phase, vm, sqlID, filesID, configID, commit, recorded, built>>
PushCommit == /\ phase = "working" /\ ~commit /\ commit' = TRUE
              /\ UNCHANGED <<confirmed, phase, vm, sqlID, filesID, configID, issue, recorded, built>>
RecordResults == /\ phase = "working" /\ issue /\ commit
                 /\ phase' = "checked" /\ recorded' = TRUE
                 /\ UNCHANGED <<confirmed, vm, sqlID, filesID, configID, issue, commit, built>>
BuildRepo == /\ phase = "checked" /\ ~built /\ built' = TRUE
             /\ UNCHANGED <<confirmed, phase, vm, sqlID, filesID, configID, issue, commit, recorded>>
Stop == /\ phase = "checked" /\ (RequireBuild => built) /\ phase' = "stopping"
        /\ UNCHANGED <<confirmed, vm, sqlID, filesID, configID, issue, commit, recorded, built>>
StopUnavailable == /\ phase = "stopping" /\ phase' = "stop-pending"
                   /\ UNCHANGED <<confirmed, vm, sqlID, filesID, configID, issue, commit, recorded, built>>
RetryStop == /\ phase = "stop-pending" /\ phase' = "stopping"
             /\ UNCHANGED <<confirmed, vm, sqlID, filesID, configID, issue, commit, recorded, built>>
ConfirmStop == /\ phase = "stopping" /\ phase' = "off" /\ confirmed' = TRUE
               /\ UNCHANGED << vm, sqlID, filesID, configID, issue, commit, recorded, built>>
Restart == /\ phase = "off" /\ recorded /\ ~ReplaceVM /\ phase' = "booting"
           /\ UNCHANGED <<confirmed, vm, sqlID, filesID, configID, issue, commit, recorded, built>>
Replace == /\ phase = "off" /\ recorded /\ ReplaceVM /\ phase' = "booting" /\ vm' = 2
           /\ UNCHANGED <<confirmed, sqlID, filesID, configID, issue, commit, recorded, built>>
Verify == /\ phase = "running" /\ recorded /\ issue /\ commit
          /\ sqlID = 1 /\ filesID = 1 /\ phase' = "recovered"
          /\ UNCHANGED <<confirmed, vm, sqlID, filesID, configID, issue, commit, recorded, built>>
Done == /\ phase = "recovered" /\ UNCHANGED vars
Next == Start \/ Attach \/ Serve \/ BeginWork \/ CreateIssue \/ PushCommit \/
        RecordResults \/ BuildRepo \/ Stop \/ StopUnavailable \/ RetryStop \/ ConfirmStop \/ Restart \/ Replace \/ Verify \/ Done
TypeOK == /\ confirmed \in BOOLEAN /\ phase \in {"off", "booting", "attached", "running", "working", "checked", "stopping", "stop-pending", "recovered"}
          /\ vm \in 0..2 /\ sqlID \in 1..2 /\ filesID \in 1..2 /\ configID \in 1..2
          /\ issue \in BOOLEAN /\ commit \in BOOLEAN /\ recorded \in BOOLEAN /\ built \in BOOLEAN
ReplacedAfterStop == vm = 2 => confirmed
SameAttachedObjects == sqlID = 1 /\ filesID = 1 /\ configID = 1
CompletedHasBuild == phase = "recovered" => built
RecordedWorkRemains == recorded => issue /\ commit
=============================================================================
