---- MODULE LinkedFolderJourney ----
\* One file in Alice's linked folder, as the people who read it see it. Alice
\* edits the file on her disk; the linking Device indexes each edit and
\* publishes it to the World, and may sleep. Two readers list the file once it
\* is published and fetch its chunks into their block store when they open it:
\* Bob, a member, and Alice herself on another Device of her account, her
\* laptop. A version is served by every holder that has it and can answer:
\* the linking Device while awake and while its disk still holds that version,
\* Spacewave Cloud in a cloud-backed Space once the upload queue sent it, and
\* for Bob, Alice's laptop while awake once it holds the file. With "keep a
\* full copy" on, the laptop's account copy fetches every published version
\* without being asked, so Bob can read with the linking Device asleep. Versions are numbered
\* 1..MaxVer; 0 means none.
EXTENDS Naturals
CONSTANTS
  MaxVer,      \* the versions Alice's disk can reach
  CloudBacked, \* the Space keeps a copy in Spacewave Cloud
  Readers,     \* who reads the file: a subset of {"owner", "member"}
  Faults       \* what may go wrong: a subset of AllFaults

\* reject: the folder's root becomes another file system. hold: the cloud
\* upload queue stops on storage or spending. lapSleeps: Alice's laptop
\* sleeps. evicts: the laptop's store evicts a copy the switch no longer keeps.
AllFaults == {"reject", "hold", "lapSleeps", "evicts"}


VARIABLES
  disk,      \* the version on Alice's disk
  pub,       \* the version published in the World
  online,    \* the linking Device is awake
  linked,    \* its node: indexing (a full sync), watching, or rejected
  cver,      \* the version Spacewave Cloud holds
  hold,      \* why the cloud upload queue stopped: none, storage, spending
  keep,      \* the laptop's "keep a full copy" setting
  lapOnline, \* Alice's laptop is awake
  copy,      \* the version the laptop's account copy holds
  file,      \* each reader's file: none, placeholder, filling, present, unavailable
  want,      \* the published version each reader is fetching
  have       \* the version each reader's block store holds
jvars == <<disk, pub, online, linked, cver, hold, keep, lapOnline, copy, file, want, have>>

Init ==
  /\ disk = 1 /\ pub = 0 /\ online = TRUE /\ linked = "indexing"
  /\ cver = 0 /\ hold = "none" /\ keep = FALSE /\ lapOnline = TRUE /\ copy = 0
  /\ file = [r \in Readers |-> "none"]
  /\ want = [r \in Readers |-> 0]
  /\ have = [r \in Readers |-> 0]

\* The linking Device serves a chunk only while its file still holds that
\* version and the folder is the one it linked.
DeviceServes(v) == online /\ linked # "rejected" /\ disk = v
\* Alice's laptop is a block peer of the Space for Bob, holding the version
\* its account copy fetched or the one Alice opened there.
LaptopServes(v) ==
  /\ lapOnline
  /\ \/ copy = v
     \/ "owner" \in Readers /\ file["owner"] = "present" /\ have["owner"] = v
Serves(r, v) ==
  /\ v > 0
  /\ \/ DeviceServes(v)
     \/ cver = v
     \/ r = "member" /\ LaptopServes(v)
\* A reader acts only while its own Device is awake.
Active(r) == r = "member" \/ lapOnline

ReadersSame == UNCHANGED <<file, want, have>>
LinkSame == UNCHANGED <<disk, pub, online, linked>>
CloudSame == UNCHANGED <<cver, hold>>
LaptopSame == UNCHANGED <<keep, lapOnline, copy>>

\* The linking Device's side.
Edit == online /\ disk < MaxVer /\ disk' = disk + 1
  /\ UNCHANGED <<pub, online, linked>> /\ CloudSame /\ LaptopSame /\ ReadersSame
Index == online /\ linked # "rejected" /\ pub < disk /\ pub' = disk
  /\ UNCHANGED <<disk, online, linked>> /\ CloudSame /\ LaptopSame /\ ReadersSame
\* A full sync ends once everything on disk is published.
Indexed == online /\ linked = "indexing" /\ pub = disk /\ linked' = "watching"
  /\ UNCHANGED <<disk, pub, online>> /\ CloudSame /\ LaptopSame /\ ReadersSame
\* The folder's root is now another file system, as after its disk is replaced.
Reject == "reject" \in Faults /\ online /\ linked # "rejected" /\ linked' = "rejected"
  /\ UNCHANGED <<disk, pub, online>> /\ CloudSame /\ LaptopSame /\ ReadersSame
\* Recording the new root starts a full sync.
Reset == online /\ linked = "rejected" /\ linked' = "indexing"
  /\ UNCHANGED <<disk, pub, online>> /\ CloudSame /\ LaptopSame /\ ReadersSame
Sleep == online /\ online' = FALSE
  /\ UNCHANGED <<disk, pub, linked>> /\ CloudSame /\ LaptopSame /\ ReadersSame
Wake == ~online /\ online' = TRUE
  /\ UNCHANGED <<disk, pub, linked>> /\ CloudSame /\ LaptopSame /\ ReadersSame

\* The upload queue reads published chunks from the file; a file that moved on
\* supersedes them, and the next index queues the new ones.
Upload == CloudBacked /\ hold = "none" /\ cver # pub /\ DeviceServes(pub)
  /\ cver' = pub /\ UNCHANGED hold /\ LinkSame /\ LaptopSame /\ ReadersSame
