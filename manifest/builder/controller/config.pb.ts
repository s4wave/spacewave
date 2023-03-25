/* eslint-disable */
import { ControllerConfig } from "@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js";
import { Backoff } from "@go/github.com/aperturerobotics/util/backoff/backoff.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";
import { BuilderConfig } from "../builder.pb.js";

export const protobufPackage = "bldr.manifest.builder.controller";

/** Config is the manifest builder controller configuration. */
export interface Config {
  /**
   * BuilderConfig contains common config for the manifest builder.
   * Overridden by the project controller.
   */
  builderConfig:
    | BuilderConfig
    | undefined;
  /**
   * ControllerConfig is the config to use for the manifest builder controller.
   * The ControllerConfig must be a manifest build controller Config.
   */
  controllerConfig:
    | ControllerConfig
    | undefined;
  /**
   * BuildBackoff is the backoff config for building manifests.
   * If unset, defaults to reasonable defaults.
   */
  buildBackoff:
    | Backoff
    | undefined;
  /** Watch enables watching for changes. */
  watch: boolean;
}

function createBaseConfig(): Config {
  return { builderConfig: undefined, controllerConfig: undefined, buildBackoff: undefined, watch: false };
}

export const Config = {
  encode(message: Config, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.builderConfig !== undefined) {
      BuilderConfig.encode(message.builderConfig, writer.uint32(10).fork()).ldelim();
    }
    if (message.controllerConfig !== undefined) {
      ControllerConfig.encode(message.controllerConfig, writer.uint32(18).fork()).ldelim();
    }
    if (message.buildBackoff !== undefined) {
      Backoff.encode(message.buildBackoff, writer.uint32(26).fork()).ldelim();
    }
    if (message.watch === true) {
      writer.uint32(32).bool(message.watch);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Config {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseConfig();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag != 10) {
            break;
          }

          message.builderConfig = BuilderConfig.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag != 18) {
            break;
          }

          message.controllerConfig = ControllerConfig.decode(reader, reader.uint32());
          continue;
        case 3:
          if (tag != 26) {
            break;
          }

          message.buildBackoff = Backoff.decode(reader, reader.uint32());
          continue;
        case 4:
          if (tag != 32) {
            break;
          }

          message.watch = reader.bool();
          continue;
      }
      if ((tag & 7) == 4 || tag == 0) {
        break;
      }
      reader.skipType(tag & 7);
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<Config, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Config | Config[]> | Iterable<Config | Config[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.encode(p).finish()];
        }
      } else {
        yield* [Config.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Config>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Config> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Config.decode(p)];
        }
      } else {
        yield* [Config.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Config {
    return {
      builderConfig: isSet(object.builderConfig) ? BuilderConfig.fromJSON(object.builderConfig) : undefined,
      controllerConfig: isSet(object.controllerConfig) ? ControllerConfig.fromJSON(object.controllerConfig) : undefined,
      buildBackoff: isSet(object.buildBackoff) ? Backoff.fromJSON(object.buildBackoff) : undefined,
      watch: isSet(object.watch) ? Boolean(object.watch) : false,
    };
  },

  toJSON(message: Config): unknown {
    const obj: any = {};
    message.builderConfig !== undefined &&
      (obj.builderConfig = message.builderConfig ? BuilderConfig.toJSON(message.builderConfig) : undefined);
    message.controllerConfig !== undefined &&
      (obj.controllerConfig = message.controllerConfig ? ControllerConfig.toJSON(message.controllerConfig) : undefined);
    message.buildBackoff !== undefined &&
      (obj.buildBackoff = message.buildBackoff ? Backoff.toJSON(message.buildBackoff) : undefined);
    message.watch !== undefined && (obj.watch = message.watch);
    return obj;
  },

  create<I extends Exact<DeepPartial<Config>, I>>(base?: I): Config {
    return Config.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Config>, I>>(object: I): Config {
    const message = createBaseConfig();
    message.builderConfig = (object.builderConfig !== undefined && object.builderConfig !== null)
      ? BuilderConfig.fromPartial(object.builderConfig)
      : undefined;
    message.controllerConfig = (object.controllerConfig !== undefined && object.controllerConfig !== null)
      ? ControllerConfig.fromPartial(object.controllerConfig)
      : undefined;
    message.buildBackoff = (object.buildBackoff !== undefined && object.buildBackoff !== null)
      ? Backoff.fromPartial(object.buildBackoff)
      : undefined;
    message.watch = object.watch ?? false;
    return message;
  },
};

type Builtin = Date | Function | Uint8Array | string | number | boolean | undefined;

export type DeepPartial<T> = T extends Builtin ? T
  : T extends Long ? string | number | Long : T extends Array<infer U> ? Array<DeepPartial<U>>
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
