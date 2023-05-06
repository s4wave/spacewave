/* eslint-disable */
import { ControllerConfig } from "@go/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.js";
import { ExecControllerRequest } from "@go/github.com/aperturerobotics/controllerbus/controller/exec/exec.pb.js";
import { BlockRef } from "@go/github.com/aperturerobotics/hydra/block/block.pb.js";
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "assembly.block";

/** Assembly is a definition of a set of configs to run on a Bus. */
export interface Assembly {
  /**
   * ControllerExec is the list of controllers to run.
   * Either ConfigSet protobuf or yaml format.
   */
  controllerExec:
    | ExecControllerRequest
    | undefined;
  /** SubAssemblies is the list of sub-assembly configs. */
  subAssemblies: SubAssembly[];
}

/**
 * SubAssembly configures a separate Bus to be run as a child.
 * Can be configured to optionally inherit parent plugins and resolvers.
 */
export interface SubAssembly {
  /**
   * Id is the identifier for the subassembly used for logging.
   * Can be empty.
   */
  id: string;
  /** Assemblies is the list of assembiles to apply to the sub-bus. */
  assemblies: Assembly[];
  /**
   * AssemblyRefs contains a list of block ref to Assembly.
   * The referenced Assembly list will be merged with assemblies.
   */
  assemblyRefs: BlockRef[];
  /**
   * DirectiveBridges configures the list of directive bridges to the parent bus.
   * Can include bridges to resolve plugins, controllers, etc.
   */
  directiveBridges: DirectiveBridge[];
}

/**
 * DirectiveBridge connects two Bus by applying Directives to the other.
 * Can be configured for one-way or two-way bridging.
 */
export interface DirectiveBridge {
  /**
   * ControllerConfig configures the directive bridge controller.
   * The controller factory will be looked up on the parent bus.
   * The controller must implement DirectiveBridgeController.
   * The controller is not run on the bus, but rather as a sub-controller.
   * If empty (nil), the directive bridge will be ignored.
   */
  controllerConfig:
    | ControllerConfig
    | undefined;
  /** BridgeToParent indicates the target is the parent, not the subassembly. */
  bridgeToParent: boolean;
}

function createBaseAssembly(): Assembly {
  return { controllerExec: undefined, subAssemblies: [] };
}

