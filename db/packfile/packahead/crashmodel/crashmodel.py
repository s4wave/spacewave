#!/usr/bin/env python3
"""Exhaustive crash model of the packahead protocol, as the code orders it.

Every step below names the file:line it models (spacewave 74b139ec1388,
hyperline cb381f52a). A breadth-first search visits every reachable state of
a small configuration, allowing a crash, kill -9 or power loss, between any
two steps, including during recovery, and checks the invariants in each state.
Each violation is reported once with its shortest trace (BFS order).

Run: python3 -I crashmodel.py [config ...]   (no arguments runs every config)

Crash kinds:
  kill9: the page cache survives: every write, rename and unlink the process
         issued stays, synced or not; process memory is lost.
  power: only fdatasync'd file data and dir-fsync'd names survive. A batch
         not yet synced survives whole, torn (present, a checksum fails) or
         not at all, and recovery keeps the leading good ones; a rename or
         unlink not yet dir-synced may or may not survive; a pack-index WAL
         frame not yet flushed may or may not survive.

Steps and their code (see STEPS below for the action names):
  W.has      packed-store.go:198 s.log.Has(key); held -> skip Append, go land
  W.reserve  log.go:269,296-315 checkReserve; may return ErrReserve (errors.go:17)
  W.write    log.go:356-361 rotate when full; log.go:403-406 held key in a
             non-final pack waits on it; log.go:439 WriteAt (page cache);
             log.go:442-460 publish: entries, table, firstAt
  W.sync     log.go:503-552 waitSynced; log.go:531 syncData (fdatasync,
             platform_linux.go:12) covers every batch written; 538 synced mark
  W.syncfail log.go:535-536 failLocked (log.go:488-498): forget unsynced
  W.land     packed-store.go:160 local.PutBlockBatch / packed-store-publish.go:52
             SubmitAtomic: the Volume commits the external entry: the ack
  W.landfull packed-store.go:109-113,151-152 divert off: full value in Volume
  M.spare    log.go:748-772 makeSpare -> createPack log.go:229-244 (O_EXCL
             create, then syncDir, sync-dir.go:7-13)
  M.rotate   log.go:557-592 rotate (by a writer or rotateLingering 776-793)
  M.fsync    log.go:803 finalizeRotated -> waitSynced
  M.tail     pack.go:156 WriteAt index tail
  M.tailsync pack.go:159 syncData
  M.rename   pack.go:195 os.Rename to <seq>-<id>.pack
  M.dirsync  pack.go:198 syncDir
  M.apply    log.go:840-853 applyFinal (GetPacks lists it from here)
  M.recfail  log.go:819-828 failed pack: recoverFile, locateRecoveredLocked
  M.drop     log.go:830-832,870-887 dropEmpty
  D.has      drain.go:143 HasPackfile -> s3/pack.go:545-558 index.Indexed
  D.putobj   drain.go:154 os.ReadFile, s3/pack.go:571 PutObject pack
  D.putentry s3/pack.go:574 PutObject entry
  D.idxwal   s3/pack.go:590 addWritten -> pack-index.go:641-670 writeSlice;
             sqlite common/tx.go:327 CommitOrdered (WAL append, no flush)
  D.idxflush common/tx.go:321 flusher.Sync; pack-index.go:536-540 indexed=true
  D.mark     drain.go:167 MarkUploaded (evict.go:74-84, memory only)
  E.forget   drain.go:174-195 evict, budget keep-mib; evict.go:46-56 table
             delete, packGone
  E.unlink   evict.go:61 os.Remove
  E.dirsync  evict.go:64 syncDir
  G.tomb     a GC tombstone of key commits in the Volume (packed-store.go:160)
  G.remove   packed-store.go:164,226-232 forgetTombstones -> log.go:644-651
  H.tick     hyperline world-backup.go:123-124 readHead, at=time.Now()
  H.advance  world-backup.go:133-140 UploadedBefore (bstore.go:248-254 ->
             writeback.go:593-622 and drain.go:199 -> evict.go:89-104
             HoldsBefore), then writeHead to B2
  B.wbup     writeback queue upload of a full Volume value (not modeled further)
  R.*        recovery, log.go:76-127 Open: per name in seq order
             (log.go:94-110): .open -> recoverPack log.go:154-190:
               R.scan     pack.go:207-229 recoverFile (record.go:113-172
                          decodeBatch: recCRC, dataCRC; truncate+fdatasync
                          only when it truncates, pack.go:220-227)
               R.remove   log.go:168-173 empty: os.Remove, then R.dirsync
               R.tail/R.tailsync/R.rename/R.dirsync  pack.go finalize again
               R.apply    log.go:183-188 applyFinal, table
             .pack -> R.load loadPack log.go:194-225 (no data checksum)
             R.create   log.go:113-119 createPack for the new open pack
             R.up       log.go:122-126 maintainer; bstore-packs.go:99 divert
                        read from config (may flip across the restart)
"""

