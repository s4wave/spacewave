/* eslint-disable */
import { util, configure, Writer, Reader } from 'protobufjs/minimal'
import * as Long from 'long'
import { ExecControllerRequest } from '../../../../../github.com/aperturerobotics/controllerbus/controller/exec/exec'
import { BlockRef } from '../../../../../github.com/aperturerobotics/hydra/block/block'
import { ControllerConfig } from '../../../../../github.com/aperturerobotics/controllerbus/controller/configset/proto/configset'

export const protobufPackage = 'assembly.block'

/** Assembly is a definition of a set of configs to run on a Bus. */
export interface Assembly {
  /**
   * ControllerExec is the list of controllers to run.
   * Either ConfigSet protobuf or yaml format.
   */
  controllerExec: ExecControllerRequest | undefined
  /** SubAssemblies is the list of sub-assembly configs. */
  subAssemblies: SubAssembly[]
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
  id: string
  /** Assemblies is the list of assembiles to apply to the sub-bus. */
  assemblies: Assembly[]
  /**
   * AssemblyRefs contains a list of block ref to Assembly.
   * The referenced Assembly list will be merged with assemblies.
   */
  assemblyRefs: BlockRef[]
  /**
   * DirectiveBridges configures the list of directive bridges to the parent bus.
   * Can include bridges to resolve plugins, controllers, etc.
   */
  directiveBridges: DirectiveBridge[]
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
  controllerConfig: ControllerConfig | undefined
  /** BridgeToParent indicates the target is the parent, not the subassembly. */
  bridgeToParent: boolean
}

const baseAssembly: object = {}

export const Assembly = {
  encode(message: Assembly, writer: Writer = Writer.create()): Writer {
    if (message.controllerExec !== undefined) {
      ExecControllerRequest.encode(
        message.controllerExec,
        writer.uint32(10).fork()
      ).ldelim()
    }
    for (const v of message.subAssemblies) {
      SubAssembly.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): Assembly {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseAssembly } as Assembly
    message.subAssemblies = []
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.controllerExec = ExecControllerRequest.decode(
            reader,
            reader.uint32()
          )
          break
        case 2:
          message.subAssemblies.push(
            SubAssembly.decode(reader, reader.uint32())
          )
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): Assembly {
    const message = { ...baseAssembly } as Assembly
    message.subAssemblies = []
    if (object.controllerExec !== undefined && object.controllerExec !== null) {
      message.controllerExec = ExecControllerRequest.fromJSON(
        object.controllerExec
      )
    } else {
      message.controllerExec = undefined
    }
    if (object.subAssemblies !== undefined && object.subAssemblies !== null) {
      for (const e of object.subAssemblies) {
        message.subAssemblies.push(SubAssembly.fromJSON(e))
      }
    }
    return message
  },

  toJSON(message: Assembly): unknown {
    const obj: any = {}
    message.controllerExec !== undefined &&
      (obj.controllerExec = message.controllerExec
        ? ExecControllerRequest.toJSON(message.controllerExec)
        : undefined)
    if (message.subAssemblies) {
      obj.subAssemblies = message.subAssemblies.map((e) =>
        e ? SubAssembly.toJSON(e) : undefined
      )
    } else {
      obj.subAssemblies = []
    }
    return obj
  },

  fromPartial(object: DeepPartial<Assembly>): Assembly {
    const message = { ...baseAssembly } as Assembly
    message.subAssemblies = []
    if (object.controllerExec !== undefined && object.controllerExec !== null) {
      message.controllerExec = ExecControllerRequest.fromPartial(
        object.controllerExec
      )
    } else {
      message.controllerExec = undefined
    }
    if (object.subAssemblies !== undefined && object.subAssemblies !== null) {
      for (const e of object.subAssemblies) {
        message.subAssemblies.push(SubAssembly.fromPartial(e))
      }
    }
    return message
  },
}

const baseSubAssembly: object = { id: '' }

