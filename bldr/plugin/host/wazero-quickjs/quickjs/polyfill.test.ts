import { describe, expect, it, vi } from "vitest";

import {
  applyPolyfills,
  installRetainedSchedulerPolyfills,
  type QuickjsSchedulerTarget,
} from "./polyfill.js";
import type { QuickjsGlobalScope } from "./quickjs.js";

describe("installRetainedSchedulerPolyfills", () => {
  it("retains microtask callbacks until the host invokes them", () => {
    const scheduled: (() => void)[] = [];
    const target = buildSchedulerTarget({
      queueMicrotask: (func) => {
        scheduled.push(func);
      },
    });
    const roots = installRetainedSchedulerPolyfills(target);
    const callback = vi.fn();

    target.queueMicrotask!(callback);

    expect(scheduled).toHaveLength(1);
    expect(scheduled[0]).not.toBe(callback);
    expect(roots.microtasks.size).toBe(1);

    scheduled[0]();

    expect(callback).toHaveBeenCalledTimes(1);
    expect(roots.microtasks.size).toBe(0);
  });

  it("releases one-shot timer callbacks after invocation or cancellation", () => {
    const scheduled = new Map<NodeJS.Timeout, () => void>();
    const target = buildSchedulerTarget({
      setTimeout: (func) => {
        const handle = scheduled.size + 1;
        scheduled.set(handle as unknown as NodeJS.Timeout, func);
        return handle as unknown as NodeJS.Timeout;
      },
      clearTimeout: (handle) => {
        scheduled.delete(handle);
      },
    });
    const roots = installRetainedSchedulerPolyfills(target);

    const callback = vi.fn();
    const first = target.setTimeout!(callback, 10);
    expect(roots.timeouts.size).toBe(1);
    scheduled.get(first)!();
    expect(callback).toHaveBeenCalledTimes(1);
    expect(roots.timeouts.size).toBe(0);

    const second = target.setTimeout!(callback, 10);
    expect(roots.timeouts.size).toBe(1);
    target.clearTimeout!(second);
    expect(roots.timeouts.size).toBe(0);
  });

  it("retains interval callbacks until cancellation", () => {
    const scheduled = new Map<NodeJS.Timeout, () => void>();
    const target = buildSchedulerTarget({
      setInterval: (func) => {
        const handle = scheduled.size + 1;
        scheduled.set(handle as unknown as NodeJS.Timeout, func);
        return handle as unknown as NodeJS.Timeout;
      },
      clearInterval: (handle) => {
        scheduled.delete(handle);
      },
    });
    const roots = installRetainedSchedulerPolyfills(target);
    const callback = vi.fn();

    const interval = target.setInterval!(callback, 10);
    expect(roots.intervals.size).toBe(1);

    scheduled.get(interval)!();

    expect(callback).toHaveBeenCalledTimes(1);
    expect(roots.intervals.size).toBe(1);

    target.clearInterval!(interval);

    expect(roots.intervals.size).toBe(0);
  });
});

describe("applyPolyfills", () => {
  it("exposes AbortSignal static helpers globally", () => {
    const target = buildPolyfillTarget();
    const polyfilled = applyPolyfills(target);

    expect(polyfilled.AbortSignal).toBe(polyfilled.AbortController.AbortSignal);
    expect(polyfilled.AbortSignal.abort).toBeTypeOf("function");
    expect(polyfilled.AbortSignal.timeout).toBeTypeOf("function");

    const signal = polyfilled.AbortSignal.abort("stopped");
    expect(signal.aborted).toBe(true);
    expect(signal.reason).toBe("stopped");
  });
});

function buildSchedulerTarget(
  overrides: Partial<QuickjsSchedulerTarget> & {
    setTimeout?: (func: () => void, delay: number) => NodeJS.Timeout;
    clearTimeout?: (handle: NodeJS.Timeout) => void;
    setInterval?: (func: () => void, delay: number) => NodeJS.Timeout;
    clearInterval?: (handle: NodeJS.Timeout) => void;
  },
): QuickjsSchedulerTarget {
  const setTimeout = overrides.setTimeout ?? vi.fn();
  const clearTimeout = overrides.clearTimeout ?? vi.fn();
  const setInterval = overrides.setInterval ?? vi.fn();
  const clearInterval = overrides.clearInterval ?? vi.fn();

  return {
    queueMicrotask: overrides.queueMicrotask,
    os: {
      setTimeout,
      clearTimeout,
      setInterval,
      clearInterval,
    },
  };
}

function buildPolyfillTarget(): QuickjsGlobalScope {
  return {
    console: {
      log: vi.fn(),
    },
    performance: {
      now: () => 0,
    },
    os: {
      setTimeout: ((func: () => void, delay: number) =>
        setTimeout(func, delay)) as QuickjsGlobalScope["os"]["setTimeout"],
      clearTimeout: ((handle: NodeJS.Timeout) =>
        clearTimeout(handle)) as QuickjsGlobalScope["os"]["clearTimeout"],
      setInterval: ((func: () => void, delay: number) =>
        setInterval(func, delay)) as QuickjsGlobalScope["os"]["setInterval"],
      clearInterval: ((handle: NodeJS.Timeout) =>
        clearInterval(handle)) as QuickjsGlobalScope["os"]["clearInterval"],
    },
  } as unknown as QuickjsGlobalScope;
}
