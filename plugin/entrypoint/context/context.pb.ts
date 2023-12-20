/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { PluginMeta } from "../../plugin.pb.js";

export const protobufPackage = "plugin.entrypoint.context";

/**
 * PluginContextInfo contains information about the running plugin attached to
 * the Context tree for access within controllers running on the plugin bus.
 */
export interface PluginContextInfo {
  /** PluginMeta is the plugin metadata. */
  pluginMeta: PluginMeta | undefined;
}

function createBasePluginContextInfo(): PluginContextInfo {
  return { pluginMeta: undefined };
}

export const PluginContextInfo = {
  encode(message: PluginContextInfo, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.pluginMeta !== undefined) {
      PluginMeta.encode(message.pluginMeta, writer.uint32(10).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): PluginContextInfo {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBasePluginContextInfo();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.pluginMeta = PluginMeta.decode(reader, reader.uint32());
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
  // Transform<PluginContextInfo, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<PluginContextInfo | PluginContextInfo[]> | Iterable<PluginContextInfo | PluginContextInfo[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [PluginContextInfo.encode(p).finish()];
        }
      } else {
        yield* [PluginContextInfo.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, PluginContextInfo>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<PluginContextInfo> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [PluginContextInfo.decode(p)];
        }
      } else {
        yield* [PluginContextInfo.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): PluginContextInfo {
    return { pluginMeta: isSet(object.pluginMeta) ? PluginMeta.fromJSON(object.pluginMeta) : undefined };
  },

  toJSON(message: PluginContextInfo): unknown {
    const obj: any = {};
    if (message.pluginMeta !== undefined) {
      obj.pluginMeta = PluginMeta.toJSON(message.pluginMeta);
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<PluginContextInfo>, I>>(base?: I): PluginContextInfo {
    return PluginContextInfo.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<PluginContextInfo>, I>>(object: I): PluginContextInfo {
    const message = createBasePluginContextInfo();
    message.pluginMeta = (object.pluginMeta !== undefined && object.pluginMeta !== null)
      ? PluginMeta.fromPartial(object.pluginMeta)
      : undefined;
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
