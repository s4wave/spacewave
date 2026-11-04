-------------------------- MODULE ReadCacheBudget --------------------------
EXTENDS Naturals, FiniteSets
\* One byte budget for the resident read caches of a process. Two classes draw
\* from it: the encoded class (packfile spans and indexes of every PackfileStore
\* that joined) and the decoded class (the decoded-block pool, whose entries
\* also carry the ReadCursor metadata, so the cursor pool needs no share of its
\* own). Callers: DecodedBlockCache.Store/readSlot admit decoded entries,
\* PackReader span inserts admit encoded bytes, and each admission or release
\* of a reader's bytes runs ReadCacheBudget.Reclaim, which evicts from the class
\* furthest above its floor and never below a floor.
\*
\* Abstraction. A block has one encoded copy and one decoded copy at most, with
\* units proportional to bytes: block b costs b encoded and b+1 decoded. Victim
\* choice inside a class abstracts the pool's queues and the span ring, checked
\* by ReadCursorClock and ResidentRing. A hold is a reader mid-read, a pin an
\* open snapshot; both protect every resident copy of the block from eviction,
\* so charged bytes may exceed the limit by the protected bytes. In Go a reader
\* keeps its block alive through its own reference and a pin keeps the block
\* in the durable store, so a protected copy models the entry being written or
\* attached under the shard lock, which eviction never takes (the keep entry).
\* Admission evicts atomically, as put does under the shard lock. EvictStep is
\* Reclaim after a hold releases. ReleaseDup drops the encoded copy of a block
\* that is also held decoded, because the decoded copy answers the read.
CONSTANTS Blocks, Limit, FloorEnc, FloorDec, MaxHolds, DecodedOn,
          CheckHolds, CheckBound, CheckFloor
VARIABLES enc, dec, hold, pin, chargedEnc, chargedDec, taken,
          badHeld, badFloor, badBound, restarted
vars == <<enc, dec, hold, pin, chargedEnc, chargedDec, taken,
          badHeld, badFloor, badBound, restarted>>

MaxCost == 1 + (CHOOSE m \in Blocks : \A c \in Blocks : c <= m)
Min(a, b) == IF a < b THEN a ELSE b
EncU(S) == Cardinality({x \in S \X (1..MaxCost) : x[2] <= x[1]})
DecU(S) == Cardinality({x \in S \X (1..MaxCost) : x[2] <= x[1] + 1})
Prot == hold \cup pin
ProtBytes == EncU(enc \cap Prot) + DecU(dec \cap Prot)
Charged == chargedEnc + chargedDec

Init == /\ enc = {} /\ dec = {} /\ hold = {} /\ pin = {}
        /\ chargedEnc = 0 /\ chargedDec = 0 /\ taken = 0
        /\ badHeld = FALSE /\ badFloor = FALSE /\ badBound = FALSE
        /\ restarted = FALSE

\* Victims leave only unprotected copies, and never take a class below its
\* floor (a class may already sit below it).
VictimsOK(ve, vd, newEnc, newDec) ==
    /\ (~CheckHolds \/ ((ve \cap Prot) = {} /\ (vd \cap Prot) = {}))
    /\ (~CheckFloor \/ ((ve = {} \/ chargedEnc > FloorEnc)
                        /\ (vd = {} \/ chargedDec > FloorDec)))

