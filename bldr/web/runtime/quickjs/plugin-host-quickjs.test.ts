import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ChannelStream,
  combineUint8ArrayListTransform,
  HandleStreamCtr,
  StreamConn,
  type PacketStream,
} from "starpc";
import { pushable } from "it-pushable";
import { pipe } from "it-pipe";

import quickJSRunner, {
  addAssetToFileSystem,
  buildQuickJSBridgeHandlers,
  canUseSynchronousBackendAssetFetch,
  collectBackendEntrypointAssetPaths,
  collectViteManifestStaticAssetPaths,
  createBackendAssetMount,
  createBackendAssetPreopens,
  handleQuickJSReadinessMarker,
  loadBackendAssets,
  pipeQuickJSBridgeStreams,
  resolveBackendAssetPath,
  selectBackendAssetLoadingMode,
  shouldPreloadBackendAssets,
  shouldPreferBoundedBackendAssetPreload,
  startQuickJSHostConnectionPipe,
  type BackendAssetCacheEntry,
} from "./plugin-host-quickjs.js";
import type { BackendAPI } from "@aptre/bldr-sdk";

describe("plugin-host-quickjs bridge handlers", () => {
  it("keeps QuickJS and WebRuntime stream directions separate", async () => {
    const quickJSStream = buildPacketStream();
    const webRuntimeStream = buildPacketStream();
    const webRuntimeTarget = buildPacketStream();
    const quickJSTarget = buildPacketStream();
    const openWebRuntimeStream = vi.fn(async () => webRuntimeTarget);
    const openQuickJSStream = vi.fn(async () => quickJSTarget);
    const pipeStreams = vi.fn();

    const handlers = buildQuickJSBridgeHandlers({
      openWebRuntimeStream,
      openQuickJSStream,
      pipeStreams,
    });

    await handlers.handleQuickJSStream(quickJSStream);
    await handlers.handleWebRuntimeStream(webRuntimeStream);

    expect(openWebRuntimeStream).toHaveBeenCalledTimes(1);
    expect(openQuickJSStream).toHaveBeenCalledTimes(1);
    expect(pipeStreams).toHaveBeenNthCalledWith(
      1,
      quickJSStream,
      expect.any(Promise),
      expect.objectContaining({ direction: "quickjs-to-web-runtime" }),
    );
    expect(pipeStreams).toHaveBeenNthCalledWith(
      2,
      webRuntimeStream,
      expect.any(Promise),
      expect.objectContaining({ direction: "web-runtime-to-quickjs" }),
    );
    await expect(pipeStreams.mock.calls[0][1]).resolves.toBe(webRuntimeTarget);
    await expect(pipeStreams.mock.calls[1][1]).resolves.toBe(quickJSTarget);
  });

  it("opens inbound WebRuntime streams before the QuickJS target stream resolves", async () => {
    const webRuntimeSource = pushable<Uint8Array>({ objectMode: true });
    const webRuntimeSinkPackets: Uint8Array[] = [];
    let markWebRuntimeSinkStarted!: () => void;
    const webRuntimeSinkStarted = new Promise<void>((resolve) => {
      markWebRuntimeSinkStarted = resolve;
    });
    const webRuntimeStream: PacketStream = {
      source: webRuntimeSource,
      sink: async (packets) => {
        markWebRuntimeSinkStarted();
        for await (const packet of packets) {
          webRuntimeSinkPackets.push(packet);
        }
      },
    };

    const quickJSTargetSource = pushable<Uint8Array>({ objectMode: true });
    const quickJSTargetSinkPackets: Uint8Array[] = [];
    const quickJSTargetStream: PacketStream = {
      source: quickJSTargetSource,
      sink: async (packets) => {
        for await (const packet of packets) {
          quickJSTargetSinkPackets.push(packet);
        }
      },
    };
    let resolveQuickJSTarget!: (stream: PacketStream) => void;
    const quickJSTargetReady = new Promise<PacketStream>((resolve) => {
      resolveQuickJSTarget = resolve;
    });

    const handlers = buildQuickJSBridgeHandlers({
      openWebRuntimeStream: vi.fn(),
      openQuickJSStream: vi.fn(() => quickJSTargetReady),
    });

    void handlers.handleWebRuntimeStream(webRuntimeStream);
    await webRuntimeSinkStarted;

    const webRuntimePacket = new Uint8Array([1, 2, 3]);
    webRuntimeSource.push(webRuntimePacket);
    resolveQuickJSTarget(quickJSTargetStream);

    await waitFor(
      () => quickJSTargetSinkPackets.length === 1,
      "QuickJS target did not receive buffered WebRuntime packet",
    );
    expect(quickJSTargetSinkPackets[0]).toEqual(webRuntimePacket);

    const quickJSPacket = new Uint8Array([4, 5, 6]);
    quickJSTargetSource.push(quickJSPacket);
    await waitFor(
      () => webRuntimeSinkPackets.length === 1,
      "WebRuntime stream did not receive QuickJS response packet",
    );
    expect(webRuntimeSinkPackets[0]).toEqual(quickJSPacket);

    webRuntimeSource.end();
    quickJSTargetSource.end();
  });

  it("forwards WebRuntime streams through the host and QuickJS yamux connection", async () => {
    const webRuntimeSource = pushable<Uint8Array>({ objectMode: true });
    const webRuntimeSinkPackets: Uint8Array[] = [];
    const webRuntimeStream: PacketStream = {
      source: webRuntimeSource,
      sink: async (packets) => {
        for await (const packet of packets) {
          webRuntimeSinkPackets.push(packet);
        }
      },
    };

    let resolveQuickJSStream!: (stream: PacketStream) => void;
    const quickJSStreamReady = new Promise<PacketStream>((resolve) => {
      resolveQuickJSStream = resolve;
    });
    const quickJSConn = new StreamConn(
      {
        handlePacketStream(stream) {
          resolveQuickJSStream(stream);
        },
      },
      {
        direction: "inbound",
        yamuxParams: { enableKeepAlive: false, maxMessageSize: 32 * 1024 },
      },
    );

    const hostConnRef: { current?: StreamConn } = {};
    const handlers = buildQuickJSBridgeHandlers({
      openWebRuntimeStream: vi.fn(),
      openQuickJSStream: () => {
        if (!hostConnRef.current) {
          throw new Error("QuickJS host connection is not initialized");
        }
        return hostConnRef.current.openStream();
      },
    });
    const hostConn = new StreamConn(
      { handlePacketStream: handlers.handleQuickJSStream },
      {
        direction: "outbound",
        yamuxParams: { enableKeepAlive: false, maxMessageSize: 32 * 1024 },
      },
    );
    hostConnRef.current = hostConn;

    const yamuxPipe = pipe(
      quickJSConn,
      hostConn,
      combineUint8ArrayListTransform(),
      quickJSConn,
    ) as Promise<unknown>;
    yamuxPipe.catch((err: unknown) => {
      hostConn.close(err instanceof Error ? err : new Error(String(err)));
    });

    void handlers.handleWebRuntimeStream(webRuntimeStream);
    const quickJSStream = await quickJSStreamReady;
    const quickJSSinkPackets: Uint8Array[] = [];
    void pipe(quickJSStream.source, async (packets) => {
      for await (const packet of packets) {
        quickJSSinkPackets.push(packet);
      }
    });

    const webRuntimePacket = new Uint8Array([7, 8, 9]);
    webRuntimeSource.push(webRuntimePacket);
    await waitFor(
      () => quickJSSinkPackets.length === 1,
      "QuickJS handler did not receive WebRuntime packet through yamux",
    );
    expect(quickJSSinkPackets[0]).toEqual(webRuntimePacket);

    const quickJSPacket = new Uint8Array([10, 11, 12]);
    await quickJSStream.sink(
      (async function* () {
        yield quickJSPacket;
      })(),
    );
    await waitFor(
      () => webRuntimeSinkPackets.length === 1,
      "WebRuntime stream did not receive QuickJS packet through yamux",
    );
    expect(webRuntimeSinkPackets[0]).toEqual(quickJSPacket);

    webRuntimeSource.end();
    hostConn.close();
    quickJSConn.close();
  });

  it("keeps WebRuntime response path open after request source ends", async () => {
    const webRuntimeSource = pushable<Uint8Array>({ objectMode: true });
    const webRuntimeSinkPackets: Uint8Array[] = [];
    const webRuntimeStream: PacketStream = {
      source: webRuntimeSource,
      sink: async (packets) => {
        for await (const packet of packets) {
          webRuntimeSinkPackets.push(packet);
        }
      },
    };

    let resolveQuickJSStream!: (stream: PacketStream) => void;
    const quickJSStreamReady = new Promise<PacketStream>((resolve) => {
      resolveQuickJSStream = resolve;
    });
    const quickJSConn = new StreamConn(
      {
        handlePacketStream(stream) {
          resolveQuickJSStream(stream);
        },
      },
      {
        direction: "inbound",
        yamuxParams: { enableKeepAlive: false, maxMessageSize: 32 * 1024 },
      },
    );

    const hostConnRef: { current?: StreamConn } = {};
    const handlers = buildQuickJSBridgeHandlers({
      openWebRuntimeStream: vi.fn(),
      openQuickJSStream: () => {
        if (!hostConnRef.current) {
          throw new Error("QuickJS host connection is not initialized");
        }
        return hostConnRef.current.openStream();
      },
    });
    const hostConn = new StreamConn(
      { handlePacketStream: handlers.handleQuickJSStream },
      {
        direction: "outbound",
        yamuxParams: { enableKeepAlive: false, maxMessageSize: 32 * 1024 },
      },
    );
    hostConnRef.current = hostConn;

    const yamuxPipe = pipe(
      quickJSConn,
      hostConn,
      combineUint8ArrayListTransform(),
      quickJSConn,
    ) as Promise<unknown>;
    yamuxPipe.catch((err: unknown) => {
      hostConn.close(err instanceof Error ? err : new Error(String(err)));
    });

    void handlers.handleWebRuntimeStream(webRuntimeStream);
    const quickJSStream = await quickJSStreamReady;
    const quickJSSinkPackets: Uint8Array[] = [];
    void pipe(quickJSStream.source, async (packets) => {
      for await (const packet of packets) {
        quickJSSinkPackets.push(packet);
      }
    });

    const requestPacket = new Uint8Array([13, 14, 15]);
    webRuntimeSource.push(requestPacket);
    await waitFor(
      () => quickJSSinkPackets.length === 1,
      "QuickJS handler did not receive WebRuntime request packet",
    );
    expect(quickJSSinkPackets[0]).toEqual(requestPacket);

    webRuntimeSource.end();

    const responsePacket = new Uint8Array([16, 17, 18]);
    await quickJSStream.sink(
      (async function* () {
        yield responsePacket;
      })(),
    );
    await waitFor(
      () => webRuntimeSinkPackets.length === 1,
      "WebRuntime stream did not receive QuickJS response after request end",
    );
    expect(webRuntimeSinkPackets[0]).toEqual(responsePacket);

    hostConn.close();
    quickJSConn.close();
  });

  it("keeps ChannelStream WebRuntime responses open after request source ends", async () => {
    const channelName = `quickjs-bridge-${Date.now()}-${Math.random()}`;
    const callerToWorker = `${channelName}-caller-to-worker`;
    const workerToCaller = `${channelName}-worker-to-caller`;
    const webRuntimeCaller = new ChannelStream<Uint8Array>("caller", {
      tx: new BroadcastChannel(callerToWorker),
      rx: new BroadcastChannel(workerToCaller),
    });
    const webRuntimeWorker = new ChannelStream<Uint8Array>("worker", {
      tx: new BroadcastChannel(workerToCaller),
      rx: new BroadcastChannel(callerToWorker),
    });
    const callerWrites = pushable<Uint8Array>({ objectMode: true });
    const callerResponses: Uint8Array[] = [];
    const callerSink = webRuntimeCaller.sink(callerWrites);
    const callerRead = pipe(webRuntimeCaller.source, async (packets) => {
      for await (const packet of packets) {
        callerResponses.push(packet);
      }
    });

    let resolveQuickJSStream!: (stream: PacketStream) => void;
    const quickJSStreamReady = new Promise<PacketStream>((resolve) => {
      resolveQuickJSStream = resolve;
    });
    const quickJSConn = new StreamConn(
      {
        handlePacketStream(stream) {
          resolveQuickJSStream(stream);
        },
      },
      {
        direction: "inbound",
        yamuxParams: { enableKeepAlive: false, maxMessageSize: 32 * 1024 },
      },
    );

    const hostConnRef: { current?: StreamConn } = {};
    const handlers = buildQuickJSBridgeHandlers({
      openWebRuntimeStream: vi.fn(),
      openQuickJSStream: () => {
        if (!hostConnRef.current) {
          throw new Error("QuickJS host connection is not initialized");
        }
        return hostConnRef.current.openStream();
      },
    });
    const hostConn = new StreamConn(
      { handlePacketStream: handlers.handleQuickJSStream },
      {
        direction: "outbound",
        yamuxParams: { enableKeepAlive: false, maxMessageSize: 32 * 1024 },
      },
    );
    hostConnRef.current = hostConn;

    const yamuxPipe = pipe(
      quickJSConn,
      hostConn,
      combineUint8ArrayListTransform(),
      quickJSConn,
    ) as Promise<unknown>;
    yamuxPipe.catch((err: unknown) => {
      hostConn.close(err instanceof Error ? err : new Error(String(err)));
    });

    void handlers.handleWebRuntimeStream(webRuntimeWorker);
    const quickJSStream = await quickJSStreamReady;
    const quickJSSinkPackets: Uint8Array[] = [];
    const quickJSRead = pipe(quickJSStream.source, async (packets) => {
      for await (const packet of packets) {
        quickJSSinkPackets.push(packet);
      }
    });

    const requestPacket = new Uint8Array([19, 20, 21]);
    callerWrites.push(requestPacket);
    await waitFor(
      () => quickJSSinkPackets.length === 1,
      "QuickJS handler did not receive ChannelStream request packet",
    );
    expect(quickJSSinkPackets[0]).toEqual(requestPacket);

    callerWrites.end();
    await expect(callerSink).resolves.toBeUndefined();

    const responsePacket = new Uint8Array([22, 23, 24]);
    await quickJSStream.sink(
      (async function* () {
        yield responsePacket;
      })(),
    );
    await waitFor(
      () => callerResponses.length === 1,
      "WebRuntime caller did not receive ChannelStream response after request end",
    );
    expect(callerResponses[0]).toEqual(responsePacket);

    await expect(quickJSRead).resolves.toBeUndefined();
    await expect(callerRead).resolves.toBeUndefined();
    hostConn.close();
    quickJSConn.close();
    webRuntimeCaller.close();
    webRuntimeWorker.close();
  });

  it("includes remote RPC method details in bridge pipe errors", async () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    const remoteError = Object.assign(new Error("context canceled"), {
      name: "RemoteRPCError",
      rpcService: "web.runtime.WebRuntimeHost",
      rpcMethod: "WebWorkerRpc",
      rpcError: "context canceled",
    });
    const targetStream: PacketStream = {
      source: failingSource(remoteError),
      sink: vi.fn(async () => {}),
    };

    await pipeQuickJSBridgeStreams(
      buildPacketStream(),
      Promise.resolve(targetStream),
      { id: 17, direction: "quickjs-to-web-runtime" },
    );
    await waitFor(
      () => consoleError.mock.calls.length > 0,
      "bridge pipe error was not logged",
    );

    expect(consoleError.mock.calls[0]?.[0]).toContain(
      "quickjs-to-web-runtime#17 target-receive",
    );
    expect(consoleError.mock.calls[0]?.[0]).toContain(
      "web.runtime.WebRuntimeHost/WebWorkerRpc error=context canceled",
    );
    expect(consoleError.mock.calls[0]?.[1]).toBe(remoteError);
  });

  it("treats root QuickJS host connection pipe errors as fatal", async () => {
    const devOutStream = pushable<Uint8Array>({ objectMode: true });
    const failure = new Error("root mux broke");
    const hostConn = {
      source: failingSource(failure),
      sink: vi.fn(async (packets: Parameters<PacketStream["sink"]>[0]) => {
        for await (const _packet of packets) {
          // drain devOutStream while the host connection source reports failure
        }
      }),
      close: vi.fn(),
    };
    const pushStdin = vi.fn();
    const onFatalError = vi.fn();

    startQuickJSHostConnectionPipe(
      devOutStream,
      hostConn,
      pushStdin,
      onFatalError,
    );

    await waitFor(
      () => onFatalError.mock.calls.length === 1,
      "root host connection pipe failure was not fatal",
    );
    expect(onFatalError).toHaveBeenCalledWith(failure);
    expect(hostConn.close).toHaveBeenCalledWith(failure);
    expect(pushStdin).not.toHaveBeenCalled();

    devOutStream.end();
  });
});

