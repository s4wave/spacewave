/* eslint-disable */
import Long from 'long'
import _m0 from 'protobufjs/minimal.js'

export const protobufPackage = 'bldr.esbuild'

/** EsbuildVarType is the list of types of esbuild output variable. */
export enum EsbuildVarType {
  /**
   * EsbuildVarType_ENTRYPOINT_PATH - EsbuildVarType_ENTRYPOINT_PATH is the path to the main entrypoint script.
   * output type is a string
   */
  EsbuildVarType_ENTRYPOINT_PATH = 0,
  /**
   * EsbuildVarType_ESBUILD_OUTPUT - EsbuildVarType_ESBUILD_OUTPUT contains a single EsbuildOutput object.
   * output type is bldr_esbuild.EsbuildOutput
   */
  EsbuildVarType_ESBUILD_OUTPUT = 1,
  UNRECOGNIZED = -1,
}

export function esbuildVarTypeFromJSON(object: any): EsbuildVarType {
  switch (object) {
    case 0:
    case 'EsbuildVarType_ENTRYPOINT_PATH':
      return EsbuildVarType.EsbuildVarType_ENTRYPOINT_PATH
    case 1:
    case 'EsbuildVarType_ESBUILD_OUTPUT':
      return EsbuildVarType.EsbuildVarType_ESBUILD_OUTPUT
    case -1:
    case 'UNRECOGNIZED':
    default:
      return EsbuildVarType.UNRECOGNIZED
  }
}

export function esbuildVarTypeToJSON(object: EsbuildVarType): string {
  switch (object) {
    case EsbuildVarType.EsbuildVarType_ENTRYPOINT_PATH:
      return 'EsbuildVarType_ENTRYPOINT_PATH'
    case EsbuildVarType.EsbuildVarType_ESBUILD_OUTPUT:
      return 'EsbuildVarType_ESBUILD_OUTPUT'
    case EsbuildVarType.UNRECOGNIZED:
    default:
      return 'UNRECOGNIZED'
  }
}

/**
 * EsbuildOutput contains a single EsbuildScript output object.
 * EsbuildVarType_ESBUILD_OUTPUT
 */
export interface EsbuildOutput {
  /**
   * EntrypointHref is the url path to the script entrypoint.
   *
   * e.x: /p/plugin-id/script.js
   */
  entrypointHref: string
  /**
   * CssHref is the url path to the css bundle (if applicable).
   *
   * May be empty.
   * e.x: /p/plugin-id/script.css
   */
  cssHref: string
}

/** EsbuildEntrypoint is an entrypoint passed to Esbuild. */
export interface EsbuildEntrypoint {
  /** InputPath is the input file path. */
  inputPath: string
  /** OutputPath is the output file path, if any. */
  outputPath: string
}

function createBaseEsbuildOutput(): EsbuildOutput {
  return { entrypointHref: '', cssHref: '' }
}

