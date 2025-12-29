# Bug Report: Incorrect Filestat Struct Offsets in write_bytes

## Summary

The `Filestat.write_bytes()` method in `wasi_defs.ts` uses incorrect memory offsets for the `atim`, `mtim`, and `ctim` timestamp fields, violating the WASI snapshot_preview1 ABI.

## Affected Code

**File:** `src/wasi_defs.ts` (in `@bjorn3/browser_wasi_shim`)

```typescript
write_bytes(view: DataView, ptr: number) {
  view.setBigUint64(ptr, this.dev, true)
  view.setBigUint64(ptr + 8, this.ino, true)
  view.setUint8(ptr + 16, this.filetype)
  view.setBigUint64(ptr + 24, this.nlink, true)
  view.setBigUint64(ptr + 32, this.size, true)
  view.setBigUint64(ptr + 38, this.atim, true)   // BUG: should be ptr + 40
  view.setBigUint64(ptr + 46, this.mtim, true)   // BUG: should be ptr + 48
  view.setBigUint64(ptr + 52, this.ctim, true)   // BUG: should be ptr + 56
}
```

## Expected Behavior

Per the WASI snapshot_preview1 specification, `__wasi_filestat_t` follows standard C struct layout rules with natural alignment. The correct struct layout is:

| Field     | Type | Offset | Size | Notes                          |
| --------- | ---- | ------ | ---- | ------------------------------ |
| dev       | u64  | 0      | 8    |                                |
| ino       | u64  | 8      | 8    |                                |
| filetype  | u8   | 16     | 1    |                                |
| (padding) | -    | 17     | 7    | Alignment padding for next u64 |
| nlink     | u64  | 24     | 8    |                                |
| size      | u64  | 32     | 8    |                                |
| atim      | u64  | 40     | 8    | **Currently written at 38**    |
| mtim      | u64  | 48     | 8    | **Currently written at 46**    |
| ctim      | u64  | 56     | 8    | **Currently written at 52**    |

Total struct size: 64 bytes

## Impact

1. **Incorrect timestamps**: WASM modules reading filestat will get corrupted timestamp values
2. **Memory misalignment**: Writing u64 values at non-8-byte-aligned offsets (38, 46, 52) may cause issues on some platforms
3. **Data overlap**: The incorrectly placed fields overlap with the correct field positions, causing data corruption

## References

- [WASI snapshot_preview1 witx definition](https://github.com/WebAssembly/WASI/blob/main/legacy/preview1/witx/wasi_snapshot_preview1.witx)
- [wasi-libc api.h](https://github.com/WebAssembly/wasi-libc/blob/main/libc-bottom-half/headers/public/wasi/api.h) - defines `__wasi_filestat_t`

## Suggested Fix

```typescript
write_bytes(view: DataView, ptr: number) {
  view.setBigUint64(ptr, this.dev, true)
  view.setBigUint64(ptr + 8, this.ino, true)
  view.setUint8(ptr + 16, this.filetype)
  view.setBigUint64(ptr + 24, this.nlink, true)
  view.setBigUint64(ptr + 32, this.size, true)
  view.setBigUint64(ptr + 40, this.atim, true)   // Fixed offset
  view.setBigUint64(ptr + 48, this.mtim, true)   // Fixed offset
  view.setBigUint64(ptr + 56, this.ctim, true)   // Fixed offset
}
```

## Upstream

This bug exists in the upstream `@bjorn3/browser_wasi_shim` npm package and should be reported to: https://github.com/bjorn3/browser_wasi_shim