import sys
import time
from collections import deque

# ---------------------------------------------------------------- state
#
# State is a tuple, see fields(). Packs are indexed by seq 0..NP-1.
# Pack tuple: (name, dur, batches, tail, mem, obj, entry, idx, b2c)
#   name: page-cache name: 'none' 'open' 'final' 'gone'
#   dur:  name that a power loss keeps for sure ('none' 'open' 'final' 'gone')
#   batches: tuple of (key, synced, torn)
#   tail: 0 none, 1 written, 2 synced
#   mem:  None or (state, uploaded, err, firstset, loaded, entries)
#         state 'spare' 'open' 'rotated' 'final' 'gone'; entries = count
#         of published batches (p.entries)
#   obj, entry: B2 holds the pack object / the entry object
#   idx:  pack index row: 0 none, 1 committed to WAL (page cache), 2 flushed
#   b2c:  keys of the uploaded object, with their torn flags, or None
# memidx: per pack, the PackStore's in-memory Indexed (pack-index.go:950).

P_NAME, P_DUR, P_BAT, P_TAIL, P_MEM, P_OBJ, P_ENT, P_IDX, P_B2C = range(9)
M_STATE, M_UPL, M_ERR, M_FIRST, M_LOADED, M_ENTRIES = range(6)


class Cfg:
    def __init__(self, name, **kw):
        self.name = name
        self.keys = ('A', 'B')
        self.writers = (('W1', 'A'), ('W2', 'B'))
        self.np = 4               # pack seqs available
        self.cap = 2              # batches per pack before rotation
        self.max_crashes = 1
        self.kinds = ('kill9', 'power')
        self.divert0 = True
        self.flip_divert = False  # divert may change across a restart
        self.reserve = True       # ErrReserve may fire
        self.syncfail = False
        self.gc = None            # key the GC tombstones, or None
        self.heads = 1            # head ticks per process lifetime
        self.evict = True
        # Fix / mutant switches.
        self.has_waits = False    # fix: a held key waits for its copy to sync
        self.data_check = True    # record.go:142 dataCRC check (mutant: off)
        self.evict_check = True   # drain.go:186 !p.Uploaded stop (mutant: off)
        self.row_after_sync = True  # Append returns after waitSynced (mutant)
        # fix: Open trusts only flushed pack-index rows (or flushes the WAL
        # before the drain's first HasPackfile).
        self.idx_load_flushed = False
        self.__dict__.update(kw)


def initial(c):
    packs = [empty_pack() for _ in range(c.np)]
    # Open: createPack(seq 0), open state (log.go:113-119).
    packs[0] = ('open', 'open', (), 0, ('open', False, False, False, False, 0),
                False, False, 0, None)
    writers = tuple((w, k, 'idle', None) for w, k in c.writers)
    gc = ('idle',) if c.gc else None
    return dict(up=True, crashes=0, divert=c.divert0, packs=tuple(packs),
                table=(), vol=tuple('absent' for _ in c.keys),
                b2full=tuple(False for _ in c.keys), memidx=tuple(False for _ in range(c.np)),
                writers=writers, drain=None, evict=None, gc=gc,
                waiting=None, ticks=0, bhead=None, rec=None, spare=None,
                openp=0, fin=None, everacked=tuple(False for _ in c.keys))


def empty_pack():
    return ('none', 'none', (), 0, None, False, False, 0, None)


KEYS_ORDER = ('up', 'crashes', 'divert', 'packs', 'table', 'vol', 'b2full',
              'memidx', 'writers', 'drain', 'evict', 'gc', 'waiting', 'ticks',
              'bhead', 'rec', 'spare', 'openp', 'fin', 'everacked')


def freeze(s):
    return tuple(s[k] for k in KEYS_ORDER)


def thaw(t):
    return dict(zip(KEYS_ORDER, t))


def setp(s, i, **kw):
    p = list(s['packs'][i])
    names = dict(name=P_NAME, dur=P_DUR, bat=P_BAT, tail=P_TAIL, mem=P_MEM,
                 obj=P_OBJ, entry=P_ENT, idx=P_IDX, b2c=P_B2C)
    for k, v in kw.items():
        p[names[k]] = v
    packs = list(s['packs'])
    packs[i] = tuple(p)
    s['packs'] = tuple(packs)


def setmem(s, i, **kw):
    m = list(s['packs'][i][P_MEM])
    names = dict(state=M_STATE, upl=M_UPL, err=M_ERR, first=M_FIRST,
                 loaded=M_LOADED, entries=M_ENTRIES)
    for k, v in kw.items():
        m[names[k]] = v
    setp(s, i, mem=tuple(m))


