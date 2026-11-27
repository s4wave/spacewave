# sched.py IDS [OUT]: order the round-2 packs (id \t wanted \t size_bytes) by
# wanted keys per byte, descending, and simulate the whole-pack FTPS fetch on
# 1 and 3 sessions with the round-1 fit 195.9 ms + size/52.6 MB/s per pack.
# Prints counts only; OUT gets the ordered list.
import sys, heapq
rows = []
for l in open(sys.argv[1]):
    i, w, s = l.rstrip("\n").split("\t")
    rows.append((i, int(w), int(s) if s != "None" else None))
nos = [r for r in rows if r[2] is None]
rows = [r for r in rows if r[2] is not None]
import os
mode = os.environ.get("ORDER", "byte")
band_of = lambda w: 0 if w >= 16 else (1 if w >= 2 else 2)
if mode == "byte": rows.sort(key=lambda r: (-r[1] / r[2], r[0]))
elif mode == "time": rows.sort(key=lambda r: (-r[1] / (0.1959 + r[2] / 52.6e6), r[0]))
elif mode == "bandbyte": rows.sort(key=lambda r: (band_of(r[1]), -r[1] / r[2], r[0]))
print("order", mode)
W = sum(r[1] for r in rows); B = sum(r[2] for r in rows)
print("ids %d no-size %d wanted-sum %d bytes %d" % (len(rows) + len(nos), len(nos), W, B))
bands = [("x>=16", lambda w: w >= 16), ("2-15", lambda w: 2 <= w < 16), ("1", lambda w: w == 1)]
for n, f in bands:
    sel = [r for r in rows if f(r[1])]
    print("band %s packs %d keys %d bytes %d" % (n, len(sel), sum(r[1] for r in sel), sum(r[2] for r in sel)))
if len(sys.argv) > 2:
    with open(sys.argv[2], "w") as o:
        for r in rows: o.write("%s\t%d\t%d\n" % r)
T = lambda s: 0.1959 + s / 52.6e6
marks = [1, 2, 3, 6, 9]
for ns in (1, 3):
    free = [0.0] * ns; heapq.heapify(free)
    done = []  # (finish time, keys, bytes)
    for i, w, s in rows:
        t = heapq.heappop(free); t2 = t + T(s); heapq.heappush(free, t2); done.append((t2, w, s))
    done.sort()
    end = max(d[0] for d in done)
    out = []; k = b = p = 0; j = 0
    for h in marks + [end / 3600]:
        while j < len(done) and done[j][0] <= h * 3600:
            k += done[j][1]; b += done[j][2]; p += 1; j += 1
        out.append("h%.2f packs %d keys %d (%.1f%%) bytes %d" % (h, p, k, 100.0 * k / W, b))
    print("sessions %d end %.2f h" % (ns, end / 3600))
    for x in out: print("  " + x)
