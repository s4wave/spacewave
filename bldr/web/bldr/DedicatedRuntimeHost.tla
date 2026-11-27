---- MODULE DedicatedRuntimeHost ----
\* The dedicated runtime host: documents of one origin elect, through a Web
\* Lock lease, the one document that runs the runtime in a dedicated Worker;
\* every other document attaches to it through the ServiceWorker.
\*
\* Each host term is a generation. Its Worker takes the generation's life lock
\* at startup and holds it until it dies, so a held life lock is the Worker's
\* life. The host watches the life lock once the Worker reports holding it, and
\* accepts attached documents only from then on; each attached document
\* watches the lock from the moment it attaches. A watch requested before the
\* Worker held the lock would take the lock itself. A watcher's request is
\* granted exactly when the Worker is gone, whether it crashed, ran out of
\* memory or was closed, with the page still alive. Then:
\*
\* - the host shows the failed startup screen, ends its term and returns the
\*   lease; Retry reloads the page into a fresh election;
\* - an attached document drops back to loading and attaches again, or wins
\*   the lease and starts the next generation itself.
\*
\* A Worker that dies before it takes its life lock is not modeled here, by
\* decision: its page is still starting, and the boot deadline fails it into
\* the same Retry. A Worker whose life lock request fails reports that it
\* holds none, and its host fails into the same Retry before acking any attach
\* (NoLifeLock), rather than run a Worker whose death nobody would see. Bounded
\* by MaxDeaths lost Workers.

EXTENDS Naturals

CONSTANTS Docs, MaxDeaths

None == <<>>

VARIABLES
  role,   \* "pending", "host", "attached" or "failed", per document
  lease,  \* the document holding the election lease, or "none"
  gen,    \* per document: the number of its latest host term
  worker, \* per document: "none", "starting", "alive" (holds its life lock)
          \* or "dead", for the Worker of its latest term
  conn,   \* per document: <<host, gen>> its runtime route, or None
  watch,  \* per document: <<host, gen>> whose life lock it watches, or None
  view,   \* per document: "loading", "app" or "failed"
  deaths  \* Workers lost so far: died, or never took their life lock
vars == <<role, lease, gen, worker, conn, watch, view, deaths>>

\* Held(r): the Worker of route r's term is running and holds its life lock.
Held(r) ==
  /\ r # None
  /\ role[r[1]] = "host" /\ gen[r[1]] = r[2]
  /\ worker[r[1]] = "alive"

\* Route(d): the term document d runs on.
Route(d) == IF role[d] = "host" THEN <<d, gen[d]>> ELSE conn[d]

Shown(d) == IF view[d] = "failed" THEN {"retry"} ELSE {}
Allowed(d, b) == b = "retry" /\ view[d] = "failed"
Hint(d) ==
  CASE view[d] = "loading" -> "Starting"
    [] view[d] = "app" -> "Running"
    [] OTHER -> ""

Init ==
  /\ role = [d \in Docs |-> "pending"]
  /\ lease = "none"
  /\ gen = [d \in Docs |-> 0]
  /\ worker = [d \in Docs |-> "none"]
  /\ conn = [d \in Docs |-> None]
  /\ watch = [d \in Docs |-> None]
  /\ view = [d \in Docs |-> "loading"]
  /\ deaths = 0

\* Web Locks grants the free lease to a waiting document: a pending one starts
\* the first term, an attached one is promoted, drops its route and its watch,
\* and starts the next term on a new Worker.
Grant(d) ==
  /\ lease = "none" /\ role[d] \in {"pending", "attached"}
  /\ lease' = d
  /\ role' = [role EXCEPT ![d] = "host"]
  /\ gen' = [gen EXCEPT ![d] = @ + 1]
  /\ worker' = [worker EXCEPT ![d] = "starting"]
  /\ conn' = [conn EXCEPT ![d] = None]
  /\ watch' = [watch EXCEPT ![d] = None]
  /\ view' = [view EXCEPT ![d] = "loading"]
  /\ UNCHANGED deaths

\* A document without a route attaches through the ServiceWorker to the
\* current host, which accepts it once its Worker has taken the life lock,
\* even if the Worker has died since; the document watches that lock.
Attach(d) ==
  /\ role[d] \in {"pending", "attached"} /\ conn[d] = None
  /\ lease \notin {"none", d} /\ role[lease] = "host"
  /\ worker[lease] # "starting"
  /\ role' = [role EXCEPT ![d] = "attached"]
  /\ conn' = [conn EXCEPT ![d] = <<lease, gen[lease]>>]
  /\ watch' = [watch EXCEPT ![d] = <<lease, gen[lease]>>]
  /\ UNCHANGED <<lease, gen, worker, view, deaths>>