def tget(s, k):
    for kk, p in s['table']:
        if kk == k:
            return p
    return None


def tset(s, k, p):
    t = [(kk, pp) for kk, pp in s['table'] if kk != k]
    if p is not None:
        t.append((k, p))
    s['table'] = tuple(sorted(t))


def setw(s, i, pc, info=None):
    w = list(s['writers'])
    w[i] = (w[i][0], w[i][1], pc, info)
    s['writers'] = tuple(w)


def kidx(c, k):
    return c.keys.index(k)


def setvol(s, c, k, v):
    vol = list(s['vol'])
    vol[kidx(c, k)] = v
    s['vol'] = tuple(vol)


def free_seq(s):
    for i, p in enumerate(s['packs']):
        if p[P_NAME] == 'none' and p[P_DUR] == 'none' and p[P_MEM] is None:
            return i
    return None


# ---------------------------------------------------------------- steps

def successors(c, t):
    s = thaw(t)
    out = []

    def emit(label, ns):
        out.append((label, freeze(ns)))

    def cp():
        return dict(s)

    if s['up']:
        writer_steps(c, s, cp, emit)
        maint_steps(c, s, cp, emit)
        drain_steps(c, s, cp, emit)
        gc_steps(c, s, cp, emit)
        head_steps(c, s, cp, emit)
        for k in c.keys:
            if s['vol'][kidx(c, k)] == 'full' and not s['b2full'][kidx(c, k)]:
                ns = cp()
                b = list(ns['b2full'])
                b[kidx(c, k)] = True
                ns['b2full'] = tuple(b)
                emit('B.wbup(%s)' % k, ns)
    else:
        recovery_steps(c, s, cp, emit)
    if s['crashes'] < c.max_crashes:
        for kind in c.kinds:
            for label, ns in crash(c, s, kind):
                emit(label, ns)
    return out


def writer_steps(c, s, cp, emit):
    for i, (w, k, pc, info) in enumerate(s['writers']):
        if pc == 'idle':
            ns = cp()
            if not s['divert']:
                setw(ns, i, 'landfull')
                emit('%s.start(divert off)' % w, ns)
                continue
            setw(ns, i, 'has')
            emit('%s.start' % w, ns)
        elif pc == 'has':
            # packed-store.go:198: Has reads the table, durable or not.
            held = tget(s, k)
            ns = cp()
            if held is not None:
                if c.has_waits:
                    setw(ns, i, 'sync', held)
                else:
                    setw(ns, i, 'land')
                emit('%s.has(%s held in pack %d)' % (w, k, held), ns)
            else:
                setw(ns, i, 'reserve')
                emit('%s.has(%s new)' % (w, k), ns)
        elif pc == 'reserve':
            ns = cp()
            setw(ns, i, 'write')
            emit('%s.reserve(ok)' % w, ns)
            if c.reserve:
                ns = cp()
                setw(ns, i, 'failed')
                emit('%s.reserve(ErrReserve)' % w, ns)
        elif pc == 'write':
            o = s['openp']
            op = s['packs'][o]
            held = tget(s, k)
            if held is not None:
                # log.go:403-406: a held key waits on its copy's pack unless
                # that pack is final.
                ns = cp()
                if s['packs'][held][P_MEM][M_STATE] == 'final':
                    setw(ns, i, 'land')
                else:
                    setw(ns, i, 'sync', held)
                emit('%s.write(%s held, log.go:403)' % (w, k), ns)
            elif op[P_MEM][M_ERR] or len(op[P_BAT]) >= c.cap:
                pass  # needs M.rotate first (log.go:356, 394)
            else:
                ns = cp()
                setp(ns, o, bat=op[P_BAT] + ((k, False, False),))
                setmem(ns, o, first=True, entries=op[P_MEM][M_ENTRIES] + 1)
                tset(ns, k, o)
                setw(ns, i, 'sync' if c.row_after_sync else 'land', o)
                emit('%s.write(%s -> pack %d)' % (w, k, o), ns)
        elif pc == 'sync':
            p = info
            pk = s['packs'][p]
            pos = [j for j, b in enumerate(pk[P_BAT]) if b[0] == k]
            done = pos and pk[P_BAT][pos[-1]][1]
            if done:
                ns = cp()
                setw(ns, i, 'land')
                emit('%s.synced' % w, ns)
            elif pk[P_MEM][M_ERR] or not pos:
                ns = cp()
                setw(ns, i, 'failed')
                emit('%s.syncerr' % w, ns)
            else:
                ns = cp()
                setp(ns, p, bat=tuple((b[0], True, b[2]) for b in pk[P_BAT]))
                setw(ns, i, 'land')
                emit('%s.fdatasync(pack %d)' % (w, p), ns)
                if c.syncfail:
                    ns = cp()
                    fail_locked(ns, p)
                    setw(ns, i, 'failed')
                    emit('%s.fdatasync(pack %d) EIO' % (w, p), ns)
        elif pc == 'land':
            ns = cp()
            setvol(ns, c, k, 'ext')
            ack(ns, c, k)
            setw(ns, i, 'done')
            emit('%s.land(%s ext, ack)' % (w, k), ns)
        elif pc == 'landfull':
            ns = cp()
            setvol(ns, c, k, 'full')
            ack(ns, c, k)
            setw(ns, i, 'done')
            emit('%s.land(%s full, ack)' % (w, k), ns)


