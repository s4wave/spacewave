# Build a cold-pack manifest offline

Use the local copy of the bucket's cold prefix. Each exported part has
`XNN/entries/<pack-id>` and `XNN/packs/<pack-id>` files, with no filename
extension. Give the builder the independent exported key list, not a list made
from the pack indexes. It accepts one SHA-256 digest or encoded pack key in hex
per line, in strictly increasing key order. It needs no bucket credentials.

```sh
spacewave-world-cold-manifest --source ./cold-copy --keys ./exported-keys.hex --output ./cold.pb
```

The command refuses to publish if the local indexes do not contain exactly
the exported keys or if a pack's original bloom filter rejects one of its keys.
The resulting manifest also binds the exact bytes of each verified index tail.
Keep the bucket packs immutable and their layout identical to the mirror.

Start a native daemon with `spacewave serve --cold-pack-store <local-store-id>
--cold-pack-backend <id> --cold-pack-prefix <prefix> --cold-pack-manifest
<file>`. Use a dedicated prefix outside the live backend's entire store prefix.
`spacewave debug cold-packs stats` shows the mounted catalog and cold counters;
`spacewave debug cold-packs off` disables lookups on the running daemon, and
`on` re-enables them. A changed manifest needs a new mount or process restart.

Do not move or delete a placement while it still relies on cold-only live
blocks. Do not hide or delete a cold pack while an enabled mount's manifest
names it. First verify its live blocks are held elsewhere, remove it from the
manifest, and replace every old mount that used that manifest; alternatively,
adopt the live blocks in the main index and keep the cold tier off.
