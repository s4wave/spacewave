import * as $ from "@goscript/builtin/index.js";
import * as protobuf from "@goscript/github.com/aperturerobotics/protobuf-go-lite/index.js";
import * as packfile from "@goscript/github.com/s4wave/spacewave/core/provider/spacewave/packfile/index.js";

const cdnRootPointerMethods: $.MethodSignature[] = [
  { name: "Reset", args: [], returns: [] },
  { name: "ProtoMessage", args: [], returns: [] },
  { name: "GetSpaceId", args: [], returns: [{ name: "", type: "string" }] },
  { name: "GetConfigChainHash", args: [], returns: [] },
  { name: "GetConfigChainSeqno", args: [], returns: [{ name: "", type: "number" }] },
  { name: "GetPacks", args: [], returns: [] },
  { name: "GetCreatedAtMs", args: [], returns: [{ name: "", type: "number" }] },
  {
    name: "MarshalVT",
    args: [],
    returns: [
      { name: "", type: { kind: $.TypeKind.Slice, elemType: "number" } },
      { name: "", type: "error" },
    ],
  },
  {
    name: "UnmarshalVT",
    args: [{ name: "data", type: { kind: $.TypeKind.Slice, elemType: "number" } }],
    returns: [{ name: "", type: "error" }],
  },
];

export class CdnRootPointer {
  public _fields: {
    unknownFields: $.VarRef<$.Bytes>;
    SpaceId: $.VarRef<string>;
    ConfigChainHash: $.VarRef<$.Bytes>;
    ConfigChainSeqno: $.VarRef<number>;
    Packs: $.VarRef<$.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null>>;
    CreatedAtMs: $.VarRef<number>;
  };

  constructor(init?: Partial<{
    unknownFields: $.Bytes;
    SpaceId: string;
    ConfigChainHash: $.Bytes;
    ConfigChainSeqno: number;
    Packs: $.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null>;
    CreatedAtMs: number;
  }>) {
    this._fields = {
      unknownFields: $.varRef(init?.unknownFields ?? null),
      SpaceId: $.varRef(init?.SpaceId ?? ""),
      ConfigChainHash: $.varRef(init?.ConfigChainHash ?? null),
      ConfigChainSeqno: $.varRef(init?.ConfigChainSeqno ?? 0),
      Packs: $.varRef(init?.Packs ?? null),
      CreatedAtMs: $.varRef(init?.CreatedAtMs ?? 0),
    };
  }

  get unknownFields(): $.Bytes { return this._fields.unknownFields.value; }
  set unknownFields(value: $.Bytes) { this._fields.unknownFields.value = value; }
  get SpaceId(): string { return this._fields.SpaceId.value; }
  set SpaceId(value: string) { this._fields.SpaceId.value = value; }
  get ConfigChainHash(): $.Bytes { return this._fields.ConfigChainHash.value; }
  set ConfigChainHash(value: $.Bytes) { this._fields.ConfigChainHash.value = value; }
  get ConfigChainSeqno(): number { return this._fields.ConfigChainSeqno.value; }
  set ConfigChainSeqno(value: number) { this._fields.ConfigChainSeqno.value = value; }
  get Packs(): $.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null> {
    return this._fields.Packs.value;
  }
  set Packs(value: $.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null>) {
    this._fields.Packs.value = value;
  }
  get CreatedAtMs(): number { return this._fields.CreatedAtMs.value; }
  set CreatedAtMs(value: number) { this._fields.CreatedAtMs.value = value; }

  Reset(): void { $.assignStruct(this, new CdnRootPointer()); }
  ProtoMessage(): void {}
  GetSpaceId(): string { return this.SpaceId; }
  GetConfigChainHash(): $.Bytes { return this.ConfigChainHash; }
  GetConfigChainSeqno(): number { return this.ConfigChainSeqno; }
  GetPacks(): $.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null> {
    return this.Packs;
  }
  GetCreatedAtMs(): number { return this.CreatedAtMs; }

  MarshalVT(): [$.Bytes, $.GoError] {
    let out: $.Slice<number> = [];
    if (this.SpaceId !== "") {
      const body = $.stringToBytes(this.SpaceId);
      out = $.append(out, 0x0a);
      out = protobuf.AppendVarint(out, $.len(body));
      out = $.append(out, ...Array.from(body as Uint8Array));
    }
    if (this.ConfigChainHash !== null && $.len(this.ConfigChainHash) !== 0) {
      out = $.append(out, 0x1a);
      out = protobuf.AppendVarint(out, $.len(this.ConfigChainHash));
      out = $.append(out, ...Array.from(this.ConfigChainHash as Uint8Array));
    }
    if (this.ConfigChainSeqno !== 0) {
      out = $.append(out, 0x20);
      out = protobuf.AppendVarint(out, this.ConfigChainSeqno);
    }
    for (const entryRef of this.Packs ?? []) {
      const entry = $.pointerValue<packfile.PackfileEntry | null>(entryRef);
      if (entry === null) {
        continue;
      }
      const [body, err] = entry.MarshalVT();
      if (err !== null) {
        return [null, err];
      }
      out = $.append(out, 0x2a);
      out = protobuf.AppendVarint(out, $.len(body));
      out = $.append(out, ...Array.from(body as Uint8Array));
    }
    if (this.CreatedAtMs !== 0) {
      out = $.append(out, 0x30);
      out = protobuf.AppendVarint(out, this.CreatedAtMs);
    }
    if (this.unknownFields !== null) {
      out = $.append(out, ...Array.from(this.unknownFields as Uint8Array));
    }
    return [out, null];
  }

