// plugin-host-quickjs.ts runs a JavaScript plugin in the QuickJS WASI reactor.
//
// This QuickJS worker runner is a dormant capability retained for future
// sandboxed plugin execution. It is intentionally not imported by the main
// bundle or any live path; the wasi-shim directory is its dependency.
//
// Architecture:
// 1. Fetch QuickJS WASM from /b/qjs/qjs-wasi.wasm
// 2. Fetch boot harness from /b/qjs/plugin-quickjs.esm.js
// 3. Fetch plugin script from scriptPath
// 4. Create WASI environment with stdin/dev-out for yamux
// 5. Call qjs.init(["qjs", "--std", bootHarnessPath]) to initialize and run boot harness
// 6. Run event loop with loopOnce()
//    - Returns >0: setTimeout(loop, ms)
//    - Returns 0: queueMicrotask(loop)
//    - Returns -1: idle, wait for I/O
//    - Returns -2: error
// 7. Yields to browser event loop between iterations

import { StreamConn, type PacketStream } from "starpc";
import { pipe } from "it-pipe";
import { pushable } from "it-pushable";
import {
  QuickJS,
  buildFileSystem,
  createReadOnlyMount,
  PollableStdin,
  LOOP_IDLE,
  LOOP_ERROR,
  type Fd,
  type ReadOnlyFileMount,
} from "quickjs-wasi-reactor";

import { BackendAPI } from "@aptre/bldr-sdk";
import { PluginStartInfo } from "../../../plugin/plugin.pb.js";
import { isNormalRuntimeClientClose } from "../../bldr/web-runtime-client.js";

type ViteManifestEntry = {
  file?: string;
  imports?: string[];
  dynamicImports?: string[];
  css?: string[];
  assets?: string[];
};

const backendAssetsRoot = 'v/b/be/'
const quickJSPluginFrontendReadyMarker =
  '__BLDR_QUICKJS_PLUGIN_FRONTEND_READY__'
const quickJSPluginCapabilityReadyMarker =
  '__BLDR_QUICKJS_PLUGIN_CAPABILITY_READY__'
const quickJSPluginReadyMarker = '__BLDR_QUICKJS_PLUGIN_READY__'

// Cached compiled QuickJS WASM module (shared across plugin restarts)
let cachedWasmModule: WebAssembly.Module | null = null;
// Cached boot harness code
let cachedBootHarness: string | null = null;

// loadQuickJSModule fetches and compiles the QuickJS WASM module.
async function loadQuickJSModule(): Promise<WebAssembly.Module> {
  if (cachedWasmModule) {
    return cachedWasmModule;
  }
  const response = await fetch("/b/qjs/qjs-wasi.wasm");
  if (!response.ok) {
    throw new Error(`Failed to fetch QuickJS WASM: ${response.status}`);
  }
  cachedWasmModule = await WebAssembly.compileStreaming(response);
  return cachedWasmModule;
}

// loadBootHarness fetches the boot harness JavaScript code.
async function loadBootHarness(): Promise<string> {
  if (cachedBootHarness) {
    return cachedBootHarness;
  }
  const response = await fetch("/b/qjs/plugin-quickjs.esm.js");
  if (!response.ok) {
    throw new Error(`Failed to fetch boot harness: ${response.status}`);
  }
  cachedBootHarness = await response.text();
  return cachedBootHarness;
}

// fetchPluginScript fetches the plugin script from the HTTP server.
async function fetchPluginScript(scriptPath: string): Promise<string> {
  const response = await fetch(scriptPath);
  if (!response.ok) {
    throw new Error(
      `Failed to fetch plugin script ${scriptPath}: ${response.status}`,
    );
  }
  return response.text();
}

