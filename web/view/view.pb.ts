/* eslint-disable */
import Long from "long";
import _m0 from "protobufjs/minimal.js";

export const protobufPackage = "web.view";

/** RenderMode is the list of available WebView rendering modes. */
export enum RenderMode {
  /**
   * RenderMode_NONE - RenderMode_NONE selects no renderer (no contents).
   * When setting this mode, everything is "reset" in the view.
   */
  RenderMode_NONE = 0,
  /**
   * RenderMode_REACT_COMPONENT - RenderMode_REACT_COMPONENT renders a React component from a JS module.
   * Renders the default export of the JS module.
   */
  RenderMode_REACT_COMPONENT = 1,
  UNRECOGNIZED = -1,
}

export function renderModeFromJSON(object: any): RenderMode {
  switch (object) {
    case 0:
    case "RenderMode_NONE":
      return RenderMode.RenderMode_NONE;
    case 1:
    case "RenderMode_REACT_COMPONENT":
      return RenderMode.RenderMode_REACT_COMPONENT;
    case -1:
    case "UNRECOGNIZED":
    default:
      return RenderMode.UNRECOGNIZED;
  }
}

export function renderModeToJSON(object: RenderMode): string {
  switch (object) {
    case RenderMode.RenderMode_NONE:
      return "RenderMode_NONE";
    case RenderMode.RenderMode_REACT_COMPONENT:
      return "RenderMode_REACT_COMPONENT";
    case RenderMode.UNRECOGNIZED:
    default:
      return "UNRECOGNIZED";
  }
}

/** SetRenderModeRequest is the request to change the render mode. */
export interface SetRenderModeRequest {
  /** RenderMode is the new render mode. */
  renderMode: RenderMode;
  /**
   * Wait waits for the mode to become active before returning.
   * If loading a script: will wait for the script to load successfully.
   * If any error is encountered, returns it as the RPC result.
   */
  wait: boolean;
  /**
   * ScriptPath is a path to a script to load to render.
   * RenderMode_REACT_COMPONENT: expects default export to be a Component.
   * Note: /b/ will be prepended to this path automatically.
   */
  scriptPath: string;
}

/** SetRenderModeResponse is the response to the SetRenderMode request. */
export interface SetRenderModeResponse {
}

/** RemoveWebViewRequest is a request to remove the web view. */
export interface RemoveWebViewRequest {
}

/** RemoveWebViewResponse is the response to the RemoveWebView request. */
export interface RemoveWebViewResponse {
  /** Removed indicates the web view was removed. */
  removed: boolean;
}

function createBaseSetRenderModeRequest(): SetRenderModeRequest {
  return { renderMode: 0, wait: false, scriptPath: "" };
}