Held == "hold" \in Faults /\ CloudBacked /\ hold = "none" /\ hold' \in {"storage", "spending"}
  /\ UNCHANGED cver /\ LinkSame /\ LaptopSame /\ ReadersSame
\* Alice frees storage or raises her limit.
Released == hold # "none" /\ hold' = "none"
  /\ UNCHANGED cver /\ LinkSame /\ LaptopSame /\ ReadersSame

\* The laptop's side.
SetKeep == keep' = ~keep
  /\ UNCHANGED <<lapOnline, copy>> /\ LinkSame /\ CloudSame /\ ReadersSame
LaptopSleep == "lapSleeps" \in Faults /\ lapOnline /\ lapOnline' = FALSE
  /\ UNCHANGED <<keep, copy>> /\ LinkSame /\ CloudSame /\ ReadersSame
LaptopWake == ~lapOnline /\ lapOnline' = TRUE
  /\ UNCHANGED <<keep, copy>> /\ LinkSame /\ CloudSame /\ ReadersSame
\* With the switch on, the account copy fetches each published version from
\* the holders other than the laptop.
Backfill == keep /\ lapOnline /\ pub > 0 /\ copy # pub
  /\ (DeviceServes(pub) \/ cver = pub)
  /\ copy' = pub
  /\ UNCHANGED <<keep, lapOnline>> /\ LinkSame /\ CloudSame /\ ReadersSame
\* With the switch off, the copy is an ordinary cache the store may evict.
Evict == "evicts" \in Faults /\ ~keep /\ copy > 0 /\ copy' = 0
  /\ UNCHANGED <<keep, lapOnline>> /\ LinkSame /\ CloudSame /\ ReadersSame

\* Each reader's side.
Others == LinkSame /\ CloudSame /\ LaptopSame
Appear(r) == Active(r) /\ file[r] = "none" /\ pub > 0
  /\ file' = [file EXCEPT ![r] = "placeholder"] /\ UNCHANGED <<want, have>> /\ Others
Open(r) == Active(r) /\ file[r] \in {"placeholder", "unavailable"}
  /\ file' = [file EXCEPT ![r] = "filling"] /\ want' = [want EXCEPT ![r] = pub]
  /\ UNCHANGED have /\ Others
\* Every chunk re-encodes to its ref, so the open's chunks reach the block store.
Filled(r) == Active(r) /\ file[r] = "filling" /\ Serves(r, want[r])
  /\ file' = [file EXCEPT ![r] = "present"] /\ have' = [have EXCEPT ![r] = want[r]]
  /\ want' = [want EXCEPT ![r] = 0] /\ Others
\* A newer version is published: fetch that one.
Retarget(r) == Active(r) /\ file[r] = "filling" /\ want[r] # pub
  /\ want' = [want EXCEPT ![r] = pub] /\ UNCHANGED <<file, have>> /\ Others
\* No holder can answer and the linking Device cannot publish a newer one:
\* say so at once instead of waiting.
NoHolder(r) == Active(r) /\ file[r] = "filling" /\ want[r] = pub
  /\ ~Serves(r, want[r]) /\ (~online \/ linked = "rejected")
  /\ file' = [file EXCEPT ![r] = "unavailable"] /\ want' = [want EXCEPT ![r] = 0]
  /\ UNCHANGED have /\ Others
\* A cached file follows newer published versions by their changed chunks.
Update(r) == Active(r) /\ file[r] = "present" /\ have[r] < pub /\ Serves(r, pub)
  /\ have' = [have EXCEPT ![r] = pub] /\ UNCHANGED <<file, want>> /\ Others

\* The steps no person on the screen takes.
World ==
  \/ Edit \/ Index \/ Indexed \/ Reject \/ Sleep \/ Wake
  \/ Upload \/ Held \/ Released \/ LaptopSleep \/ LaptopWake \/ Backfill \/ Evict
  \/ \E r \in Readers :
       Appear(r) \/ Filled(r) \/ Retarget(r) \/ NoHolder(r) \/ Update(r)

Next == World \/ Reset \/ SetKeep \/ \E r \in Readers : Open(r)

TypeOK ==
  /\ Faults \subseteq AllFaults /\ Readers \subseteq {"owner", "member"}
  /\ disk \in 1..MaxVer /\ pub \in 0..MaxVer /\ online \in BOOLEAN
  /\ linked \in {"indexing", "watching", "rejected"}
  /\ cver \in 0..MaxVer /\ hold \in {"none", "storage", "spending"}
  /\ keep \in BOOLEAN /\ lapOnline \in BOOLEAN /\ copy \in 0..MaxVer
  /\ file \in [Readers -> {"none", "placeholder", "filling", "present", "unavailable"}]
  /\ want \in [Readers -> 0..MaxVer] /\ have \in [Readers -> 0..MaxVer]

\* No reader holds bytes that were not published: no wrong or mixed version.
PresentIsPublished ==
  /\ \A r \in Readers : file[r] = "present" => have[r] \in 1..pub
  /\ copy <= pub
\* A reader fetches only a version the World names.
FetchesPublished == \A r \in Readers : file[r] = "filling" => want[r] \in 1..pub
\* The World never names a version Alice's disk has not reached.
PublishedFromDisk == pub <= disk
\* Spacewave Cloud holds only published versions.
CloudFromPublished == cver <= pub /\ (~CloudBacked => cver = 0)
\* Nobody sees the file before Alice publishes.
NothingBeforePublish == pub = 0 => \A r \in Readers : file[r] = "none"
====