\* The Worker takes its life lock and tells its host, which starts watching it.
Hold(d) ==
  /\ role[d] = "host" /\ worker[d] = "starting"
  /\ worker' = [worker EXCEPT ![d] = "alive"]
  /\ watch' = [watch EXCEPT ![d] = <<d, gen[d]>>]
  /\ UNCHANGED <<role, lease, gen, conn, view, deaths>>

\* The Worker cannot take its life lock and says so. Its death could never be
\* seen, so the host ends its term, ends the Worker and returns the lease, as
\* when a watch fires.
NoLifeLock(d) ==
  /\ role[d] = "host" /\ worker[d] = "starting" /\ deaths < MaxDeaths
  /\ role' = [role EXCEPT ![d] = "failed"]
  /\ lease' = "none"
  /\ worker' = [worker EXCEPT ![d] = "none"]
  /\ view' = [view EXCEPT ![d] = "failed"]
  /\ deaths' = deaths + 1
  /\ UNCHANGED <<gen, conn, watch>>

\* The runtime answers: the document shows the app.
Connect(d) ==
  /\ view[d] = "loading" /\ role[d] \in {"host", "attached"}
  /\ Held(Route(d))
  /\ view' = [view EXCEPT ![d] = "app"]
  /\ UNCHANGED <<role, lease, gen, worker, conn, watch, deaths>>

\* The Worker dies alone; its page stays open.
WorkerDies(h) ==
  /\ worker[h] = "alive" /\ deaths < MaxDeaths
  /\ worker' = [worker EXCEPT ![h] = "dead"]
  /\ deaths' = deaths + 1
  /\ UNCHANGED <<role, lease, gen, conn, watch, view>>

\* A watcher's request is granted once the term's Worker no longer holds the
\* life lock. The host ends its term, ends the dead Worker and returns the
\* lease; an attached document drops its route to re-elect. A grant for a term
\* the document no longer runs on changes nothing else.
WatchFires(d) ==
  LET r == watch[d] IN
  /\ r # None /\ ~Held(r)
  /\ watch' = [watch EXCEPT ![d] = None]
  /\ IF role[d] = "host" /\ r = <<d, gen[d]>>
       THEN /\ role' = [role EXCEPT ![d] = "failed"]
            /\ lease' = "none"
            /\ worker' = [worker EXCEPT ![d] = "none"]
            /\ view' = [view EXCEPT ![d] = "failed"]
            /\ UNCHANGED conn
       ELSE IF role[d] = "attached" /\ r = conn[d]
       THEN /\ conn' = [conn EXCEPT ![d] = None]
            /\ view' = [view EXCEPT ![d] = "loading"]
            /\ UNCHANGED <<role, lease, worker>>
       ELSE UNCHANGED <<role, lease, worker, conn, view>>
  /\ UNCHANGED <<gen, deaths>>

\* Retry reloads the page: the document starts over as a new election entrant.
Retry(d) ==
  /\ "retry" \in Shown(d)
  /\ role' = [role EXCEPT ![d] = "pending"]
  /\ conn' = [conn EXCEPT ![d] = None]
  /\ watch' = [watch EXCEPT ![d] = None]
  /\ view' = [view EXCEPT ![d] = "loading"]
  /\ UNCHANGED <<lease, gen, worker, deaths>>

Next ==
  \E d \in Docs :
    \/ Grant(d) \/ Attach(d) \/ Hold(d) \/ Connect(d)
    \/ NoLifeLock(d) \/ WorkerDies(d) \/ WatchFires(d) \/ Retry(d)

Spec == Init /\ [][Next]_vars

S == INSTANCE Surface WITH People <- Docs, Actions <- {"retry"}, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck

\* One host at a time, and it holds the lease.
OneHost == \A d \in Docs : role[d] = "host" <=> lease = d
\* A failed document holds no lease, so a waiting document can take it.
FailedHoldsNoLease == \A d \in Docs : role[d] = "failed" => lease # d
\* A watch is armed only on a held life lock or a term that has since ended,
\* never on a Worker that has not taken it yet, which the watcher would take.
WatchFollowsHold ==
  \A d \in Docs :
    watch[d] # None => worker[watch[d][1]] # "starting"
                       \/ gen[watch[d][1]] # watch[d][2]
\* Every document showing the app watches the term it runs on, so the death
\* of its Worker is always noticed.
AppIsWatched ==
  \A d \in Docs : view[d] = "app" => watch[d] = Route(d) /\ watch[d] # None
\* Only the host whose Worker died shows the failure; attached documents
\* re-elect instead.
OnlyTheHostFails == \A d \in Docs : view[d] = "failed" <=> role[d] = "failed"
\* Every page at rest runs the app on a live Worker.
OnlyHealthyPagesRest ==
  ~ENABLED Next => \A d \in Docs : view[d] = "app" /\ Held(Route(d))
====
