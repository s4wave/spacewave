import { QuickjsGlobalScope } from "./quickjs.js";
import {
  createEvent,
  createEventTarget,
  createCustomEvent,
} from "./polyfill-event.js";
import {
  createAbortController,
  type AbortControllerPolyfillConstructor,
  type AbortSignalPolyfillConstructor,
} from "./polyfill-abort-controller.js";
import { createSymbolPolyfills } from "./polyfill-symbol.js";
import { TextEncoder, TextDecoder } from "./text-encoding.js";
import { createQuickjsConsole, type Console } from "./console.js";
import { createQuickjsPerformance, type Performance } from "./performance.js";
import { atob, btoa } from "./base64.js";

// quickjs has a reduced standard library.
// this file polyfills exactly what we need and probably will need to be expanded over time.
// the implementations are often significantly simplified from the "real" ones.
// it may be better long term to integrate a more capable js wasm engine.
// see: https://github.com/saghul/txiki.js

// QuickjsPolyfillGlobalScope represents QuickjsGlobalScope after the polyfills are applied.
export interface QuickjsPolyfillGlobalScope extends QuickjsGlobalScope {
  // AbortController is the polyfilled abort controller type.
  AbortController: AbortControllerPolyfillConstructor;
  // AbortSignal is the polyfilled abort signal constructor and static helpers.
  AbortSignal: AbortSignalPolyfillConstructor;
  // Event is the polyfilled event constructor type.
  Event: typeof Event;
  // EventTarget is the polyfilled EventTarget constructor type.
  EventTarget: typeof EventTarget;
  // CustomEvent is the polyfilled CustomEvent constructor type.
  CustomEvent: typeof CustomEvent;
  // TextEncoder is the polyfilled text encoder type.
  TextEncoder: typeof TextEncoder;
  // TextDecoder is the polyfilled text encoder type.
  TextDecoder: typeof TextDecoder;

  // console is the polyfilled console object.
  console: Console;
  // performance is the polyfilled performance object.
  performance: Performance;

  /**
   * Call the function func after delay ms. Return a handle to the timer.
   * @param func - Function to call
   * @param delay - Delay in milliseconds
   */
  setTimeout(func: () => void, delay: number): NodeJS.Timeout;

  /**
   * Cancel a timer.
   * @param handle - Timer handle
   */
  clearTimeout(handle: NodeJS.Timeout): void;

  /**
   * Call the function func periodically with the given interval. Return a handle to the timer.
   * @param func - Function to call
   * @param delay - Interval in milliseconds
   */
  setInterval(func: () => void, delay: number): NodeJS.Timeout;

  /**
   * Cancel an interval timer.
   * @param handle - Timer handle
   */
  clearInterval(handle: NodeJS.Timeout): void;

  /**
   * Queue a microtask.
   * @param func - Function to call
   */
  queueMicrotask(func: () => void): void;

  // global is the polyfilled global reference.
  global: QuickjsPolyfillGlobalScope;
  // window is the polyfilled window reference.
  window: QuickjsPolyfillGlobalScope;
  // self is the polyfilled self reference.
  self: QuickjsPolyfillGlobalScope;

  // atob decodes a base64 encoded string.
  atob: typeof atob;
  // btoa encodes a string to base64.
  btoa: typeof btoa;
}

export interface QuickjsSchedulerTarget {
  os: Pick<
    QuickjsGlobalScope["os"],
    "setTimeout" | "clearTimeout" | "setInterval" | "clearInterval"
  >;
  setTimeout?: QuickjsPolyfillGlobalScope["setTimeout"];
  clearTimeout?: QuickjsPolyfillGlobalScope["clearTimeout"];
  setInterval?: QuickjsPolyfillGlobalScope["setInterval"];
  clearInterval?: QuickjsPolyfillGlobalScope["clearInterval"];
  queueMicrotask?: QuickjsPolyfillGlobalScope["queueMicrotask"];
}

export interface QuickjsSchedulerRoots {
  microtasks: Set<() => void>;
  timeouts: Map<NodeJS.Timeout, () => void>;
  intervals: Map<NodeJS.Timeout, () => void>;
}

export function installRetainedSchedulerPolyfills(
  target: QuickjsSchedulerTarget,
): QuickjsSchedulerRoots {
  const roots: QuickjsSchedulerRoots = {
    microtasks: new Set(),
    timeouts: new Map(),
    intervals: new Map(),
  };

  const queueMicrotask =
    target.queueMicrotask?.bind(target) ??
    ((func: () => void) => Promise.resolve().then(func));
  target.queueMicrotask = (func: () => void): void => {
    const retained = () => {
      try {
        func();
      } finally {
        roots.microtasks.delete(retained);
      }
    };
    roots.microtasks.add(retained);
    queueMicrotask(retained);
  };

  const setTimeout = target.os.setTimeout.bind(target.os);
  const clearTimeout = target.os.clearTimeout.bind(target.os);
  target.setTimeout = (func: () => void, delay: number): NodeJS.Timeout => {
    const retained = () => {
      try {
        func();
      } finally {
        roots.timeouts.delete(handle);
      }
    };
    const handle = setTimeout(retained, delay);
    roots.timeouts.set(handle, retained);
    return handle;
  };
  target.clearTimeout = (handle: NodeJS.Timeout): void => {
    roots.timeouts.delete(handle);
    clearTimeout(handle);
  };

  const setInterval = target.os.setInterval.bind(target.os);
  const clearInterval = target.os.clearInterval.bind(target.os);
  target.setInterval = (func: () => void, delay: number): NodeJS.Timeout => {
    const retained = () => {
      func();
    };
    const handle = setInterval(retained, delay);
    roots.intervals.set(handle, retained);
    return handle;
  };
  target.clearInterval = (handle: NodeJS.Timeout): void => {
    roots.intervals.delete(handle);
    clearInterval(handle);
  };

  return roots;
}

// applyPolyfills applies the polyfills to the global scope.
export function applyPolyfills(
  to: QuickjsGlobalScope,
): QuickjsPolyfillGlobalScope {
  const target: QuickjsPolyfillGlobalScope = to as QuickjsPolyfillGlobalScope;

  // Define global scope references that all point to the same object
  const globalRefs = ["global", "window", "self"];
  globalRefs.forEach((name) => {
    Object.defineProperty(to, name, {
      enumerable: true,
      get() {
        return to;
      },
      set() {},
    });
  });

  createSymbolPolyfills();

  target.console = createQuickjsConsole(target.console);
  target.performance = createQuickjsPerformance(target.performance);
  target.Event = createEvent();
  target.EventTarget = createEventTarget();
  target.CustomEvent = createCustomEvent();
  const AbortControllerImpl = createAbortController();
  target.AbortController = AbortControllerImpl;
  target.AbortSignal = AbortControllerImpl.AbortSignal;
  target.TextEncoder = TextEncoder;
  target.TextDecoder = TextDecoder;
  installRetainedSchedulerPolyfills(target);

  target.atob = atob;
  target.btoa = btoa;

  return target;
}