export const Assembly = {
  encode(message: Assembly, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.controllerExec !== undefined) {
      ExecControllerRequest.encode(message.controllerExec, writer.uint32(10).fork()).ldelim();
    }
    for (const v of message.subAssemblies) {
      SubAssembly.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): Assembly {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseAssembly();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.controllerExec = ExecControllerRequest.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.subAssemblies.push(SubAssembly.decode(reader, reader.uint32()));
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
  // Transform<Assembly, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<Assembly | Assembly[]> | Iterable<Assembly | Assembly[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Assembly.encode(p).finish()];
        }
      } else {
        yield* [Assembly.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, Assembly>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<Assembly> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [Assembly.decode(p)];
        }
      } else {
        yield* [Assembly.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): Assembly {
    return {
      controllerExec: isSet(object.controllerExec) ? ExecControllerRequest.fromJSON(object.controllerExec) : undefined,
      subAssemblies: Array.isArray(object?.subAssemblies)
        ? object.subAssemblies.map((e: any) => SubAssembly.fromJSON(e))
        : [],
    };
  },

  toJSON(message: Assembly): unknown {
    const obj: any = {};
    message.controllerExec !== undefined &&
      (obj.controllerExec = message.controllerExec ? ExecControllerRequest.toJSON(message.controllerExec) : undefined);
    if (message.subAssemblies) {
      obj.subAssemblies = message.subAssemblies.map((e) => e ? SubAssembly.toJSON(e) : undefined);
    } else {
      obj.subAssemblies = [];
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<Assembly>, I>>(base?: I): Assembly {
    return Assembly.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<Assembly>, I>>(object: I): Assembly {
    const message = createBaseAssembly();
    message.controllerExec = (object.controllerExec !== undefined && object.controllerExec !== null)
      ? ExecControllerRequest.fromPartial(object.controllerExec)
      : undefined;
    message.subAssemblies = object.subAssemblies?.map((e) => SubAssembly.fromPartial(e)) || [];
    return message;
  },
};

function createBaseSubAssembly(): SubAssembly {
  return { id: "", assemblies: [], assemblyRefs: [], directiveBridges: [] };
}

export const SubAssembly = {
  encode(message: SubAssembly, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.id !== "") {
      writer.uint32(10).string(message.id);
    }
    for (const v of message.assemblies) {
      Assembly.encode(v!, writer.uint32(18).fork()).ldelim();
    }
    for (const v of message.assemblyRefs) {
      BlockRef.encode(v!, writer.uint32(26).fork()).ldelim();
    }
    for (const v of message.directiveBridges) {
      DirectiveBridge.encode(v!, writer.uint32(34).fork()).ldelim();
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): SubAssembly {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSubAssembly();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.id = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.assemblies.push(Assembly.decode(reader, reader.uint32()));
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          message.assemblyRefs.push(BlockRef.decode(reader, reader.uint32()));
          continue;
        case 4:
          if (tag !== 34) {
            break;
          }

          message.directiveBridges.push(DirectiveBridge.decode(reader, reader.uint32()));
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
  // Transform<SubAssembly, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<SubAssembly | SubAssembly[]> | Iterable<SubAssembly | SubAssembly[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SubAssembly.encode(p).finish()];
        }
      } else {
        yield* [SubAssembly.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SubAssembly>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<SubAssembly> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SubAssembly.decode(p)];
        }
      } else {
        yield* [SubAssembly.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): SubAssembly {
    return {
      id: isSet(object.id) ? String(object.id) : "",
      assemblies: Array.isArray(object?.assemblies) ? object.assemblies.map((e: any) => Assembly.fromJSON(e)) : [],
      assemblyRefs: Array.isArray(object?.assemblyRefs)
        ? object.assemblyRefs.map((e: any) => BlockRef.fromJSON(e))
        : [],
      directiveBridges: Array.isArray(object?.directiveBridges)
        ? object.directiveBridges.map((e: any) => DirectiveBridge.fromJSON(e))
        : [],
    };
  },

  toJSON(message: SubAssembly): unknown {
    const obj: any = {};
    message.id !== undefined && (obj.id = message.id);
    if (message.assemblies) {
      obj.assemblies = message.assemblies.map((e) => e ? Assembly.toJSON(e) : undefined);
    } else {
      obj.assemblies = [];
    }
    if (message.assemblyRefs) {
      obj.assemblyRefs = message.assemblyRefs.map((e) => e ? BlockRef.toJSON(e) : undefined);
    } else {
      obj.assemblyRefs = [];
    }
    if (message.directiveBridges) {
      obj.directiveBridges = message.directiveBridges.map((e) => e ? DirectiveBridge.toJSON(e) : undefined);
    } else {
      obj.directiveBridges = [];
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<SubAssembly>, I>>(base?: I): SubAssembly {
    return SubAssembly.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<SubAssembly>, I>>(object: I): SubAssembly {
    const message = createBaseSubAssembly();
    message.id = object.id ?? "";
    message.assemblies = object.assemblies?.map((e) => Assembly.fromPartial(e)) || [];
    message.assemblyRefs = object.assemblyRefs?.map((e) => BlockRef.fromPartial(e)) || [];
    message.directiveBridges = object.directiveBridges?.map((e) => DirectiveBridge.fromPartial(e)) || [];
    return message;
  },
};

function createBaseDirectiveBridge(): DirectiveBridge {
  return { controllerConfig: undefined, bridgeToParent: false };
}

export const DirectiveBridge = {
  encode(message: DirectiveBridge, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.controllerConfig !== undefined) {
      ControllerConfig.encode(message.controllerConfig, writer.uint32(10).fork()).ldelim();
    }
    if (message.bridgeToParent === true) {
      writer.uint32(16).bool(message.bridgeToParent);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): DirectiveBridge {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseDirectiveBridge();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.controllerConfig = ControllerConfig.decode(reader, reader.uint32());
          continue;
        case 2:
          if (tag !== 16) {
            break;
          }

          message.bridgeToParent = reader.bool();
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
  // Transform<DirectiveBridge, Uint8Array>
  async *encodeTransform(
    source: AsyncIterable<DirectiveBridge | DirectiveBridge[]> | Iterable<DirectiveBridge | DirectiveBridge[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DirectiveBridge.encode(p).finish()];
        }
      } else {
        yield* [DirectiveBridge.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, DirectiveBridge>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<DirectiveBridge> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [DirectiveBridge.decode(p)];
        }
      } else {
        yield* [DirectiveBridge.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): DirectiveBridge {
    return {
      controllerConfig: isSet(object.controllerConfig) ? ControllerConfig.fromJSON(object.controllerConfig) : undefined,
      bridgeToParent: isSet(object.bridgeToParent) ? Boolean(object.bridgeToParent) : false,
    };
  },

  toJSON(message: DirectiveBridge): unknown {
    const obj: any = {};
    message.controllerConfig !== undefined &&
      (obj.controllerConfig = message.controllerConfig ? ControllerConfig.toJSON(message.controllerConfig) : undefined);
    message.bridgeToParent !== undefined && (obj.bridgeToParent = message.bridgeToParent);
    return obj;
  },

  create<I extends Exact<DeepPartial<DirectiveBridge>, I>>(base?: I): DirectiveBridge {
    return DirectiveBridge.fromPartial(base ?? {});
  },

  fromPartial<I extends Exact<DeepPartial<DirectiveBridge>, I>>(object: I): DirectiveBridge {
    const message = createBaseDirectiveBridge();
    message.controllerConfig = (object.controllerConfig !== undefined && object.controllerConfig !== null)
      ? ControllerConfig.fromPartial(object.controllerConfig)
      : undefined;
    message.bridgeToParent = object.bridgeToParent ?? false;
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