// collectViteManifestStaticAssetPaths returns the static asset graph for entrypoint files.
export function collectViteManifestStaticAssetPaths(
  manifest: Record<string, ViteManifestEntry>,
  entryAssetPaths: string[],
): string[] {
  const entryByResolvedFile = new Map<string, string>();
  for (const [ref, entry] of Object.entries(manifest)) {
    if (entry.file) {
      entryByResolvedFile.set(resolveBackendAssetPath(entry.file), ref);
    }
  }

  const paths = new Set<string>();
  const addPath = (path?: string) => {
    if (!path) {
      return;
    }
    paths.add(path);
  };

  const visited = new Set<string>();
  const visitRef = (ref: string) => {
    const entry = manifest[ref];
    if (!entry) {
      addPath(ref);
      return;
    }
    if (visited.has(ref)) {
      return;
    }
    visited.add(ref);

    addPath(entry.file);
    for (const path of entry.css ?? []) {
      addPath(path);
    }
    for (const path of entry.assets ?? []) {
      addPath(path);
    }
    for (const dep of entry.imports ?? []) {
      visitRef(dep);
    }
  };

  for (const entryPath of entryAssetPaths) {
    const resolvedPath = resolveBackendAssetPath(entryPath);
    const ref = entryByResolvedFile.get(resolvedPath);
    if (ref) {
      visitRef(ref);
      continue;
    }
    addPath(resolvedPath);
  }
  return [...paths];
}

