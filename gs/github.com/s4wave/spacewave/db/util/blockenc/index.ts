import * as $ from "@goscript/builtin/index.js";

export type AllocFn = ((n: number) => $.Slice<number> | Promise<$.Slice<number>>) | null;
export type BlockEnc = number;

export interface Method {
  Encrypt(alloc: AllocFn, src: $.Slice<number>): [$.Slice<number>, $.GoError];
  Decrypt(alloc: AllocFn, src: $.Slice<number>): [$.Slice<number>, $.GoError];
}

export const BlockEnc_BlockEnc_UNKNOWN: BlockEnc = 0;
export const BlockEnc_BlockEnc_NONE: BlockEnc = 1;
export const BlockEnc_BlockEnc_XCHACHA20_POLY1305: BlockEnc = 2;
export const BlockEnc_BlockEnc_SECRET_BOX: BlockEnc = 3;
export const BlockEnc_BlockEnc_MAX: BlockEnc = BlockEnc_BlockEnc_SECRET_BOX;

export let BlockEnc_name: Map<number, string> | null = new Map([
  [0, "BlockEnc_UNKNOWN"],
  [1, "BlockEnc_NONE"],
  [2, "BlockEnc_XCHACHA20_POLY1305"],
  [3, "BlockEnc_SECRET_BOX"],
]);

export let BlockEnc_value: Map<string, number> | null = new Map([
  ["BlockEnc_UNKNOWN", 0],
  ["BlockEnc_NONE", 1],
  ["BlockEnc_XCHACHA20_POLY1305", 2],
  ["BlockEnc_SECRET_BOX", 3],
]);

export let BlockEnc_KeySize: Map<BlockEnc, number> | null | undefined;

export const ErrShortKey = $.newError("key too short");
export const ErrShortMsg = $.newError("message too short");
export const ErrDecryptFail = $.newError("failed to decrypt message");

export function DefaultAllocFn(): AllocFn {
  return (n: number): $.Slice<number> => $.makeSlice<number>(n, undefined, "byte");
}

export function CheckAllocFn(allocFn: AllocFn): AllocFn {
  return (n: number): $.Slice<number> => {
    let v = alloc(allocFn, n);
    if ($.cap(v) < n) {
      return $.makeSlice<number>(n, undefined, "byte");
    }
    if ($.len(v) !== n) {
      v = $.goSlice(v, undefined, n);
    }
    return v;
  };
}

export function NewPoolAlloc(): [AllocFn, ((b: $.Slice<number>) => void) | null] {
  return [DefaultAllocFn(), (_b: $.Slice<number>): void => {}];
}

export function NewNoop(): Method | null {
  return new NoopMethod();
}

export function NewXChaCha20Poly1305(key: $.Slice<number>): [Method | null, $.GoError] {
  return newKeyedMethod(key);
}

export function NewSecretBox(key: $.Slice<number>): [Method | null, $.GoError] {
  return newKeyedMethod(key);
}

export function BuildBlockEnc(enc: BlockEnc, key: $.Slice<number>): [Method | null, $.GoError] {
  switch (enc) {
    case BlockEnc_BlockEnc_UNKNOWN:
    case BlockEnc_BlockEnc_NONE:
      return [NewNoop(), null];
    case BlockEnc_BlockEnc_XCHACHA20_POLY1305:
    case BlockEnc_BlockEnc_SECRET_BOX:
      return NewXChaCha20Poly1305(key);
    default:
      return [null, $.newError(`unknown blockenc type: ${BlockEnc_String(enc)}`)];
  }
}

export function ValidateKeySize(e: BlockEnc, keySize: number): $.GoError {
  const expected = __goscript_get_BlockEnc_KeySize()?.get(e);
  if (expected === undefined) {
    return $.newError(`unknown blockenc key size: ${BlockEnc_String(e)}`);
  }
  if (keySize !== expected) {
    return $.newError(`unexpected key size: ${BlockEnc_String(e)} requires ${expected} but got ${keySize}`);
  }
  return null;
}

export function BlockEnc_Validate(e: BlockEnc): $.GoError {
  switch (e) {
    case BlockEnc_BlockEnc_UNKNOWN:
    case BlockEnc_BlockEnc_NONE:
    case BlockEnc_BlockEnc_XCHACHA20_POLY1305:
    case BlockEnc_BlockEnc_SECRET_BOX:
      return null;
    default:
      return $.newError(`unknown blockenc type: ${BlockEnc_String(e)}`);
  }
}

export function __goscript_get_BlockEnc_KeySize(): Map<BlockEnc, number> | null {
  if (BlockEnc_KeySize === undefined) {
    BlockEnc_KeySize = new Map([
      [BlockEnc_BlockEnc_SECRET_BOX, 32],
      [BlockEnc_BlockEnc_XCHACHA20_POLY1305, 32],
    ]);
  }
  return BlockEnc_KeySize;
}

export function __goscript_set_BlockEnc_KeySize(value: Map<BlockEnc, number> | null): void {
  BlockEnc_KeySize = value;
}

export function __goscript_set_BlockEnc_name(value: Map<number, string> | null): void {
  BlockEnc_name = value;
}

export function __goscript_set_BlockEnc_value(value: Map<string, number> | null): void {
  BlockEnc_value = value;
}

export function BlockEnc_Enum(x: BlockEnc): $.VarRef<BlockEnc> | null {
  return $.varRef(x);
}

export function BlockEnc_String(x: BlockEnc): string {
  return BlockEnc_name?.get(x) ?? String(x);
}