describe("plugin-host-quickjs runner lifecycle", () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    Object.defineProperty(globalThis, "fetch", {
      value: originalFetch,
      configurable: true,
      writable: true,
    });
    vi.restoreAllMocks();
  });

  it("rejects when QuickJS exits nonzero from the reactor loop", async () => {
    const wasm = readFileSync(
      resolve("node_modules/quickjs-wasi-reactor/qjs-wasi.wasm"),
    );
    const pluginPath = "/b/pd/quickjs-exit-test/plugin.mjs";
    const pluginScript = `
      export default async function main() {
        std.exit(1)
      }
    `;
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestInfoURL(input);
      if (url === "/b/qjs/qjs-wasi.wasm") {
        return new Response(wasm, {
          headers: { "Content-Type": "application/wasm" },
        });
      }
      if (url === "/b/qjs/plugin-quickjs.esm.js") {
        return new Response(
          readFileSync(
            resolve("bldr/plugin/host/wazero-quickjs/plugin-quickjs.esm.js"),
            "utf8",
          ),
        );
      }
      if (url === pluginPath) {
        return new Response(pluginScript);
      }
      return new Response("", { status: 404 });
    });
    Object.defineProperty(globalThis, "fetch", {
      value: fetch,
      configurable: true,
      writable: true,
    });
    vi.spyOn(WebAssembly, "compileStreaming").mockImplementation(async () =>
      WebAssembly.compile(wasm),
    );
    vi.spyOn(console, "error").mockImplementation(() => {});

    const api = {
      startInfo: { pluginId: "quickjs-exit-test" },
      openStream: vi.fn(async () => buildPacketStream()),
      handleStreamCtr: new HandleStreamCtr(),
      utils: {
        pluginAssetHttpPath(pluginId: string, path: string): string {
          return `/b/pa/${pluginId}/${path}`;
        },
      },
    } as unknown as BackendAPI;

    await expect(
      quickJSRunner(api, new AbortController().signal, pluginPath),
    ).rejects.toThrow("quickjs-runner: Plugin exited with code 1");
  });

  it("resolves when QuickJS exits cleanly before readiness", async () => {
    const wasm = readFileSync(
      resolve("node_modules/quickjs-wasi-reactor/qjs-wasi.wasm"),
    );
    const pluginPath = "/b/pd/quickjs-clean-exit-test/plugin.mjs";
    const pluginScript = `
      export default async function main() {
        std.exit(0)
      }
    `;
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestInfoURL(input);
      if (url === "/b/qjs/qjs-wasi.wasm") {
        return new Response(wasm, {
          headers: { "Content-Type": "application/wasm" },
        });
      }
      if (url === "/b/qjs/plugin-quickjs.esm.js") {
        return new Response(
          readFileSync(
            resolve("bldr/plugin/host/wazero-quickjs/plugin-quickjs.esm.js"),
            "utf8",
          ),
        );
      }
      if (url === pluginPath) {
        return new Response(pluginScript);
      }
      return new Response("", { status: 404 });
    });
    Object.defineProperty(globalThis, "fetch", {
      value: fetch,
      configurable: true,
      writable: true,
    });
    vi.spyOn(WebAssembly, "compileStreaming").mockImplementation(async () =>
      WebAssembly.compile(wasm),
    );

    const api = {
      startInfo: { pluginId: "quickjs-clean-exit-test" },
      openStream: vi.fn(async () => buildPacketStream()),
      handleStreamCtr: new HandleStreamCtr(),
      utils: {
        pluginAssetHttpPath(pluginId: string, path: string): string {
          return `/b/pa/${pluginId}/${path}`;
        },
      },
    } as unknown as BackendAPI;

    await expect(
      quickJSRunner(api, new AbortController().signal, pluginPath),
    ).resolves.toBeUndefined();
  });

  it("delivers WebRuntime streams through the production runner after ready", async () => {
    const wasm = readFileSync(
      resolve("node_modules/quickjs-wasi-reactor/qjs-wasi.wasm"),
    );
    const pluginPath = "/b/pd/quickjs-stream-test/plugin.mjs";
    const pluginScript = `
      export default function main(api) {
        api.handleStreamCtr.set(async (stream) => {
          // Duplex WebRuntime streams may keep the response side open after the
          // request packet. Read one packet without closing the source iterator.
          const first =
            (await stream.source[Symbol.asyncIterator]().next()).value ||
            new Uint8Array(0)
          await stream.sink((async function* () {
            const response = new Uint8Array(first.length + 1)
            response[0] = 42
            response.set(first, 1)
            yield response
          })())
        })
        console.info('__BLDR_QUICKJS_PLUGIN_READY__')
      }
    `;
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestInfoURL(input);
      if (url === "/b/qjs/qjs-wasi.wasm") {
        return new Response(wasm, {
          headers: { "Content-Type": "application/wasm" },
        });
      }
      if (url === "/b/qjs/plugin-quickjs.esm.js") {
        return new Response(
          readFileSync(
            resolve("bldr/plugin/host/wazero-quickjs/plugin-quickjs.esm.js"),
            "utf8",
          ),
        );
      }
      if (url === pluginPath) {
        return new Response(pluginScript);
      }
      return new Response("", { status: 404 });
    });
    Object.defineProperty(globalThis, "fetch", {
      value: fetch,
      configurable: true,
      writable: true,
    });
    vi.spyOn(WebAssembly, "compileStreaming").mockImplementation(async () =>
      WebAssembly.compile(wasm),
    );

    const api = {
      startInfo: { pluginId: "quickjs-stream-test" },
      openStream: vi.fn(async () => buildPacketStream()),
      handleStreamCtr: new HandleStreamCtr(),
      utils: {
        pluginAssetHttpPath(pluginId: string, path: string): string {
          return `/b/pa/${pluginId}/${path}`;
        },
      },
    } as unknown as BackendAPI;

    const controller = new AbortController();
    let ready = false;
    const runner = quickJSRunner(api, controller.signal, pluginPath, {
      onReady: () => {
        ready = true;
      },
    });

    try {
      await Promise.race([
        waitFor(() => ready, "QuickJS runner did not report ready"),
        runner.then(
          () => {
            throw new Error("QuickJS runner exited before ready");
          },
          (err) => {
            throw err;
          },
        ),
      ]);

      const requestSource = pushable<Uint8Array>({ objectMode: true });
      const responsePackets: Uint8Array[] = [];
      const webRuntimeStream: PacketStream = {
        source: requestSource,
        sink: async (packets) => {
          for await (const packet of packets) {
            responsePackets.push(packet);
          }
        },
      };

      void api.handleStreamCtr.handleStreamFunc(webRuntimeStream);
      requestSource.push(new Uint8Array([7, 8, 9]));
      requestSource.end();

      await waitFor(
        () => responsePackets.length === 1,
        "QuickJS runner did not return WebRuntime stream response",
      );
      expect([...responsePackets[0]]).toEqual([42, 7, 8, 9]);
    } finally {
      controller.abort();
      await runner;
    }
  });

  it("delivers QuickJS-originated streams through the production runner before ready", async () => {
    const wasm = readFileSync(
      resolve("node_modules/quickjs-wasi-reactor/qjs-wasi.wasm"),
    );
    const pluginPath = "/b/pd/quickjs-open-stream-test/plugin.mjs";
    const pluginScript = `
      export default async function main(api) {
        const stream = await api.openStream()
        const responsePackets = []
        const readResponse = (async () => {
          for await (const packet of stream.source) {
            responsePackets.push(packet)
            break
          }
        })()
        await stream.sink((async function* () {
          yield new Uint8Array([7, 8, 9])
        })())
        await readResponse
        const response = responsePackets[0] || new Uint8Array(0)
        if (response[0] !== 42 || response[1] !== 7 || response[2] !== 8 || response[3] !== 9) {
          throw new Error('unexpected WebRuntime response packet: ' + Array.from(response).join(','))
        }
        console.info('__BLDR_QUICKJS_PLUGIN_READY__')
      }
    `;
    const fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestInfoURL(input);
      if (url === "/b/qjs/qjs-wasi.wasm") {
        return new Response(wasm, {
          headers: { "Content-Type": "application/wasm" },
        });
      }
      if (url === "/b/qjs/plugin-quickjs.esm.js") {
        return new Response(
          readFileSync(
            resolve("bldr/plugin/host/wazero-quickjs/plugin-quickjs.esm.js"),
            "utf8",
          ),
        );
      }
      if (url === pluginPath) {
        return new Response(pluginScript);
      }
      return new Response("", { status: 404 });
    });
    Object.defineProperty(globalThis, "fetch", {
      value: fetch,
      configurable: true,
      writable: true,
    });
    vi.spyOn(WebAssembly, "compileStreaming").mockImplementation(async () =>
      WebAssembly.compile(wasm),
    );

    const webRuntimeResponseSource = pushable<Uint8Array>({ objectMode: true });
    const webRuntimeRequestPackets: Uint8Array[] = [];
    const api = {
      startInfo: { pluginId: "quickjs-open-stream-test" },
      openStream: vi.fn(async () => ({
        source: webRuntimeResponseSource,
        sink: async (packets: AsyncIterable<Uint8Array>) => {
          for await (const packet of packets) {
            webRuntimeRequestPackets.push(packet);
            const response = new Uint8Array(packet.length + 1);
            response[0] = 42;
            response.set(packet, 1);
            webRuntimeResponseSource.push(response);
          }
          webRuntimeResponseSource.end();
        },
      })),
      handleStreamCtr: new HandleStreamCtr(),
      utils: {
        pluginAssetHttpPath(pluginId: string, path: string): string {
          return `/b/pa/${pluginId}/${path}`;
        },
      },
    } as unknown as BackendAPI;

    const controller = new AbortController();
    let ready = false;
    const runner = quickJSRunner(api, controller.signal, pluginPath, {
      onReady: () => {
        ready = true;
      },
    });

    try {
      await Promise.race([
        waitFor(() => ready, "QuickJS runner did not report outbound ready"),
        runner.then(
          () => {
            throw new Error("QuickJS runner exited before outbound ready");
          },
          (err) => {
            throw err;
          },
        ),
      ]);

      expect(api.openStream).toHaveBeenCalledTimes(1);
      expect(webRuntimeRequestPackets.map((packet) => [...packet])).toEqual([
        [7, 8, 9],
      ]);
    } finally {
      controller.abort();
      await runner;
    }
  });
});