// collectBackendEntrypointAssetPaths extracts backend asset imports from the
// compiled plugin wrapper.
export function collectBackendEntrypointAssetPaths(
  pluginScript: string,
): string[] {
  const paths = new Set<string>();
  const assetPathRe = /["'`]\/assets\/(v\/b\/be\/[^"'`\\\s?#)]+)/g;
  for (;;) {
    const match = assetPathRe.exec(pluginScript);
    if (!match) {
      break;
    }
    paths.add("/assets/" + match[1]);
  }
  return [...paths];
}

// resolveBackendAssetPath normalizes a backend asset path to the v/b/be asset tree.
export function resolveBackendAssetPath(path: string): string {
  const trimmed = path.replace(/^\/+/, "");
  if (!trimmed) {
    return "";
  }
  if (trimmed.startsWith("v/")) {
    return trimmed;
  }
  if (trimmed.startsWith("assets/")) {
    return resolveBackendAssetPath(trimmed.slice("assets/".length));
  }
  if (trimmed.startsWith("b/")) {
    return "v/" + trimmed;
  }
  return backendAssetsRoot + trimmed;
}

// addAssetToFileSystem mirrors an asset under both its asset-relative path and /assets/.
export function addAssetToFileSystem(
  files: Map<string, string | Uint8Array>,
  assetPath: string,
  content: string | Uint8Array,
): void {
  const resolvedPath = resolveBackendAssetPath(assetPath);
  if (!resolvedPath) {
    return;
  }
  files.set(resolvedPath, content);
  files.set("/assets/" + resolvedPath, content);
}

export type BackendAssetFetchFailureResult =
  | 'missing'
  | 'unavailable'
  | 'generation-closed'
  | 'root-changed'
  | 'runtime-unavailable'
  | 'canceled'

export type BackendAssetCacheEntry =
  | { ok: true; data: Uint8Array }
  | {
      ok: false
      status: number
      url: string
      result: BackendAssetFetchFailureResult
      message?: string
    }

export type BackendAssetLoadingMode = "lazy-http" | "bounded-preload";

export type QuickJSRunnerOptions = {
  onFrontendReady?: () => void
  onCapabilityReady?: () => void
  onReady?: () => void
}

export type QuickJSRunnerReadiness = {
  frontendReady: boolean
  capabilityReady: boolean
  ready: boolean
}

type BackendAssetAPI = {
  startInfo: Pick<BackendAPI["startInfo"], "pluginId">;
  utils: Pick<BackendAPI["utils"], "pluginAssetHttpPath">;
};

type QuickJSBridgeDirection =
  | "quickjs-to-web-runtime"
  | "web-runtime-to-quickjs"
  | "unknown";
type QuickJSBridgeOpenStream = () => Promise<PacketStream>;
export type QuickJSBridgePipeLabel = {
  id: number;
  direction: QuickJSBridgeDirection;
};
type QuickJSBridgePipeStreams = (
  source: PacketStream,
  target: Promise<PacketStream>,
  label: QuickJSBridgePipeLabel,
) => Promise<void> | void;

export type QuickJSBridgeHandlers = {
  handleQuickJSStream: (stream: PacketStream) => Promise<void>;
  handleWebRuntimeStream: (stream: PacketStream) => Promise<void>;
};

export type QuickJSHostConnectionPipe = PacketStream & {
  close?: (err?: Error) => void;
};

export type QuickJSBridgeHandlerOptions = {
  openWebRuntimeStream: QuickJSBridgeOpenStream;
  openQuickJSStream: QuickJSBridgeOpenStream;
  pipeStreams?: QuickJSBridgePipeStreams;
};

let nextQuickJSBridgeStreamID = 1;

export function buildQuickJSBridgeHandlers({
  openWebRuntimeStream,
  openQuickJSStream,
  pipeStreams = pipeQuickJSBridgeStreams,
}: QuickJSBridgeHandlerOptions): QuickJSBridgeHandlers {
  const forward = async (
    stream: PacketStream,
    openTargetStream: QuickJSBridgeOpenStream,
    direction: QuickJSBridgeDirection,
  ) => {
    await pipeStreams(stream, Promise.resolve().then(openTargetStream), {
      id: nextQuickJSBridgeStreamID++,
      direction,
    });
  };

  return {
    handleQuickJSStream: (stream) =>
      forward(stream, openWebRuntimeStream, "quickjs-to-web-runtime"),
    handleWebRuntimeStream: (stream) =>
      forward(stream, openQuickJSStream, "web-runtime-to-quickjs"),
  };
}

export async function pipeQuickJSBridgeStreams(
  source: PacketStream,
  target: Promise<PacketStream>,
  label: QuickJSBridgePipeLabel = { id: 0, direction: "unknown" },
): Promise<void> {
  const sourceToTarget = pushable<Uint8Array>({ objectMode: true });
  const targetToSource = pushable<Uint8Array>({ objectMode: true });

  const sourceReadPipe = pipe(source.source, async (packets) => {
    try {
      for await (const packet of packets) {
        sourceToTarget.push(packet);
      }
      sourceToTarget.end();
    } catch (err) {
      const error = toQuickJSBridgeError(err);
      sourceToTarget.end(error);
      targetToSource.end(error);
      throw error;
    }
  }).catch((err) => {
    const error = toQuickJSBridgeError(err);
    sourceToTarget.end(error);
    targetToSource.end(error);
    logQuickJSBridgePipeError(label, "source-read", err);
    throw error;
  });

  const sourceWritePipe = pipe(targetToSource, async (packets) => {
    try {
      await source.sink(
        (async function* () {
          for await (const packet of packets) {
            yield packet;
          }
        })(),
      );
    } catch (err) {
      const error = toQuickJSBridgeError(err);
      sourceToTarget.end(error);
      throw error;
    }
  }).catch((err) => {
    const error = toQuickJSBridgeError(err);
    sourceToTarget.end(error);
    logQuickJSBridgePipeError(label, "source-write", err);
    throw error;
  });

  try {
    const targetStream = await target;
    const sendPipe = pipe(sourceToTarget, async (packets) => {
      try {
        await targetStream.sink(
          (async function* () {
            for await (const packet of packets) {
              yield packet;
            }
          })(),
        );
      } catch (err) {
        const error = toQuickJSBridgeError(err);
        targetToSource.end(error);
        throw error;
      }
    }).catch((err) => {
      const error = toQuickJSBridgeError(err);
      targetToSource.end(error);
      logQuickJSBridgePipeError(label, "target-send", err);
      throw error;
    });
    const receivePipe = pipe(targetStream.source, async (packets) => {
      try {
        for await (const packet of packets) {
          targetToSource.push(packet);
        }
        targetToSource.end();
      } catch (err) {
        targetToSource.end(toQuickJSBridgeError(err));
        throw err;
      }
    }).catch((err) => {
      const error = toQuickJSBridgeError(err);
      logQuickJSBridgePipeError(label, "target-receive", err);
      throw error;
    });
    void Promise.all([
      sourceReadPipe,
      sourceWritePipe,
      sendPipe,
      receivePipe,
    ]).catch(() => {});
  } catch (err) {
    const error = toQuickJSBridgeError(err);
    sourceToTarget.end(error);
    targetToSource.end(error);
    if (err === error) {
      throw error;
    }
    console.error("quickjs-runner: stream open error:", err);
    throw error;
  }
}

function toQuickJSBridgeError(err: unknown): Error {
  if (err instanceof Error) {
    return err;
  }
  return new Error(String(err));
}

function logQuickJSBridgePipeError(
  label: QuickJSBridgePipeLabel,
  stage: string,
  err: unknown,
): void {
  const detail = describeQuickJSBridgePipeError(err);
  // Normal-close generation teardown ends bridge pipes by design; it is
  // lifecycle cleanup, not a plugin failure, so keep it out of console.error.
  if (isExpectedQuickJSBridgeClose(err)) {
    console.debug(
      `quickjs-runner: stream pipe closed (${label.direction}#${label.id} ${stage})${detail}:`,
      err,
    );
    return;
  }
  console.error(
    `quickjs-runner: stream pipe error (${label.direction}#${label.id} ${stage})${detail}:`,
    err,
  );
}

function isExpectedQuickJSBridgeClose(err: unknown): boolean {
  if (isNormalRuntimeClientClose(err)) {
    return true;
  }
  return (
    err instanceof Error &&
    /^WebDocumentTracker: .+: closed while waiting for WebDocument$/.test(
      err.message,
    )
  );
}

function describeQuickJSBridgePipeError(err: unknown): string {
  if (typeof err !== "object" || err === null) {
    return "";
  }
  const maybeRemote = err as {
    name?: unknown;
    rpcService?: unknown;
    rpcMethod?: unknown;
    rpcError?: unknown;
  };
  if (
    typeof maybeRemote.rpcService !== "string" ||
    typeof maybeRemote.rpcMethod !== "string"
  ) {
    return "";
  }
  const rpcError =
    typeof maybeRemote.rpcError === "string" && maybeRemote.rpcError
      ? ` error=${maybeRemote.rpcError}`
      : "";
  return ` ${maybeRemote.rpcService}/${maybeRemote.rpcMethod}${rpcError}`;
}

export function startQuickJSHostConnectionPipe(
  devOutStream: AsyncIterable<Uint8Array>,
  hostConn: QuickJSHostConnectionPipe,
  pushStdin: (data: Uint8Array) => void,
  onFatalError: (err: Error) => void,
): void {
  void pipe(devOutStream, hostConn, async (source) => {
    for await (const chunk of source) {
      pushStdin(normalizeQuickJSHostConnectionChunk(chunk));
    }
  }).catch((err) => {
    const error = toQuickJSBridgeError(err);
    hostConn.close?.(error);
    onFatalError(error);
  });
}

function normalizeQuickJSHostConnectionChunk(
  chunk: Uint8Array | { subarray(): Uint8Array },
): Uint8Array {
  return chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk.subarray());
}

