------------------------------ MODULE OSEnroll ------------------------------
EXTENDS Naturals, TLC
\* First boot of an installed card: pin the card's accepted signers, verify the
\* selected OSRootManifest against the pinned set before serving the root, make
\* the Device key, and redeem the card's bootstrap grant (a bearer Space invite
\* that requires approval) until the Space admits the key or revokes the grant.
\* A power cut may end any boot; the key, the pinned signers and the installed
\* grant are durable in the data Volume, everything else is lost.
\* Once the signers are pinned, an attacker may rewrite the card's signers and
\* manifest; a card rewritten before the pin is physical control of a machine
\* that trusts nothing yet, and out of scope.
\* Bounds: one card, one Space owner, MaxBoots boots.
CONSTANT MaxBoots
VARIABLES phase, boots, net, cardSigner, manifestSigner, tampered, pinned,
          key, keyGen, enroll, installed, grant, queued, admitted
vars == <<phase, boots, net, cardSigner, manifestSigner, tampered, pinned,
          key, keyGen, enroll, installed, grant, queued, admitted>>

Signers == {"owner", "other"}

Init == /\ phase = "off" /\ boots = 0 /\ net = FALSE
        /\ cardSigner = "owner" /\ manifestSigner = "owner" /\ tampered = FALSE
        /\ pinned = "none" /\ key = "none" /\ keyGen = 0
        /\ enroll = "idle" /\ installed = FALSE
        /\ grant = "open" /\ queued = 0 /\ admitted = 0

\* Boot starts the check; an installed grant means the Device is enrolled.
Boot == /\ phase = "off" /\ boots < MaxBoots
        /\ phase' = "checking" /\ boots' = boots + 1
        /\ enroll' = IF installed THEN "enrolled" ELSE "idle"
        /\ UNCHANGED <<net, cardSigner, manifestSigner, tampered, pinned, key,
                       keyGen, installed, grant, queued, admitted>>

\* First boot pins the card's signers into the Volume.
Pin == /\ phase = "checking" /\ pinned = "none"
       /\ pinned' = cardSigner
       /\ UNCHANGED <<phase, boots, net, cardSigner, manifestSigner, tampered,
                      key, keyGen, enroll, installed, grant, queued, admitted>>

Verify == /\ phase = "checking" /\ pinned # "none"
          /\ manifestSigner = pinned
          /\ phase' = "serving"
          /\ UNCHANGED <<boots, net, cardSigner, manifestSigner, tampered,
                         pinned, key, keyGen, enroll, installed, grant, queued,
                         admitted>>

RefuseManifest == /\ phase = "checking" /\ pinned # "none"
                  /\ manifestSigner # pinned
                  /\ phase' = "refused"
                  /\ UNCHANGED <<boots, net, cardSigner, manifestSigner,
                                 tampered, pinned, key, keyGen, enroll,
                                 installed, grant, queued, admitted>>

\* The daemon makes the Device key in the Volume.
MakeKey == /\ phase = "serving" /\ key = "none"
           /\ key' = "volume" /\ keyGen' = keyGen + 1
           /\ UNCHANGED <<phase, boots, net, cardSigner, manifestSigner,
                          tampered, pinned, enroll, installed, grant, queued,
                          admitted>>