def ack(ns, c, k):
    e = list(ns['everacked'])
    e[kidx(c, k)] = True
    ns['everacked'] = tuple(e)


def fail_locked(ns, p):
    # log.go:488-498
    setmem(ns, p, err=True)
    for b in ns['packs'][p][P_BAT]:
        if not b[1] and tget(ns, b[0]) == p:
            tset(ns, b[0], None)


def maint_steps(c, s, cp, emit):
    # Finalize the oldest rotated pack first (log.go:727-731).
    rot = [i for i, p in enumerate(s['packs'])
           if p[P_MEM] is not None and p[P_MEM][M_STATE] == 'rotated']
    if rot:
        p = rot[0]
        pk = s['packs'][p]
        st = s['fin']
        if st is None:
            ns = cp()
            if pk[P_MEM][M_ERR]:
                ns['fin'] = 'recfail'
            else:
                setp(ns, p, bat=tuple((b[0], True, b[2]) for b in pk[P_BAT]))
                ns['fin'] = 'synced'
            emit('M.fsync(pack %d, log.go:803)' % p, ns)
        elif st == 'recfail':
            # log.go:819-828: recoverFile reads the file (page cache), keeps
            # its good batches, and relocates them.
            ns = cp()
            for b in pk[P_BAT]:
                if tget(ns, b[0]) in (None, p):
                    tset(ns, b[0], p)
            setp(ns, p, bat=tuple((b[0], True, b[2]) for b in pk[P_BAT]))
            ns['fin'] = 'synced'
            emit('M.recfail(pack %d, log.go:821)' % p, ns)
        elif st == 'synced':
            ns = cp()
            if not pk[P_BAT]:
                setp(ns, p, name='gone', dur='gone', mem=None)
                ns['fin'] = None
                emit('M.drop(pack %d, log.go:870)' % p, ns)
            else:
                setp(ns, p, tail=1)
                ns['fin'] = 'tail'
                emit('M.tail(pack %d, pack.go:156)' % p, ns)
        elif st == 'tail':
            ns = cp()
            setp(ns, p, tail=2)
            ns['fin'] = 'tailsync'
            emit('M.tailsync(pack %d, pack.go:159)' % p, ns)
        elif st == 'tailsync':
            ns = cp()
            setp(ns, p, name='final')
            ns['fin'] = 'rename'
            emit('M.rename(pack %d, pack.go:195)' % p, ns)
        elif st == 'rename':
            ns = cp()
            setp(ns, p, dur='final')
            ns['fin'] = 'dirsync'
            emit('M.dirsync(pack %d, pack.go:198)' % p, ns)
        elif st == 'dirsync':
            ns = cp()
            setmem(ns, p, state='final')
            ns['fin'] = None
            emit('M.apply(pack %d, log.go:842)' % p, ns)
    # makeSpare (log.go:734-735,748-772).
    if s['spare'] is None:
        f = free_seq(s)
        if f is not None:
            ns = cp()
            setp(ns, f, name='open', dur='open',
                 mem=('spare', False, False, False, False, 0))
            ns['spare'] = f
            emit('M.spare(pack %d, log.go:755)' % f, ns)
    # rotate (log.go:557-592): the writer when full or failed, or the
    # lingering rotation (log.go:737-744, 776-793) once it holds a batch.
    o = s['openp']
    op = s['packs'][o]
    if op[P_MEM][M_ENTRIES] > 0 or op[P_MEM][M_ERR]:
        if s['spare'] is not None:
            ns = cp()
            setmem(ns, o, state='rotated')
            setmem(ns, s['spare'], state='open')
            ns['openp'], ns['spare'] = s['spare'], None
            emit('M.rotate(pack %d -> %d)' % (o, s['spare']), ns)


def oldest_unuploaded(s):
    for i, p in enumerate(s['packs']):
        m = p[P_MEM]
        if m is not None and m[M_STATE] == 'final' and not m[M_UPL]:
            return i
    return None