// canUseSynchronousBackendAssetFetch returns true when the worker can service
// WASI filesystem misses without suspending the QuickJS import path.
export function canUseSynchronousBackendAssetFetch(): boolean {
  if (typeof XMLHttpRequest === "undefined") {
    return false;
  }
  return typeof XMLHttpRequest === "function";
}

export function shouldPreferBoundedBackendAssetPreload(): boolean {
  return /\bFirefox\//.test(globalThis.navigator?.userAgent ?? "");
}

export function selectBackendAssetLoadingMode(): BackendAssetLoadingMode {
  if (shouldPreferBoundedBackendAssetPreload()) {
    return "bounded-preload";
  }
  return canUseSynchronousBackendAssetFetch() ? "lazy-http" : "bounded-preload";
}

export function shouldPreloadBackendAssets(
  mode: BackendAssetLoadingMode,
  entryAssetPaths: string[],
): boolean {
  return mode === "bounded-preload" && entryAssetPaths.length !== 0;
}

// createBackendAssetMount returns a synchronous read-only mount over plugin HTTP assets.
export function createBackendAssetMount(
  api: BackendAssetAPI,
  signal: AbortSignal,
  pathPrefix = "",
): ReadOnlyFileMount | null {
  return createBackendAssetMountWithCache(
    api,
    signal,
    pathPrefix,
    new Map<string, BackendAssetCacheEntry>(),
  );
}