\* Redeem the grant over the network with the current key. The Space answers
\* with the grant if it admitted this key, queues the key for approval while
\* the grant is open and nothing was admitted, and refuses otherwise.
Redeem == /\ phase = "serving" /\ net /\ key # "none" /\ ~installed
          /\ enroll \in {"idle", "pending"}
          /\ \/ /\ admitted = keyGen
                /\ enroll' = "granted"
                /\ UNCHANGED queued
             \/ /\ admitted = 0 /\ grant = "open"
                /\ enroll' = "pending" /\ queued' = keyGen
             \/ /\ admitted # keyGen
                /\ (admitted # 0 \/ grant = "revoked")
                /\ enroll' = "refused" /\ UNCHANGED queued
          /\ UNCHANGED <<phase, boots, net, cardSigner, manifestSigner,
                         tampered, pinned, key, keyGen, installed, grant,
                         admitted>>

\* The Device installs the returned grant in its account.
Install == /\ phase = "serving" /\ enroll = "granted"
           /\ installed' = TRUE /\ enroll' = "enrolled"
           /\ UNCHANGED <<phase, boots, net, cardSigner, manifestSigner,
                          tampered, pinned, key, keyGen, grant, queued,
                          admitted>>

\* The owner, or the grant's policy, approves the queued key.
Approve == /\ queued # 0 /\ admitted = 0 /\ grant = "open"
           /\ admitted' = queued /\ queued' = 0
           /\ UNCHANGED <<phase, boots, net, cardSigner, manifestSigner,
                          tampered, pinned, key, keyGen, enroll, installed,
                          grant>>

\* The owner revokes the grant; an admitted key stays admitted.
Revoke == /\ grant = "open" /\ grant' = "revoked" /\ queued' = 0
          /\ UNCHANGED <<phase, boots, net, cardSigner, manifestSigner,
                         tampered, pinned, key, keyGen, enroll, installed,
                         admitted>>

NetUp == /\ ~net /\ net' = TRUE
         /\ UNCHANGED <<phase, boots, cardSigner, manifestSigner, tampered,
                        pinned, key, keyGen, enroll, installed, grant, queued,
                        admitted>>

NetDown == /\ net /\ net' = FALSE
           /\ UNCHANGED <<phase, boots, cardSigner, manifestSigner, tampered,
                          pinned, key, keyGen, enroll, installed, grant, queued,
                          admitted>>

\* A power cut loses everything outside the Volume. The last boot is not cut,
\* so liveness asks what a boot that runs does.
PowerCut == /\ phase # "off" /\ boots < MaxBoots
            /\ phase' = "off"
            /\ key' = IF key = "memory" THEN "none" ELSE key
            /\ enroll' = "idle"
            /\ UNCHANGED <<boots, net, cardSigner, manifestSigner, tampered,
                           pinned, keyGen, installed, grant, queued, admitted>>

\* After the pin, an attacker rewrites the card's signers and manifest.
Tamper == /\ phase = "off" /\ pinned # "none" /\ ~tampered
          /\ tampered' = TRUE
          /\ cardSigner' = "other" /\ manifestSigner' = "other"
          /\ UNCHANGED <<phase, boots, net, pinned, key, keyGen, enroll,
                         installed, grant, queued, admitted>>

Next == Boot \/ Pin \/ Verify \/ RefuseManifest \/ MakeKey \/
        Redeem \/ Install \/ Approve \/ Revoke \/ NetUp \/ NetDown \/
        PowerCut \/ Tamper

Spec == /\ Init /\ [][Next]_vars
        /\ WF_vars(Boot) /\ WF_vars(Pin) /\ WF_vars(Verify)
        /\ WF_vars(RefuseManifest) /\ WF_vars(MakeKey) /\ SF_vars(Redeem)
        /\ WF_vars(Install) /\ WF_vars(Approve) /\ WF_vars(NetUp)

TypeOK == /\ phase \in {"off", "checking", "serving", "refused"}
          /\ pinned \in Signers \cup {"none"}
          /\ key \in {"none", "volume", "memory", "card"}
          /\ enroll \in {"idle", "pending", "granted", "enrolled", "refused"}
          /\ grant \in {"open", "revoked"}

\* The card never carries a Device key.
KeyNeverOnCard == key # "card"

\* A served root is one the owner signed, checked against the pinned signers.
ServesOwnerImage == phase = "serving" => pinned = "owner" /\ manifestSigner = "owner"

\* The Space never admits a key the Device has lost.
AdmittedKeyHeld == admitted # 0 => admitted = keyGen /\ key # "none"

\* The Device gives up only on a grant the Space revoked before admitting it.
RefusedOnlyByRevoke == enroll = "refused" => grant = "revoked" /\ admitted = 0

\* A boot that runs ends enrolled, refused by revocation, or refused a manifest.
EventuallySettled == <>[](installed \/ grant = "revoked" \/ phase = "refused")
=============================================================================
