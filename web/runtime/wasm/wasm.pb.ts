/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "web.runtime.wasm";

/** WebWorkerWasmPluginInit is a message passed to initialize a web worker with a wasm plugin. */
export interface WebWorkerWasmPluginInit {
  /**
   * Entrypoint is the path to the wasm entrypoint.
   * /b/pd/{plugin-id}/{plugin-id}.wasm
   */
  entrypoint: string;
  /** Argv is the set of command line arguments to pass to the plugin. */
  argv: string[];
  /** Env is the set of environment variables to pass to the plugin. */
  env: { [key: string]: string };
}

export interface WebWorkerWasmPluginInit_EnvEntry {
  key: string;
  value: string;
}

function createBaseWebWorkerWasmPluginInit(): WebWorkerWasmPluginInit {
  return { entrypoint: "", argv: [], env: {} };
}

export const WebWorkerWasmPluginInit = {
  encode(message: WebWorkerWasmPluginInit, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.entrypoint !== "") {
      writer.uint32(10).string(message.entrypoint);
    }
    for (const v of message.argv) {
      writer.uint32(18).string(v!);
    }
    Object.entries(message.env).forEach(([key, value]) => {
      WebWorkerWasmPluginInit_EnvEntry.encode({ key: key as any, value }, writer.uint32(26).fork()).ldelim();
    });
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebWorkerWasmPluginInit {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseWebWorkerWasmPluginInit();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.entrypoint = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.argv.push(reader.string());
          continue;
        case 3:
          if (tag !== 26) {
            break;
          }

          const entry3 = WebWorkerWasmPluginInit_EnvEntry.decode(reader, reader.uint32());
          if (entry3.value !== undefined) {
            message.env[entry3.key] = entry3.value;
          }
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
  // Transform<WebWorkerWasmPluginInit, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebWorkerWasmPluginInit | WebWorkerWasmPluginInit[]>
      | Iterable<WebWorkerWasmPluginInit | WebWorkerWasmPluginInit[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [WebWorkerWasmPluginInit.encode(p).finish()];
        }
      } else {
        yield* [WebWorkerWasmPluginInit.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebWorkerWasmPluginInit>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebWorkerWasmPluginInit> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [WebWorkerWasmPluginInit.decode(p)];
        }
      } else {
        yield* [WebWorkerWasmPluginInit.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): WebWorkerWasmPluginInit {
    return {
      entrypoint: isSet(object.entrypoint) ? globalThis.String(object.entrypoint) : "",
      argv: globalThis.Array.isArray(object?.argv) ? object.argv.map((e: any) => globalThis.String(e)) : [],
      env: isObject(object.env)
        ? Object.entries(object.env).reduce<{ [key: string]: string }>((acc, [key, value]) => {
          acc[key] = String(value);
          return acc;
        }, {})
        : {},
    };
  },

  toJSON(message: WebWorkerWasmPluginInit): unknown {
    const obj: any = {};
    if (message.entrypoint !== "") {
      obj.entrypoint = message.entrypoint;
    }
    if (message.argv?.length) {
      obj.argv = message.argv;
    }
    if (message.env) {
      const entries = Object.entries(message.env);
      if (entries.length > 0) {
        obj.env = {};
        entries.forEach(([k, v]) => {
          obj.env[k] = v;
        });
      }
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<WebWorkerWasmPluginInit>, I>>(base?: I): WebWorkerWasmPluginInit {
    return WebWorkerWasmPluginInit.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<WebWorkerWasmPluginInit>, I>>(object: I): WebWorkerWasmPluginInit {
    const message = createBaseWebWorkerWasmPluginInit();
    message.entrypoint = object.entrypoint ?? "";
    message.argv = object.argv?.map((e) => e) || [];
    message.env = Object.entries(object.env ?? {}).reduce<{ [key: string]: string }>((acc, [key, value]) => {
      if (value !== undefined) {
        acc[key] = globalThis.String(value);
      }
      return acc;
    }, {});
    return message;
  },
};

function createBaseWebWorkerWasmPluginInit_EnvEntry(): WebWorkerWasmPluginInit_EnvEntry {
  return { key: "", value: "" };
}

export const WebWorkerWasmPluginInit_EnvEntry = {
  encode(message: WebWorkerWasmPluginInit_EnvEntry, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.key !== "") {
      writer.uint32(10).string(message.key);
    }
    if (message.value !== "") {
      writer.uint32(18).string(message.value);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): WebWorkerWasmPluginInit_EnvEntry {
    const reader = input instanceof _m0.Reader ? input : _m0.Reader.create(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseWebWorkerWasmPluginInit_EnvEntry();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          if (tag !== 10) {
            break;
          }

          message.key = reader.string();
          continue;
        case 2:
          if (tag !== 18) {
            break;
          }

          message.value = reader.string();
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
  // Transform<WebWorkerWasmPluginInit_EnvEntry, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<WebWorkerWasmPluginInit_EnvEntry | WebWorkerWasmPluginInit_EnvEntry[]>
      | Iterable<WebWorkerWasmPluginInit_EnvEntry | WebWorkerWasmPluginInit_EnvEntry[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [WebWorkerWasmPluginInit_EnvEntry.encode(p).finish()];
        }
      } else {
        yield* [WebWorkerWasmPluginInit_EnvEntry.encode(pkt as any).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, WebWorkerWasmPluginInit_EnvEntry>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<WebWorkerWasmPluginInit_EnvEntry> {
    for await (const pkt of source) {
      if (globalThis.Array.isArray(pkt)) {
        for (const p of (pkt as any)) {
          yield* [WebWorkerWasmPluginInit_EnvEntry.decode(p)];
        }
      } else {
        yield* [WebWorkerWasmPluginInit_EnvEntry.decode(pkt as any)];
      }
    }
  },

  fromJSON(object: any): WebWorkerWasmPluginInit_EnvEntry {
    return {
      key: isSet(object.key) ? globalThis.String(object.key) : "",
      value: isSet(object.value) ? globalThis.String(object.value) : "",
    };
  },

  toJSON(message: WebWorkerWasmPluginInit_EnvEntry): unknown {
    const obj: any = {};
    if (message.key !== "") {
      obj.key = message.key;
    }
    if (message.value !== "") {
      obj.value = message.value;
    }
    return obj;
  },

  create<I extends Exact<DeepPartial<WebWorkerWasmPluginInit_EnvEntry>, I>>(
    base?: I,
  ): WebWorkerWasmPluginInit_EnvEntry {
    return WebWorkerWasmPluginInit_EnvEntry.fromPartial(base ?? ({} as any));
  },
  fromPartial<I extends Exact<DeepPartial<WebWorkerWasmPluginInit_EnvEntry>, I>>(
    object: I,
  ): WebWorkerWasmPluginInit_EnvEntry {
    const message = createBaseWebWorkerWasmPluginInit_EnvEntry();
    message.key = object.key ?? "";
    message.value = object.value ?? "";
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

function isObject(value: any): boolean {
  return typeof value === "object" && value !== null;
}

function isSet(value: any): boolean {
  return value !== null && value !== undefined;
}