function createBackendAssetMountWithCache(
  api: BackendAssetAPI,
  signal: AbortSignal,
  pathPrefix: string,
  cache: Map<string, BackendAssetCacheEntry>,
): ReadOnlyFileMount | null {
  const pluginId = api.startInfo.pluginId;
  if (!pluginId) {
    return null;
  }

  return {
    getFile(path: string) {
      const resolvedPath = resolveBackendAssetPath(pathPrefix + path);
      if (!resolvedPath) {
        return null;
      }
      if (signal.aborted) {
        throw new Error(`QuickJS backend asset read aborted: ${resolvedPath}`);
      }

      const cached = cache.get(resolvedPath);
      if (cached?.ok) {
        return backendAssetFile(cached.data);
      }
      if (cached && cached.result === 'missing') {
        return null
      }
      if (cached) {
        throw new Error(formatBackendAssetFailure(cached))
      }

      const url = api.utils.pluginAssetHttpPath(pluginId, resolvedPath);
      const entry = fetchBackendAssetSync(url);
      cache.set(resolvedPath, entry);
      if (entry.ok) {
        return backendAssetFile(entry.data);
      }
      if (entry.result === 'missing') {
        return null
      }
      throw new Error(formatBackendAssetFailure(entry))
    },
  };
}

export function createBackendAssetPreopens(
  api: BackendAssetAPI,
  signal: AbortSignal,
  cache = new Map<string, BackendAssetCacheEntry>(),
): Fd[] {
  const assetsMount = createBackendAssetMountWithCache(api, signal, "", cache);
  const rootVMount = createBackendAssetMountWithCache(api, signal, "v/", cache);
  const preopens: Fd[] = [];
  if (assetsMount) {
    preopens.push(createReadOnlyMount("/assets", assetsMount));
  }
  if (rootVMount) {
    preopens.push(createReadOnlyMount("/v", rootVMount));
  }
  return preopens;
}

function backendAssetFile(data: Uint8Array) {
  return {
    size: BigInt(data.byteLength),
    readAt(offset: bigint, size: number) {
      return data.slice(Number(offset), Number(offset) + size);
    },
  };
}

function fetchBackendAssetSync(url: string): BackendAssetCacheEntry {
  const xhr = new XMLHttpRequest();
  xhr.open("GET", url, false);
  xhr.responseType = "arraybuffer";
  xhr.send();

  if (xhr.status < 200 || xhr.status >= 300) {
    return {
      ok: false,
      status: xhr.status,
      url,
      result: backendAssetFetchFailureResult(
        xhr.status,
        xhr.getResponseHeader.bind(xhr),
      ),
      message: xhr.responseText || undefined,
    }
  }
  const response = xhr.response;
  if (response instanceof ArrayBuffer) {
    return { ok: true, data: new Uint8Array(response) };
  }
  return {
    ok: true,
    data: new TextEncoder().encode(xhr.responseText),
  };
}

function backendAssetFetchFailureResult(
  status: number,
  getHeader: (name: string) => string | null,
): BackendAssetFetchFailureResult {
  const result = getHeader('X-Bldr-Plugin-Asset-Fetch-Result')
  if (
    result === 'generation-closed' ||
    result === 'root-changed' ||
    result === 'runtime-unavailable' ||
    result === 'canceled' ||
    result === 'missing' ||
    result === 'unavailable'
  ) {
    return result
  }
  if (status === 404) {
    return 'missing'
  }
  return 'unavailable'
}

async function backendAssetFetchFailureFromResponse(
  url: string,
  response: Response,
): Promise<Extract<BackendAssetCacheEntry, { ok: false }>> {
  return {
    ok: false,
    status: response.status,
    url,
    result: backendAssetFetchFailureResult(
      response.status,
      response.headers.get.bind(response.headers),
    ),
    message: (await response.text()).trim() || undefined,
  }
}

function formatBackendAssetFailure(
  failure: Extract<BackendAssetCacheEntry, { ok: false }>,
): string {
  const detail = failure.message ? `: ${failure.message}` : ''
  switch (failure.result) {
    case 'missing':
      return `Missing backend asset ${failure.url}: ${failure.status}${detail}`
    case 'generation-closed':
      return `QuickJS backend asset generation closed ${failure.url}: ${failure.status}${detail}`
    case 'root-changed':
      return `QuickJS backend asset root changed ${failure.url}: ${failure.status}${detail}`
    case 'runtime-unavailable':
      return `QuickJS backend asset runtime unavailable ${failure.url}: ${failure.status}${detail}`
    case 'canceled':
      return `QuickJS backend asset fetch canceled ${failure.url}: ${failure.status}${detail}`
    case 'unavailable':
      return `QuickJS backend asset unavailable ${failure.url}: ${failure.status}${detail}`
  }
}

