# Build a cold-pack manifest offline

Use the local copy of the bucket's cold prefix. Each exported part has
`XNN/entries/<pack-id>` and `XNN/packs/<pack-id>` files, with no filename
extension. Give the builder the independent exported key list, not a list made
from the pack indexes. It accepts one SHA-256 digest or encoded pack key in hex
per line, in strictly increasing key order. It needs no bucket credentials.

```sh
spacewave-world-cold-manifest --source ./cold-copy --keys ./exported-keys.hex --output ./cold.pb
```

The command verifies every local block against its index key. It refuses to
publish if a block fails verification, the pack indexes disagree with the
exported set, or a pack's original bloom filter rejects one of its keys.
Export each key once, even if more than one pack holds it.

On each range response the mount checks the object size. It also checks the
pack SHA-256 when it receives the entire object or the bucket supplies a
whole-object SHA-256. Without that proof, the index can locate a block but only
verified bytes can establish presence. Keep the packs immutable: a cached proof
cannot see a replacement or deletion until a range read observes it. Rebuild
manifests made before whole-pack SHA-256 was recorded.

Start a native daemon with `spacewave serve --cold-pack-store <local-store-id>
--cold-pack-backend <id> --cold-pack-prefix <prefix> --cold-pack-manifest
<file>`. Use a dedicated prefix outside the live backend's entire store prefix.
`spacewave debug cold-packs stats` shows the mounted catalog and cold counters;
`spacewave debug cold-packs off` disables lookups on the running daemon, and
`on` re-enables them. A changed manifest needs a new mount or process restart.

Before moving a placement, keep its old cold mount enabled and readable so the
move can copy cold-only blocks. Do not hide or delete a cold pack while an
enabled mount's manifest names it. First verify its live blocks are held elsewhere, remove it from the
manifest, and replace every old mount that used that manifest; alternatively,
adopt the live blocks in the main index and keep the cold tier off.
