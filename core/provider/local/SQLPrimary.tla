---- MODULE SQLPrimary ----
\* A World whose block store writes to its SQL backend (sqlPrimaryStores), as
\* the single-writer deferred engine runs it with the SQL publisher
\* (bstore-sql-publish.go). Each commit is admitted under the writer lock:
\* its blocks join the publisher's overlay and queue for insertion, and the
\* engine's published head advances at once, before MariaDB holds a row.
\* Workers insert the queued blocks with INSERT IGNORE, several at a time and
\* in any order. The committer writes the durable head of commit k to s4db
\* (the local publisher) only after commit k's rows landed and commit k-1's
\* head was written, so the durable head advances one commit at a time. A
\* head that fails to commit fails every commit admitted after it, and the
\* engine reinstalls from the durable head; their queued blocks still land.
\* In-process readers read through the overlay; peers and a restarted backend
\* read the durable head through the local bucket, read only, then SQL. The
\* uploader keeps copying the blocks its markers name from the bucket to SQL.
\*
\* Before the switch the head r0 and the block o it references sit in the
\* bucket with upload markers pending. Commit 1 writes the root n1, which
\* references o; commit 2 writes n2, which references n1 and o. A failed
\* insert stays queued and its worker retries it after a back-off. A restart
\* drops the overlay and the commits after the durable head, and may remount
\* with SQL primary off when Rollback holds: commits then write to the
\* bucket, and the account GC cleanup and eviction run again. The GC sweeps
\* bucket blocks its ref graph cannot reach from either head; the graph
\* records the references only of blocks written to the bucket.
\*
\* The World engine (core/sobject/world/engine) serves writers the admitted
\* head mem and readers the durable head rd, which installs each commit once
\* its head is durable and keeps the World before a failed one. A writer
\* forks under the engine's writer lock, runs its body, during which the
\* consumer (Hyperline) takes its own lock, and commits; CommitAdmit releases
\* the writer lock at admission, and the consumer releases its lock after.
\* A failed head is settled under the writer lock, by the receipt's watcher or
\* by the next writer as it forks: the admitted head returns to the durable
\* one and the reinstall hook, which takes the consumer's lock, reports it. A
\* commit whose base follows a failed head fails without settling it.
\* SettleInCommit models the earlier engine, which settled inside the commit
\* and ran the hook on the writer's goroutine.
\*
\* With PageView the engine keeps each commit's World pages in memory
\* (world-pages.go) and its publication carries only the operation's payload
\* and the replay save. A checkpoint or stall base names a World at or after
\* the last written position in a publication after the admitted ones, which
\* carries the World's kept pages; once it is durable the pages leave memory
\* for the store, and a checkpoint above it adopts it as the replay base. Each
\* state update sweeps the kept pages no live World reaches: the base, the
\* positions from it on, the admitted head, the durable head and the Worlds
\* queued for it (QueuedLive). A restart loses the kept pages and the new
\* engine replays the durable head from the base and the stored payloads.
\*
\* Not modeled: peers, the decoded cache, which holds only blocks already read,
\* the chunking of a commit's blocks, whose rows the committer waits for as a
\* whole, a block shared with an earlier commit, which that commit inserts
\* and commits first, the admission bound, which only delays Commit, a
\* naming publication that fails alone rather than after a failed head, the
\* page view after a rollback remount, and a write's hold on the World after
\* it, which keeps that World's pages until the write is released after it
\* is durable: the page sweep keeps the queued Worlds without relying on it.

EXTENDS Naturals

\* AckDurable: MariaDB has made a row durable when its INSERT returns, as
\* innodb_flush_log_at_trx_commit = 1 does.
AckDurable == TRUE
\* Rollback: a restart may turn SQL primary off.
Rollback == FALSE
\* SettleInCommit: a commit settles a failed head itself, running the hook
\* while its writer holds the consumer's lock.
SettleInCommit == FALSE
\* PageView: the engine keeps the World pages in memory under SQL primary.
PageView == TRUE
\* QueuedLive: the page sweep keeps the Worlds queued for the durable head.
QueuedLive == TRUE
\* DurableLive: the page sweep keeps the durable head's World, which its
\* root pin through the view and the sweep's own live roots both do.
DurableLive == TRUE

