/* eslint-disable */
import { VolumeInfo } from "@go/github.com/aperturerobotics/hydra/volume/volume.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "devtool.web";

/** DevtoolInitBrowser is the message initializing the browser from the devtool. */
export interface DevtoolInitBrowser {
  /** AppId is the application id for browser storage. */
  appId: string;
  /** DevtoolPeerId is the peer id to use for the devtool link. */
  devtoolPeerId: string;
  /**
   * DevtoolVolumeInfo is the information for the devtool Volume.
   * The volume is exposed with a ProxyVolume.
   */
  devtoolVolumeInfo:
    | VolumeInfo
    | undefined;
  /** StartPlugins is a list of plugins to LoadPlugin at startup. */
  startPlugins: string[];
}

function createBaseDevtoolInitBrowser(): DevtoolInitBrowser {
  return { appId: "", devtoolPeerId: "", devtoolVolumeInfo: undefined, startPlugins: [] };
}

export const DevtoolInitBrowser = {
  encode(message: DevtoolInitBrowser, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.appId !== "") {
      writer.uint32(10).string(message.appId);
    }
    if (message.devtoolPeerId !== "") {
      writer.uint32(18).string(message.devtoolPeerId);
    }
    if (message.devtoolVolumeInfo !== undefined) {
      VolumeInfo.encode(message.devtoolVolumeInfo, writer.uint32(26).fork()).ldelim();
    }
    for (const v of message.startPlugins) {
      writer.uint32(34).string(v!);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DevtoolInitBrowser {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDevtoolInitBrowser();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.appId = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.devtoolPeerId = reader.string();
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.devtoolVolumeInfo = VolumeInfo.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.startPlugins.push(reader.string());
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
  // Transform<DevtoolInitBrowser, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<DevtoolInitBrowser | DevtoolInitBrowser[]>
      | Iterable<DevtoolInitBrowser | DevtoolInitBrowser[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [DevtoolInitBrowser.encode(p).finish()];
        }
      } else {
        yield* [DevtoolInitBrowser.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DevtoolInitBrowser>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DevtoolInitBrowser> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [DevtoolInitBrowser.decode(p)];
        }
      } else {
        yield* [DevtoolInitBrowser.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): DevtoolInitBrowser {
    return {
      appId: isSet(object.appId) ? globalThis.String(object.appId) : "",
      devtoolPeerId: isSet(object.devtoolPeerId) ? globalThis.String(object.devtoolPeerId) : "",
      devtoolVolumeInfo: isSet(object.devtoolVolumeInfo) ? VolumeInfo.fromJSON(object.devtoolVolumeInfo) : undefined,
      startPlugins: globalThis.Array.isArray(object?.startPlugins)
        ? object.startPlugins.map((e: any) => globalThis.String(e))
        : [],
    };
  },

  toJSON(message: DevtoolInitBrowser): unknown {
    const obj: any = {};
    if (message.appId !== "") {
      obj.appId = message.appId;
    }
    if (message.devtoolPeerId !== "") {
      obj.devtoolPeerId = message.devtoolPeerId;
    }
    if (message.devtoolVolumeInfo !== undefined) {
      obj.devtoolVolumeInfo = VolumeInfo.toJSON(message.devtoolVolumeInfo);
    }
    if (message.startPlugins?.length) {
      obj.startPlugins = message.startPlugins;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<DevtoolInitBrowser>, I>>(base?: I): DevtoolInitBrowser {
    return DevtoolInitBrowser.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<DevtoolInitBrowser>, I>>(object: I): DevtoolInitBrowser {
    const message = createBaseDevtoolInitBrowser();
    message.appId = object.appId ?? "";
    message.devtoolPeerId = object.devtoolPeerId ?? "";
    message.devtoolVolumeInfo = (object.devtoolVolumeInfo !== undefined && object.devtoolVolumeInfo !== null)
      ? VolumeInfo.fromPartial(object.devtoolVolumeInfo)
      : undefined;
    message.startPlugins = object.startPlugins?.map((e) => e) || [];
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
