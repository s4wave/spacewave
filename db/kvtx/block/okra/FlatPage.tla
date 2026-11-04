---------------------------- MODULE FlatPage ----------------------------
EXTENDS Naturals, FiniteSets
\* Models the resident arena of a flat decoded page (flatPage): the encoded page
\* bytes plus an offset index, allocated once by UnmarshalDecodedBlock, published
\* to the decoded-block pool by Store, evicted by the pool's budget, and borrowed
\* by readers as views (entryKey, rawInlineValue) that stay in use after the
\* pool drops the entry.
\* Callers: Decode is Cursor.Unmarshal's decode of fetched bytes; Publish is
\* DecodedBlockCache.Store/pool.put with its budget eviction; Evict is the
\* pool's removal or InvalidateRef; Acquire is a hit in UnmarshalReadBlock and
\* Release is the end of the reader's use of the page and its views. Collect is
\* the Go garbage collector reclaiming an arena nothing names. The pool keeps
\* the entry pointer and a reader keeps a view pointer; both keep the arena
\* alive, so Collect needs neither.
\* Abstraction: arena content is the identity of the stored page it was decoded
\* from, which stands for every byte of the buffer and index. A page's charge is
\* its Size, the arena's real bytes. Readers holding a view of an evicted arena
\* are outside the pool budget, as in ReadCursor.
\* Mutants (testdata/*.cfg): RecycleOnEvict reuses an evicted arena at once, as
\* an arena free list would; MutatePublished rewrites a published arena in
\* place; Undercharge charges less than the arena holds.
CONSTANTS Arenas, Readers, Budget, RecycleOnEvict, MutatePublished, Undercharge
VARIABLES phase, content, stamp, cache, held, seen, retained
vars == <<phase, content, stamp, cache, held, seen, retained>>
Refs == {"a", "b"}
\* Size is the arena's byte count for each stored page.
Size == [r \in Refs |-> IF r = "a" THEN 2 ELSE 3]
Charge(r) == IF Undercharge THEN 1 ELSE Size[r]
Resident(c) == (IF c["a"] # 0 THEN Size["a"] ELSE 0) + (IF c["b"] # 0 THEN Size["b"] ELSE 0)
Init == /\ phase = [a \in Arenas |-> "free"]
        /\ content = [a \in Arenas |-> "none"]
        /\ stamp = [a \in Arenas |-> "none"]
        /\ cache = [r \in Refs |-> 0]
        /\ held = [h \in Readers |-> 0]
        /\ seen = [h \in Readers |-> "none"]
        /\ retained = 0
\* The decoder fills a fresh arena that no one else can name yet.
Decode(r, a) ==
    /\ phase[a] = "free"
    /\ phase' = [phase EXCEPT ![a] = "building"]
    /\ content' = [content EXCEPT ![a] = r]
    /\ UNCHANGED <<stamp, cache, held, seen, retained>>
\* Publication installs the arena after evicting victims to fit the budget.
Publish(a) ==
    LET r == content[a] IN
    /\ phase[a] = "building" /\ cache[r] = 0
    /\ \E victims \in SUBSET {x \in Refs : cache[x] # 0} :
       LET kept == [x \in Refs |-> IF x \in victims THEN 0 ELSE cache[x]] IN
       /\ Resident(kept) + Size[r] <= Budget
       /\ cache' = [kept EXCEPT ![r] = a]
       /\ retained' = retained - (IF victims = {} THEN 0 ELSE
                         (IF "a" \in victims THEN Charge("a") ELSE 0)
                         + (IF "b" \in victims THEN Charge("b") ELSE 0))
                      + Charge(r)
       /\ phase' = [x \in Arenas |->
                      IF x = a THEN "published"
                      ELSE IF \E v \in victims : cache[v] = x
                           THEN (IF RecycleOnEvict THEN "free" ELSE "retired")
                           ELSE phase[x]]
    /\ stamp' = [stamp EXCEPT ![a] = r]
    /\ UNCHANGED <<content, held, seen>>
\* A decode that lost the race, or does not fit the budget, is dropped.
Discard(a) ==
    /\ phase[a] = "building"
    /\ \/ cache[content[a]] # 0
       \/ Size[content[a]] > Budget
    /\ phase' = [phase EXCEPT ![a] = "retired"]
    /\ UNCHANGED <<content, stamp, cache, held, seen, retained>>
Evict(r) ==
    /\ cache[r] # 0
    /\ phase' = [phase EXCEPT ![cache[r]] = IF RecycleOnEvict THEN "free" ELSE "retired"]
    /\ cache' = [cache EXCEPT ![r] = 0]
    /\ retained' = retained - Charge(r)
    /\ UNCHANGED <<content, stamp, held, seen>>
Acquire(h, r) ==
    /\ held[h] = 0 /\ cache[r] # 0
    /\ held' = [held EXCEPT ![h] = cache[r]]
    /\ seen' = [seen EXCEPT ![h] = content[cache[r]]]
    /\ UNCHANGED <<phase, content, stamp, cache, retained>>
Release(h) ==
    /\ held[h] # 0
    /\ held' = [held EXCEPT ![h] = 0]
    /\ seen' = [seen EXCEPT ![h] = "none"]
    /\ UNCHANGED <<phase, content, stamp, cache, retained>>
\* The collector frees an arena only when neither the pool nor a view names it.
Collect(a) ==
    /\ phase[a] = "retired" /\ \A h \in Readers : held[h] # a
    /\ phase' = [phase EXCEPT ![a] = "free"]
    /\ content' = [content EXCEPT ![a] = "none"]
    /\ UNCHANGED <<stamp, cache, held, seen, retained>>
\* Mutant only: an in-place rewrite of a published arena.
Mutate(a, r) ==
    /\ MutatePublished /\ phase[a] = "published"
    /\ content' = [content EXCEPT ![a] = r]
    /\ UNCHANGED <<phase, stamp, cache, held, seen, retained>>
Next == \/ \E r \in Refs, a \in Arenas : Decode(r, a)
        \/ \E a \in Arenas : Publish(a) \/ Discard(a) \/ Collect(a)
        \/ \E r \in Refs : Evict(r)
        \/ \E h \in Readers, r \in Refs : Acquire(h, r)
        \/ \E h \in Readers : Release(h)
        \/ \E a \in Arenas, r \in Refs : Mutate(a, r)
TypeOK == /\ phase \in [Arenas -> {"free", "building", "published", "retired"}]
          /\ cache \in [Refs -> 0..Cardinality(Arenas)]
          /\ retained \in Nat
\* A published arena still holds the bytes it held when it was published.
Immutable == \A a \in Arenas : phase[a] = "published" => content[a] = stamp[a]
\* Every entry the pool serves is a published arena of its own page.
CacheSound == \A r \in Refs : cache[r] # 0 =>
                  /\ phase[cache[r]] = "published" /\ content[cache[r]] = r
\* A reader's view still shows what it saw, and its arena was not reused.
ViewStable == \A h \in Readers : held[h] # 0 =>
                  /\ phase[held[h]] # "free"
                  /\ content[held[h]] = seen[h]
\* The budget charge equals the bytes of the resident arenas.
Accounted == retained = Resident(cache)
InBudget == retained <= Budget
Spec == Init /\ [][Next]_vars
=========================================================================