MaxCommit == 2
Root == <<"r0", "n1", "n2">>
RootOf(k) == Root[k + 1]
\* Commit 1 writes the root n1 and the leaf l1; commit 2 writes the root n2,
\* which shares l1 and o, so n1 is World 1's alone.
Written(k) == IF k = 1 THEN {"n1", "l1"} ELSE {"n2"}
Payload(k) == IF k = 1 THEN "p1" ELSE "p2"
Refs == [b \in {"r0", "o", "n1", "n2", "l1", "p1", "p2"} |->
          CASE b = "r0" -> {"o"}
            [] b = "n1" -> {"o", "l1"}
            [] b = "n2" -> {"o", "l1"}
            [] OTHER -> {}]
Reach(k) == {RootOf(k)} \cup Refs[RootOf(k)]
            \cup UNION {Refs[c] : c \in Refs[RootOf(k)]}
ReachAll(S) == UNION {Reach(k) : k \in S}

VARIABLES
  mode,     \* "sql" while SQL primary, "local" after a rollback
  mem,      \* the engine's published head
  dur,      \* the durable head in s4db
  buf,      \* admitted blocks the publisher's overlay holds, not landed
  dbUp,     \* MariaDB answers
  rows,     \* rows MariaDB holds durably
  acked,    \* rows MariaDB acknowledged but has not made durable
  local,    \* blocks in the local bucket
  marker,   \* blocks with an upload marker pending
  edges,    \* references the bucket's ref graph records
  rd,       \* the durable head readers see
  failed,   \* a head failed and the engine has not settled it
  wlock,    \* the engine's writer lock: "free", "writer" or "watcher"
  wst,      \* the writer: "idle", "forked", "body" or "done"
  base,     \* the admitted head the writer forked from
  held,     \* the writer holds the consumer's lock
  kept,     \* World pages the page view holds in memory
  wpos,     \* the replay base, the last World a checkpoint adopted
  wr,       \* the last written position: a World before it is not readable
  nw,       \* the World a pending naming publication names, or 0
  na,       \* the commit the pending naming publication follows
  stored    \* the pending naming publication is durable
store == <<mode, dur, buf, dbUp, rows, acked, local, marker, edges>>
eng == <<rd, failed, wlock, wst, base, held>>
view == <<kept, wpos, wr, nw, na, stored>>
vars == <<mode, mem, dur, buf, dbUp, rows, acked, local, marker, edges,
          rd, failed, wlock, wst, base, held, kept, wpos, wr, nw, na, stored>>

\* What a store holds: the bucket, and MariaDB's rows.
Stored == local \cup rows \cup acked

Init ==
  /\ mode = "sql" /\ mem = 0 /\ dur = 0 /\ buf = {}
  /\ dbUp = TRUE /\ rows = {} /\ acked = {}
  /\ local = {"r0", "o"} /\ marker = {"r0", "o"}
  /\ edges = {<<"r0", "o">>}
  /\ rd = 0 /\ failed = FALSE /\ wlock = "free" /\ wst = "idle"
  /\ base = 0 /\ held = FALSE
  /\ kept = {} /\ wpos = 0 /\ wr = 0 /\ nw = 0 /\ na = 0 /\ stored = FALSE

\* MariaDB stores an acknowledged row durably, or only in memory.
Store(S) ==
  IF AckDurable
  THEN rows' = rows \cup S /\ UNCHANGED acked
  ELSE acked' = acked \cup (S \ rows) /\ UNCHANGED rows

\* A writer takes the writer lock and forks from the admitted head. A failed
\* head is settled first: the admitted head returns to the durable one and
\* the hook reports it, before the body takes the consumer's lock.
Fork ==
  /\ wst = "idle" /\ wlock = "free" /\ mem < MaxCommit
  /\ wlock' = "writer" /\ wst' = "forked"
  /\ IF failed
     THEN mem' = dur /\ failed' = FALSE /\ base' = dur
     ELSE UNCHANGED <<mem, failed>> /\ base' = mem
  /\ UNCHANGED <<store, rd, held, view>>

\* The body takes the consumer's lock and writes.
Body ==
  /\ wst = "forked"
  /\ held' = TRUE /\ wst' = "body"
  /\ UNCHANGED <<store, mem, rd, failed, wlock, base, view>>

