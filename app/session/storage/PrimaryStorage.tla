------------------------ MODULE PrimaryStorage ------------------------
EXTENDS FiniteSets, TLC
\* NewSpaceStorageChoice, SpaceStorageCard, SpaceStorageMove and the default
\* action in StorageBackendRow read the same backend-location capability.
\* Adding a Drive connection must never make it a primary-storage choice.
Backends == {"bucket", "drive"}
VARIABLES available, chosen, defaultBackend
vars == <<available, chosen, defaultBackend>>
PrimaryChoices == available \ {"drive"}
Init == /\ available = {"bucket"} /\ chosen = "account" /\ defaultBackend = "account"
ConnectDrive == /\ "drive" \notin available /\ available' = available \cup {"drive"}
                /\ UNCHANGED <<chosen, defaultBackend>>
Choose(b) == /\ b \in PrimaryChoices /\ chosen' = b
             /\ UNCHANGED <<available, defaultBackend>>
UseDefault(b) == /\ b \in PrimaryChoices /\ defaultBackend' = b
                 /\ UNCHANGED <<available, chosen>>
UseAccount == /\ chosen' = "account" /\ UNCHANGED <<available, defaultBackend>>
Next == ConnectDrive \/ (\E b \in Backends: Choose(b) \/ UseDefault(b)) \/ UseAccount
TypeOK == /\ available \subseteq Backends
          /\ chosen \in Backends \cup {"account"} /\ defaultBackend \in Backends \cup {"account"}
NoBackupAsPrimary == chosen # "drive" /\ defaultBackend # "drive"
Spec == Init /\ [][Next]_vars
=============================================================================