describe("plugin-host-quickjs asset helpers", () => {
  const originalXMLHttpRequest = globalThis.XMLHttpRequest;
  const originalFetch = globalThis.fetch;
  const originalNavigatorDescriptor = Object.getOwnPropertyDescriptor(
    globalThis,
    "navigator",
  );
  const api = {
    startInfo: { pluginId: "notes" },
    utils: {
      pluginAssetHttpPath(pluginId: string, path: string) {
        return `/asset/${pluginId}/${path}`;
      },
    },
  };

  afterEach(() => {
    Object.defineProperty(globalThis, "XMLHttpRequest", {
      value: originalXMLHttpRequest,
      configurable: true,
      writable: true,
    });
    Object.defineProperty(globalThis, "fetch", {
      value: originalFetch,
      configurable: true,
      writable: true,
    });
    if (originalNavigatorDescriptor) {
      Object.defineProperty(
        globalThis,
        "navigator",
        originalNavigatorDescriptor,
      );
    } else {
      Reflect.deleteProperty(globalThis, "navigator");
    }
    vi.restoreAllMocks();
  });

  it("collects bounded static vite manifest asset paths for backend entrypoints", () => {
    const paths = collectViteManifestStaticAssetPaths(
      {
        "plugin/notes/backend.ts": {
          file: "plugin/notes/backend-abc123.mjs",
          imports: ["_chunk-shared-1.mjs"],
          dynamicImports: ["_chunk-lazy-2.mjs"],
          css: ["assets/backend.css"],
          assets: ["assets/icon.svg"],
        },
        "_chunk-shared-1.mjs": {
          file: "chunks/shared-1.mjs",
        },
        "_chunk-lazy-2.mjs": {
          file: "chunks/lazy-2.mjs",
        },
        "plugin/v86/backend.ts": {
          file: "plugin/v86/backend-def456.mjs",
        },
      },
      ["/assets/v/b/be/plugin/notes/backend-abc123.mjs"],
    );

    expect(paths).toEqual([
      "plugin/notes/backend-abc123.mjs",
      "assets/backend.css",
      "assets/icon.svg",
      "chunks/shared-1.mjs",
    ]);
  });

  it("normalizes backend asset paths into the v/b/be tree", () => {
    expect(resolveBackendAssetPath("plugin/notes/backend-abc123.mjs")).toBe(
      "v/b/be/plugin/notes/backend-abc123.mjs",
    );
    expect(
      resolveBackendAssetPath("b/be/plugin/notes/backend-abc123.mjs"),
    ).toBe("v/b/be/plugin/notes/backend-abc123.mjs");
    expect(
      resolveBackendAssetPath("v/b/be/plugin/notes/backend-abc123.mjs"),
    ).toBe("v/b/be/plugin/notes/backend-abc123.mjs");
  });

  it("extracts backend entrypoint asset paths from the plugin wrapper", () => {
    const paths = collectBackendEntrypointAssetPaths(`
      const backendEntrypoints = [
        { importPath: "/assets/v/b/be/plugin/notes/backend-abc123.mjs" },
        { importPath: '/assets/v/b/be/plugin/notes/backend-abc123.mjs' },
        { importPath: "/assets/v/b/fe/plugin/notes/App-def456.mjs" },
      ]
    `);

    expect(paths).toEqual(["/assets/v/b/be/plugin/notes/backend-abc123.mjs"]);
  });

  it("preloads backend assets from wrapper imports without treating /b/pd as an asset", async () => {
    const requests: string[] = [];
    const manifest = JSON.stringify({
      "plugin/notes/backend.ts": {
        file: "plugin/notes/backend-abc123.mjs",
        imports: ["_chunk-shared-1.mjs"],
        css: ["assets/backend.css"],
      },
      "_chunk-shared-1.mjs": {
        file: "chunks/shared-1.mjs",
      },
    });
    const bodies = new Map<string, string>([
      ["/asset/notes/v/b/be/.vite/manifest.json", manifest],
      [
        "/asset/notes/v/b/be/plugin/notes/backend-abc123.mjs",
        "export default function backend() {}",
      ],
      ["/asset/notes/v/b/be/chunks/shared-1.mjs", "export const shared = true"],
      ["/asset/notes/v/b/be/backend.css", ".backend{}"],
    ]);

    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => {
        requests.push(url);
        const body = bodies.get(url);
        if (body == null) {
          return new Response("missing", { status: 404 });
        }
        return new Response(body, { status: 200 });
      }),
    );

    const files = new Map<string, string | Uint8Array>();
    const cache = new Map<string, BackendAssetCacheEntry>();
    const loaded = await loadBackendAssets(
      api,
      new AbortController().signal,
      files,
      collectBackendEntrypointAssetPaths(
        'import("/assets/v/b/be/plugin/notes/backend-abc123.mjs")',
      ),
      cache,
    );

    expect(loaded).toBe(true);
    expect(requests).toEqual([
      "/asset/notes/v/b/be/.vite/manifest.json",
      "/asset/notes/v/b/be/plugin/notes/backend-abc123.mjs",
      "/asset/notes/v/b/be/backend.css",
      "/asset/notes/v/b/be/chunks/shared-1.mjs",
    ]);
    expect(requests.some((url) => url.includes("/v/b/pd/"))).toBe(false);
    expect(files.has("v/b/be/plugin/notes/backend-abc123.mjs")).toBe(true);
    expect(files.has("v/b/be/chunks/shared-1.mjs")).toBe(true);
    expect(cache.get("v/b/be/plugin/notes/backend-abc123.mjs")?.ok).toBe(true);
    expect(cache.get("v/b/be/chunks/shared-1.mjs")?.ok).toBe(true);
  });

  it("does not fall back to whole-manifest preload without backend entrypoints", async () => {
    const fetchMock = vi.fn(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const files = new Map<string, string | Uint8Array>();
    const loaded = await loadBackendAssets(
      api,
      new AbortController().signal,
      files,
      [],
    );

    expect(loaded).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(files.size).toBe(0);
  });

  it("mirrors assets under both asset-relative and /assets paths", () => {
    const files = new Map<string, string | Uint8Array>();

    addAssetToFileSystem(
      files,
      "plugin/notes/backend-abc123.mjs",
      "export default {}",
    );

    expect(files.get("v/b/be/plugin/notes/backend-abc123.mjs")).toBe(
      "export default {}",
    );
    expect(files.get("/assets/v/b/be/plugin/notes/backend-abc123.mjs")).toBe(
      "export default {}",
    );
  });

  it("lazily fetches backend assets through synchronous XHR and caches reads", () => {
    const requests: string[] = [];
    const enc = new TextEncoder();

    class MockXMLHttpRequest {
      status = 0;
      response: ArrayBuffer | null = null;
      responseText = "";
      responseType = "";
      private url = "";

      open(_method: string, url: string, async: boolean) {
        expect(async).toBe(false);
        this.url = url;
      }

      getResponseHeader() {
        return null
      }

      send() {
        requests.push(this.url);
        if (this.url.endsWith("/v/b/be/plugin/app.mjs")) {
          const data = enc.encode("export const ok = true");
          this.status = 200;
          this.response = data.buffer;
          return;
        }
        this.status = 404;
      }
    }

    Object.defineProperty(globalThis, "XMLHttpRequest", {
      value: MockXMLHttpRequest,
      configurable: true,
      writable: true,
    });

    expect(canUseSynchronousBackendAssetFetch()).toBe(true);
    const mount = createBackendAssetMount(api, new AbortController().signal);
    expect(mount).not.toBeNull();
    expect(requests).toEqual([]);

    const file = mount?.getFile("v/b/be/plugin/app.mjs");
    expect(new TextDecoder().decode(file?.readAt(0n, 64))).toBe(
      "export const ok = true",
    );
    expect(mount?.getFile("v/b/be/plugin/app.mjs")?.size).toBe(22n);
    expect(mount?.getFile("v/b/be/plugin/missing.mjs")).toBeNull();
    expect(mount?.getFile("v/b/be/plugin/missing.mjs")).toBeNull();
    expect(requests).toEqual([
      "/asset/notes/v/b/be/plugin/app.mjs",
      "/asset/notes/v/b/be/plugin/missing.mjs",
    ]);
  });

  it("selects lazy backend asset loading when sync XHR is available", () => {
    Object.defineProperty(globalThis, "XMLHttpRequest", {
      value: undefined,
      configurable: true,
      writable: true,
    });
    expect(canUseSynchronousBackendAssetFetch()).toBe(false);
    expect(selectBackendAssetLoadingMode()).toBe("bounded-preload");

    Object.defineProperty(globalThis, "XMLHttpRequest", {
      value: class MockXMLHttpRequest {},
      configurable: true,
      writable: true,
    });
    Object.defineProperty(globalThis, "navigator", {
      value: { userAgent: "Chrome/126.0.0.0" },
      configurable: true,
      writable: true,
    });
    expect(canUseSynchronousBackendAssetFetch()).toBe(true);
    expect(selectBackendAssetLoadingMode()).toBe("lazy-http");
  });

  it("selects bounded backend asset preload for Firefox workers", () => {
    Object.defineProperty(globalThis, "XMLHttpRequest", {
      value: class MockXMLHttpRequest {},
      configurable: true,
      writable: true,
    });
    Object.defineProperty(globalThis, "navigator", {
      value: { userAgent: "Mozilla/5.0 Firefox/126.0" },
      configurable: true,
      writable: true,
    });

    expect(canUseSynchronousBackendAssetFetch()).toBe(true);
    expect(shouldPreferBoundedBackendAssetPreload()).toBe(true);
    expect(selectBackendAssetLoadingMode()).toBe("bounded-preload");
  });

  it("preloads backend assets only for bounded fallback mode", () => {
    const entrypoints = ["/assets/v/b/be/plugin/app.mjs"];

    expect(shouldPreloadBackendAssets("lazy-http", entrypoints)).toBe(false);
    expect(shouldPreloadBackendAssets("bounded-preload", entrypoints)).toBe(
      true,
    );
    expect(shouldPreloadBackendAssets("bounded-preload", [])).toBe(false);
  });

  it("serves compiler-emitted backend import paths from lazy preopens", () => {
    const requests: string[] = [];
    const enc = new TextEncoder();

    class MockXMLHttpRequest {
      status = 0;
      response: ArrayBuffer | null = null;
      responseText = "";
      responseType = "";
      private url = "";

      open(_method: string, url: string, async: boolean) {
        expect(async).toBe(false);
        this.url = url;
      }

      send() {
        requests.push(this.url);
        const data = enc.encode("export const path = true");
        this.status = 200;
        this.response = data.buffer;
      }
    }

    Object.defineProperty(globalThis, "XMLHttpRequest", {
      value: MockXMLHttpRequest,
      configurable: true,
      writable: true,
    });

    const preopens = createBackendAssetPreopens(
      api,
      new AbortController().signal,
    );

    const assetsOpen = preopens[0].path_open(
      0,
      "v/b/be/plugin/app.mjs",
      0,
      0n,
      0n,
      0,
    );
    const rootVOpen = preopens[1].path_open(
      0,
      "b/be/plugin/app.mjs",
      0,
      0n,
      0n,
      0,
    );

    expect(assetsOpen.ret).toBe(0);
    expect(rootVOpen.ret).toBe(0);
    expect(new TextDecoder().decode(assetsOpen.fd_obj?.fd_read(64).data)).toBe(
      "export const path = true",
    );
    expect(new TextDecoder().decode(rootVOpen.fd_obj?.fd_read(64).data)).toBe(
      "export const path = true",
    );
    expect(requests).toEqual(["/asset/notes/v/b/be/plugin/app.mjs"]);
  });

  it("serves warmed backend assets from lazy preopens without sync XHR", () => {
    const enc = new TextEncoder();
    const cache = new Map<string, BackendAssetCacheEntry>([
      [
        "v/b/be/plugin/app.mjs",
        { ok: true, data: enc.encode("export const warmed = true") },
      ],
    ]);

    class MockXMLHttpRequest {
      open() {}

      send() {
        throw new Error("unexpected XHR");
      }
    }

    Object.defineProperty(globalThis, "XMLHttpRequest", {
      value: MockXMLHttpRequest,
      configurable: true,
      writable: true,
    });

    const preopens = createBackendAssetPreopens(
      api,
      new AbortController().signal,
      cache,
    );

    const assetsOpen = preopens[0].path_open(
      0,
      "v/b/be/plugin/app.mjs",
      0,
      0n,
      0n,
      0,
    );
    const rootVOpen = preopens[1].path_open(
      0,
      "b/be/plugin/app.mjs",
      0,
      0n,
      0n,
      0,
    );

    expect(assetsOpen.ret).toBe(0);
    expect(rootVOpen.ret).toBe(0);
    expect(new TextDecoder().decode(assetsOpen.fd_obj?.fd_read(64).data)).toBe(
      "export const warmed = true",
    );
    expect(new TextDecoder().decode(rootVOpen.fd_obj?.fd_read(64).data)).toBe(
      "export const warmed = true",
    );
  });

  it("surfaces lazy backend asset failures without whole-manifest fallback", () => {
    class MockXMLHttpRequest {
      status = 503;
      response: ArrayBuffer | null = null;
      responseText = "";
      responseType = "";

      open(_method: string, _url: string, async: boolean) {
        expect(async).toBe(false);
      }

      getResponseHeader() {
        return null
      }

      send() {}
    }

    Object.defineProperty(globalThis, "XMLHttpRequest", {
      value: MockXMLHttpRequest,
      configurable: true,
      writable: true,
    });

    const mount = createBackendAssetMount(api, new AbortController().signal)
    expect(() => mount?.getFile('v/b/be/plugin/app.mjs')).toThrow(
      'QuickJS backend asset unavailable /asset/notes/v/b/be/plugin/app.mjs: 503',
    )
  })

  it('surfaces typed lazy backend asset runtime unavailability', () => {
    class MockXMLHttpRequest {
      status = 503
      response: ArrayBuffer | null = null
      responseText = '{"code":"runtime-unavailable"}'
      responseType = ''

      open(_method: string, _url: string, async: boolean) {
        expect(async).toBe(false)
      }

      getResponseHeader(name: string) {
        if (name === 'X-Bldr-Plugin-Asset-Fetch-Result') {
          return 'runtime-unavailable'
        }
        return null
      }

      send() {}
    }

    Object.defineProperty(globalThis, 'XMLHttpRequest', {
      value: MockXMLHttpRequest,
      configurable: true,
      writable: true,
    })

    const mount = createBackendAssetMount(api, new AbortController().signal)
    expect(() => mount?.getFile('v/b/be/plugin/app.mjs')).toThrow(
      'QuickJS backend asset runtime unavailable /asset/notes/v/b/be/plugin/app.mjs: 503: {"code":"runtime-unavailable"}',
    )
  })

  it('returns false for optional missing backend manifests', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('missing', { status: 404 })),
    )

    const loaded = await loadBackendAssets(
      api,
      new AbortController().signal,
      new Map<string, string | Uint8Array>(),
      ['/assets/v/b/be/plugin/notes/backend-abc123.mjs'],
    )

    expect(loaded).toBe(false)
  })

  it('surfaces typed bounded-preload backend manifest generation closure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        return new Response('generation closed', {
          status: 410,
          headers: {
            'X-Bldr-Plugin-Asset-Fetch-Result': 'generation-closed',
          },
        })
      }),
    )

    await expect(
      loadBackendAssets(
        api,
        new AbortController().signal,
        new Map<string, string | Uint8Array>(),
        ['/assets/v/b/be/plugin/notes/backend-abc123.mjs'],
      ),
    ).rejects.toThrow(
      'QuickJS backend asset generation closed /asset/notes/v/b/be/.vite/manifest.json: 410: generation closed',
    )
  })

  it('surfaces typed bounded-preload backend asset misses', async () => {
    const manifest = JSON.stringify({
      'plugin/notes/backend.ts': {
        file: 'plugin/notes/backend-abc123.mjs',
      },
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/v/b/be/.vite/manifest.json')) {
          return new Response(manifest, { status: 200 })
        }
        return new Response('asset missing', {
          status: 404,
          headers: {
            'X-Bldr-Plugin-Asset-Fetch-Result': 'missing',
          },
        })
      }),
    )

    await expect(
      loadBackendAssets(
        api,
        new AbortController().signal,
        new Map<string, string | Uint8Array>(),
        ['/assets/v/b/be/plugin/notes/backend-abc123.mjs'],
      ),
    ).rejects.toThrow(
      'Missing backend asset /asset/notes/v/b/be/plugin/notes/backend-abc123.mjs: 404: asset missing',
    )
  })

  it('maps QuickJS readiness markers to frontend, capability, and running callbacks', () => {
    const readiness = {
      frontendReady: false,
      capabilityReady: false,
      ready: false,
    }
    const onFrontendReady = vi.fn()
    const onCapabilityReady = vi.fn()
    const onReady = vi.fn()

    expect(
      handleQuickJSReadinessMarker(
        '__BLDR_QUICKJS_PLUGIN_FRONTEND_READY__',
        readiness,
        { onFrontendReady, onCapabilityReady, onReady },
      ),
    ).toBe(true)
    expect(readiness).toMatchObject({
      frontendReady: true,
      capabilityReady: false,
      ready: false,
    })
    expect(onFrontendReady).toHaveBeenCalledOnce()
    expect(onCapabilityReady).not.toHaveBeenCalled()
    expect(onReady).not.toHaveBeenCalled()

    expect(
      handleQuickJSReadinessMarker(
        '__BLDR_QUICKJS_PLUGIN_CAPABILITY_READY__',
        readiness,
        { onFrontendReady, onCapabilityReady, onReady },
      ),
    ).toBe(true)
    expect(readiness).toMatchObject({
      frontendReady: true,
      capabilityReady: true,
      ready: false,
    })
    expect(onFrontendReady).toHaveBeenCalledOnce()
    expect(onCapabilityReady).toHaveBeenCalledOnce()
    expect(onReady).not.toHaveBeenCalled()

    expect(
      handleQuickJSReadinessMarker('__BLDR_QUICKJS_PLUGIN_READY__', readiness, {
        onFrontendReady,
        onCapabilityReady,
        onReady,
      }),
    ).toBe(true)
    expect(readiness).toMatchObject({
      frontendReady: true,
      capabilityReady: true,
      ready: true,
    })
    expect(onFrontendReady).toHaveBeenCalledOnce()
    expect(onCapabilityReady).toHaveBeenCalledOnce()
    expect(onReady).toHaveBeenCalledOnce()
  })
})

function buildPacketStream(): PacketStream {
  return {
    source: (async function* () {})(),
    sink: vi.fn(async () => {}),
  }
}

function failingSource(error: Error): AsyncGenerator<Uint8Array> {
  return {
    [Symbol.asyncIterator]() {
      return this;
    },
    next: async () => {
      throw error;
    },
    return: async () => ({ done: true, value: undefined }),
    throw: async (err?: unknown) => {
      throw err ?? error;
    },
    [Symbol.asyncDispose]: async () => {},
  };
}

function requestInfoURL(input: RequestInfo | URL): string {
  if (typeof input === 'string') {
    return input
  }
  if (input instanceof URL) {
    return input.toString()
  }
  return input.url
}

async function waitFor(
  predicate: () => boolean,
  errorMessage: string,
): Promise<void> {
  for (let attempt = 0; attempt < 25; attempt++) {
    if (predicate()) {
      return
    }
    await new Promise((resolve) => setTimeout(resolve, 10))
  }
  throw new Error(errorMessage)
}