\* CommitAdmit writes the commit's blocks and payload: into the overlay under
\* SQL primary, the World pages into the page view instead with PageView,
\* and into the bucket, with markers and graph edges, otherwise. The admitted
\* head
\* advances before any row lands, and the writer lock is released. A commit
\* whose base follows a failed head fails, leaving the failure to the
\* watcher or the next writer; under SettleInCommit it settles it, which
\* waits for the consumer's lock its own writer holds.
Admit ==
  /\ wst = "body" /\ wlock' = "free" /\ wst' = "done"
  /\ IF failed
     THEN /\ ~SettleInCommit \/ ~held
          /\ IF SettleInCommit
             THEN mem' = dur /\ failed' = FALSE
             ELSE UNCHANGED <<mem, failed>>
          /\ UNCHANGED <<buf, local, marker, edges, kept>>
     ELSE /\ LET w == Written(mem + 1)
                 p == Payload(mem + 1) IN
             CASE mode = "sql" /\ PageView ->
                    /\ kept' = kept \cup w /\ buf' = buf \cup {p}
                    /\ UNCHANGED <<local, marker, edges>>
               [] mode = "sql" ->
                    /\ buf' = buf \cup w \cup {p}
                    /\ UNCHANGED <<local, marker, edges, kept>>
               [] OTHER ->
                    /\ local' = local \cup w \cup {p}
                    /\ marker' = marker \cup w \cup {p}
                    /\ edges' = edges \cup {<<b, c>> : b \in w, c \in UNION {Refs[x] : x \in w}}
                    /\ UNCHANGED <<buf, kept>>
          /\ mem' = mem + 1 /\ UNCHANGED failed
  /\ UNCHANGED <<mode, dur, dbUp, rows, acked, rd, base, held, wpos, wr, nw, na,
                 stored>>

\* The consumer commits its own transaction and releases its lock.
Release ==
  /\ wst = "done"
  /\ held' = FALSE /\ wst' = "idle"
  /\ UNCHANGED <<store, mem, rd, failed, wlock, base, view>>

\* The receipt's watcher takes the writer lock after a failure, then settles
\* it once the hook takes the consumer's lock.
WatchLock ==
  /\ failed /\ wlock = "free"
  /\ wlock' = "watcher"
  /\ UNCHANGED <<store, mem, rd, failed, wst, base, held, view>>
WatchHook ==
  /\ wlock = "watcher" /\ ~held
  /\ mem' = dur /\ failed' = FALSE /\ wlock' = "free"
  /\ UNCHANGED <<store, rd, wst, base, held, view>>

\* The durable head installs the next durable commit for readers.
Promote ==
  /\ rd < dur
  /\ rd' = rd + 1
  /\ UNCHANGED <<store, mem, failed, wlock, wst, base, held, view>>

\* runWorker: one insert lands any queued block, in any order, and drops it
\* from the overlay. A failed insert keeps it queued; the stutter is its
\* back-off.
Drain ==
  /\ dbUp
  /\ \E b \in buf : Store({b}) /\ buf' = buf \ {b}
  /\ UNCHANGED <<mode, mem, dur, dbUp, local, marker, edges, eng, view>>

\* runCommitter: once the next commit's rows landed, its head commits to
\* s4db after the one before it, and after a naming publication admitted
\* before it. A local-store commit writes its head at once, its blocks
\* already in the bucket.
Sync ==
  /\ dur < mem /\ ~failed
  /\ (Written(dur + 1) \cup {Payload(dur + 1)}) \cap buf = {}
  /\ nw # 0 /\ na = dur => stored
  /\ dur' = dur + 1
  /\ UNCHANGED <<mode, mem, buf, dbUp, rows, acked, local, marker, edges, eng,
                 view>>

\* The next head fails to commit: it and every commit admitted after it
\* fail, and readers keep the durable head. The engine settles the failure
\* under the writer lock. Their queued blocks still land, as rows no head
\* reaches.
SyncFail ==
  /\ dur < mem /\ ~failed
  /\ failed' = TRUE
  /\ IF nw # 0 /\ ~stored THEN nw' = 0 ELSE UNCHANGED nw
  /\ UNCHANGED <<mode, mem, dur, buf, dbUp, rows, acked, local, marker, edges,
                 rd, wlock, wst, base, held, kept, wpos, wr, na, stored>>