export const SetRenderModeRequest = {
  encode(message: SetRenderModeRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.renderMode !== 0) {
      writer.uint32(8).int32(message.renderMode);
    }
    if (message.wait === true) {
      writer.uint32(16).bool(message.wait);
    }
    if (message.scriptPath !== "") {
      writer.uint32(26).string(message.scriptPath);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): SetRenderModeRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSetRenderModeRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.renderMode = reader.int32() as any;
          break;
        case 2:
          message.wait = reader.bool();
          break;
        case 3:
          message.scriptPath = reader.string();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<SetRenderModeRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<SetRenderModeRequest | SetRenderModeRequest[]>
      | Iterable<SetRenderModeRequest | SetRenderModeRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetRenderModeRequest.encode(p).finish()];
        }
      } else {
        yield* [SetRenderModeRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SetRenderModeRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<SetRenderModeRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetRenderModeRequest.decode(p)];
        }
      } else {
        yield* [SetRenderModeRequest.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): SetRenderModeRequest {
    return {
      renderMode: isSet(object.renderMode) ? renderModeFromJSON(object.renderMode) : 0,
      wait: isSet(object.wait) ? Boolean(object.wait) : false,
      scriptPath: isSet(object.scriptPath) ? String(object.scriptPath) : "",
    };
  },

  toJSON(message: SetRenderModeRequest): unknown {
    const obj: any = {};
    message.renderMode !== undefined && (obj.renderMode = renderModeToJSON(message.renderMode));
    message.wait !== undefined && (obj.wait = message.wait);
    message.scriptPath !== undefined && (obj.scriptPath = message.scriptPath);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<SetRenderModeRequest>, I>>(object: I): SetRenderModeRequest {
    const message = createBaseSetRenderModeRequest();
    message.renderMode = object.renderMode ?? 0;
    message.wait = object.wait ?? false;
    message.scriptPath = object.scriptPath ?? "";
    return message;
  },
};

function createBaseSetRenderModeResponse(): SetRenderModeResponse {
  return {};
}

export const SetRenderModeResponse = {
  encode(_: SetRenderModeResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): SetRenderModeResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseSetRenderModeResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<SetRenderModeResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<SetRenderModeResponse | SetRenderModeResponse[]>
      | Iterable<SetRenderModeResponse | SetRenderModeResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetRenderModeResponse.encode(p).finish()];
        }
      } else {
        yield* [SetRenderModeResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, SetRenderModeResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<SetRenderModeResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [SetRenderModeResponse.decode(p)];
        }
      } else {
        yield* [SetRenderModeResponse.decode(pkt)];
      }
    }
  },

  fromJSON(_: any): SetRenderModeResponse {
    return {};
  },

  toJSON(_: SetRenderModeResponse): unknown {
    const obj: any = {};
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<SetRenderModeResponse>, I>>(_: I): SetRenderModeResponse {
    const message = createBaseSetRenderModeResponse();
    return message;
  },
};

function createBaseRemoveWebViewRequest(): RemoveWebViewRequest {
  return {};
}

export const RemoveWebViewRequest = {
  encode(_: RemoveWebViewRequest, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RemoveWebViewRequest {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRemoveWebViewRequest();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RemoveWebViewRequest, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RemoveWebViewRequest | RemoveWebViewRequest[]>
      | Iterable<RemoveWebViewRequest | RemoveWebViewRequest[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebViewRequest.encode(p).finish()];
        }
      } else {
        yield* [RemoveWebViewRequest.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoveWebViewRequest>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RemoveWebViewRequest> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebViewRequest.decode(p)];
        }
      } else {
        yield* [RemoveWebViewRequest.decode(pkt)];
      }
    }
  },

  fromJSON(_: any): RemoveWebViewRequest {
    return {};
  },

  toJSON(_: RemoveWebViewRequest): unknown {
    const obj: any = {};
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<RemoveWebViewRequest>, I>>(_: I): RemoveWebViewRequest {
    const message = createBaseRemoveWebViewRequest();
    return message;
  },
};

function createBaseRemoveWebViewResponse(): RemoveWebViewResponse {
  return { removed: false };
}

export const RemoveWebViewResponse = {
  encode(message: RemoveWebViewResponse, writer: _m0.Writer = _m0.Writer.create()): _m0.Writer {
    if (message.removed === true) {
      writer.uint32(8).bool(message.removed);
    }
    return writer;
  },

  decode(input: _m0.Reader | Uint8Array, length?: number): RemoveWebViewResponse {
    const reader = input instanceof _m0.Reader ? input : new _m0.Reader(input);
    let end = length === undefined ? reader.len : reader.pos + length;
    const message = createBaseRemoveWebViewResponse();
    while (reader.pos < end) {
      const tag = reader.uint32();
      switch (tag >>> 3) {
        case 1:
          message.removed = reader.bool();
          break;
        default:
          reader.skipType(tag & 7);
          break;
      }
    }
    return message;
  },

  // encodeTransform encodes a source of message objects.
  // Transform<RemoveWebViewResponse, Uint8Array>
  async *encodeTransform(
    source:
      | AsyncIterable<RemoveWebViewResponse | RemoveWebViewResponse[]>
      | Iterable<RemoveWebViewResponse | RemoveWebViewResponse[]>,
  ): AsyncIterable<Uint8Array> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebViewResponse.encode(p).finish()];
        }
      } else {
        yield* [RemoveWebViewResponse.encode(pkt).finish()];
      }
    }
  },

  // decodeTransform decodes a source of encoded messages.
  // Transform<Uint8Array, RemoveWebViewResponse>
  async *decodeTransform(
    source: AsyncIterable<Uint8Array | Uint8Array[]> | Iterable<Uint8Array | Uint8Array[]>,
  ): AsyncIterable<RemoveWebViewResponse> {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [RemoveWebViewResponse.decode(p)];
        }
      } else {
        yield* [RemoveWebViewResponse.decode(pkt)];
      }
    }
  },

  fromJSON(object: any): RemoveWebViewResponse {
    return { removed: isSet(object.removed) ? Boolean(object.removed) : false };
  },

  toJSON(message: RemoveWebViewResponse): unknown {
    const obj: any = {};
    message.removed !== undefined && (obj.removed = message.removed);
    return obj;
  },

  fromPartial<I extends Exact<DeepPartial<RemoveWebViewResponse>, I>>(object: I): RemoveWebViewResponse {
    const message = createBaseRemoveWebViewResponse();
    message.removed = object.removed ?? false;
    return message;
  },
};