export async function loadBackendAssets(
  api: BackendAssetAPI,
  signal: AbortSignal,
  files: Map<string, string | Uint8Array>,
  entryAssetPaths: string[],
  cache?: Map<string, BackendAssetCacheEntry>,
): Promise<boolean> {
  const pluginId = api.startInfo.pluginId;
  if (!pluginId || entryAssetPaths.length === 0) {
    return false;
  }

  const manifestPath = backendAssetsRoot + '.vite/manifest.json'
  const manifestURL = api.utils.pluginAssetHttpPath(pluginId, manifestPath)
  const manifestResponse = await fetch(manifestURL, { signal })
  if (!manifestResponse.ok) {
    const failure = await backendAssetFetchFailureFromResponse(
      manifestURL,
      manifestResponse,
    )
    if (failure.result === 'missing') {
      return false
    }
    throw new Error(formatBackendAssetFailure(failure))
  }

  const manifestText = await manifestResponse.text();
  addAssetToFileSystem(files, manifestPath, manifestText);

  const manifest = JSON.parse(manifestText) as Record<
    string,
    ViteManifestEntry
  >;
  const assetPaths = collectViteManifestStaticAssetPaths(
    manifest,
    entryAssetPaths,
  );

  const resolvedPaths = assetPaths
    .map((assetPath) => resolveBackendAssetPath(assetPath))
    .filter((resolvedPath) => resolvedPath !== "");
  await mapWithConcurrency(resolvedPaths, 8, async (resolvedPath) => {
    const assetURL = api.utils.pluginAssetHttpPath(pluginId, resolvedPath);
    const assetResponse = await fetch(assetURL, { signal });
    if (!assetResponse.ok) {
      const failure = await backendAssetFetchFailureFromResponse(
        assetURL,
        assetResponse,
      )
      cache?.set(resolvedPath, failure)
      throw new Error(formatBackendAssetFailure(failure))
    }
    const data = new Uint8Array(await assetResponse.arrayBuffer());
    cache?.set(resolvedPath, { ok: true, data });
    addAssetToFileSystem(files, resolvedPath, data);
  });
  return true;
}

async function mapWithConcurrency<T>(
  values: T[],
  concurrency: number,
  fn: (value: T) => Promise<void>,
): Promise<void> {
  let next = 0;
  const workers = Array.from(
    { length: Math.min(concurrency, values.length) },
    async () => {
      for (;;) {
        const index = next;
        next += 1;
        if (index >= values.length) {
          return;
        }
        await fn(values[index]);
      }
    },
  );
  await Promise.all(workers);
}

export function handleQuickJSReadinessMarker(
  line: string,
  readiness: QuickJSRunnerReadiness,
  opts: QuickJSRunnerOptions = {},
): boolean {
  if (line === quickJSPluginFrontendReadyMarker) {
    if (!readiness.frontendReady) {
      readiness.frontendReady = true
      opts.onFrontendReady?.()
    }
    return true
  }
  if (line === quickJSPluginCapabilityReadyMarker) {
    if (!readiness.capabilityReady) {
      readiness.capabilityReady = true
      opts.onCapabilityReady?.()
    }
    return true
  }
  if (line === quickJSPluginReadyMarker) {
    if (!readiness.frontendReady) {
      readiness.frontendReady = true
      opts.onFrontendReady?.()
    }
    if (!readiness.capabilityReady) {
      readiness.capabilityReady = true
      opts.onCapabilityReady?.()
    }
    if (!readiness.ready) {
      readiness.ready = true
      opts.onReady?.()
    }
    return true
  }
  return false
}

function quickJSReactorExited(qjs: QuickJS): boolean {
  return Reflect.get(qjs, "running") === false
}

function markQuickJSReactorRunning(qjs: QuickJS): void {
  Reflect.set(qjs, "running", true)
}