def drain_steps(c, s, cp, emit):
    d = s['drain']
    if d is None:
        p = oldest_unuploaded(s)
        if p is not None:
            ns = cp()
            if s['memidx'][p]:
                setmem(ns, p, upl=True)
                emit('D.has(pack %d listed)+D.mark' % p, ns)
            else:
                ns['drain'] = (p, 'putobj')
                emit('D.has(pack %d not listed)' % p, ns)
            return
        evict_steps(c, s, cp, emit)
        return
    if not c.evict_check:
        evict_steps(c, s, cp, emit)
    p, st = d
    pk = s['packs'][p]
    ns = cp()
    if pk[P_MEM] is None or pk[P_NAME] == 'gone':
        ns['drain'] = None  # only the evict mutant gets here: a failed step
        emit('D.fail(pack %d gone)' % p, ns)
        return
    if st == 'putobj':
        # drain.go:154 os.ReadFile then the upload.
        setp(ns, p, obj=True, b2c=tuple((b[0], b[2]) for b in pk[P_BAT]))
        ns['drain'] = (p, 'putentry')
        emit('D.putobj(pack %d)' % p, ns)
    elif st == 'putentry':
        setp(ns, p, entry=True)
        ns['drain'] = (p, 'idxwal')
        emit('D.putentry(pack %d)' % p, ns)
    elif st == 'idxwal':
        setp(ns, p, idx=max(pk[P_IDX], 1))
        ns['drain'] = (p, 'idxflush')
        emit('D.idxwal(pack %d)' % p, ns)
    elif st == 'idxflush':
        # A WAL flush makes every earlier frame durable too.
        for q in range(c.np):
            if ns['packs'][q][P_IDX] == 1:
                setp(ns, q, idx=2)
        mi = list(ns['memidx'])
        mi[p] = True
        ns['memidx'] = tuple(mi)
        ns['drain'] = (p, 'mark')
        emit('D.idxflush(pack %d)' % p, ns)
    elif st == 'mark':
        setmem(ns, p, upl=True)
        ns['drain'] = None
        emit('D.mark(pack %d)' % p, ns)


def evict_steps(c, s, cp, emit):
    if not c.evict:
        return
    e = s['evict']
    if e is None:
        for i, p in enumerate(s['packs']):
            m = p[P_MEM]
            if m is None or m[M_STATE] != 'final':
                continue
            if c.evict_check and not m[M_UPL]:
                break  # drain.go:186 stops at the first not uploaded
            ns = cp()
            for k in c.keys:
                if tget(ns, k) == i:
                    tset(ns, k, None)
            setmem(ns, i, state='gone')
            ns['evict'] = (i, 'unlink')
            emit('E.forget(pack %d, evict.go:46)' % i, ns)
            break  # oldest first
        return
    p, st = e
    ns = cp()
    if st == 'unlink':
        setp(ns, p, name='gone')
        ns['evict'] = (p, 'dirsync')
        emit('E.unlink(pack %d, evict.go:61)' % p, ns)
    else:
        setp(ns, p, dur='gone', mem=None)
        ns['evict'] = None
        emit('E.dirsync(pack %d, evict.go:64)' % p, ns)


def gc_steps(c, s, cp, emit):
    g = s['gc']
    if g is None:
        return
    k = c.gc
    if g[0] == 'idle' and s['vol'][kidx(c, k)] in ('ext', 'full'):
        ns = cp()
        setvol(ns, c, k, 'tomb')
        ns['gc'] = ('tombed',)
        emit('G.tomb(%s)' % k, ns)
    elif g[0] == 'tombed':
        ns = cp()
        tset(ns, k, None)
        ns['gc'] = ('done',)
        emit('G.remove(%s, log.go:649)' % k, ns)


def live_keys(c, s):
    return tuple(k for k in c.keys if s['vol'][kidx(c, k)] in ('ext', 'full'))


def head_steps(c, s, cp, emit):
    if s['waiting'] is None and s['ticks'] < c.heads:
        keys = live_keys(c, s)
        if keys:
            # firstAt before at: packs that published a batch, or were loaded.
            before = tuple(i for i, p in enumerate(s['packs'])
                           if p[P_MEM] is not None and (p[P_MEM][M_FIRST] or p[P_MEM][M_LOADED]))
            fulls = tuple(k for k in keys if s['vol'][kidx(c, k)] == 'full')
            ns = cp()
            ns['waiting'] = (keys, before, fulls)
            ns['ticks'] = s['ticks'] + 1
            emit('H.tick(%s)' % ','.join(keys), ns)
    if s['waiting'] is not None:
        keys, before, fulls = s['waiting']
        # writeback.go:593-622, simplified: every full value queued before
        # at uploaded.
        wb = all(s['b2full'][kidx(c, k)] for k in fulls)
        holds = False
        for i, p in enumerate(s['packs']):
            m = p[P_MEM]
            if m is None or m[M_STATE] in ('gone', 'spare') or m[M_UPL]:
                continue
            if m[M_STATE] != 'final' and m[M_ENTRIES] == 0:
                continue
            if not m[M_LOADED] and i not in before:
                continue
            holds = True
        if wb and not holds:
            ns = cp()
            ns['bhead'] = keys
            ns['waiting'] = None
            emit('H.advance(%s)' % ','.join(keys), ns)