export const EsbuildOutput = {
  encode(
    message: EsbuildOutput,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.entrypointHref !== '') {
      writer.uint32(10).string(message.entrypointHref)
    }
    if (message.cssHref !== '') {
      writer.uint32(18).string(message.cssHref)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EsbuildOutput {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseEsbuildOutput()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.entrypointHref = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.cssHref = reader.string()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EsbuildOutput, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<EsbuildOutput | EsbuildOutput[]>
      | Iterable<EsbuildOutput | EsbuildOutput[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [EsbuildOutput.encode(p).finish()]
        }
      } else {
        yield* [EsbuildOutput.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EsbuildOutput>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<EsbuildOutput> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [EsbuildOutput.decode(p)]
        }
      } else {
        yield* [EsbuildOutput.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): EsbuildOutput {
    return {
      entrypointHref: isSet(object.entrypointHref)
        ? globalThis.String(object.entrypointHref)
        : '',
      cssHref: isSet(object.cssHref) ? globalThis.String(object.cssHref) : '',
    }
  },

  toJSON(message: EsbuildOutput): unknown {
    const obj: any = {}
    if (message.entrypointHref !== '') {
      obj.entrypointHref = message.entrypointHref
    }
    if (message.cssHref !== '') {
      obj.cssHref = message.cssHref
    }
    return obj
  },

  create<I extends Exact<DeepPartial<EsbuildOutput>, I>>(
    base?: I,
  ): EsbuildOutput {
    return EsbuildOutput.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<EsbuildOutput>, I>>(
    object: I,
  ): EsbuildOutput {
    const message = createBaseEsbuildOutput()
    message.entrypointHref = object.entrypointHref ?? ''
    message.cssHref = object.cssHref ?? ''
    return message
  },
}

function createBaseEsbuildEntrypoint(): EsbuildEntrypoint {
  return { inputPath: '', outputPath: '' }
}

export const EsbuildEntrypoint = {
  encode(
    message: EsbuildEntrypoint,
    writer: _m0.Writer = _m0.Writer.create(),
  ): _m0.Writer {
    if (message.inputPath !== '') {
      writer.uint32(10).string(message.inputPath)
    }
    if (message.outputPath !== '') {
      writer.uint32(18).string(message.outputPath)
    }
    return writer
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): EsbuildEntrypoint {
    const reader =
      input instanceof _m0.Reader ? input : _m0.Reader.create(input)
    let end = length === undefined ? reader.len : reader.pos + length
    const message = createBaseEsbuildEntrypoint()
    while (reader.pos < end) {
      const tag = reader.uint32()
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break
          }

          message.inputPath = reader.string()
          continue
        case 2:
          if (tag !== 18) {
            break
          }

          message.outputPath = reader.string()
          continue
      }
      if ((tag & 7) === 4 || tag === 0) {
        break
      }
      reader.skipType(tag & 7)
    }
    return message
  },

  // encodeTransform encodes a source of message objects.
  // Transform<EsbuildEntrypoint, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<EsbuildEntrypoint | EsbuildEntrypoint[]>
      | Iterable<EsbuildEntrypoint | EsbuildEntrypoint[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [EsbuildEntrypoint.encode(p).finish()]
        }
      } else {
        yield* [EsbuildEntrypoint.encode(pkt as any).finish()]
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, EsbuildEntrypoint>
  async *decodeTransform(
    source:
      | AsyncIterable<Uint8Array | Uint8Array[]>
      | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<EsbuildEntrypoint> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of pkt as any) {
          yield* [EsbuildEntrypoint.decode(p)]
        }
      } else {
        yield* [EsbuildEntrypoint.decode(pkt as any)]
      }
    }
  },

  fromJSON(object: any): EsbuildEntrypoint {
    return {
      inputPath: isSet(object.inputPath)
        ? globalThis.String(object.inputPath)
        : '',
      outputPath: isSet(object.outputPath)
        ? globalThis.String(object.outputPath)
        : '',
    }
  },

  toJSON(message: EsbuildEntrypoint): unknown {
    const obj: any = {}
    if (message.inputPath !== '') {
      obj.inputPath = message.inputPath
    }
    if (message.outputPath !== '') {
      obj.outputPath = message.outputPath
    }
    return obj
  },

  create<I extends Exact<DeepPartial<EsbuildEntrypoint>, I>>(
    base?: I,
  ): EsbuildEntrypoint {
    return EsbuildEntrypoint.fromPartial(base ?? ({} as any))
  },
  fromPartial<I extends Exact<DeepPartial<EsbuildEntrypoint>, I>>(
    object: I,
  ): EsbuildEntrypoint {
    const message = createBaseEsbuildEntrypoint()
    message.inputPath = object.inputPath ?? ''
    message.outputPath = object.outputPath ?? ''
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
  : T extends Long
    ? string | number | Long
    : T extends globalThis.Array<infer U>
      ? globalThis.Array<DeepPartial<U>>
      : T extends ReadonlyArray<infer U>
        ? ReadonlyArray<DeepPartial<U>>
        : T extends { $case: string }
          ? { [K in keyof Omit<T, '$case'>]?: DeepPartial<T[K]> } & {
              $case: T['$case']
            }
          : T extends {}
            ? { [K in keyof T]?: DeepPartial<T[K]> }
            : Partial<T>

type KeysOfUnion<T> = T extends T ? keyof T : never
export type Exact<P, I extends P> = P extends Builtin
  ? P
  : P & { [K in keyof P]: Exact<P[K], I[K]> } & {
      [K in Exclude<keyof I, KeysOfUnion<P>>]: never
    }

if (_m0.util.Long !== Long) {
  _m0.util.Long = Long as any
  _m0.configure()
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined
}