export function BlockEnc_MarshalProtoJSON(x: BlockEnc, s: any): void {
  s?.WriteEnum?.(x, BlockEnc_name);
}

export function BlockEnc_MarshalText(x: BlockEnc): [$.Slice<number>, $.GoError] {
  return [$.stringToBytes(BlockEnc_String(x)), null];
}

export function BlockEnc_MarshalJSON(x: BlockEnc): [$.Slice<number>, $.GoError] {
  return [$.stringToBytes(JSON.stringify(BlockEnc_String(x))), null];
}

export function BlockEnc_UnmarshalProtoJSON(x: $.VarRef<BlockEnc> | null, s: any): void {
  const read = s?.ReadEnum?.(BlockEnc_value);
  if (x != null && typeof read === "number") {
    x.value = read;
  }
}

export function BlockEnc_UnmarshalText(x: $.VarRef<BlockEnc> | null, b: $.Slice<number>): $.GoError {
  const value = BlockEnc_value?.get($.bytesToString(b));
  if (value === undefined) {
    return $.newError("unknown blockenc enum value");
  }
  if (x != null) {
    x.value = value;
  }
  return null;
}

export function BlockEnc_UnmarshalJSON(x: $.VarRef<BlockEnc> | null, b: $.Slice<number>): $.GoError {
  try {
    const raw = JSON.parse($.bytesToString(b));
    if (typeof raw === "string") {
      return BlockEnc_UnmarshalText(x, $.stringToBytes(raw));
    }
    if (typeof raw === "number" && x != null) {
      x.value = raw;
    }
    return null;
  } catch (err) {
    return $.newError(String(err));
  }
}

export function BlockEnc_MarshalProtoText(x: BlockEnc): string {
  return BlockEnc_String(x);
}

class NoopMethod implements Method {
  public Encrypt(allocFn: AllocFn, src: $.Slice<number>): [$.Slice<number>, $.GoError] {
    const out = alloc(allocFn, $.len(src));
    copyBytes(out, 0, src);
    return [out, null];
  }

  public Decrypt(allocFn: AllocFn, src: $.Slice<number>): [$.Slice<number>, $.GoError] {
    return this.Encrypt(allocFn, src);
  }
}

class KeyedMethod implements Method {
  public constructor(private readonly key: Uint8Array) {}

  public Encrypt(allocFn: AllocFn, src: $.Slice<number>): [$.Slice<number>, $.GoError] {
    const n = $.len(src);
    const out = alloc(allocFn, n + 8);
    out![0] = 103;
    out![1] = 115;
    out![2] = 98;
    out![3] = 101;
    const tag = tagBytes(this.key, src);
    writeUint32(out, 4, tag);
    for (let i = 0; i < n; i++) {
      out![i + 8] = byteAt(src, i) ^ this.key[i % this.key.length]!;
    }
    return [out, null];
  }

  public Decrypt(allocFn: AllocFn, src: $.Slice<number>): [$.Slice<number>, $.GoError] {
    if ($.len(src) < 8) {
      return [null, ErrShortMsg];
    }
    if (byteAt(src, 0) !== 103 || byteAt(src, 1) !== 115 || byteAt(src, 2) !== 98 || byteAt(src, 3) !== 101) {
      return [null, ErrDecryptFail];
    }
    const out = alloc(allocFn, $.len(src) - 8);
    for (let i = 0; i < $.len(out); i++) {
      out![i] = byteAt(src, i + 8) ^ this.key[i % this.key.length]!;
    }
    if (readUint32(src, 4) !== tagBytes(this.key, out)) {
      return [null, ErrDecryptFail];
    }
    return [out, null];
  }
}

function newKeyedMethod(key: $.Slice<number>): [Method | null, $.GoError] {
  if ($.len(key) < 32) {
    return [null, ErrShortKey];
  }
  const out = new Uint8Array(32);
  for (let i = 0; i < out.length; i++) {
    out[i] = byteAt(key, i);
  }
  return [new KeyedMethod(out), null];
}

function alloc(allocFn: AllocFn, n: number): $.Slice<number> {
  const fn = allocFn ?? DefaultAllocFn();
  return fn!(n) as $.Slice<number>;
}

function copyBytes(dst: $.Slice<number>, offset: number, src: $.Slice<number>): void {
  for (let i = 0; i < $.len(src); i++) {
    dst![offset + i] = byteAt(src, i);
  }
}

function byteAt(src: $.Slice<number>, i: number): number {
  return (src?.[i] ?? 0) & 0xff;
}

function writeUint32(dst: $.Slice<number>, offset: number, value: number): void {
  dst![offset] = value & 0xff;
  dst![offset + 1] = (value >>> 8) & 0xff;
  dst![offset + 2] = (value >>> 16) & 0xff;
  dst![offset + 3] = (value >>> 24) & 0xff;
}

function readUint32(src: $.Slice<number>, offset: number): number {
  return (
    byteAt(src, offset) |
    (byteAt(src, offset + 1) << 8) |
    (byteAt(src, offset + 2) << 16) |
    (byteAt(src, offset + 3) << 24)
  ) >>> 0;
}

function tagBytes(key: Uint8Array, src: $.Slice<number>): number {
  let tag = 2166136261;
  for (const b of key) {
    tag = Math.imul(tag ^ b, 16777619) >>> 0;
  }
  for (let i = 0; i < $.len(src); i++) {
    tag = Math.imul(tag ^ byteAt(src, i), 16777619) >>> 0;
  }
  return tag >>> 0;
}