# ---------------------------------------------------------------- crash

def crash(c, s, kind):
    """Yield every state the crash can leave."""
    base = dict(s)
    base['up'] = False
    base['crashes'] = s['crashes'] + 1
    base['writers'] = tuple((w, k, 'failed' if pc not in ('done', 'failed', 'idle') else pc, None)
                            for w, k, pc, _ in s['writers'])
    base['drain'] = None
    base['evict'] = None
    base['waiting'] = None
    base['ticks'] = 0
    base['table'] = ()
    base['memidx'] = tuple(False for _ in range(c.np))
    base['spare'] = None
    base['fin'] = None
    base['rec'] = ()
    if s['gc'] is not None and s['gc'][0] == 'tombed':
        base['gc'] = ('done',)
    per_pack = []
    for p in s['packs']:
        per_pack.append(list(pack_outcomes(p, kind)))
    for combo in product(per_pack):
        ns = dict(base)
        ns['packs'] = tuple(combo)
        yield ('CRASH(%s)' % kind, ns)


def product(lists):
    if not lists:
        yield ()
        return
    for head in lists[0]:
        for rest in product(lists[1:]):
            yield (head,) + rest


def pack_outcomes(p, kind):
    name, dur, bat, tail, mem, obj, ent, idx, b2c = p
    if kind == 'kill9':
        # Page cache survives: names, data, WAL frames all stay as issued.
        yield (name, dur, bat, tail, None, obj, ent, idx, b2c)
        return
    names = {dur} if name == dur else {dur, name}
    if 'none' in names and len(names) > 1:
        names = {n for n in names if n != 'none'}  # create is dir-synced at once
    idxs = {idx} if idx != 1 else {0, 1}
    nsync = 0
    while nsync < len(bat) and bat[nsync][1]:
        nsync += 1
    cuts = []
    for keep in range(nsync, len(bat) + 1):
        kept = tuple((b[0], True, b[2]) for b in bat[:keep])
        cuts.append(kept)
        if keep < len(bat):
            torn = bat[keep]
            cuts.append(kept + ((torn[0], True, True),))
    for n in sorted(names):
        for kept in cuts:
            for ix in sorted(idxs):
                t = tail
                if n == 'final' and tail < 2:
                    raise AssertionError('final name with an unsynced tail')
                if n != 'final':
                    t = 0 if tail < 2 else tail
                yield (n, n, kept, t, None, obj, ent, ix, b2c)


# ---------------------------------------------------------------- recovery

