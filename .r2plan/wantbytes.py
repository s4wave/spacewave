# wantbytes.py (RUN AFTER Q5, nice -n 19): the round-2 kept bytes under rule B.
# For each key of round2scan/r2/typed.k with an h row in view.db (mode=ro), sum
# the h row's extent size (value bytes 12:16) and count rows with no extent;
# lookups 500 at a time, as viewjoin.py. Prints counts only.
import sqlite3, hashlib
S = "/media/storage-2/cjs-goalc/"
pre = b"pi" + hashlib.sha256(b"hyperline-world").digest()[:8]
db = sqlite3.connect("file:" + S + "subidx/view.db?mode=ro", uri=True)
n = hit = noext = 0; tot = 0; hist = {}
def flush(b):
    global hit, noext, tot
    rows = db.execute("select value from pack_index where key in (%s)" % ",".join("?" * len(b)),
                      [pre + b"h" + k[-16:] for k in b])
    for (v,) in rows:
        hit += 1
        if len(v) >= 16:
            s = int.from_bytes(v[12:16], "big"); tot += s
            hist[s.bit_length()] = hist.get(s.bit_length(), 0) + 1
        else: noext += 1
b = []
for l in open(S + "round2scan/r2/typed.k"):
    b.append(bytes.fromhex(l.strip())); n += 1
    if len(b) == 500: flush(b); b = []
if b: flush(b)
print("keys %d h-row %d extent-bytes %d no-extent %d" % (n, hit, tot, noext))
print("size log2 hist", sorted(hist.items()))
