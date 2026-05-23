import * as $ from "@goscript/builtin/index.js";
import * as protobuf from "@goscript/github.com/aperturerobotics/protobuf-go-lite/index.js";
import * as packfile from "@goscript/github.com/s4wave/spacewave/core/provider/spacewave/packfile/index.js";

const cdnRootPointerMethods: $.MethodSignature[] = [
  { name: "Reset", args: [], returns: [] },
  { name: "ProtoMessage", args: [], returns: [] },
  { name: "GetSpaceId", args: [], returns: [{ name: "", type: "string" }] },
  { name: "GetPacks", args: [], returns: [] },
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
    Packs: $.VarRef<$.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null>>;
  };

  constructor(init?: Partial<{
    unknownFields: $.Bytes;
    SpaceId: string;
    Packs: $.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null>;
  }>) {
    this._fields = {
      unknownFields: $.varRef(init?.unknownFields ?? null),
      SpaceId: $.varRef(init?.SpaceId ?? ""),
      Packs: $.varRef(init?.Packs ?? null),
    };
  }

  get unknownFields(): $.Bytes { return this._fields.unknownFields.value; }
  set unknownFields(value: $.Bytes) { this._fields.unknownFields.value = value; }
  get SpaceId(): string { return this._fields.SpaceId.value; }
  set SpaceId(value: string) { this._fields.SpaceId.value = value; }
  get Packs(): $.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null> {
    return this._fields.Packs.value;
  }
  set Packs(value: $.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null>) {
    this._fields.Packs.value = value;
  }

  Reset(): void { $.assignStruct(this, new CdnRootPointer()); }
  ProtoMessage(): void {}
  GetSpaceId(): string { return this.SpaceId; }
  GetPacks(): $.Slice<packfile.PackfileEntry | $.VarRef<packfile.PackfileEntry> | null> {
    return this.Packs;
  }

  MarshalVT(): [$.Bytes, $.GoError] {
    let out: $.Slice<number> = [];
    if (this.SpaceId !== "") {
      const body = $.stringToBytes(this.SpaceId);
      out = $.append(out, 0x0a);
      out = protobuf.AppendVarint(out, $.len(body));
      out = $.append(out, ...Array.from(body as Uint8Array));
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