def recovery_steps(c, s, cp, emit):
    rec = s['rec']
    # rec: tuple of done seqs, or ('work', seq, stage) as last element.
    if rec and isinstance(rec[-1], tuple):
        _, p, st = rec[-1]
        done = rec[:-1]
        pk = s['packs'][p]
        ns = cp()
        if st == 'tail':
            setp(ns, p, tail=1)
            ns['rec'] = done + (('work', p, 'tailsync'),)
            emit('R.tail(pack %d, pack.go:156)' % p, ns)
        elif st == 'tailsync':
            setp(ns, p, tail=2, bat=tuple((b[0], True, b[2]) for b in pk[P_BAT]))
            ns['rec'] = done + (('work', p, 'rename'),)
            emit('R.tailsync(pack %d, pack.go:159)' % p, ns)
        elif st == 'rename':
            setp(ns, p, name='final')
            ns['rec'] = done + (('work', p, 'dirsync'),)
            emit('R.rename(pack %d, pack.go:195)' % p, ns)
        elif st == 'dirsync':
            setp(ns, p, dur='final')
            ns['rec'] = done + (('work', p, 'apply'),)
            emit('R.dirsync(pack %d, pack.go:198)' % p, ns)
        elif st == 'apply':
            setp(ns, p, mem=('final', False, False, False, True, len(pk[P_BAT])))
            for b in pk[P_BAT]:
                tset(ns, b[0], p)
            ns['rec'] = done + (p,)
            emit('R.apply(pack %d, log.go:183)' % p, ns)
        elif st == 'rmdirsync':
            setp(ns, p, dur='gone')
            ns['rec'] = done + (p,)
            emit('R.dirsync-remove(pack %d, log.go:173)' % p, ns)
        return
    done = rec
    for i, p in enumerate(s['packs']):
        if i in done:
            continue
        if p[P_NAME] == 'open':
            # recoverFile: keep the leading good batches (record.go:113-172).
            kept = []
            for b in p[P_BAT]:
                if b[2] and c.data_check:
                    break
                kept.append(b)
            truncated = len(kept) < len(p[P_BAT])
            if truncated:
                kept = [(b[0], True, b[2]) for b in kept]  # pack.go:221-226
            ns = cp()
            setp(ns, i, bat=tuple(kept), tail=0)
            if not kept:
                setp(ns, i, name='gone')
                ns['rec'] = done + (('work', i, 'rmdirsync'),)
                emit('R.scan+remove(pack %d, log.go:168-170)' % i, ns)
            else:
                ns['rec'] = done + (('work', i, 'tail'),)
                emit('R.scan(pack %d, pack.go:219)' % i, ns)
            return
        if p[P_NAME] == 'final':
            ns = cp()
            setp(ns, i, mem=('final', False, False, False, True, len(p[P_BAT])))
            for b in p[P_BAT]:
                tset(ns, b[0], i)
            ns['rec'] = done + (i,)
            emit('R.load(pack %d, log.go:194)' % i, ns)
            return
    # Every pack done: the pack index loads its durable rows, then createPack.
    f = free_seq(s)
    if f is None:
        return  # bound: no seq left; the branch ends (counted as a bound stop)
    for divert in ((True, False) if c.flip_divert else (s['divert'],)):
        ns = cp()
        need = 2 if c.idx_load_flushed else 1
        mi = tuple(s['packs'][q][P_IDX] >= need for q in range(c.np))
        ns['memidx'] = mi
        setp(ns, f, name='open', dur='open',
             mem=('open', False, False, False, False, 0))
        ns['openp'] = f
        ns['rec'] = None
        ns['up'] = True
        ns['divert'] = divert
        emit('R.create+up(pack %d, divert=%s)' % (f, divert), ns)


# ---------------------------------------------------------------- invariants

def durable_local(c, s, k):
    """k survives any power loss in a local pack file."""
    for p in s['packs']:
        if p[P_NAME] in ('none', 'gone') or p[P_DUR] in ('none', 'gone'):
            continue
        for b in p[P_BAT]:
            if b[0] == k and b[1] and not b[2]:
                return True
    return False


def in_b2(c, s, k, need_idx=2):
    for p in s['packs']:
        if p[P_OBJ] and p[P_ENT] and p[P_IDX] >= need_idx and p[P_B2C]:
            if any(kk == k and not torn for kk, torn in p[P_B2C]):
                return True
    return False


def readable(c, s, k):
    """packed-store.go:245-266 then the backend: log.Get, local, B2."""
    loc = tget(s, k)
    if loc is not None and s['packs'][loc][P_NAME] not in ('gone', 'none'):
        return True
    if s['vol'][kidx(c, k)] == 'full':
        return True
    for i, p in enumerate(s['packs']):
        if p[P_OBJ] and p[P_ENT] and (s['memidx'][i] or p[P_IDX] == 2) and p[P_B2C]:
            if any(kk == k for kk, _ in p[P_B2C]):
                return True
    return False


def check(c, s):
    bad = []
    live = live_keys(c, s)
    # I1/I4: every acknowledged, live block survives any power loss now.
    for k in live:
        if s['vol'][kidx(c, k)] == 'full':
            continue
        if not (durable_local(c, s, k) or in_b2(c, s, k)):
            bad.append(('I1', k))
    # I2: an unlinked pack is durably in B2 with its index row.
    for i, p in enumerate(s['packs']):
        if p[P_NAME] == 'gone' and p[P_B2C] is not None:
            if not (p[P_OBJ] and p[P_ENT] and p[P_IDX] == 2):
                bad.append(('I2', i))
        if p[P_NAME] == 'gone' and p[P_B2C] is None and any(
                b[0] in live and not b[2] for b in p[P_BAT]) and p[P_MEM] is None and False:
            pass
    # I3: nothing torn is served or uploaded.
    for kk, pi in s['table']:
        pk = s['packs'][pi]
        mine = [b for b in pk[P_BAT] if b[0] == kk]
        if mine and mine[-1][2]:
            bad.append(('I3-served', kk))
    for i, p in enumerate(s['packs']):
        if p[P_B2C] and any(t for _, t in p[P_B2C]):
            bad.append(('I3-uploaded', i))
    # I5: the bucket head names only blocks B2 holds.
    if s['bhead']:
        for k in s['bhead']:
            if not (in_b2(c, s, k, need_idx=1) or s['b2full'][kidx(c, k)]):
                bad.append(('I5', k))
    # R: a read of a live, acknowledged block finds it (while up).
    if s['up']:
        for k in live:
            if not readable(c, s, k):
                bad.append(('R-notfound', k))
    return bad