/**
 * WebViewHost is the service exposed by the Go runtime.
 *
 * Accessed by the WebView renderer.
 */
export interface WebViewHost {
}

export class WebViewHostClientImpl implements WebViewHost {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "web.view.WebViewHost";
    this.rpc = rpc;
  }
}

/**
 * WebViewHost is the service exposed by the Go runtime.
 *
 * Accessed by the WebView renderer.
 */
export type WebViewHostDefinition = typeof WebViewHostDefinition;
export const WebViewHostDefinition = { name: "WebViewHost", fullName: "web.view.WebViewHost", methods: {} } as const;

/** WebView exposes a remote WebView via rpc. */
export interface WebView {
  /** SetRenderMode sets the rendering mode of the view. */
  SetRenderMode(request: SetRenderModeRequest): Promise<SetRenderModeResponse>;
  /** RemoveWebView requests to remove a WebView from the root level. */
  RemoveWebView(request: RemoveWebViewRequest): Promise<RemoveWebViewResponse>;
}

export class WebViewClientImpl implements WebView {
  private readonly rpc: Rpc;
  private readonly service: string;
  constructor(rpc: Rpc, opts?: { service?: string }) {
    this.service = opts?.service || "web.view.WebView";
    this.rpc = rpc;
    this.SetRenderMode = this.SetRenderMode.bind(this);
    this.RemoveWebView = this.RemoveWebView.bind(this);
  }
  SetRenderMode(request: SetRenderModeRequest): Promise<SetRenderModeResponse> {
    const data = SetRenderModeRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "SetRenderMode", data);
    return promise.then((data) => SetRenderModeResponse.decode(new _m0.Reader(data)));
  }

  RemoveWebView(request: RemoveWebViewRequest): Promise<RemoveWebViewResponse> {
    const data = RemoveWebViewRequest.encode(request).finish();
    const promise = this.rpc.request(this.service, "RemoveWebView", data);
    return promise.then((data) => RemoveWebViewResponse.decode(new _m0.Reader(data)));
  }
}

/** WebView exposes a remote WebView via rpc. */
export type WebViewDefinition = typeof WebViewDefinition;
export const WebViewDefinition = {
  name: "WebView",
  fullName: "web.view.WebView",
  methods: {
    /** SetRenderMode sets the rendering mode of the view. */
    setRenderMode: {
      name: "SetRenderMode",
      requestType: SetRenderModeRequest,
      requestStream: false,
      responseType: SetRenderModeResponse,
      responseStream: false,
      options: {},
    },
    /** RemoveWebView requests to remove a WebView from the root level. */
    removeWebView: {
      name: "RemoveWebView",
      requestType: RemoveWebViewRequest,
      requestStream: false,
      responseType: RemoveWebViewResponse,
      responseStream: false,
      options: {},
    },
  },
} as const;

interface Rpc {
  request(service: string, method: string, data: Uint8Array): Promise<Uint8Array>;
}

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