\* The uploader copies a marked block from the bucket to SQL.
Upload ==
  /\ dbUp
  /\ \E b \in marker \cap local :
       /\ Store({b})
       /\ marker' = marker \ {b}
  /\ UNCHANGED <<mode, mem, dur, buf, dbUp, local, edges, eng, view>>

\* Eviction, off under SQL primary, drops an uploaded block from the bucket.
Evict ==
  /\ mode = "local"
  /\ \E b \in local \ marker :
       /\ b \in rows \cup acked
       /\ local' = local \ {b}
  /\ UNCHANGED <<mode, mem, dur, buf, dbUp, rows, acked, marker, edges, eng,
                 view>>

\* The account GC cleanup, off under SQL primary, sweeps bucket blocks the
\* graph cannot reach from the durable head or the engine's pinned head, with
\* their markers.
GraphReach ==
  LET roots == {RootOf(dur), RootOf(mem), RootOf(rd)}
      one == {c \in local : \E r \in roots : <<r, c>> \in edges}
  IN roots \cup one \cup {c \in local : \E p \in one : <<p, c>> \in edges}
Sweep ==
  /\ mode = "local"
  /\ local \ GraphReach # {}
  /\ local' = local \cap GraphReach
  /\ marker' = marker \cap GraphReach
  /\ UNCHANGED <<mode, mem, dur, buf, dbUp, rows, acked, edges, eng, view>>

\* MariaDB crashes, losing the rows it had not made durable, and restarts.
DbCrash ==
  /\ dbUp' = FALSE /\ acked' = {}
  /\ UNCHANGED <<mode, mem, dur, buf, rows, local, marker, edges, eng, view>>
DbRestart ==
  /\ ~dbUp /\ dbUp' = TRUE
  /\ UNCHANGED <<mode, mem, dur, buf, rows, acked, local, marker, edges, eng,
                 view>>

\* A checkpoint or stall base names a readable World after the base, one at
\* or after the last written position, in a publication after the admitted
\* ones, which carries its kept pages. A stall base names the head and
\* writes every position. The pages stay kept until the publication is
\* durable.
Name ==
  /\ PageView /\ mode = "sql" /\ nw = 0 /\ ~failed /\ wlock = "free"
  /\ \E k \in (IF wr > wpos THEN wr ELSE wpos + 1)..mem :
       /\ nw' = k /\ na' = mem /\ stored' = FALSE
       /\ buf' = buf \cup (Reach(k) \cap kept)
       /\ wr' \in IF k = mem THEN {wr, mem} ELSE {wr}
  /\ UNCHANGED <<mode, mem, dur, dbUp, rows, acked, local, marker, edges, eng,
                 kept, wpos>>

\* The naming publication is durable once its pages landed after the commit
\* it follows: the pages it carries leave the view for the store.
NameLand ==
  /\ nw # 0 /\ ~stored /\ dur >= na
  /\ (Reach(nw) \cap kept) \cap buf = {}
  /\ stored' = TRUE /\ kept' = kept \ Reach(nw)
  /\ UNCHANGED <<mode, mem, dur, buf, dbUp, rows, acked, local, marker, edges,
                 eng, wpos, wr, nw, na>>

\* A checkpoint above the named World adopts it as the replay base once its
\* publication is durable, dropping the positions up to it. Nothing orders
\* this after the durable head installs the Worlds before it. The written
\* positions above it stay written.
Adopt ==
  /\ nw # 0 /\ stored
  /\ wpos' = nw /\ nw' = 0 /\ stored' = FALSE
  /\ wr' = IF wr > nw THEN wr ELSE nw
  /\ UNCHANGED <<mode, mem, dur, buf, dbUp, rows, acked, local, marker, edges,
                 eng, kept, na>>