# ---------------------------------------------------------------- search

def explore(c, limit=20_000_000):
    t0 = time.time()
    init = freeze(initial(c))
    parent = {init: None}
    q = deque([init])
    found = {}
    edges = 0
    while q:
        t = q.popleft()
        s = thaw(t)
        for v in check(c, s):
            if v not in found:
                found[v] = t
        for label, nt in successors(c, t):
            edges += 1
            if nt not in parent:
                parent[nt] = (t, label)
                q.append(nt)
                if len(parent) > limit:
                    raise RuntimeError('state limit')
    return dict(states=len(parent), edges=edges, secs=time.time() - t0,
                found=found, parent=parent)


def trace(parent, t):
    out = []
    while parent[t] is not None:
        t, label = parent[t]
        out.append(label)
    return list(reversed(out))


CONFIGS = [
    Cfg('base', writers=(('W1', 'A'), ('W2', 'B')), cap=1, np=4, max_crashes=2,
        flip_divert=True),
    Cfg('dup', writers=(('W1', 'A'), ('W2', 'A')), cap=2, np=4, max_crashes=1,
        flip_divert=True),
    Cfg('dup-fixed', writers=(('W1', 'A'), ('W2', 'A')), cap=2, np=4,
        max_crashes=1, has_waits=True),
    Cfg('gc', writers=(('W1', 'A'), ('W2', 'A')), cap=2, np=3, max_crashes=1,
        gc='A', heads=0, reserve=False),
    Cfg('syncfail', writers=(('W1', 'A'), ('W2', 'A')), cap=2, np=3,
        max_crashes=0, syncfail=True, heads=0, reserve=False, has_waits=False),
    Cfg('syncfail-fixed', writers=(('W1', 'A'), ('W2', 'A')), cap=2, np=3,
        max_crashes=0, syncfail=True, heads=0, reserve=False, has_waits=True),
    Cfg('base-fixed', writers=(('W1', 'A'), ('W2', 'B')), cap=1, np=4,
        max_crashes=2, flip_divert=True, has_waits=True, idx_load_flushed=True),
    Cfg('dup-fixed2', writers=(('W1', 'A'), ('W2', 'A')), cap=2, np=4,
        max_crashes=2, flip_divert=True, has_waits=True, idx_load_flushed=True),
    Cfg('gc-fixed', writers=(('W1', 'A'), ('W2', 'A')), cap=2, np=3,
        max_crashes=1, gc='A', heads=0, reserve=False, has_waits=True,
        idx_load_flushed=True),
    Cfg('mutant-nodatacheck', idx_load_flushed=True, has_waits=True, writers=(('W1', 'A'),), cap=1, np=3,
        max_crashes=1, data_check=False, heads=0),
    Cfg('mutant-noevictcheck', idx_load_flushed=True, has_waits=True, writers=(('W1', 'A'),), cap=1, np=3,
        max_crashes=0, evict_check=False, heads=0),
    Cfg('mutant-ackbeforesync', idx_load_flushed=True, has_waits=True, writers=(('W1', 'A'),), cap=1, np=3,
        max_crashes=1, row_after_sync=False, heads=0),
]


def main(argv):
    want = set(argv)
    for c in CONFIGS:
        if want and c.name not in want:
            continue
        r = explore(c)
        print('== %s: %d states, %d edges, %.1fs' % (c.name, r['states'], r['edges'], r['secs']))
        print('   bounds: keys=%s writers=%s cap=%d packs=%d crashes=%d kinds=%s '
              'flip_divert=%s reserve=%s syncfail=%s gc=%s heads=%d has_waits=%s '
              'data_check=%s evict_check=%s row_after_sync=%s idx_load_flushed=%s' % (
                  c.keys, [w for w in c.writers], c.cap, c.np, c.max_crashes,
                  c.kinds, c.flip_divert, c.reserve, c.syncfail, c.gc, c.heads,
                  c.has_waits, c.data_check, c.evict_check, c.row_after_sync,
                  c.idx_load_flushed))
        if not r['found']:
            print('   all invariants hold')
        for v, t in sorted(r['found'].items(), key=lambda kv: len(trace(r['parent'], kv[1]))):
            tr = trace(r['parent'], t)
            print('   VIOLATION %s (%d steps): %s' % (v, len(tr), ' ; '.join(tr)))


if __name__ == '__main__':
    main(sys.argv[1:])