  UnmarshalVT(data: $.Bytes): $.GoError {
    let index = 0;
    const bytes = $.normalizeBytes(data);
    while (index < bytes.length) {
      const preIndex = index;
      const [wire, next, wireErr] = protobuf.DecodeVarint(bytes, index);
      if (wireErr !== null) {
        return wireErr;
      }
      index = next;
      const fieldNumber = $.int($.uint64Shr(wire, 3), 32);
      const wireType = $.int($.uint64And(wire, 0x7), 32);
      if (fieldNumber <= 0) {
        return $.newError(`proto: CdnRootPointer: illegal tag ${fieldNumber} (wire type ${wire})`);
      }
      if (fieldNumber === 1) {
        if (wireType !== 2) {
          return $.newError(`proto: wrong wireType = ${wireType} for field SpaceId`);
        }
        const [size, sizeNext, sizeErr] = protobuf.DecodeVarint(bytes, index);
        if (sizeErr !== null) {
          return sizeErr;
        }
        index = sizeNext;
        const postIndex = index + $.int(size);
        if (postIndex < index || postIndex > bytes.length) {
          return protobuf.ErrInvalidLength;
        }
        this.SpaceId = $.bytesToString(bytes.subarray(index, postIndex));
        index = postIndex;
        continue;
      }
      if (fieldNumber === 3) {
        if (wireType !== 2) {
          return $.newError(`proto: wrong wireType = ${wireType} for field ConfigChainHash`);
        }
        const [size, sizeNext, sizeErr] = protobuf.DecodeVarint(bytes, index);
        if (sizeErr !== null) {
          return sizeErr;
        }
        index = sizeNext;
        const postIndex = index + $.int(size);
        if (postIndex < index || postIndex > bytes.length) {
          return protobuf.ErrInvalidLength;
        }
        this.ConfigChainHash = Uint8Array.from(bytes.subarray(index, postIndex));
        index = postIndex;
        continue;
      }
      if (fieldNumber === 4) {
        if (wireType !== 0) {
          return $.newError(`proto: wrong wireType = ${wireType} for field ConfigChainSeqno`);
        }
        const [value, valueNext, valueErr] = protobuf.DecodeVarint(bytes, index);
        if (valueErr !== null) {
          return valueErr;
        }
        this.ConfigChainSeqno = Number(value);
        index = valueNext;
        continue;
      }
      if (fieldNumber === 5) {
        if (wireType !== 2) {
          return $.newError(`proto: wrong wireType = ${wireType} for field Packs`);
        }
        const [size, sizeNext, sizeErr] = protobuf.DecodeVarint(bytes, index);
        if (sizeErr !== null) {
          return sizeErr;
        }
        index = sizeNext;
        const postIndex = index + $.int(size);
        if (postIndex < index || postIndex > bytes.length) {
          return protobuf.ErrInvalidLength;
        }
        const entry = new packfile.PackfileEntry();
        const err = entry.UnmarshalVT(bytes.subarray(index, postIndex));
        if (err !== null) {
          return err;
        }
        this.Packs = $.append(this.Packs ?? [], entry);
        index = postIndex;
        continue;
      }
      if (fieldNumber === 6) {
        if (wireType !== 0) {
          return $.newError(`proto: wrong wireType = ${wireType} for field CreatedAtMs`);
        }
        const [value, valueNext, valueErr] = protobuf.DecodeVarint(bytes, index);
        if (valueErr !== null) {
          return valueErr;
        }
        this.CreatedAtMs = Number(value);
        index = valueNext;
        continue;
      }
      const [skip, skipErr] = protobuf.Skip(bytes.subarray(preIndex));
      if (skipErr !== null) {
        return skipErr;
      }
      if (skip < 0 || preIndex + skip > bytes.length) {
        return protobuf.ErrInvalidLength;
      }
      index = preIndex + skip;
    }
    if (index > bytes.length) {
      return protobuf.ErrInvalidLength;
    }
    return null;
  }

  SizeVT(): number {
    const [data] = this.MarshalVT();
    return $.len(data);
  }

  static __typeInfo = $.registerStructType(
    "cdn.CdnRootPointer",
    () => new CdnRootPointer(),
    cdnRootPointerMethods,
    CdnRootPointer,
    {},
  );
}
