/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { EsbuildOutput } from "../../web/esbuild/esbuild.pb.js";

export const protobufPackage = "bldr.plugin.compiler.vardef";

/** PluginDevInfo is information passed as a .bin file as part of the development plugin entrypoint. */
export interface PluginDevInfo {
  /** PluginVars is the set of variables to set at init time by the development plugin. */
  pluginVars: PluginVar[];
}

/** PluginVar contains a definition of a variable set by the development plugin entrypoint. */
export interface PluginVar {
  /** PkgImportPath is the go package import path. */
  pkgImportPath: string;
  /** PkgVar is the variable within the go package. */
  pkgVar: string;
  body?:
    | { $case: "stringValue"; stringValue: string }
    | { $case: "esbuildOutput"; esbuildOutput: EsbuildOutput }
    | undefined;
}

function createBasePluginDevInfo(): PluginDevInfo {
  return { pluginVars: [] };
}

export const PluginDevInfo = {
  encode(message: PluginDevInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    for (const v of message.pluginVars) {
      PluginVar.encode(v!, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginDevInfo {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginDevInfo();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.pluginVars.push(PluginVar.decode(reader, reader.uint32()));
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginDevInfo, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<PluginDevInfo | PluginDevInfo[]> | Iterable<PluginDevInfo | PluginDevInfo[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [PluginDevInfo.encode(p).finish()];
        }
      } else {
        yield* [PluginDevInfo.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginDevInfo>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginDevInfo> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [PluginDevInfo.decode(p)];
        }
      } else {
        yield* [PluginDevInfo.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): PluginDevInfo {
    return {
      pluginVars: globalThis.Array.isArray(object?.pluginVars)
        ? object.pluginVars.map((e: any) => PluginVar.fromJSON(e))
        : [],
    };
  },

  toJSON(message: PluginDevInfo): unknown {
    const obj: any = {};
    if (message.pluginVars?.length) {
      obj.pluginVars = message.pluginVars.map((e) => PluginVar.toJSON(e));
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginDevInfo>, I>>(base?: I): PluginDevInfo {
    return PluginDevInfo.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<PluginDevInfo>, I>>(object: I): PluginDevInfo {
    const message = createBasePluginDevInfo();
    message.pluginVars = object.pluginVars?.map((e) => PluginVar.fromPartial(e)) || [];
    return message;
  },
};

function createBasePluginVar(): PluginVar {
  return { pkgImportPath: "", pkgVar: "", body: undefined };
}

export const PluginVar = {
  encode(message: PluginVar, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pkgImportPath !== "") {
      writer.uint32(10).string(message.pkgImportPath);
    }
    if (message.pkgVar !== "") {
      writer.uint32(18).string(message.pkgVar);
    }
    switch (message.body?.$case) {
      case "stringValue":
        writer.uint32(26).string(message.body.stringValue);
        break;
      case "esbuildOutput":
        EsbuildOutput.encode(message.body.esbuildOutput, writer.uint32(34).fork()).ldelim();
        break;
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginVar {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginVar();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.pkgImportPath = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.pkgVar = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.body = { $case: "stringValue", stringValue: reader.string() };
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.body = { $case: "esbuildOutput", esbuildOutput: EsbuildOutput.decode(reader, reader.uint32()) };
          continue;
      }
      if ((tag & 7) === 4 || tag === 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<PluginVar, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<PluginVar | PluginVar[]> | Iterable<PluginVar | PluginVar[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [PluginVar.encode(p).finish()];
        }
      } else {
        yield* [PluginVar.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginVar>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginVar> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [PluginVar.decode(p)];
        }
      } else {
        yield* [PluginVar.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): PluginVar {
    return {
      pkgImportPath: isSet(object.pkgImportPath) ? globalThis.String(object.pkgImportPath) : "",
      pkgVar: isSet(object.pkgVar) ? globalThis.String(object.pkgVar) : "",
      body: isSet(object.stringValue)
        ? { $case: "stringValue", stringValue: globalThis.String(object.stringValue) }
        : isSet(object.esbuildOutput)
        ? { $case: "esbuildOutput", esbuildOutput: EsbuildOutput.fromJSON(object.esbuildOutput) }
        : undefined,
    };
  },

  toJSON(message: PluginVar): unknown {
    const obj: any = {};
    if (message.pkgImportPath !== "") {
      obj.pkgImportPath = message.pkgImportPath;
    }
    if (message.pkgVar !== "") {
      obj.pkgVar = message.pkgVar;
    }
    if (message.body?.$case === "stringValue") {
      obj.stringValue = message.body.stringValue;
    }
    if (message.body?.$case === "esbuildOutput") {
      obj.esbuildOutput = EsbuildOutput.toJSON(message.body.esbuildOutput);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginVar>, I>>(base?: I): PluginVar {
    return PluginVar.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<PluginVar>, I>>(object: I): PluginVar {
    const message = createBasePluginVar();
    message.pkgImportPath = object.pkgImportPath ?? "";
    message.pkgVar = object.pkgVar ?? "";
    if (
      object.body?.$case === "stringValue" &&
      object.body?.stringValue !== undefined &&
      object.body?.stringValue !== null
    ) {
      message.body = { $case: "stringValue", stringValue: object.body.stringValue };
    }
    if (
      object.body?.$case === "esbuildOutput" &&
      object.body?.esbuildOutput !== undefined &&
      object.body?.esbuildOutput !== null
    ) {
      message.body = { $case: "esbuildOutput", esbuildOutput: EsbuildOutput.fromPartial(object.body.esbuildOutput) };
    }
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends Long ? string | number | Long : T extends globalThis.Array<infer U> ? globalThis.Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U> ? ReadonlyArray<DeepPartial<U>>
  : T extends { $case: string } ? { [K in keyof Omit<T, "$case">]?: DeepPartial<T[K]> } & { $case: T["$case"] }
  : T extends {} ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>;

type KeysOfUnion<T> = T extends T ? keyof T : never;
export type Exact<P, I extends P> = P extends Builtin ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & { [K in Exclude<keyof I, KeysOfUnion<P>>]: never };

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any;
  _m0.configure();
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