\* A state update drops the kept pages no live World reaches: the base and a
\* pending named World, the written positions on, and the heads.
Live ==
  {wpos} \cup (IF nw # 0 THEN {nw} ELSE {})
  \cup ((IF wr > wpos THEN wr ELSE wpos)..mem)
  \cup (IF DurableLive THEN {rd} ELSE {})
  \cup (IF QueuedLive THEN (rd + 1)..mem ELSE {})
PageSweep ==
  /\ PageView /\ wlock = "free"
  /\ kept \ ReachAll(Live) # {}
  /\ kept' = kept \cap ReachAll(Live)
  /\ UNCHANGED <<mode, mem, dur, buf, dbUp, rows, acked, local, marker, edges,
                 eng, wpos, wr, nw, na, stored>>

\* The backend crashes or restarts: it loses the overlay and the commits
\* after the durable head, whose rows may have landed already, and remounts,
\* with SQL primary off when Rollback holds. The writer and the engine's locks
\* end with it, and the new engine serves readers the durable head. The kept
\* pages and a pending naming publication go with it; the new engine replays
\* the durable head from the base, keeps the pages it computes and writes
\* every position up to it.
Restart ==
  /\ mem' = dur /\ buf' = {}
  /\ mode' \in IF Rollback THEN {"sql", "local"} ELSE {"sql"}
  /\ rd' = dur /\ failed' = FALSE /\ wlock' = "free" /\ wst' = "idle"
  /\ held' = FALSE
  /\ kept' = IF PageView THEN Reach(dur) \ Stored ELSE {}
  /\ nw' = 0 /\ stored' = FALSE /\ wr' = dur
  /\ UNCHANGED <<dur, dbUp, rows, acked, local, marker, edges, base, wpos, na>>

Next ==
  \/ Fork \/ Body \/ Admit \/ Release \/ WatchLock \/ WatchHook \/ Promote
  \/ Drain \/ Sync \/ SyncFail \/ Upload \/ Evict \/ Sweep
  \/ Name \/ NameLand \/ Adopt \/ PageSweep
  \/ DbCrash \/ DbRestart \/ Restart
  \/ UNCHANGED vars
Spec == Init /\ [][Next]_vars

\* Every block the durable head reaches stays stored, so a restarted backend
\* and every peer reads it. With PageView a restart rebuilds it instead: the
\* replay base is stored, with the payload of every durable commit after it.
DurableStored ==
  IF PageView
  THEN Reach(wpos) \subseteq Stored
       /\ \A k \in (wpos + 1)..dur : Payload(k) \in Stored
  ELSE Reach(dur) \subseteq Stored
\* The durable head reaches only rows MariaDB holds: the payload of every
\* durable commit has landed, and without PageView its root too.
DurableRows ==
  \A k \in 1..dur : Payload(k) \in rows \cup acked
                   /\ (PageView \/ RootOf(k) \in rows \cup acked)
\* A reader of the engine's published head finds each block it reaches in the
\* view, the overlay or a store, so no reader sees a commit before its blocks.
PublishedReadable == Reach(mem) \subseteq kept \cup buf \cup Stored
\* Readers see only durable commits, and find every block they reach.
ReadersDurable == rd <= dur /\ Reach(rd) \subseteq kept \cup Stored
\* Every World queued for the durable head stays readable until it installs,
\* so promoting it never meets a missing page.
QueuedReadable ==
  ~failed => \A k \in (rd + 1)..mem : Reach(k) \subseteq kept \cup buf \cup Stored
\* Nothing settles while a writer holds the writer lock, so a commit lands
\* on the head its writer forked from and is never replayed onto another.
ForkedHead == wst \in {"forked", "body"} => base = mem

\* Liveness, under the fairness below: once MariaDB stays up, the durable
\* head keeps catching up with the published one, and readers with it,
\* without a restart. Fairness on Drain stands for workers that retry a
\* failed insert, and on NameLand for a naming publication whose pages
\* landed. A failure is settled and the writer finishes: the hook never waits
\* for a lock its own writer holds.
FairSpec == Spec /\ WF_vars(Drain) /\ WF_vars(Sync) /\ WF_vars(DbRestart)
              /\ WF_vars(Body) /\ WF_vars(Admit) /\ WF_vars(Release)
              /\ WF_vars(WatchLock) /\ WF_vars(WatchHook) /\ WF_vars(Promote)
              /\ WF_vars(NameLand)
CatchesUp == <>[]dbUp => []<>(dur = mem /\ rd = dur)
Settles == failed ~> ~failed
WriterFinishes == (wst # "idle") ~> (wst = "idle")
====