// main runs a JavaScript plugin in the QuickJS WASI reactor.
//
// Unlike native JS plugins that run directly in the browser, QuickJS plugins
// run inside a WebAssembly-based JavaScript VM. This provides isolation and
// allows running plugins that use synchronous I/O patterns.
export default async function main(
  api: BackendAPI,
  signal: AbortSignal,
  scriptPath: string,
  opts: QuickJSRunnerOptions = {},
): Promise<void> {
  console.log("quickjs-runner: loading QuickJS and boot harness...");

  // Load WASM module, boot harness, and plugin script in parallel
  const [wasmModule, bootHarness] = await Promise.all([
    loadQuickJSModule(),
    loadBootHarness(),
  ]);

  console.log("quickjs-runner: setting up WASI environment...");

  // Create pollable stdin for yamux communication (host -> plugin)
  const stdin = new PollableStdin();

  // Track /dev/out writes for yamux communication (plugin -> host)
  const devOutStream = pushable<Uint8Array>({ objectMode: true });

  // Build virtual filesystem with boot harness and plugin script
  // The scriptPath is expected to be like /b/pd/plugin-name/plugin-HASH.mjs
  const files = new Map<string, string | Uint8Array>();

  // Add boot harness at /boot/plugin-quickjs.esm.js
  files.set("/boot/plugin-quickjs.esm.js", bootHarness);
  const pluginScript = await fetchPluginScript(scriptPath);
  files.set(scriptPath, pluginScript);

  const assetLoadingMode = selectBackendAssetLoadingMode();
  const entryAssetPaths = collectBackendEntrypointAssetPaths(pluginScript);
  const backendAssetCache = new Map<string, BackendAssetCacheEntry>();
  const loadedBackendAssets = shouldPreloadBackendAssets(
    assetLoadingMode,
    entryAssetPaths,
  )
    ? await loadBackendAssets(
        api,
        signal,
        files,
        entryAssetPaths,
        backendAssetCache,
      )
    : false;

  const preopens =
    assetLoadingMode === "lazy-http"
      ? createBackendAssetPreopens(api, signal, backendAssetCache)
      : [];

  if (assetLoadingMode === "bounded-preload") {
    if (loadedBackendAssets) {
      console.log("quickjs-runner: using bounded backend asset preload");
    }
    if (!loadedBackendAssets) {
      console.log(
        "quickjs-runner: backend asset preload unavailable; using direct backend script",
      );
    }
  }
  if (assetLoadingMode === "lazy-http") {
    console.log("quickjs-runner: using lazy backend asset mount");
  }

  const fs = buildFileSystem(files);

  // Encode start info for the plugin
  const startInfoB64 = btoa(PluginStartInfo.toJsonString(api.startInfo));

  console.log("quickjs-runner: instantiating QuickJS reactor...");

  const readiness: QuickJSRunnerReadiness = {
    frontendReady: false,
    capabilityReady: false,
    ready: false,
  }

  const qjs = new QuickJS(wasmModule, {
    args: ["qjs"],
    env: [
      `BLDR_SCRIPT_PATH=${scriptPath}`,
      `BLDR_PLUGIN_START_INFO=${startInfoB64}`,
    ],
    fs,
    preopens,
    stdin,
    stdout: (line) => {
      if (handleQuickJSReadinessMarker(line, readiness, opts)) {
        return
      }
      console.log("[QuickJS stdout]", line);
    },
    stderr: (line) => console.log("[QuickJS stderr]", line),
    onDevOut: (data) => devOutStream.push(new Uint8Array(data)),
  });

  console.log("quickjs-runner: initializing QuickJS...");

  // Initialize QuickJS with --std flag and boot harness path.
  // This sets up the module loader and evaluates the boot harness as the main script.
  qjs.init(["qjs", "--std", "/boot/plugin-quickjs.esm.js"]);
  markQuickJSReactorRunning(qjs)

  console.log("quickjs-runner: starting reactor event loop...");

  let running = true;
  let exitCode = 0;
  let fatalError: Error | undefined;
  let pendingTimeout: ReturnType<typeof setTimeout> | null = null;
  let loopScheduled = false;
  let exitResolve: (() => void) | null = null;

  const finishWithError = (message: string, err?: unknown) => {
    running = false;
    exitCode = 1;
    if (!fatalError) {
      fatalError =
        err === undefined ? new Error(message) : toQuickJSBridgeError(err);
    }
    if (err !== undefined) {
      console.error(message, err);
    } else {
      console.error(message);
    }
    exitResolve?.();
  };

  const hostConnRef: { current?: StreamConn } = {};
  const bridgeHandlers = buildQuickJSBridgeHandlers({
    openWebRuntimeStream: api.openStream.bind(api),
    openQuickJSStream: () => {
      if (!hostConnRef.current) {
        throw new Error("QuickJS host connection is not initialized");
      }
      return hostConnRef.current.openStream();
    },
  });

  // Set up yamux connection for RPC.
  // QuickJS-originated streams travel through yamux and then out to WebRuntime.
  const hostConn = new StreamConn(
    { handlePacketStream: bridgeHandlers.handleQuickJSStream },
    {
      direction: "outbound",
      yamuxParams: {
        enableKeepAlive: false,
        maxMessageSize: 32 * 1024,
      },
    },
  );
  hostConnRef.current = hostConn;

  // Pipe devOut to hostConn, and hostConn output to stdin
  startQuickJSHostConnectionPipe(
    devOutStream,
    hostConn as unknown as QuickJSHostConnectionPipe,
    (data) => qjs.pushStdin(data),
    (err) => finishWithError("quickjs-runner: yamux pipe error:", err),
  );

  // WebRuntime-originated streams enter through the outer API and then open a
  // yamux stream back into QuickJS.
  api.handleStreamCtr.set(bridgeHandlers.handleWebRuntimeStream);

  const pollPendingStdin = (): boolean => {
    if (!qjs.hasStdinData()) {
      return false;
    }
    try {
      qjs.pollIO(0);
    } catch (e) {
      finishWithError("quickjs-runner: error in pollIO:", e);
      return false;
    }
    return running;
  };

  const scheduleRunLoop = () => {
    if (!running || loopScheduled) {
      return;
    }
    loopScheduled = true;
    queueMicrotask(runLoop);
  };

  const wakeRunLoop = () => {
    if (pendingTimeout !== null) {
      clearTimeout(pendingTimeout);
      pendingTimeout = null;
    }
    scheduleRunLoop();
  };

  // Handle abort signal
  const onAbort = () => {
    running = false;
    if (pendingTimeout !== null) {
      clearTimeout(pendingTimeout);
      pendingTimeout = null;
    }
    exitResolve?.();
  };
  signal.addEventListener("abort", onAbort);

  // Wake callback: when stdin receives data, cancel any pending timeout and run
  // immediately. Stdin may arrive while QuickJS has pending JS work.
  qjs.onStdinWake(wakeRunLoop);

  const runLoop = () => {
    loopScheduled = false;
    if (!running) {
      exitResolve?.();
      return;
    }
    pendingTimeout = null;

    let result: number;
    try {
      result = qjs.loopOnce();
    } catch (e) {
      finishWithError("quickjs-runner: error in loopOnce:", e);
      return;
    }
    const quickJSExitCode = qjs.getExitCode();
    if (quickJSExitCode !== 0) {
      finishWithError(
        `quickjs-runner: Plugin exited with code ${quickJSExitCode}`,
      );
      return;
    }

    if (result === LOOP_ERROR) {
      finishWithError("quickjs-runner: JavaScript error occurred");
      return;
    }

    if (result === 0) {
      if (pollPendingStdin()) {
        scheduleRunLoop();
        return;
      }
      // More microtasks pending, continue immediately
      scheduleRunLoop();
      return;
    }

    if (result > 0) {
      // Timer pending - but also check stdin
      if (pollPendingStdin()) {
        scheduleRunLoop();
        return;
      }
      if (!running) {
        return;
      }
      // Wait for timer (onStdinWake will interrupt if data arrives)
      pendingTimeout = setTimeout(runLoop, result);
      return;
    }

    if (result === LOOP_IDLE) {
      // Idle - check if stdin has data
      if (pollPendingStdin()) {
        scheduleRunLoop();
        return;
      }
      if (quickJSReactorExited(qjs)) {
        running = false;
        exitResolve?.();
        return;
      }
      if (!running) {
        return;
      }
      // No data - wait for onStdinWake callback to restart the loop. There is
      // intentionally no timeout here because backgrounded tabs throttle timers.
      return;
    }
  };

  // Start the event loop
  scheduleRunLoop();

  // Wait for the plugin to exit
  await new Promise<void>((resolve) => {
    exitResolve = resolve;
    if (!running) resolve();
  });

  const finalExitCode = exitCode !== 0 ? exitCode : qjs.getExitCode();

  // Cleanup
  signal.removeEventListener("abort", onAbort);
  devOutStream.end();
  qjs.onStdinWake(null);
  qjs.destroy();

  if (finalExitCode !== 0) {
    throw fatalError ?? new Error(`Plugin exited with code ${finalExitCode}`);
  }
}