export const SubAssembly = {
  encode(message: SubAssembly, writer: Writer = Writer.create()): Writer {
    if (message.id !== '') {
      writer.uint32(10).string(message.id)
    }
    for (const v of message.assemblies) {
      Assembly.encode(v!, writer.uint32(18).fork()).ldelim()
    }
    for (const v of message.assemblyRefs) {
      BlockRef.encode(v!, writer.uint32(26).fork()).ldelim()
    }
    for (const v of message.directiveBridges) {
      DirectiveBridge.encode(v!, writer.uint32(34).fork()).ldelim()
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): SubAssembly {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseSubAssembly } as SubAssembly
    message.assemblies = []
    message.assemblyRefs = []
    message.directiveBridges = []
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.id = reader.string()
          break
        case 2:
          message.assemblies.push(Assembly.decode(reader, reader.uint32()))
          break
        case 3:
          message.assemblyRefs.push(BlockRef.decode(reader, reader.uint32()))
          break
        case 4:
          message.directiveBridges.push(
            DirectiveBridge.decode(reader, reader.uint32())
          )
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): SubAssembly {
    const message = { ...baseSubAssembly } as SubAssembly
    message.assemblies = []
    message.assemblyRefs = []
    message.directiveBridges = []
    if (object.id !== undefined && object.id !== null) {
      message.id = String(object.id)
    } else {
      message.id = ''
    }
    if (object.assemblies !== undefined && object.assemblies !== null) {
      for (const e of object.assemblies) {
        message.assemblies.push(Assembly.fromJSON(e))
      }
    }
    if (object.assemblyRefs !== undefined && object.assemblyRefs !== null) {
      for (const e of object.assemblyRefs) {
        message.assemblyRefs.push(BlockRef.fromJSON(e))
      }
    }
    if (
      object.directiveBridges !== undefined &&
      object.directiveBridges !== null
    ) {
      for (const e of object.directiveBridges) {
        message.directiveBridges.push(DirectiveBridge.fromJSON(e))
      }
    }
    return message
  },

  toJSON(message: SubAssembly): unknown {
    const obj: any = {}
    message.id !== undefined && (obj.id = message.id)
    if (message.assemblies) {
      obj.assemblies = message.assemblies.map((e) =>
        e ? Assembly.toJSON(e) : undefined
      )
    } else {
      obj.assemblies = []
    }
    if (message.assemblyRefs) {
      obj.assemblyRefs = message.assemblyRefs.map((e) =>
        e ? BlockRef.toJSON(e) : undefined
      )
    } else {
      obj.assemblyRefs = []
    }
    if (message.directiveBridges) {
      obj.directiveBridges = message.directiveBridges.map((e) =>
        e ? DirectiveBridge.toJSON(e) : undefined
      )
    } else {
      obj.directiveBridges = []
    }
    return obj
  },

  fromPartial(object: DeepPartial<SubAssembly>): SubAssembly {
    const message = { ...baseSubAssembly } as SubAssembly
    message.assemblies = []
    message.assemblyRefs = []
    message.directiveBridges = []
    if (object.id !== undefined && object.id !== null) {
      message.id = object.id
    } else {
      message.id = ''
    }
    if (object.assemblies !== undefined && object.assemblies !== null) {
      for (const e of object.assemblies) {
        message.assemblies.push(Assembly.fromPartial(e))
      }
    }
    if (object.assemblyRefs !== undefined && object.assemblyRefs !== null) {
      for (const e of object.assemblyRefs) {
        message.assemblyRefs.push(BlockRef.fromPartial(e))
      }
    }
    if (
      object.directiveBridges !== undefined &&
      object.directiveBridges !== null
    ) {
      for (const e of object.directiveBridges) {
        message.directiveBridges.push(DirectiveBridge.fromPartial(e))
      }
    }
    return message
  },
}

const baseDirectiveBridge: object = { bridgeToParent: false }

export const DirectiveBridge = {
  encode(message: DirectiveBridge, writer: Writer = Writer.create()): Writer {
    if (message.controllerConfig !== undefined) {
      ControllerConfig.encode(
        message.controllerConfig,
        writer.uint32(10).fork()
      ).ldelim()
    }
    if (message.bridgeToParent === true) {
      writer.uint32(16).bool(message.bridgeToParent)
    }
    return writer
  },

  decode(input: Reader | Uint8Array, length?: number): DirectiveBridge {
    const reader = input instanceof Reader ? input : new Reader(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = { ...baseDirectiveBridge } as DirectiveBridge
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          message.controllerConfig = ControllerConfig.decode(
            reader,
            reader.uint32()
          )
          break
        case 2:
          message.bridgeToParent = reader.bool()
          break
        default:
          reader.skipType(tag & 7)
          break
      }
    }
    return message
  },

  fromJSON(object: any): DirectiveBridge {
    const message = { ...baseDirectiveBridge } as DirectiveBridge
    if (
      object.controllerConfig !== undefined &&
      object.controllerConfig !== null
    ) {
      message.controllerConfig = ControllerConfig.fromJSON(
        object.controllerConfig
      )
    } else {
      message.controllerConfig = undefined
    }
    if (object.bridgeToParent !== undefined && object.bridgeToParent !== null) {
      message.bridgeToParent = Boolean(object.bridgeToParent)
    } else {
      message.bridgeToParent = false
    }
    return message
  },

  toJSON(message: DirectiveBridge): unknown {
    const obj: any = {}
    message.controllerConfig !== undefined &&
      (obj.controllerConfig = message.controllerConfig
        ? ControllerConfig.toJSON(message.controllerConfig)
        : undefined)
    message.bridgeToParent !== undefined &&
      (obj.bridgeToParent = message.bridgeToParent)
    return obj
  },

  fromPartial(object: DeepPartial<DirectiveBridge>): DirectiveBridge {
    const message = { ...baseDirectiveBridge } as DirectiveBridge
    if (
      object.controllerConfig !== undefined &&
      object.controllerConfig !== null
    ) {
      message.controllerConfig = ControllerConfig.fromPartial(
        object.controllerConfig
      )
    } else {
      message.controllerConfig = undefined
    }
    if (object.bridgeToParent !== undefined && object.bridgeToParent !== null) {
      message.bridgeToParent = object.bridgeToParent
    } else {
      message.bridgeToParent = false
    }
    return message
  },
}

type Builtin =
  | Date
  | Function
  | Uint8Array
  | string
  | number
  | boolean
  | undefined
export type DeepPartial<T> = T extends Builtin
  ? T
  : T extends Array<infer U>
  ? Array<DeepPartial<U>>
  : T extends ReadonlyArray<infer U>
  ? ReadonlyArray<DeepPartial<U>>
  : T extends {}
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : Partial<T>

// If you get a compile-error about 'Constructor<Long> and ... have no overlap',
// add '--ts_proto_opt=esModuleInterop=true' as a flag when calling 'protoc'.
if (util.Long !== Long) {
  util.Long = Long as any
  configure()
}