\* Admit an encoded copy of b, evicting ve and vd in the same step.
AdmitEnc(b) ==
    /\ b \notin enc
    /\ \E ve \in SUBSET (enc \ {b}), vd \in SUBSET dec :
       LET newEnc == chargedEnc - EncU(ve) + b
           newDec == chargedDec - DecU(vd)
           enc2 == (enc \ ve) \cup {b}
           dec2 == dec \ vd
           prot2 == EncU(enc2 \cap Prot) + DecU(dec2 \cap Prot)
       IN /\ VictimsOK(ve, vd, newEnc, newDec)
          /\ (~CheckBound \/ newEnc + newDec <= Limit + prot2)
          /\ enc' = enc2 /\ dec' = dec2
          /\ chargedEnc' = newEnc /\ chargedDec' = newDec
          /\ badHeld' = (badHeld \/ (ve \cap Prot) # {} \/ (vd \cap Prot) # {})
          /\ badFloor' = (badFloor \/ (ve # {} /\ chargedEnc <= FloorEnc)
                                   \/ (vd # {} /\ chargedDec <= FloorDec))
          /\ badBound' = (badBound \/ newEnc + newDec > Limit + prot2)
    /\ UNCHANGED <<hold, pin, taken, restarted>>

\* Admit a decoded copy of b. A disabled decoded class admits nothing.
AdmitDec(b) ==
    /\ DecodedOn /\ b \notin dec
    /\ \E ve \in SUBSET enc, vd \in SUBSET (dec \ {b}) :
       LET newEnc == chargedEnc - EncU(ve)
           newDec == chargedDec - DecU(vd) + b + 1
           enc2 == enc \ ve
           dec2 == (dec \ vd) \cup {b}
           prot2 == EncU(enc2 \cap Prot) + DecU(dec2 \cap Prot)
       IN /\ VictimsOK(ve, vd, newEnc, newDec)
          /\ (~CheckBound \/ newEnc + newDec <= Limit + prot2)
          /\ enc' = enc2 /\ dec' = dec2
          /\ chargedEnc' = newEnc /\ chargedDec' = newDec
          /\ badHeld' = (badHeld \/ (ve \cap Prot) # {} \/ (vd \cap Prot) # {})
          /\ badFloor' = (badFloor \/ (ve # {} /\ chargedEnc <= FloorEnc)
                                   \/ (vd # {} /\ chargedDec <= FloorDec))
          /\ badBound' = (badBound \/ newEnc + newDec > Limit + prot2)
    /\ UNCHANGED <<hold, pin, taken, restarted>>

\* Reclaim one copy while the budget is over its limit. A class gives bytes only
\* while it holds more than its floor; the copy that crosses the floor may go. The Go reclaim also asks the class
\* furthest above its floor first; that order only picks among legal victims.
EvictEnc(b) ==
    /\ Charged > Limit /\ b \in enc
    /\ (~CheckHolds \/ b \notin Prot)
    /\ (~CheckFloor \/ chargedEnc > FloorEnc)
    /\ enc' = enc \ {b} /\ chargedEnc' = chargedEnc - b
    /\ badHeld' = (badHeld \/ b \in Prot)
    /\ badFloor' = (badFloor \/ chargedEnc <= FloorEnc)
    /\ UNCHANGED <<dec, hold, pin, chargedDec, taken, badBound, restarted>>
EvictDec(b) ==
    /\ Charged > Limit /\ b \in dec
    /\ (~CheckHolds \/ b \notin Prot)
    /\ (~CheckFloor \/ chargedDec > FloorDec)
    /\ dec' = dec \ {b} /\ chargedDec' = chargedDec - (b + 1)
    /\ badHeld' = (badHeld \/ b \in Prot)
    /\ badFloor' = (badFloor \/ chargedDec <= FloorDec)
    /\ UNCHANGED <<enc, hold, pin, chargedEnc, taken, badBound, restarted>>

\* Drop the encoded copy of a block that is also resident decoded. A held
\* block keeps its decoded copy, which serves the reader.
ReleaseDup(b) ==
    /\ b \in enc /\ b \in dec
    /\ enc' = enc \ {b} /\ chargedEnc' = chargedEnc - b
    /\ UNCHANGED <<dec, hold, pin, chargedDec, taken, badHeld, badFloor, badBound, restarted>>

Hold(b) ==
    /\ taken < MaxHolds /\ b \notin hold /\ (b \in enc \/ b \in dec)
    /\ hold' = hold \cup {b} /\ taken' = taken + 1
    /\ UNCHANGED <<enc, dec, pin, chargedEnc, chargedDec, badHeld, badFloor, badBound, restarted>>
ReleaseHold(b) ==
    /\ b \in hold /\ hold' = hold \ {b}
    /\ UNCHANGED <<enc, dec, pin, chargedEnc, chargedDec, taken, badHeld, badFloor, badBound, restarted>>
Pin(b) ==
    /\ taken < MaxHolds /\ b \notin pin /\ (b \in enc \/ b \in dec) /\ Cardinality(pin) < 1
    /\ pin' = pin \cup {b} /\ taken' = taken + 1
    /\ UNCHANGED <<enc, dec, hold, chargedEnc, chargedDec, badHeld, badFloor, badBound, restarted>>
Unpin(b) ==
    /\ b \in pin /\ pin' = pin \ {b}
    /\ UNCHANGED <<enc, dec, hold, chargedEnc, chargedDec, taken, badHeld, badFloor, badBound, restarted>>

\* A process restart empties every cache and every hold and pin.
Restart ==
    /\ ~restarted /\ restarted' = TRUE
    /\ enc' = {} /\ dec' = {} /\ hold' = {} /\ pin' = {}
    /\ chargedEnc' = 0 /\ chargedDec' = 0
    /\ UNCHANGED <<taken, badHeld, badFloor, badBound>>

Next == (\E b \in Blocks : AdmitEnc(b) \/ AdmitDec(b) \/ EvictEnc(b) \/ EvictDec(b)
                           \/ ReleaseDup(b) \/ Hold(b) \/ ReleaseHold(b)
                           \/ Pin(b) \/ Unpin(b))
        \/ Restart
        \/ (restarted /\ UNCHANGED vars)

Evictable == {<<"enc", b>> : b \in {c \in enc : c \notin Prot /\ chargedEnc > FloorEnc}}
             \cup {<<"dec", b>> : b \in {c \in dec : c \notin Prot /\ chargedDec > FloorDec}}
Fairness == /\ \A b \in Blocks : WF_vars(EvictEnc(b)) /\ WF_vars(EvictDec(b))
                                 /\ WF_vars(ReleaseHold(b)) /\ WF_vars(Unpin(b))
Spec == Init /\ [][Next]_vars /\ Fairness

\* Accounting equals the resident bytes of both classes.
Accounting == chargedEnc = EncU(enc) /\ chargedDec = DecU(dec)
\* Charged bytes exceed the limit only by the protected bytes, or while a
\* release has left an eviction pending (a hold ends without running reclaim;
\* the next admission or EvictStep runs it).
Bounded == Charged <= Limit + ProtBytes \/ Evictable # {}
\* Admission never leaves the total above the limit plus the protected bytes.
AdmitBounded == ~badBound
\* No protected copy is evicted: every held or pinned block stays resident.
HeldResident == Prot \subseteq (enc \cup dec)
NoHeldEvicted == ~badHeld
\* Eviction never takes from a class already at or below its floor.
FloorKept == ~badFloor
\* A disabled decoded class stays empty, as a disabled cache does today.
DisabledEmpty == ~DecodedOn => (dec = {} /\ chargedDec = 0)
\* Once holds and pins release, reclaim brings the total back under the limit.
ReclaimProgress == (Charged > Limit) ~> (Charged <= Limit \/ Evictable = {})
=============================================================================
