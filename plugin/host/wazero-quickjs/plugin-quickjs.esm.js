/* eslint-disable */
(() => {
  // quickjs/polyfill-symbol.ts
  function createSymbolPolyfills() {
    Symbol.asyncIterator || Object.defineProperty(Symbol, "asyncIterator", {
      value: /* @__PURE__ */ Symbol("Symbol.asyncIterator"),
      writable: false,
      enumerable: false,
      configurable: false
    }), Symbol.dispose || Object.defineProperty(Symbol, "dispose", {
      value: /* @__PURE__ */ Symbol("Symbol.dispose"),
      writable: false,
      enumerable: false,
      configurable: false
    }), Symbol.asyncDispose || Object.defineProperty(Symbol, "asyncDispose", {
      value: /* @__PURE__ */ Symbol("Symbol.asyncDispose"),
      writable: false,
      enumerable: false,
      configurable: false
    });
  }

  // quickjs/event-target.js
  var privateData = /* @__PURE__ */ new WeakMap();
  function pd(event) {
    let retv = privateData.get(event);
    if (!retv)
      throw new Error("'this' is expected an Event object, but got " + event);
    return retv;
  }
  function setCancelFlag(data) {
    if (data.passiveListener !== null) {
      console.error(
        "Unable to preventDefault inside passive event listener invocation.",
        data.passiveListener
      );
      return;
    }
    data.eventInit.cancelable && (data.canceled = true);
  }
  var Event = class {
    constructor(eventType, eventInit = {}) {
      if (eventInit && typeof eventInit != "object")
        throw TypeError("Value must be an object.");
      privateData.set(this, {
        eventInit,
        eventPhase: 2,
        eventType: String(eventType),
        currentTarget: null,
        canceled: false,
        stopped: false,
        immediateStopped: false,
        passiveListener: null,
        timeStamp: Date.now()
      }), Object.defineProperty(this, "isTrusted", { value: false, enumerable: true });
    }
    /**
     * The type of this event.
     * @type {string}
     */
    get type() {
      return pd(this).eventType;
    }
    /**
     * The target of this event.
     * @type {EventTarget}
     */
    get target() {
      return this.currentTarget;
    }
    /**
     * The target of this event.
     * @type {EventTarget}
     */
    get currentTarget() {
      return pd(this).currentTarget;
    }
    /**
     * @returns {EventTarget[]} The composed path of this event.
     */
    composedPath() {
      let currentTarget = pd(this).currentTarget;
      return currentTarget ? [currentTarget] : [];
    }
    /**
     * Constant of NONE.
     * @type {number}
     */
    get NONE() {
      return 0;
    }
    /**
     * Constant of CAPTURING_PHASE.
     * @type {number}
     */
    get CAPTURING_PHASE() {
      return 1;
    }
    /**
     * Constant of AT_TARGET.
     * @type {number}
     */
    get AT_TARGET() {
      return 2;
    }
    /**
     * Constant of BUBBLING_PHASE.
     * @type {number}
     */
    get BUBBLING_PHASE() {
      return 3;
    }
    /**
     * The target of this event.
     * @type {number}
     */
    get eventPhase() {
      return pd(this).eventPhase;
    }
    /**
     * Stop event bubbling.
     * @returns {void}
     */
    stopPropagation() {
      pd(this).stopped = true;
    }
    /**
     * Stop event bubbling.
     * @returns {void}
     */
    stopImmediatePropagation() {
      let data = pd(this);
      data.stopped = true, data.immediateStopped = true;
    }
    /**
     * The flag to be bubbling.
     * @type {boolean}
     */
    get bubbles() {
      return !!pd(this).eventInit.bubbles;
    }
    /**
     * The flag to be cancelable.
     * @type {boolean}
     */
    get cancelable() {
      return !!pd(this).eventInit.cancelable;
    }
    /**
     * Cancel this event.
     * @returns {void}
     */
    preventDefault() {
      setCancelFlag(pd(this));
    }
    /**
     * The flag to indicate cancellation state.
     * @type {boolean}
     */
    get defaultPrevented() {
      return pd(this).canceled;
    }
    /**
     * The flag to be composed.
     * @type {boolean}
     */
    get composed() {
      return !!pd(this).eventInit.composed;
    }
    /**
     * The unix time of this event.
     * @type {number}
     */
    get timeStamp() {
      return pd(this).timeStamp;
    }
  }, CustomEvent = class extends Event {
    /**
     * Any data passed when initializing the event.
     * @type {any}
     */
    get detail() {
      return pd(this).eventInit.detail;
    }
  };
  function isStopped(event) {
    return pd(event).immediateStopped;
  }
  function setEventPhase(event, eventPhase) {
    pd(event).eventPhase = eventPhase;
  }
  function setCurrentTarget(event, currentTarget) {
    pd(event).currentTarget = currentTarget;
  }
  function setPassiveListener(event, passiveListener) {
    pd(event).passiveListener = passiveListener;
  }
  var listenersMap = /* @__PURE__ */ new WeakMap(), CAPTURE = 1, BUBBLE = 2, ATTRIBUTE = 3;
  function isObject(x) {
    return x !== null && typeof x == "object";
  }
  function getListeners(eventTarget) {
    let listeners = listenersMap.get(eventTarget);
    if (!listeners)
      throw new TypeError(
        "'this' is expected an EventTarget object, but got another value."
      );
    return listeners;
  }
  var EventTarget = class {
    constructor() {
      this.__init();
    }
    __init() {
      listenersMap.set(this, /* @__PURE__ */ new Map());
    }
    /**
     * Add a given listener to this event target.
     * @param {string} eventName The event name to add.
     * @param {Function} listener The listener to add.
     * @param {boolean|{capture?:boolean,passive?:boolean,once?:boolean}} [options] The options for this listener.
     * @returns {void}
     */
    addEventListener(eventName, listener, options) {
      if (!listener)
        return;
      if (typeof listener != "function" && !isObject(listener))
        throw new TypeError("'listener' should be a function or an object.");
      let listeners = getListeners(this ?? globalThis), optionsIsObj = isObject(options), listenerType = (optionsIsObj ? !!options.capture : !!options) ? CAPTURE : BUBBLE, newNode = {
        listener,
        listenerType,
        passive: optionsIsObj && !!options.passive,
        once: optionsIsObj && !!options.once,
        next: null
      }, node = listeners.get(eventName);
      if (node === void 0) {
        listeners.set(eventName, newNode);
        return;
      }
      let prev = null;
      for (; node; ) {
        if (node.listener === listener && node.listenerType === listenerType)
          return;
        prev = node, node = node.next;
      }
      prev.next = newNode;
    }
    /**
     * Remove a given listener from this event target.
     * @param {string} eventName The event name to remove.
     * @param {Function} listener The listener to remove.
     * @param {boolean|{capture?:boolean,passive?:boolean,once?:boolean}} [options] The options for this listener.
     * @returns {void}
     */
    removeEventListener(eventName, listener, options) {
      if (!listener)
        return;
      let listeners = getListeners(this ?? globalThis), listenerType = (isObject(options) ? !!options.capture : !!options) ? CAPTURE : BUBBLE, prev = null, node = listeners.get(eventName);
      for (; node; ) {
        if (node.listener === listener && node.listenerType === listenerType) {
          prev !== null ? prev.next = node.next : node.next !== null ? listeners.set(eventName, node.next) : listeners.delete(eventName);
          return;
        }
        prev = node, node = node.next;
      }
    }
    /**
     * Dispatch a given event.
     * @param {Event|{type:string}} event The event to dispatch.
     * @returns {boolean} `false` if canceled.
     */
    dispatchEvent(event) {
      if (typeof event != "object")
        throw new TypeError("Argument 1 of EventTarget.dispatchEvent is not an object.");
      if (!(event instanceof Event))
        throw new TypeError("Argument 1 of EventTarget.dispatchEvent does not implement interface Event.");
      let self = this ?? globalThis;
      setCurrentTarget(event, self);
      let listeners = getListeners(self), eventName = event.type, node = listeners.get(eventName);
      if (!node)
        return true;
      let prev = null;
      for (; node && (node.once ? prev !== null ? prev.next = node.next : node.next !== null ? listeners.set(eventName, node.next) : listeners.delete(eventName) : prev = node, setPassiveListener(event, node.passive ? node.listener : null), typeof node.listener == "function" ? node.listener.call(self, event) : node.listenerType !== ATTRIBUTE && typeof node.listener.handleEvent == "function" && node.listener.handleEvent(event), !isStopped(event)); )
        node = node.next;
      return setPassiveListener(event, null), setEventPhase(event, 0), !event.defaultPrevented;
    }
  };

  // quickjs/banner.ts
  createSymbolPolyfills();
  globalThis.Event = Event;
  globalThis.EventTarget = EventTarget;
  globalThis.CustomEvent = CustomEvent;
})();

var __create = Object.create;
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __getOwnPropNames = Object.getOwnPropertyNames;
var __getProtoOf = Object.getPrototypeOf;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __commonJS = (cb, mod) => function __require() {
  return mod || (0, cb[__getOwnPropNames(cb)[0]])((mod = { exports: {} }).exports, mod), mod.exports;
};
var __copyProps = (to, from, except, desc) => {
  if (from && typeof from === "object" || typeof from === "function") {
    for (let key of __getOwnPropNames(from))
      if (!__hasOwnProp.call(to, key) && key !== except)
        __defProp(to, key, { get: () => from[key], enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable });
  }
  return to;
};
var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(
  // If the importer is in node compatibility mode or this is not an ESM
  // file that has been converted to a CommonJS file using a Babel-
  // compatible transform (i.e. "__esModule" has not been set), then set
  // "default" to the CommonJS "module.exports" for node compatibility.
  __defProp(target, "default", { value: mod, enumerable: true }) ,
  mod
));

// ../../../node_modules/event-iterator/lib/event-iterator.js
var require_event_iterator = __commonJS({
  "../../../node_modules/event-iterator/lib/event-iterator.js"(exports$1) {
    Object.defineProperty(exports$1, "__esModule", { value: true });
    var EventQueue = class {
      constructor() {
        this.pullQueue = [];
        this.pushQueue = [];
        this.eventHandlers = {};
        this.isPaused = false;
        this.isStopped = false;
      }
      push(value) {
        if (this.isStopped)
          return;
        const resolution = { value, done: false };
        if (this.pullQueue.length) {
          const placeholder = this.pullQueue.shift();
          if (placeholder)
            placeholder.resolve(resolution);
        } else {
          this.pushQueue.push(Promise.resolve(resolution));
          if (this.highWaterMark !== void 0 && this.pushQueue.length >= this.highWaterMark && !this.isPaused) {
            this.isPaused = true;
            if (this.eventHandlers.highWater) {
              this.eventHandlers.highWater();
            } else if (console) {
              console.warn(`EventIterator queue reached ${this.pushQueue.length} items`);
            }
          }
        }
      }
      stop() {
        if (this.isStopped)
          return;
        this.isStopped = true;
        this.remove();
        for (const placeholder of this.pullQueue) {
          placeholder.resolve({ value: void 0, done: true });
        }
        this.pullQueue.length = 0;
      }
      fail(error) {
        if (this.isStopped)
          return;
        this.isStopped = true;
        this.remove();
        if (this.pullQueue.length) {
          for (const placeholder of this.pullQueue) {
            placeholder.reject(error);
          }
          this.pullQueue.length = 0;
        } else {
          const rejection = Promise.reject(error);
          rejection.catch(() => {
          });
          this.pushQueue.push(rejection);
        }
      }
      remove() {
        Promise.resolve().then(() => {
          if (this.removeCallback)
            this.removeCallback();
        });
      }
      [Symbol.asyncIterator]() {
        return {
          next: (value) => {
            const result = this.pushQueue.shift();
            if (result) {
              if (this.lowWaterMark !== void 0 && this.pushQueue.length <= this.lowWaterMark && this.isPaused) {
                this.isPaused = false;
                if (this.eventHandlers.lowWater) {
                  this.eventHandlers.lowWater();
                }
              }
              return result;
            } else if (this.isStopped) {
              return Promise.resolve({ value: void 0, done: true });
            } else {
              return new Promise((resolve, reject) => {
                this.pullQueue.push({ resolve, reject });
              });
            }
          },
          return: () => {
            this.isStopped = true;
            this.pushQueue.length = 0;
            this.remove();
            return Promise.resolve({ value: void 0, done: true });
          }
        };
      }
    };
    var EventIterator4 = class {
      constructor(listen, { highWaterMark = 100, lowWaterMark = 1 } = {}) {
        const queue = new EventQueue();
        queue.highWaterMark = highWaterMark;
        queue.lowWaterMark = lowWaterMark;
        queue.removeCallback = listen({
          push: (value) => queue.push(value),
          stop: () => queue.stop(),
          fail: (error) => queue.fail(error),
          on: (event, fn) => {
            queue.eventHandlers[event] = fn;
          }
        }) || (() => {
        });
        this[Symbol.asyncIterator] = () => queue[Symbol.asyncIterator]();
        Object.freeze(this);
      }
    };
    exports$1.EventIterator = EventIterator4;
    exports$1.default = EventIterator4;
  }
});

// ../../../node_modules/event-iterator/lib/dom.js
var require_dom = __commonJS({
  "../../../node_modules/event-iterator/lib/dom.js"(exports$1) {
    Object.defineProperty(exports$1, "__esModule", { value: true });
    var event_iterator_1 = require_event_iterator();
    exports$1.EventIterator = event_iterator_1.EventIterator;
    function subscribe(event, options, evOptions) {
      return new event_iterator_1.EventIterator(({ push }) => {
        this.addEventListener(event, push, options);
        return () => this.removeEventListener(event, push, options);
      }, evOptions);
    }
    exports$1.subscribe = subscribe;
    exports$1.default = event_iterator_1.EventIterator;
  }
});

// ../../../node_modules/starpc/dist/srpc/errors.js
var ERR_RPC_ABORT = "ERR_RPC_ABORT";

// ../../../node_modules/p-defer/index.js
function pDefer() {
  const deferred = {};
  deferred.promise = new Promise((resolve, reject) => {
    deferred.resolve = resolve;
    deferred.reject = reject;
  });
  return deferred;
}

// ../../../node_modules/it-pushable/dist/src/fifo.js
var FixedFIFO = class {
  buffer;
  mask;
  top;
  btm;
  next;
  constructor(hwm) {
    if (!(hwm > 0) || (hwm - 1 & hwm) !== 0) {
      throw new Error("Max size for a FixedFIFO should be a power of two");
    }
    this.buffer = new Array(hwm);
    this.mask = hwm - 1;
    this.top = 0;
    this.btm = 0;
    this.next = null;
  }
  push(data) {
    if (this.buffer[this.top] !== void 0) {
      return false;
    }
    this.buffer[this.top] = data;
    this.top = this.top + 1 & this.mask;
    return true;
  }
  shift() {
    const last = this.buffer[this.btm];
    if (last === void 0) {
      return void 0;
    }
    this.buffer[this.btm] = void 0;
    this.btm = this.btm + 1 & this.mask;
    return last;
  }
  isEmpty() {
    return this.buffer[this.btm] === void 0;
  }
};
var FIFO = class {
  size;
  hwm;
  head;
  tail;
  constructor(options = {}) {
    this.hwm = options.splitLimit ?? 16;
    this.head = new FixedFIFO(this.hwm);
    this.tail = this.head;
    this.size = 0;
  }
  calculateSize(obj) {
    if (obj?.byteLength != null) {
      return obj.byteLength;
    }
    return 1;
  }
  push(val) {
    if (val?.value != null) {
      this.size += this.calculateSize(val.value);
    }
    if (!this.head.push(val)) {
      const prev = this.head;
      this.head = prev.next = new FixedFIFO(2 * this.head.buffer.length);
      this.head.push(val);
    }
  }
  shift() {
    let val = this.tail.shift();
    if (val === void 0 && this.tail.next != null) {
      const next = this.tail.next;
      this.tail.next = null;
      this.tail = next;
      val = this.tail.shift();
    }
    if (val?.value != null) {
      this.size -= this.calculateSize(val.value);
    }
    return val;
  }
  isEmpty() {
    return this.head.isEmpty();
  }
};

// ../../../node_modules/it-pushable/dist/src/index.js
var AbortError = class extends Error {
  type;
  code;
  constructor(message, code) {
    super(message ?? "The operation was aborted");
    this.type = "aborted";
    this.code = code ?? "ABORT_ERR";
  }
};
function pushable(options = {}) {
  const getNext = (buffer) => {
    const next = buffer.shift();
    if (next == null) {
      return { done: true };
    }
    if (next.error != null) {
      throw next.error;
    }
    return {
      done: next.done === true,
      // @ts-expect-error if done is false, value will be present
      value: next.value
    };
  };
  return _pushable(getNext, options);
}
function _pushable(getNext, options) {
  options = options ?? {};
  let onEnd = options.onEnd;
  let buffer = new FIFO();
  let pushable2;
  let onNext;
  let ended;
  let drain = pDefer();
  const waitNext = async () => {
    try {
      if (!buffer.isEmpty()) {
        return getNext(buffer);
      }
      if (ended) {
        return { done: true };
      }
      return await new Promise((resolve, reject) => {
        onNext = (next) => {
          onNext = null;
          buffer.push(next);
          try {
            resolve(getNext(buffer));
          } catch (err) {
            reject(err);
          }
          return pushable2;
        };
      });
    } finally {
      if (buffer.isEmpty()) {
        queueMicrotask(() => {
          drain.resolve();
          drain = pDefer();
        });
      }
    }
  };
  const bufferNext = (next) => {
    if (onNext != null) {
      return onNext(next);
    }
    buffer.push(next);
    return pushable2;
  };
  const bufferError = (err) => {
    buffer = new FIFO();
    if (onNext != null) {
      return onNext({ error: err });
    }
    buffer.push({ error: err });
    return pushable2;
  };
  const push = (value) => {
    if (ended) {
      return pushable2;
    }
    if (options?.objectMode !== true && value?.byteLength == null) {
      throw new Error("objectMode was not true but tried to push non-Uint8Array value");
    }
    return bufferNext({ done: false, value });
  };
  const end = (err) => {
    if (ended)
      return pushable2;
    ended = true;
    return err != null ? bufferError(err) : bufferNext({ done: true });
  };
  const _return = () => {
    buffer = new FIFO();
    end();
    return { done: true };
  };
  const _throw = (err) => {
    end(err);
    return { done: true };
  };
  pushable2 = {
    [Symbol.asyncIterator]() {
      return this;
    },
    next: waitNext,
    return: _return,
    throw: _throw,
    push,
    end,
    get readableLength() {
      return buffer.size;
    },
    onEmpty: async (options2) => {
      const signal = options2?.signal;
      signal?.throwIfAborted();
      if (buffer.isEmpty()) {
        return;
      }
      let cancel;
      let listener;
      if (signal != null) {
        cancel = new Promise((resolve, reject) => {
          listener = () => {
            reject(new AbortError());
          };
          signal.addEventListener("abort", listener);
        });
      }
      try {
        await Promise.race([
          drain.promise,
          cancel
        ]);
      } finally {
        if (listener != null && signal != null) {
          signal?.removeEventListener("abort", listener);
        }
      }
    }
  };
  if (onEnd == null) {
    return pushable2;
  }
  const _pushable2 = pushable2;
  pushable2 = {
    [Symbol.asyncIterator]() {
      return this;
    },
    next() {
      return _pushable2.next();
    },
    throw(err) {
      _pushable2.throw(err);
      if (onEnd != null) {
        onEnd(err);
        onEnd = void 0;
      }
      return { done: true };
    },
    return() {
      _pushable2.return();
      if (onEnd != null) {
        onEnd();
        onEnd = void 0;
      }
      return { done: true };
    },
    push,
    end(err) {
      _pushable2.end(err);
      if (onEnd != null) {
        onEnd(err);
        onEnd = void 0;
      }
      return pushable2;
    },
    get readableLength() {
      return _pushable2.readableLength;
    },
    onEmpty: (opts) => {
      return _pushable2.onEmpty(opts);
    }
  };
  return pushable2;
}

// ../../../node_modules/it-queueless-pushable/node_modules/race-signal/dist/src/index.js
function defaultTranslate(signal) {
  return signal.reason;
}
async function raceSignal(promise, signal, opts) {
  if (signal == null) {
    return promise;
  }
  const translateError = opts?.translateError ?? defaultTranslate;
  if (signal.aborted) {
    promise.catch(() => {
    });
    return Promise.reject(translateError(signal));
  }
  let listener;
  try {
    return await Promise.race([
      promise,
      new Promise((resolve, reject) => {
        listener = () => {
          reject(translateError(signal));
        };
        signal.addEventListener("abort", listener);
      })
    ]);
  } finally {
    if (listener != null) {
      signal.removeEventListener("abort", listener);
    }
  }
}

// ../../../node_modules/it-queueless-pushable/dist/src/index.js
var QueuelessPushable = class {
  readNext;
  haveNext;
  ended;
  nextResult;
  error;
  constructor() {
    this.ended = false;
    this.readNext = pDefer();
    this.haveNext = pDefer();
  }
  [Symbol.asyncIterator]() {
    return this;
  }
  async next() {
    if (this.nextResult == null) {
      await this.haveNext.promise;
    }
    if (this.nextResult == null) {
      throw new Error("HaveNext promise resolved but nextResult was undefined");
    }
    const nextResult = this.nextResult;
    this.nextResult = void 0;
    this.readNext.resolve();
    this.readNext = pDefer();
    return nextResult;
  }
  async throw(err) {
    this.ended = true;
    this.error = err;
    if (err != null) {
      this.haveNext.promise.catch(() => {
      });
      this.haveNext.reject(err);
    }
    const result = {
      done: true,
      value: void 0
    };
    return result;
  }
  async return() {
    const result = {
      done: true,
      value: void 0
    };
    this.ended = true;
    this.nextResult = result;
    this.haveNext.resolve();
    return result;
  }
  async push(value, options) {
    await this._push(value, options);
  }
  async end(err, options) {
    if (err != null) {
      await this.throw(err);
    } else {
      await this._push(void 0, options);
    }
  }
  async _push(value, options) {
    if (value != null && this.ended) {
      throw this.error ?? new Error("Cannot push value onto an ended pushable");
    }
    while (this.nextResult != null) {
      await this.readNext.promise;
    }
    if (value != null) {
      this.nextResult = { done: false, value };
    } else {
      this.ended = true;
      this.nextResult = { done: true, value: void 0 };
    }
    this.haveNext.resolve();
    this.haveNext = pDefer();
    await raceSignal(this.readNext.promise, options?.signal, options);
  }
};
function queuelessPushable() {
  return new QueuelessPushable();
}

// ../../../node_modules/it-merge/dist/src/index.js
function isAsyncIterable(thing) {
  return thing[Symbol.asyncIterator] != null;
}
async function addAllToPushable(sources, output, signal) {
  try {
    await Promise.all(sources.map(async (source) => {
      for await (const item of source) {
        await output.push(item, {
          signal
        });
        signal.throwIfAborted();
      }
    }));
    await output.end(void 0, {
      signal
    });
  } catch (err) {
    await output.end(err, {
      signal
    }).catch(() => {
    });
  }
}
async function* mergeSources(sources) {
  const controller = new AbortController();
  const output = queuelessPushable();
  addAllToPushable(sources, output, controller.signal).catch(() => {
  });
  try {
    yield* output;
  } finally {
    controller.abort();
  }
}
function* mergeSyncSources(syncSources) {
  for (const source of syncSources) {
    yield* source;
  }
}
function merge(...sources) {
  const syncSources = [];
  for (const source of sources) {
    if (!isAsyncIterable(source)) {
      syncSources.push(source);
    }
  }
  if (syncSources.length === sources.length) {
    return mergeSyncSources(syncSources);
  }
  return mergeSources(sources);
}
var src_default = merge;

// ../../../node_modules/it-pipe/dist/src/index.js
function pipe(first, ...rest) {
  if (first == null) {
    throw new Error("Empty pipeline");
  }
  if (isDuplex(first)) {
    const duplex = first;
    first = () => duplex.source;
  } else if (isIterable(first) || isAsyncIterable2(first)) {
    const source = first;
    first = () => source;
  }
  const fns = [first, ...rest];
  if (fns.length > 1) {
    if (isDuplex(fns[fns.length - 1])) {
      fns[fns.length - 1] = fns[fns.length - 1].sink;
    }
  }
  if (fns.length > 2) {
    for (let i = 1; i < fns.length - 1; i++) {
      if (isDuplex(fns[i])) {
        fns[i] = duplexPipelineFn(fns[i]);
      }
    }
  }
  return rawPipe(...fns);
}
var rawPipe = (...fns) => {
  let res;
  while (fns.length > 0) {
    res = fns.shift()(res);
  }
  return res;
};
var isAsyncIterable2 = (obj) => {
  return obj?.[Symbol.asyncIterator] != null;
};
var isIterable = (obj) => {
  return obj?.[Symbol.iterator] != null;
};
var isDuplex = (obj) => {
  if (obj == null) {
    return false;
  }
  return obj.sink != null && obj.source != null;
};
var duplexPipelineFn = (duplex) => {
  return (source) => {
    const p = duplex.sink(source);
    if (p?.then != null) {
      const stream = pushable({
        objectMode: true
      });
      p.then(() => {
        stream.end();
      }, (err) => {
        stream.end(err);
      });
      let sourceWrap;
      const source2 = duplex.source;
      if (isAsyncIterable2(source2)) {
        sourceWrap = async function* () {
          yield* source2;
          stream.end();
        };
      } else if (isIterable(source2)) {
        sourceWrap = function* () {
          yield* source2;
          stream.end();
        };
      } else {
        throw new Error("Unknown duplex source type - must be Iterable or AsyncIterable");
      }
      return src_default(stream, sourceWrap());
    }
    return duplex.source;
  };
};

// ../../../node_modules/@aptre/protobuf-es-lite/dist/assert.js
function assert(condition, msg) {
  if (!condition) {
    throw new Error(msg);
  }
}
var FLOAT32_MAX = 34028234663852886e22;
var FLOAT32_MIN = -34028234663852886e22;
var UINT32_MAX = 4294967295;
var INT32_MAX = 2147483647;
var INT32_MIN = -2147483648;
function assertInt32(arg) {
  if (typeof arg !== "number")
    throw new Error("invalid int 32: " + typeof arg);
  if (!Number.isInteger(arg) || arg > INT32_MAX || arg < INT32_MIN)
    throw new Error("invalid int 32: " + arg);
}
function assertUInt32(arg) {
  if (typeof arg !== "number")
    throw new Error("invalid uint 32: " + typeof arg);
  if (!Number.isInteger(arg) || arg > UINT32_MAX || arg < 0)
    throw new Error("invalid uint 32: " + arg);
}
function assertFloat32(arg) {
  if (typeof arg !== "number")
    throw new Error("invalid float 32: " + typeof arg);
  if (!Number.isFinite(arg))
    return;
  if (arg > FLOAT32_MAX || arg < FLOAT32_MIN)
    throw new Error("invalid float 32: " + arg);
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/varint.js
function varint64read() {
  let lowBits = 0;
  let highBits = 0;
  for (let shift = 0; shift < 28; shift += 7) {
    let b = this.buf[this.pos++];
    lowBits |= (b & 127) << shift;
    if ((b & 128) == 0) {
      this.assertBounds();
      return [lowBits, highBits];
    }
  }
  let middleByte = this.buf[this.pos++];
  lowBits |= (middleByte & 15) << 28;
  highBits = (middleByte & 112) >> 4;
  if ((middleByte & 128) == 0) {
    this.assertBounds();
    return [lowBits, highBits];
  }
  for (let shift = 3; shift <= 31; shift += 7) {
    let b = this.buf[this.pos++];
    highBits |= (b & 127) << shift;
    if ((b & 128) == 0) {
      this.assertBounds();
      return [lowBits, highBits];
    }
  }
  throw new Error("invalid varint");
}
function varint64write(lo, hi, bytes) {
  for (let i = 0; i < 28; i = i + 7) {
    const shift = lo >>> i;
    const hasNext = !(shift >>> 7 == 0 && hi == 0);
    const byte = (hasNext ? shift | 128 : shift) & 255;
    bytes.push(byte);
    if (!hasNext) {
      return;
    }
  }
  const splitBits = lo >>> 28 & 15 | (hi & 7) << 4;
  const hasMoreBits = !(hi >> 3 == 0);
  bytes.push((hasMoreBits ? splitBits | 128 : splitBits) & 255);
  if (!hasMoreBits) {
    return;
  }
  for (let i = 3; i < 31; i = i + 7) {
    const shift = hi >>> i;
    const hasNext = !(shift >>> 7 == 0);
    const byte = (hasNext ? shift | 128 : shift) & 255;
    bytes.push(byte);
    if (!hasNext) {
      return;
    }
  }
  bytes.push(hi >>> 31 & 1);
}
var TWO_PWR_32_DBL = 4294967296;
function int64FromString(dec) {
  const minus = dec[0] === "-";
  if (minus) {
    dec = dec.slice(1);
  }
  const base = 1e6;
  let lowBits = 0;
  let highBits = 0;
  function add1e6digit(begin, end) {
    const digit1e6 = Number(dec.slice(begin, end));
    highBits *= base;
    lowBits = lowBits * base + digit1e6;
    if (lowBits >= TWO_PWR_32_DBL) {
      highBits = highBits + (lowBits / TWO_PWR_32_DBL | 0);
      lowBits = lowBits % TWO_PWR_32_DBL;
    }
  }
  add1e6digit(-24, -18);
  add1e6digit(-18, -12);
  add1e6digit(-12, -6);
  add1e6digit(-6);
  return minus ? negate(lowBits, highBits) : newBits(lowBits, highBits);
}
function int64ToString(lo, hi) {
  let bits = newBits(lo, hi);
  const negative = bits.hi & 2147483648;
  if (negative) {
    bits = negate(bits.lo, bits.hi);
  }
  const result = uInt64ToString(bits.lo, bits.hi);
  return negative ? "-" + result : result;
}
function uInt64ToString(lo, hi) {
  ({ lo, hi } = toUnsigned(lo, hi));
  if (hi <= 2097151) {
    return String(TWO_PWR_32_DBL * hi + lo);
  }
  const low = lo & 16777215;
  const mid = (lo >>> 24 | hi << 8) & 16777215;
  const high = hi >> 16 & 65535;
  let digitA = low + mid * 6777216 + high * 6710656;
  let digitB = mid + high * 8147497;
  let digitC = high * 2;
  const base = 1e7;
  if (digitA >= base) {
    digitB += Math.floor(digitA / base);
    digitA %= base;
  }
  if (digitB >= base) {
    digitC += Math.floor(digitB / base);
    digitB %= base;
  }
  return digitC.toString() + decimalFrom1e7WithLeadingZeros(digitB) + decimalFrom1e7WithLeadingZeros(digitA);
}
function toUnsigned(lo, hi) {
  return { lo: lo >>> 0, hi: hi >>> 0 };
}
function newBits(lo, hi) {
  return { lo: lo | 0, hi: hi | 0 };
}
function negate(lowBits, highBits) {
  highBits = ~highBits;
  if (lowBits) {
    lowBits = ~lowBits + 1;
  } else {
    highBits += 1;
  }
  return newBits(lowBits, highBits);
}
var decimalFrom1e7WithLeadingZeros = (digit1e7) => {
  const partial = String(digit1e7);
  return "0000000".slice(partial.length) + partial;
};
function varint32write(value, bytes) {
  if (value >= 0) {
    while (value > 127) {
      bytes.push(value & 127 | 128);
      value = value >>> 7;
    }
    bytes.push(value);
  } else {
    for (let i = 0; i < 9; i++) {
      bytes.push(value & 127 | 128);
      value = value >> 7;
    }
    bytes.push(1);
  }
}
function varint32read() {
  let b = this.buf[this.pos++];
  let result = b & 127;
  if ((b & 128) == 0) {
    this.assertBounds();
    return result;
  }
  b = this.buf[this.pos++];
  result |= (b & 127) << 7;
  if ((b & 128) == 0) {
    this.assertBounds();
    return result;
  }
  b = this.buf[this.pos++];
  result |= (b & 127) << 14;
  if ((b & 128) == 0) {
    this.assertBounds();
    return result;
  }
  b = this.buf[this.pos++];
  result |= (b & 127) << 21;
  if ((b & 128) == 0) {
    this.assertBounds();
    return result;
  }
  b = this.buf[this.pos++];
  result |= (b & 15) << 28;
  for (let readBytes = 5; (b & 128) !== 0 && readBytes < 10; readBytes++)
    b = this.buf[this.pos++];
  if ((b & 128) != 0)
    throw new Error("invalid varint");
  this.assertBounds();
  return result >>> 0;
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/proto-int64.js
function makeInt64Support() {
  const dv = new DataView(new ArrayBuffer(8));
  const ok = typeof BigInt === "function" && typeof dv.getBigInt64 === "function" && typeof dv.getBigUint64 === "function" && typeof dv.setBigInt64 === "function" && typeof dv.setBigUint64 === "function" && (typeof process != "object" || typeof process.env != "object" || process.env.BUF_BIGINT_DISABLE !== "1");
  if (ok) {
    const MIN = BigInt("-9223372036854775808"), MAX = BigInt("9223372036854775807"), UMIN = BigInt("0"), UMAX = BigInt("18446744073709551615");
    return {
      zero: BigInt(0),
      supported: true,
      parse(value) {
        const bi = typeof value == "bigint" ? value : BigInt(value);
        if (bi > MAX || bi < MIN) {
          throw new Error(`int64 invalid: ${value}`);
        }
        return bi;
      },
      uParse(value) {
        const bi = typeof value == "bigint" ? value : BigInt(value);
        if (bi > UMAX || bi < UMIN) {
          throw new Error(`uint64 invalid: ${value}`);
        }
        return bi;
      },
      enc(value) {
        dv.setBigInt64(0, this.parse(value), true);
        return {
          lo: dv.getInt32(0, true),
          hi: dv.getInt32(4, true)
        };
      },
      uEnc(value) {
        dv.setBigInt64(0, this.uParse(value), true);
        return {
          lo: dv.getInt32(0, true),
          hi: dv.getInt32(4, true)
        };
      },
      dec(lo, hi) {
        dv.setInt32(0, lo, true);
        dv.setInt32(4, hi, true);
        return dv.getBigInt64(0, true);
      },
      uDec(lo, hi) {
        dv.setInt32(0, lo, true);
        dv.setInt32(4, hi, true);
        return dv.getBigUint64(0, true);
      }
    };
  }
  const assertInt64String = (value) => assert(/^-?[0-9]+$/.test(value), `int64 invalid: ${value}`);
  const assertUInt64String = (value) => assert(/^[0-9]+$/.test(value), `uint64 invalid: ${value}`);
  return {
    zero: "0",
    supported: false,
    parse(value) {
      if (typeof value != "string") {
        value = value.toString();
      }
      assertInt64String(value);
      return value;
    },
    uParse(value) {
      if (typeof value != "string") {
        value = value.toString();
      }
      assertUInt64String(value);
      return value;
    },
    enc(value) {
      if (typeof value != "string") {
        value = value.toString();
      }
      assertInt64String(value);
      return int64FromString(value);
    },
    uEnc(value) {
      if (typeof value != "string") {
        value = value.toString();
      }
      assertUInt64String(value);
      return int64FromString(value);
    },
    dec(lo, hi) {
      return int64ToString(lo, hi);
    },
    uDec(lo, hi) {
      return uInt64ToString(lo, hi);
    }
  };
}
var protoInt64 = makeInt64Support();

// ../../../node_modules/@aptre/protobuf-es-lite/dist/scalar.js
var ScalarType;
(function(ScalarType2) {
  ScalarType2[ScalarType2["DOUBLE"] = 1] = "DOUBLE";
  ScalarType2[ScalarType2["FLOAT"] = 2] = "FLOAT";
  ScalarType2[ScalarType2["INT64"] = 3] = "INT64";
  ScalarType2[ScalarType2["UINT64"] = 4] = "UINT64";
  ScalarType2[ScalarType2["INT32"] = 5] = "INT32";
  ScalarType2[ScalarType2["FIXED64"] = 6] = "FIXED64";
  ScalarType2[ScalarType2["FIXED32"] = 7] = "FIXED32";
  ScalarType2[ScalarType2["BOOL"] = 8] = "BOOL";
  ScalarType2[ScalarType2["STRING"] = 9] = "STRING";
  ScalarType2[ScalarType2["BYTES"] = 12] = "BYTES";
  ScalarType2[ScalarType2["UINT32"] = 13] = "UINT32";
  ScalarType2[ScalarType2["SFIXED32"] = 15] = "SFIXED32";
  ScalarType2[ScalarType2["SFIXED64"] = 16] = "SFIXED64";
  ScalarType2[ScalarType2["SINT32"] = 17] = "SINT32";
  ScalarType2[ScalarType2["SINT64"] = 18] = "SINT64";
  ScalarType2[ScalarType2["DATE"] = 100] = "DATE";
})(ScalarType || (ScalarType = {}));
var LongType;
(function(LongType2) {
  LongType2[LongType2["BIGINT"] = 0] = "BIGINT";
  LongType2[LongType2["STRING"] = 1] = "STRING";
})(LongType || (LongType = {}));
function scalarEquals(type, a, b) {
  if (a === b) {
    return true;
  }
  if (a == null || b == null) {
    return a === b;
  }
  if (type == ScalarType.BYTES) {
    if (!(a instanceof Uint8Array) || !(b instanceof Uint8Array)) {
      return false;
    }
    if (a.length !== b.length) {
      return false;
    }
    for (let i = 0; i < a.length; i++) {
      if (a[i] !== b[i]) {
        return false;
      }
    }
    return true;
  }
  if (type == ScalarType.DATE) {
    const dateA = toDate(a, false);
    const dateB = toDate(b, false);
    if (dateA == null || dateB == null) {
      return dateA === dateB;
    }
    return dateA != null && dateB != null && +dateA === +dateB;
  }
  switch (type) {
    case ScalarType.UINT64:
    case ScalarType.FIXED64:
    case ScalarType.INT64:
    case ScalarType.SFIXED64:
    case ScalarType.SINT64:
      return a == b;
  }
  return false;
}
function scalarZeroValue(type, longType) {
  switch (type) {
    case ScalarType.BOOL:
      return false;
    case ScalarType.UINT64:
    case ScalarType.FIXED64:
    case ScalarType.INT64:
    case ScalarType.SFIXED64:
    case ScalarType.SINT64:
      return longType == 0 ? protoInt64.zero : "0";
    case ScalarType.DOUBLE:
    case ScalarType.FLOAT:
      return 0;
    case ScalarType.BYTES:
      return new Uint8Array(0);
    case ScalarType.STRING:
      return "";
    case ScalarType.DATE:
      return null;
    default:
      return 0;
  }
}
var dateZeroValue = +/* @__PURE__ */ new Date(0);
function isScalarZeroValue(type, value) {
  switch (type) {
    case ScalarType.DATE:
      return value == null || +value === dateZeroValue;
    case ScalarType.BOOL:
      return value === false;
    case ScalarType.STRING:
      return value === "";
    case ScalarType.BYTES:
      return value instanceof Uint8Array && !value.byteLength;
    default:
      return value == 0;
  }
}
function normalizeScalarValue(type, value, clone, longType = LongType.BIGINT) {
  if (value == null) {
    return scalarZeroValue(type, longType);
  }
  if (type === ScalarType.BYTES) {
    return toU8Arr(value, clone);
  }
  if (isScalarZeroValue(type, value)) {
    return scalarZeroValue(type, longType);
  }
  if (type === ScalarType.DATE) {
    return toDate(value, clone);
  }
  return value;
}
function toU8Arr(input, clone) {
  return !clone && input instanceof Uint8Array ? input : new Uint8Array(input);
}
function toDate(input, clone) {
  if (input instanceof Date) {
    return clone ? new Date(input.getTime()) : input;
  }
  if (typeof input === "string" || typeof input === "number") {
    const date = new Date(input);
    return isNaN(date.getTime()) ? null : date;
  }
  return null;
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/names.js
function localFieldName(protoName, inOneof) {
  const name = protoCamelCase(protoName);
  if (inOneof) {
    return name;
  }
  return safeObjectProperty(safeMessageProperty(name));
}
function localOneofName(protoName) {
  return localFieldName(protoName, false);
}
function protoCamelCase(snakeCase) {
  let capNext = false;
  const b = [];
  for (let i = 0; i < snakeCase.length; i++) {
    let c = snakeCase.charAt(i);
    switch (c) {
      case "_":
        capNext = true;
        break;
      case "0":
      case "1":
      case "2":
      case "3":
      case "4":
      case "5":
      case "6":
      case "7":
      case "8":
      case "9":
        b.push(c);
        capNext = false;
        break;
      default:
        if (capNext) {
          capNext = false;
          c = c.toUpperCase();
        }
        b.push(c);
        break;
    }
  }
  return b.join("");
}
var reservedObjectProperties = /* @__PURE__ */ new Set([
  // names reserved by JavaScript
  "constructor",
  "toString",
  "toJSON",
  "valueOf",
  "__proto__",
  "prototype"
]);
var reservedMessageProperties = /* @__PURE__ */ new Set(["__proto__"]);
var fallback = (name) => `${name}$`;
var safeMessageProperty = (name) => {
  if (reservedMessageProperties.has(name)) {
    return fallback(name);
  }
  return name;
};
var safeObjectProperty = (name) => {
  if (reservedObjectProperties.has(name)) {
    return fallback(name);
  }
  return name;
};
function checkSanitizeKey(key) {
  return typeof key === "string" && !!key.length && !reservedObjectProperties.has(key);
}
function throwSanitizeKey(key) {
  if (typeof key !== "string") {
    throw new Error("illegal non-string object key: " + typeof key);
  }
  if (!checkSanitizeKey(key)) {
    throw new Error("illegal object key: " + key);
  }
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/field.js
var FieldList = class {
  _fields;
  _normalizer;
  all;
  numbersAsc;
  jsonNames;
  numbers;
  members;
  constructor(fields, normalizer) {
    this._fields = fields;
    this._normalizer = normalizer;
  }
  /**
   * Find field information by field name or json_name.
   */
  findJsonName(jsonName) {
    if (!this.jsonNames) {
      const t = {};
      for (const f of this.list()) {
        t[f.jsonName] = t[f.name] = f;
      }
      this.jsonNames = t;
    }
    return this.jsonNames[jsonName];
  }
  /**
   * Find field information by proto field number.
   */
  find(fieldNo) {
    if (!this.numbers) {
      const t = {};
      for (const f of this.list()) {
        t[f.no] = f;
      }
      this.numbers = t;
    }
    return this.numbers[fieldNo];
  }
  /**
   * Return field information in the order they appear in the source.
   */
  list() {
    if (!this.all) {
      this.all = this._normalizer(this._fields);
    }
    return this.all;
  }
  /**
   * Return field information ordered by field number ascending.
   */
  byNumber() {
    if (!this.numbersAsc) {
      this.numbersAsc = this.list().concat().sort((a, b) => a.no - b.no);
    }
    return this.numbersAsc;
  }
  /**
   * In order of appearance in the source, list fields and
   * oneof groups.
   */
  byMember() {
    if (!this.members) {
      this.members = [];
      const a = this.members;
      let o;
      for (const f of this.list()) {
        if (f.oneof) {
          if (f.oneof !== o) {
            o = f.oneof;
            a.push(o);
          }
        } else {
          a.push(f);
        }
      }
    }
    return this.members;
  }
};
function newFieldList(fields, packedByDefault) {
  return new FieldList(fields, (source) => normalizeFieldInfos(source, packedByDefault));
}
function isFieldSet(field, target) {
  const localName2 = field.localName;
  if (!target) {
    return false;
  }
  if (field.repeated) {
    return !!target[localName2]?.length;
  }
  if (field.oneof) {
    return target[field.oneof.localName]?.case === localName2;
  }
  switch (field.kind) {
    case "enum":
    case "scalar":
      if (field.opt || field.req) {
        return target[localName2] != null;
      }
      if (field.kind == "enum") {
        return target[localName2] !== field.T.values[0].no;
      }
      return !isScalarZeroValue(field.T, target[localName2]);
    case "message":
      return target[localName2] != null;
    case "map":
      return target[localName2] != null && !!Object.keys(target[localName2]).length;
  }
}
var fieldJsonName = protoCamelCase;
function resolveMessageType(t) {
  if (t instanceof Function) {
    return t();
  }
  return t;
}
var InternalOneofInfo = class {
  kind = "oneof";
  name;
  localName;
  repeated = false;
  packed = false;
  opt = false;
  req = false;
  default = void 0;
  fields = [];
  _lookup;
  constructor(name) {
    this.name = name;
    this.localName = localOneofName(name);
  }
  addField(field) {
    assert(field.oneof === this, `field ${field.name} not one of ${this.name}`);
    this.fields.push(field);
  }
  findField(localName2) {
    if (!this._lookup) {
      this._lookup = /* @__PURE__ */ Object.create(null);
      for (let i = 0; i < this.fields.length; i++) {
        this._lookup[this.fields[i].localName] = this.fields[i];
      }
    }
    return this._lookup[localName2];
  }
};
function normalizeFieldInfos(fieldInfos, packedByDefault) {
  const r = [];
  let o;
  for (const field of typeof fieldInfos == "function" ? fieldInfos() : fieldInfos) {
    const f = field;
    f.localName = localFieldName(field.name, field.oneof !== void 0);
    f.jsonName = field.jsonName ?? fieldJsonName(field.name);
    f.repeated = field.repeated ?? false;
    if (field.kind == "scalar") {
      f.L = field.L ?? LongType.BIGINT;
    }
    f.delimited = field.delimited ?? false;
    f.req = field.req ?? false;
    f.opt = field.opt ?? false;
    if (field.packed === void 0) {
      if (packedByDefault) {
        f.packed = field.kind == "enum" || field.kind == "scalar" && field.T != ScalarType.BYTES && field.T != ScalarType.STRING;
      } else {
        f.packed = false;
      }
    }
    if (field.oneof !== void 0) {
      const ooname = typeof field.oneof == "string" ? field.oneof : field.oneof.name;
      if (!o || o.name != ooname) {
        o = new InternalOneofInfo(ooname);
      }
      f.oneof = o;
      o.addField(f);
    }
    r.push(f);
  }
  return r;
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/enum.js
function createEnumType(typeName, values) {
  const names = /* @__PURE__ */ Object.create(null);
  const numbers = /* @__PURE__ */ Object.create(null);
  const normalValues = [];
  for (const value of values) {
    const n = "localName" in value ? value : { ...value, localName: value.name };
    normalValues.push(n);
    names[value.name] = n;
    numbers[value.no] = n;
  }
  return {
    typeName,
    values: normalValues,
    // We do not surface options at this time
    // options: opt?.options ?? Object.create(null),
    findName(name) {
      return names[name];
    },
    findNumber(no) {
      return numbers[no];
    }
  };
}
function enumZeroValue(info) {
  if (info.values.length < 1) {
    throw new Error("invalid enum: missing at least one value");
  }
  const zeroValue = info.values[0];
  return zeroValue.no;
}
function normalizeEnumValue(info, value) {
  const zeroValue = enumZeroValue(info);
  if (value == null) {
    return zeroValue;
  }
  if (value === "" || value === zeroValue) {
    return zeroValue;
  }
  if (typeof value === "string") {
    const val = info.findName(value);
    if (!val) {
      throw new Error(`enum ${info.typeName}: invalid value: "${value}"`);
    }
    return val.no;
  }
  return value;
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/partial.js
function applyPartialMessage(source, target, fields, clone = false) {
  if (source == null || target == null) {
    return;
  }
  const t = target, s = source;
  for (const member of fields.byMember()) {
    const localName2 = member.localName;
    throwSanitizeKey(localName2);
    if (!(localName2 in s) || s[localName2] === void 0) {
      continue;
    }
    const sourceValue = s[localName2];
    if (sourceValue === null) {
      delete t[localName2];
      continue;
    }
    switch (member.kind) {
      case "oneof": {
        if (typeof sourceValue !== "object") {
          throw new Error(`field ${localName2}: invalid oneof: must be an object with case and value`);
        }
        const { case: sk, value: sv } = sourceValue;
        const sourceField = sk != null ? member.findField(sk) : null;
        let dv = localName2 in t ? t[localName2] : void 0;
        if (typeof dv !== "object") {
          dv = /* @__PURE__ */ Object.create(null);
        }
        if (sk != null && sourceField == null) {
          throw new Error(`field ${localName2}: invalid oneof case: ${sk}`);
        }
        dv.case = sk;
        if (dv.case !== sk || sk == null) {
          delete dv.value;
        }
        t[localName2] = dv;
        if (!sourceField) {
          break;
        }
        if (sourceField.kind === "message") {
          let dest = dv.value;
          if (typeof dest !== "object") {
            dest = dv.value = /* @__PURE__ */ Object.create(null);
          }
          if (sv != null) {
            const sourceFieldMt = resolveMessageType(sourceField.T);
            applyPartialMessage(sv, dest, sourceFieldMt.fields);
          }
        } else if (sourceField.kind === "scalar") {
          dv.value = normalizeScalarValue(sourceField.T, sv, clone);
        } else {
          dv.value = sv;
        }
        break;
      }
      case "scalar": {
        if (member.repeated) {
          if (!Array.isArray(sourceValue)) {
            throw new Error(`field ${localName2}: invalid value: must be array`);
          }
          let dst = localName2 in t ? t[localName2] : null;
          if (dst == null || !Array.isArray(dst)) {
            dst = t[localName2] = [];
          }
          dst.push(...sourceValue.map((v) => normalizeScalarValue(member.T, v, clone)));
          break;
        }
        t[localName2] = normalizeScalarValue(member.T, sourceValue, clone);
        break;
      }
      case "enum": {
        t[localName2] = normalizeEnumValue(member.T, sourceValue);
        break;
      }
      case "map": {
        if (typeof sourceValue !== "object") {
          throw new Error(`field ${member.localName}: invalid value: must be object`);
        }
        let tMap = t[localName2];
        if (typeof tMap !== "object") {
          tMap = t[localName2] = /* @__PURE__ */ Object.create(null);
        }
        applyPartialMap(sourceValue, tMap, member.V, clone);
        break;
      }
      case "message": {
        const mt = resolveMessageType(member.T);
        if (member.repeated) {
          if (!Array.isArray(sourceValue)) {
            throw new Error(`field ${localName2}: invalid value: must be array`);
          }
          let tArr = t[localName2];
          if (!Array.isArray(tArr)) {
            tArr = t[localName2] = [];
          }
          for (const v of sourceValue) {
            if (v != null) {
              if (mt.fieldWrapper) {
                tArr.push(mt.fieldWrapper.unwrapField(mt.fieldWrapper.wrapField(v)));
              } else {
                tArr.push(mt.create(v));
              }
            }
          }
          break;
        }
        if (mt.fieldWrapper) {
          t[localName2] = mt.fieldWrapper.unwrapField(mt.fieldWrapper.wrapField(sourceValue));
        } else {
          if (typeof sourceValue !== "object") {
            throw new Error(`field ${member.localName}: invalid value: must be object`);
          }
          let destMsg = t[localName2];
          if (typeof destMsg !== "object") {
            destMsg = t[localName2] = /* @__PURE__ */ Object.create(null);
          }
          applyPartialMessage(sourceValue, destMsg, mt.fields);
        }
        break;
      }
    }
  }
}
function applyPartialMap(sourceMap, targetMap, value, clone) {
  if (sourceMap == null) {
    return;
  }
  if (typeof sourceMap !== "object") {
    throw new Error(`invalid map: must be object`);
  }
  switch (value.kind) {
    case "scalar":
      for (const [k, v] of Object.entries(sourceMap)) {
        throwSanitizeKey(k);
        if (v !== void 0) {
          targetMap[k] = normalizeScalarValue(value.T, v, clone);
        } else {
          delete targetMap[k];
        }
      }
      break;
    case "enum":
      for (const [k, v] of Object.entries(sourceMap)) {
        throwSanitizeKey(k);
        if (v !== void 0) {
          targetMap[k] = normalizeEnumValue(value.T, v);
        } else {
          delete targetMap[k];
        }
      }
      break;
    case "message": {
      const messageType = resolveMessageType(value.T);
      for (const [k, v] of Object.entries(sourceMap)) {
        throwSanitizeKey(k);
        if (v === void 0) {
          delete targetMap[k];
          continue;
        }
        if (typeof v !== "object") {
          throw new Error(`invalid value: must be object`);
        }
        let val = targetMap[k];
        if (messageType.fieldWrapper) {
          val = targetMap[k] = createCompleteMessage(messageType.fields);
        } else if (typeof val !== "object") {
          val = targetMap[k] = /* @__PURE__ */ Object.create(null);
        }
        applyPartialMessage(v, val, messageType.fields);
      }
      break;
    }
  }
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/unknown.js
var unknownFieldsSymbol = /* @__PURE__ */ Symbol("@aptre/protobuf-es-lite/unknown-fields");
function handleUnknownField(message, no, wireType, data) {
  if (typeof message !== "object") {
    return;
  }
  const m = message;
  if (!Array.isArray(m[unknownFieldsSymbol])) {
    m[unknownFieldsSymbol] = [];
  }
  m[unknownFieldsSymbol].push({ no, wireType, data });
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/field-wrapper.js
function wrapField(fieldWrapper, value) {
  if (!fieldWrapper) {
    return value;
  }
  return fieldWrapper.wrapField(value);
}
function unwrapField(fieldWrapper, value) {
  return fieldWrapper ? fieldWrapper.unwrapField(value) : value;
}
({
  "google.protobuf.Timestamp": ScalarType.DATE,
  "google.protobuf.DoubleValue": ScalarType.DOUBLE,
  "google.protobuf.FloatValue": ScalarType.FLOAT,
  "google.protobuf.Int64Value": ScalarType.INT64,
  "google.protobuf.UInt64Value": ScalarType.UINT64,
  "google.protobuf.Int32Value": ScalarType.INT32,
  "google.protobuf.UInt32Value": ScalarType.UINT32,
  "google.protobuf.BoolValue": ScalarType.BOOL,
  "google.protobuf.StringValue": ScalarType.STRING,
  "google.protobuf.BytesValue": ScalarType.BYTES
});

// ../../../node_modules/@aptre/protobuf-es-lite/dist/binary-encoding.js
var WireType;
(function(WireType2) {
  WireType2[WireType2["Varint"] = 0] = "Varint";
  WireType2[WireType2["Bit64"] = 1] = "Bit64";
  WireType2[WireType2["LengthDelimited"] = 2] = "LengthDelimited";
  WireType2[WireType2["StartGroup"] = 3] = "StartGroup";
  WireType2[WireType2["EndGroup"] = 4] = "EndGroup";
  WireType2[WireType2["Bit32"] = 5] = "Bit32";
})(WireType || (WireType = {}));
var BinaryWriter = class {
  /**
   * We cannot allocate a buffer for the entire output
   * because we don't know it's size.
   *
   * So we collect smaller chunks of known size and
   * concat them later.
   *
   * Use `raw()` to push data to this array. It will flush
   * `buf` first.
   */
  chunks;
  /**
   * A growing buffer for byte values. If you don't know
   * the size of the data you are writing, push to this
   * array.
   */
  buf;
  /**
   * Previous fork states.
   */
  stack = [];
  /**
   * Text encoder instance to convert UTF-8 to bytes.
   */
  textEncoder;
  constructor(textEncoder) {
    this.textEncoder = textEncoder ?? new TextEncoder();
    this.chunks = [];
    this.buf = [];
  }
  /**
   * Return all bytes written and reset this writer.
   */
  finish() {
    this.chunks.push(new Uint8Array(this.buf));
    let len = 0;
    for (let i = 0; i < this.chunks.length; i++)
      len += this.chunks[i].length;
    let bytes = new Uint8Array(len);
    let offset = 0;
    for (let i = 0; i < this.chunks.length; i++) {
      bytes.set(this.chunks[i], offset);
      offset += this.chunks[i].length;
    }
    this.chunks = [];
    return bytes;
  }
  /**
   * Start a new fork for length-delimited data like a message
   * or a packed repeated field.
   *
   * Must be joined later with `join()`.
   */
  fork() {
    this.stack.push({ chunks: this.chunks, buf: this.buf });
    this.chunks = [];
    this.buf = [];
    return this;
  }
  /**
   * Join the last fork. Write its length and bytes, then
   * return to the previous state.
   */
  join() {
    let chunk = this.finish();
    let prev = this.stack.pop();
    if (!prev)
      throw new Error("invalid state, fork stack empty");
    this.chunks = prev.chunks;
    this.buf = prev.buf;
    this.uint32(chunk.byteLength);
    return this.raw(chunk);
  }
  /**
   * Writes a tag (field number and wire type).
   *
   * Equivalent to `uint32( (fieldNo << 3 | type) >>> 0 )`.
   *
   * Generated code should compute the tag ahead of time and call `uint32()`.
   */
  tag(fieldNo, type) {
    return this.uint32((fieldNo << 3 | type) >>> 0);
  }
  /**
   * Write a chunk of raw bytes.
   */
  raw(chunk) {
    if (this.buf.length) {
      this.chunks.push(new Uint8Array(this.buf));
      this.buf = [];
    }
    this.chunks.push(chunk);
    return this;
  }
  /**
   * Write a `uint32` value, an unsigned 32 bit varint.
   */
  uint32(value) {
    assertUInt32(value);
    while (value > 127) {
      this.buf.push(value & 127 | 128);
      value = value >>> 7;
    }
    this.buf.push(value);
    return this;
  }
  /**
   * Write a `int32` value, a signed 32 bit varint.
   */
  int32(value) {
    assertInt32(value);
    varint32write(value, this.buf);
    return this;
  }
  /**
   * Write a `bool` value, a variant.
   */
  bool(value) {
    this.buf.push(value ? 1 : 0);
    return this;
  }
  /**
   * Write a `bytes` value, length-delimited arbitrary data.
   */
  bytes(value) {
    this.uint32(value.byteLength);
    return this.raw(value);
  }
  /**
   * Write a `string` value, length-delimited data converted to UTF-8 text.
   */
  string(value) {
    let chunk = this.textEncoder.encode(value);
    this.uint32(chunk.byteLength);
    return this.raw(chunk);
  }
  /**
   * Write a `float` value, 32-bit floating point number.
   */
  float(value) {
    assertFloat32(value);
    let chunk = new Uint8Array(4);
    new DataView(chunk.buffer).setFloat32(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `double` value, a 64-bit floating point number.
   */
  double(value) {
    let chunk = new Uint8Array(8);
    new DataView(chunk.buffer).setFloat64(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `fixed32` value, an unsigned, fixed-length 32-bit integer.
   */
  fixed32(value) {
    assertUInt32(value);
    let chunk = new Uint8Array(4);
    new DataView(chunk.buffer).setUint32(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `sfixed32` value, a signed, fixed-length 32-bit integer.
   */
  sfixed32(value) {
    assertInt32(value);
    let chunk = new Uint8Array(4);
    new DataView(chunk.buffer).setInt32(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `sint32` value, a signed, zigzag-encoded 32-bit varint.
   */
  sint32(value) {
    assertInt32(value);
    value = (value << 1 ^ value >> 31) >>> 0;
    varint32write(value, this.buf);
    return this;
  }
  /**
   * Write a `fixed64` value, a signed, fixed-length 64-bit integer.
   */
  sfixed64(value) {
    let chunk = new Uint8Array(8), view = new DataView(chunk.buffer), tc = protoInt64.enc(value);
    view.setInt32(0, tc.lo, true);
    view.setInt32(4, tc.hi, true);
    return this.raw(chunk);
  }
  /**
   * Write a `fixed64` value, an unsigned, fixed-length 64 bit integer.
   */
  fixed64(value) {
    let chunk = new Uint8Array(8), view = new DataView(chunk.buffer), tc = protoInt64.uEnc(value);
    view.setInt32(0, tc.lo, true);
    view.setInt32(4, tc.hi, true);
    return this.raw(chunk);
  }
  /**
   * Write a `int64` value, a signed 64-bit varint.
   */
  int64(value) {
    let tc = protoInt64.enc(value);
    varint64write(tc.lo, tc.hi, this.buf);
    return this;
  }
  /**
   * Write a `sint64` value, a signed, zig-zag-encoded 64-bit varint.
   */
  sint64(value) {
    let tc = protoInt64.enc(value), sign = tc.hi >> 31, lo = tc.lo << 1 ^ sign, hi = (tc.hi << 1 | tc.lo >>> 31) ^ sign;
    varint64write(lo, hi, this.buf);
    return this;
  }
  /**
   * Write a `uint64` value, an unsigned 64-bit varint.
   */
  uint64(value) {
    let tc = protoInt64.uEnc(value);
    varint64write(tc.lo, tc.hi, this.buf);
    return this;
  }
};
var BinaryReader = class {
  /**
   * Current position.
   */
  pos;
  /**
   * Number of bytes available in this reader.
   */
  len;
  buf;
  view;
  textDecoder;
  constructor(buf, textDecoder) {
    this.buf = buf;
    this.len = buf.length;
    this.pos = 0;
    this.view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    this.textDecoder = textDecoder ?? new TextDecoder();
  }
  /**
   * Reads a tag - field number and wire type.
   */
  tag() {
    let tag = this.uint32(), fieldNo = tag >>> 3, wireType = tag & 7;
    if (fieldNo <= 0 || wireType < 0 || wireType > 5)
      throw new Error("illegal tag: field no " + fieldNo + " wire type " + wireType);
    return [fieldNo, wireType];
  }
  /**
   * Skip one element on the wire and return the skipped data.
   * Supports WireType.StartGroup since v2.0.0-alpha.23.
   */
  skip(wireType) {
    let start = this.pos;
    switch (wireType) {
      case WireType.Varint:
        while (this.buf[this.pos++] & 128) {
        }
        break;
      // eslint-disable-next-line
      // @ts-ignore TS7029: Fallthrough case in switch
      case WireType.Bit64:
        this.pos += 4;
      // eslint-disable-next-line
      // @ts-ignore TS7029: Fallthrough case in switch
      case WireType.Bit32:
        this.pos += 4;
        break;
      case WireType.LengthDelimited:
        let len = this.uint32();
        this.pos += len;
        break;
      case WireType.StartGroup:
        let t;
        while ((t = this.tag()[1]) !== WireType.EndGroup) {
          this.skip(t);
        }
        break;
      default:
        throw new Error("cant skip wire type " + wireType);
    }
    this.assertBounds();
    return this.buf.subarray(start, this.pos);
  }
  varint64 = varint64read;
  // dirty cast for `this`
  /**
   * Throws error if position in byte array is out of range.
   */
  assertBounds() {
    if (this.pos > this.len)
      throw new RangeError("premature EOF");
  }
  /**
   * Read a `uint32` field, an unsigned 32 bit varint.
   */
  uint32 = varint32read;
  // dirty cast for `this` and access to protected `buf`
  /**
   * Read a `int32` field, a signed 32 bit varint.
   */
  int32() {
    return this.uint32() | 0;
  }
  /**
   * Read a `sint32` field, a signed, zigzag-encoded 32-bit varint.
   */
  sint32() {
    let zze = this.uint32();
    return zze >>> 1 ^ -(zze & 1);
  }
  /**
   * Read a `int64` field, a signed 64-bit varint.
   */
  int64() {
    return protoInt64.dec(...this.varint64());
  }
  /**
   * Read a `uint64` field, an unsigned 64-bit varint.
   */
  uint64() {
    return protoInt64.uDec(...this.varint64());
  }
  /**
   * Read a `sint64` field, a signed, zig-zag-encoded 64-bit varint.
   */
  sint64() {
    let [lo, hi] = this.varint64();
    let s = -(lo & 1);
    lo = (lo >>> 1 | (hi & 1) << 31) ^ s;
    hi = hi >>> 1 ^ s;
    return protoInt64.dec(lo, hi);
  }
  /**
   * Read a `bool` field, a variant.
   */
  bool() {
    let [lo, hi] = this.varint64();
    return lo !== 0 || hi !== 0;
  }
  /**
   * Read a `fixed32` field, an unsigned, fixed-length 32-bit integer.
   */
  fixed32() {
    return this.view.getUint32((this.pos += 4) - 4, true);
  }
  /**
   * Read a `sfixed32` field, a signed, fixed-length 32-bit integer.
   */
  sfixed32() {
    return this.view.getInt32((this.pos += 4) - 4, true);
  }
  /**
   * Read a `fixed64` field, an unsigned, fixed-length 64 bit integer.
   */
  fixed64() {
    return protoInt64.uDec(this.sfixed32(), this.sfixed32());
  }
  /**
   * Read a `fixed64` field, a signed, fixed-length 64-bit integer.
   */
  sfixed64() {
    return protoInt64.dec(this.sfixed32(), this.sfixed32());
  }
  /**
   * Read a `float` field, 32-bit floating point number.
   */
  float() {
    return this.view.getFloat32((this.pos += 4) - 4, true);
  }
  /**
   * Read a `double` field, a 64-bit floating point number.
   */
  double() {
    return this.view.getFloat64((this.pos += 8) - 8, true);
  }
  /**
   * Read a `bytes` field, length-delimited arbitrary data.
   */
  bytes() {
    let len = this.uint32(), start = this.pos;
    this.pos += len;
    this.assertBounds();
    return this.buf.subarray(start, start + len);
  }
  /**
   * Read a `string` field, length-delimited data converted to UTF-8 text.
   */
  string() {
    return this.textDecoder.decode(this.bytes());
  }
};

// ../../../node_modules/@aptre/protobuf-es-lite/dist/binary.js
var readDefaults = {
  readUnknownFields: true,
  readerFactory: (bytes) => new BinaryReader(bytes)
};
var writeDefaults = {
  writeUnknownFields: true,
  writerFactory: () => new BinaryWriter()
};
function makeReadOptions(options) {
  return options ? { ...readDefaults, ...options } : readDefaults;
}
function makeWriteOptions(options) {
  return options ? { ...writeDefaults, ...options } : writeDefaults;
}
function readField(target, reader, field, wireType, options) {
  const { repeated } = field;
  let { localName: localName2 } = field;
  if (field.oneof) {
    let oneofMsg = target[field.oneof.localName];
    if (!oneofMsg) {
      oneofMsg = target[field.oneof.localName] = /* @__PURE__ */ Object.create(null);
    }
    target = oneofMsg;
    if (target.case != localName2) {
      delete target.value;
    }
    target.case = localName2;
    localName2 = "value";
  }
  switch (field.kind) {
    case "scalar":
    case "enum": {
      const scalarType = field.kind == "enum" ? ScalarType.INT32 : field.T;
      let read = readScalar;
      if (field.kind == "scalar" && field.L > 0) {
        read = readScalarLTString;
      }
      if (repeated) {
        let tgtArr = target[localName2];
        if (!Array.isArray(tgtArr)) {
          tgtArr = target[localName2] = [];
        }
        const isPacked = wireType == WireType.LengthDelimited && scalarType != ScalarType.STRING && scalarType != ScalarType.BYTES;
        if (isPacked) {
          const e = reader.uint32() + reader.pos;
          while (reader.pos < e) {
            tgtArr.push(read(reader, scalarType));
          }
        } else {
          tgtArr.push(read(reader, scalarType));
        }
      } else {
        target[localName2] = read(reader, scalarType);
      }
      break;
    }
    case "message": {
      const fieldT = field.T;
      const messageType = fieldT instanceof Function ? fieldT() : fieldT;
      if (repeated) {
        let tgtArr = target[localName2];
        if (!Array.isArray(tgtArr)) {
          tgtArr = target[localName2] = [];
        }
        tgtArr.push(unwrapField(messageType.fieldWrapper, readMessageField(reader, /* @__PURE__ */ Object.create(null), messageType.fields, options, field)));
      } else {
        target[localName2] = unwrapField(messageType.fieldWrapper, readMessageField(reader, /* @__PURE__ */ Object.create(null), messageType.fields, options, field));
      }
      break;
    }
    case "map": {
      const [mapKey, mapVal] = readMapEntry(field, reader, options);
      if (typeof target[localName2] !== "object") {
        target[localName2] = /* @__PURE__ */ Object.create(null);
      }
      target[localName2][mapKey] = mapVal;
      break;
    }
  }
}
function readMapEntry(field, reader, options) {
  const length = reader.uint32(), end = reader.pos + length;
  let key, val;
  while (reader.pos < end) {
    const [fieldNo] = reader.tag();
    switch (fieldNo) {
      case 1:
        key = readScalar(reader, field.K);
        break;
      case 2:
        switch (field.V.kind) {
          case "scalar":
            val = readScalar(reader, field.V.T);
            break;
          case "enum":
            val = reader.int32();
            break;
          case "message": {
            val = readMessageField(reader, /* @__PURE__ */ Object.create(null), resolveMessageType(field.V.T).fields, options, void 0);
            break;
          }
        }
        break;
    }
  }
  if (key === void 0) {
    key = scalarZeroValue(field.K, LongType.BIGINT);
  }
  if (typeof key !== "string" && typeof key !== "number") {
    key = key?.toString() ?? "";
  }
  if (val === void 0) {
    const fieldKind = field.V.kind;
    switch (fieldKind) {
      case "scalar":
        val = scalarZeroValue(field.V.T, LongType.BIGINT);
        break;
      case "enum":
        val = field.V.T.values[0].no;
        break;
      case "message":
        val = /* @__PURE__ */ Object.create(null);
        break;
    }
  }
  return [key, val];
}
function readScalar(reader, type) {
  switch (type) {
    case ScalarType.STRING:
      return reader.string();
    case ScalarType.BOOL:
      return reader.bool();
    case ScalarType.DOUBLE:
      return reader.double();
    case ScalarType.FLOAT:
      return reader.float();
    case ScalarType.INT32:
      return reader.int32();
    case ScalarType.INT64:
      return reader.int64();
    case ScalarType.UINT64:
      return reader.uint64();
    case ScalarType.FIXED64:
      return reader.fixed64();
    case ScalarType.BYTES:
      return reader.bytes();
    case ScalarType.FIXED32:
      return reader.fixed32();
    case ScalarType.SFIXED32:
      return reader.sfixed32();
    case ScalarType.SFIXED64:
      return reader.sfixed64();
    case ScalarType.SINT64:
      return reader.sint64();
    case ScalarType.UINT32:
      return reader.uint32();
    case ScalarType.SINT32:
      return reader.sint32();
    case ScalarType.DATE:
      throw new Error("cannot read a date with readScalar");
    default:
      throw new Error("unknown scalar type");
  }
}
function readScalarLTString(reader, type) {
  const v = readScalar(reader, type);
  return typeof v == "bigint" ? v.toString() : v;
}
function readMessageField(reader, message, fields, options, field) {
  readMessage(message, fields, reader, field?.delimited ? field.no : reader.uint32(), options, field?.delimited ?? false);
  return message;
}
function readMessage(message, fields, reader, lengthOrEndTagFieldNo, options, delimitedMessageEncoding) {
  const end = delimitedMessageEncoding ? reader.len : reader.pos + lengthOrEndTagFieldNo;
  let fieldNo, wireType;
  while (reader.pos < end) {
    [fieldNo, wireType] = reader.tag();
    if (wireType == WireType.EndGroup) {
      break;
    }
    const field = fields.find(fieldNo);
    if (!field) {
      const data = reader.skip(wireType);
      if (options.readUnknownFields) {
        handleUnknownField(message, fieldNo, wireType, data);
      }
      continue;
    }
    readField(message, reader, field, wireType, options);
  }
  if (delimitedMessageEncoding && (wireType != WireType.EndGroup || fieldNo !== lengthOrEndTagFieldNo)) {
    throw new Error(`invalid end group tag`);
  }
}
function writeMessage(message, fields, writer, options) {
  for (const field of fields.byNumber()) {
    if (!isFieldSet(field, message)) {
      if (field.req) {
        throw new Error(`cannot encode field ${field.name} to binary: required field not set`);
      }
      continue;
    }
    const value = field.oneof ? message[field.oneof.localName].value : message[field.localName];
    if (value !== void 0) {
      writeField(field, value, writer, options);
    }
  }
  if (options.writeUnknownFields) {
    writeUnknownFields(message, writer);
  }
}
function writeField(field, value, writer, options) {
  assert(value !== void 0);
  const repeated = field.repeated;
  switch (field.kind) {
    case "scalar":
    case "enum": {
      const scalarType = field.kind == "enum" ? ScalarType.INT32 : field.T;
      if (repeated) {
        assert(Array.isArray(value));
        if (field.packed) {
          writePacked(writer, scalarType, field.no, value);
        } else {
          for (const item of value) {
            writeScalar(writer, scalarType, field.no, item);
          }
        }
      } else {
        writeScalar(writer, scalarType, field.no, value);
      }
      break;
    }
    case "message":
      if (repeated) {
        assert(Array.isArray(value));
        for (const item of value) {
          writeMessageField(writer, options, field, item);
        }
      } else {
        writeMessageField(writer, options, field, value);
      }
      break;
    case "map":
      assert(typeof value == "object" && value != null);
      for (const [key, val] of Object.entries(value)) {
        writeMapEntry(writer, options, field, key, val);
      }
      break;
  }
}
function writeUnknownFields(message, writer) {
  const m = message;
  const c = m[unknownFieldsSymbol];
  if (c) {
    for (const f of c) {
      writer.tag(f.no, f.wireType).raw(f.data);
    }
  }
}
function writeMessageField(writer, options, field, value) {
  const messageType = resolveMessageType(field.T);
  const message = wrapField(messageType.fieldWrapper, value);
  if (field.delimited)
    writer.tag(field.no, WireType.StartGroup).raw(messageType.toBinary(message, options)).tag(field.no, WireType.EndGroup);
  else
    writer.tag(field.no, WireType.LengthDelimited).bytes(messageType.toBinary(message, options));
}
function writeScalar(writer, type, fieldNo, value) {
  assert(value !== void 0);
  const [wireType, method] = scalarTypeInfo(type);
  writer.tag(fieldNo, wireType)[method](value);
}
function writePacked(writer, type, fieldNo, value) {
  if (!value.length) {
    return;
  }
  writer.tag(fieldNo, WireType.LengthDelimited).fork();
  const [, method] = scalarTypeInfo(type);
  for (let i = 0; i < value.length; i++) {
    writer[method](value[i]);
  }
  writer.join();
}
function scalarTypeInfo(type) {
  let wireType = WireType.Varint;
  switch (type) {
    case ScalarType.BYTES:
    case ScalarType.STRING:
      wireType = WireType.LengthDelimited;
      break;
    case ScalarType.DOUBLE:
    case ScalarType.FIXED64:
    case ScalarType.SFIXED64:
      wireType = WireType.Bit64;
      break;
    case ScalarType.FIXED32:
    case ScalarType.SFIXED32:
    case ScalarType.FLOAT:
      wireType = WireType.Bit32;
      break;
  }
  const method = ScalarType[type].toLowerCase();
  return [wireType, method];
}
function writeMapEntry(writer, options, field, key, value) {
  writer.tag(field.no, WireType.LengthDelimited);
  writer.fork();
  let keyValue = key;
  switch (field.K) {
    case ScalarType.INT32:
    case ScalarType.FIXED32:
    case ScalarType.UINT32:
    case ScalarType.SFIXED32:
    case ScalarType.SINT32:
      keyValue = Number.parseInt(key);
      break;
    case ScalarType.BOOL:
      assert(key == "true" || key == "false");
      keyValue = key == "true";
      break;
  }
  writeScalar(writer, field.K, 1, keyValue);
  switch (field.V.kind) {
    case "scalar":
      writeScalar(writer, field.V.T, 2, value);
      break;
    case "enum":
      writeScalar(writer, ScalarType.INT32, 2, value);
      break;
    case "message": {
      assert(value !== void 0);
      const messageType = resolveMessageType(field.V.T);
      writer.tag(2, WireType.LengthDelimited).bytes(messageType.toBinary(value, options));
      break;
    }
  }
  writer.join();
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/proto-base64.js
var encTable = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".split("");
var decTable = [];
for (let i = 0; i < encTable.length; i++)
  decTable[encTable[i].charCodeAt(0)] = i;
decTable["-".charCodeAt(0)] = encTable.indexOf("+");
decTable["_".charCodeAt(0)] = encTable.indexOf("/");
var protoBase64 = {
  /**
   * Decodes a base64 string to a byte array.
   *
   * - ignores white-space, including line breaks and tabs
   * - allows inner padding (can decode concatenated base64 strings)
   * - does not require padding
   * - understands base64url encoding:
   *   "-" instead of "+",
   *   "_" instead of "/",
   *   no padding
   */
  dec(base64Str) {
    let es = base64Str.length * 3 / 4;
    if (base64Str[base64Str.length - 2] == "=")
      es -= 2;
    else if (base64Str[base64Str.length - 1] == "=")
      es -= 1;
    let bytes = new Uint8Array(es), bytePos = 0, groupPos = 0, b, p = 0;
    for (let i = 0; i < base64Str.length; i++) {
      b = decTable[base64Str.charCodeAt(i)];
      if (b === void 0) {
        switch (base64Str[i]) {
          // @ts-ignore TS7029: Fallthrough case in switch
          case "=":
            groupPos = 0;
          // reset state when padding found
          // @ts-ignore TS7029: Fallthrough case in switch
          case "\n":
          case "\r":
          case "	":
          case " ":
            continue;
          // skip white-space, and padding
          default:
            throw Error("invalid base64 string.");
        }
      }
      switch (groupPos) {
        case 0:
          p = b;
          groupPos = 1;
          break;
        case 1:
          bytes[bytePos++] = p << 2 | (b & 48) >> 4;
          p = b;
          groupPos = 2;
          break;
        case 2:
          bytes[bytePos++] = (p & 15) << 4 | (b & 60) >> 2;
          p = b;
          groupPos = 3;
          break;
        case 3:
          bytes[bytePos++] = (p & 3) << 6 | b;
          groupPos = 0;
          break;
      }
    }
    if (groupPos == 1)
      throw Error("invalid base64 string.");
    return bytes.subarray(0, bytePos);
  },
  /**
   * Encode a byte array to a base64 string.
   */
  enc(bytes) {
    let base64 = "", groupPos = 0, b, p = 0;
    for (let i = 0; i < bytes.length; i++) {
      b = bytes[i];
      switch (groupPos) {
        case 0:
          base64 += encTable[b >> 2];
          p = (b & 3) << 4;
          groupPos = 1;
          break;
        case 1:
          base64 += encTable[p | b >> 4];
          p = (b & 15) << 2;
          groupPos = 2;
          break;
        case 2:
          base64 += encTable[p | b >> 6];
          base64 += encTable[b & 63];
          groupPos = 0;
          break;
      }
    }
    if (groupPos) {
      base64 += encTable[p];
      base64 += "=";
      if (groupPos == 1)
        base64 += "=";
    }
    return base64;
  }
};

// ../../../node_modules/@aptre/protobuf-es-lite/dist/json.js
var jsonReadDefaults = {
  ignoreUnknownFields: false
};
var jsonWriteDefaults = {
  emitDefaultValues: false,
  enumAsInteger: false,
  useProtoFieldName: false,
  prettySpaces: 0
};
function makeReadOptions2(options) {
  return options ? { ...jsonReadDefaults, ...options } : jsonReadDefaults;
}
function makeWriteOptions2(options) {
  return options ? { ...jsonWriteDefaults, ...options } : jsonWriteDefaults;
}
function jsonDebugValue(json) {
  if (json === null) {
    return "null";
  }
  switch (typeof json) {
    case "object":
      return Array.isArray(json) ? "array" : "object";
    case "string":
      return json.length > 100 ? "string" : `"${json.split('"').join('\\"')}"`;
    default:
      return String(json);
  }
}
function readMessage2(fields, typeName, json, options, message) {
  if (json == null || Array.isArray(json) || typeof json != "object") {
    throw new Error(`cannot decode message ${typeName} from JSON: ${jsonDebugValue(json)}`);
  }
  const oneofSeen = /* @__PURE__ */ new Map();
  for (const [jsonKey, jsonValue] of Object.entries(json)) {
    const field = fields.findJsonName(jsonKey);
    if (field) {
      if (field.oneof) {
        if (jsonValue === null && field.kind == "scalar") {
          continue;
        }
        const seen = oneofSeen.get(field.oneof);
        if (seen !== void 0) {
          throw new Error(`cannot decode message ${typeName} from JSON: multiple keys for oneof "${field.oneof.name}" present: "${seen}", "${jsonKey}"`);
        }
        oneofSeen.set(field.oneof, jsonKey);
      }
      readField2(message, jsonValue, field, options);
    } else {
      if (!options.ignoreUnknownFields) {
        throw new Error(`cannot decode message ${typeName} from JSON: key "${jsonKey}" is unknown`);
      }
    }
  }
  return message;
}
function writeMessage2(message, fields, options) {
  const json = /* @__PURE__ */ Object.create(null);
  let field;
  try {
    for (field of fields.byNumber()) {
      if (!isFieldSet(field, message)) {
        if (field.req) {
          throw `required field not set`;
        }
        if (!options.emitDefaultValues) {
          continue;
        }
        if (!canEmitFieldDefaultValue(field)) {
          continue;
        }
      }
      const value = field.oneof ? message[field.oneof.localName].value : message[field.localName];
      const jsonValue = writeField2(field, value, options);
      if (jsonValue !== void 0) {
        json[options.useProtoFieldName ? field.name : field.jsonName] = jsonValue;
      }
    }
  } catch (e) {
    const m = field ? `cannot encode field ${field.name} to JSON` : `cannot encode message to JSON`;
    const r = e instanceof Error ? e.message : String(e);
    throw new Error(m + (r.length > 0 ? `: ${r}` : ""), { cause: e });
  }
  return json;
}
function readField2(target, jsonValue, field, options) {
  let localName2 = field.localName;
  if (field.repeated) {
    assert(field.kind != "map");
    if (jsonValue === null) {
      return;
    }
    if (!Array.isArray(jsonValue)) {
      throw new Error(`cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`);
    }
    let targetArray = target[localName2];
    if (!Array.isArray(targetArray)) {
      targetArray = target[localName2] = [];
    }
    for (const jsonItem of jsonValue) {
      if (jsonItem === null) {
        throw new Error(`cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonItem)}`);
      }
      switch (field.kind) {
        case "message": {
          const messageType = resolveMessageType(field.T);
          targetArray.push(unwrapField(messageType.fieldWrapper, messageType.fromJson(jsonItem, options)));
          break;
        }
        case "enum": {
          const enumValue = readEnum(field.T, jsonItem, options.ignoreUnknownFields, true);
          if (enumValue !== tokenIgnoredUnknownEnum) {
            targetArray.push(enumValue);
          }
          break;
        }
        case "scalar":
          try {
            targetArray.push(readScalar2(field.T, jsonItem, field.L, true));
          } catch (e) {
            let m = `cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonItem)}`;
            if (e instanceof Error && e.message.length > 0) {
              m += `: ${e.message}`;
            }
            throw new Error(m, { cause: e });
          }
          break;
      }
    }
  } else if (field.kind == "map") {
    if (jsonValue === null) {
      return;
    }
    if (typeof jsonValue != "object" || Array.isArray(jsonValue)) {
      throw new Error(`cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`);
    }
    let targetMap = target[localName2];
    if (typeof targetMap !== "object") {
      targetMap = target[localName2] = /* @__PURE__ */ Object.create(null);
    }
    for (const [jsonMapKey, jsonMapValue] of Object.entries(jsonValue)) {
      if (jsonMapValue === null) {
        throw new Error(`cannot decode field ${field.name} from JSON: map value null`);
      }
      let key;
      try {
        key = readMapKey(field.K, jsonMapKey);
      } catch (e) {
        let m = `cannot decode map key for field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`;
        if (e instanceof Error && e.message.length > 0) {
          m += `: ${e.message}`;
        }
        throw new Error(m, { cause: e });
      }
      throwSanitizeKey(key);
      switch (field.V.kind) {
        case "message": {
          const messageType = resolveMessageType(field.V.T);
          targetMap[key] = messageType.fromJson(jsonMapValue, options);
          break;
        }
        case "enum": {
          const enumValue = readEnum(field.V.T, jsonMapValue, options.ignoreUnknownFields, true);
          if (enumValue !== tokenIgnoredUnknownEnum) {
            targetMap[key] = enumValue;
          }
          break;
        }
        case "scalar":
          try {
            targetMap[key] = readScalar2(field.V.T, jsonMapValue, LongType.BIGINT, true);
          } catch (e) {
            let m = `cannot decode map value for field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`;
            if (e instanceof Error && e.message.length > 0) {
              m += `: ${e.message}`;
            }
            throw new Error(m, { cause: e });
          }
          break;
      }
    }
  } else {
    if (field.oneof) {
      target = target[field.oneof.localName] = { case: localName2 };
      localName2 = "value";
    }
    switch (field.kind) {
      case "message": {
        const messageType = resolveMessageType(field.T);
        if (jsonValue === null && messageType.typeName != "google.protobuf.Value") {
          return;
        }
        target[localName2] = unwrapField(messageType.fieldWrapper, messageType.fromJson(jsonValue, options));
        break;
      }
      case "enum": {
        const enumValue = readEnum(field.T, jsonValue, options.ignoreUnknownFields, false);
        switch (enumValue) {
          case tokenNull:
            clearField(field, target);
            break;
          case tokenIgnoredUnknownEnum:
            break;
          default:
            target[localName2] = enumValue;
            break;
        }
        break;
      }
      case "scalar":
        try {
          const scalarValue = readScalar2(field.T, jsonValue, field.L, false);
          switch (scalarValue) {
            case tokenNull:
              clearField(field, target);
              break;
            default:
              target[localName2] = scalarValue;
              break;
          }
        } catch (e) {
          let m = `cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`;
          if (e instanceof Error && e.message.length > 0) {
            m += `: ${e.message}`;
          }
          throw new Error(m, { cause: e });
        }
        break;
    }
  }
}
var tokenNull = /* @__PURE__ */ Symbol();
var tokenIgnoredUnknownEnum = /* @__PURE__ */ Symbol();
function readEnum(type, json, ignoreUnknownFields, nullAsZeroValue) {
  if (json === null) {
    if (type.typeName == "google.protobuf.NullValue") {
      return 0;
    }
    return nullAsZeroValue ? type.values[0].no : tokenNull;
  }
  switch (typeof json) {
    case "number":
      if (Number.isInteger(json)) {
        return json;
      }
      break;
    case "string": {
      const value = type.findName(json);
      if (value !== void 0) {
        return value.no;
      }
      if (ignoreUnknownFields) {
        return tokenIgnoredUnknownEnum;
      }
      break;
    }
  }
  throw new Error(`cannot decode enum ${type.typeName} from JSON: ${jsonDebugValue(json)}`);
}
function readScalar2(type, json, longType = LongType.BIGINT, nullAsZeroValue = true) {
  if (json == null) {
    if (nullAsZeroValue) {
      return scalarZeroValue(type, longType);
    }
    return tokenNull;
  }
  switch (type) {
    // float, double: JSON value will be a number or one of the special string values "NaN", "Infinity", and "-Infinity".
    // Either numbers or strings are accepted. Exponent notation is also accepted.
    case ScalarType.DOUBLE:
    case ScalarType.FLOAT: {
      if (json === "NaN")
        return Number.NaN;
      if (json === "Infinity")
        return Number.POSITIVE_INFINITY;
      if (json === "-Infinity")
        return Number.NEGATIVE_INFINITY;
      if (json === "") {
        break;
      }
      if (typeof json == "string" && json.trim().length !== json.length) {
        break;
      }
      if (typeof json != "string" && typeof json != "number") {
        break;
      }
      const float = Number(json);
      if (Number.isNaN(float)) {
        break;
      }
      if (!Number.isFinite(float)) {
        break;
      }
      if (type == ScalarType.FLOAT)
        assertFloat32(float);
      return float;
    }
    // int32, fixed32, uint32: JSON value will be a decimal number. Either numbers or strings are accepted.
    case ScalarType.INT32:
    case ScalarType.FIXED32:
    case ScalarType.SFIXED32:
    case ScalarType.SINT32:
    case ScalarType.UINT32: {
      let int32;
      if (typeof json == "number")
        int32 = json;
      else if (typeof json == "string" && json.length > 0) {
        if (json.trim().length === json.length)
          int32 = Number(json);
      }
      if (int32 === void 0)
        break;
      if (type == ScalarType.UINT32 || type == ScalarType.FIXED32)
        assertUInt32(int32);
      else
        assertInt32(int32);
      return int32;
    }
    // int64, fixed64, uint64: JSON value will be a decimal string. Either numbers or strings are accepted.
    case ScalarType.INT64:
    case ScalarType.SFIXED64:
    case ScalarType.SINT64: {
      if (typeof json != "number" && typeof json != "string")
        break;
      const long = protoInt64.parse(json);
      return longType ? long.toString() : long;
    }
    case ScalarType.FIXED64:
    case ScalarType.UINT64: {
      if (typeof json != "number" && typeof json != "string")
        break;
      const uLong = protoInt64.uParse(json);
      return longType ? uLong.toString() : uLong;
    }
    // bool:
    case ScalarType.BOOL:
      if (typeof json !== "boolean")
        break;
      return json;
    // string:
    case ScalarType.STRING:
      if (typeof json !== "string") {
        break;
      }
      try {
        encodeURIComponent(json);
      } catch (_e) {
        throw new Error("invalid UTF8", { cause: _e });
      }
      return json;
    // bytes: JSON value will be the data encoded as a string using standard base64 encoding with paddings.
    // Either standard or URL-safe base64 encoding with/without paddings are accepted.
    case ScalarType.BYTES:
      if (json === "")
        return new Uint8Array(0);
      if (typeof json !== "string")
        break;
      return protoBase64.dec(json);
  }
  throw new Error();
}
function readMapKey(type, json) {
  if (type === ScalarType.BOOL) {
    switch (json) {
      case "true":
        json = true;
        break;
      case "false":
        json = false;
        break;
    }
  }
  return readScalar2(type, json, LongType.BIGINT, true)?.toString() ?? "";
}
function clearField(field, target) {
  const localName2 = field.localName;
  const implicitPresence = !field.opt && !field.req;
  if (field.repeated) {
    target[localName2] = [];
  } else if (field.oneof) {
    target[field.oneof.localName] = { case: void 0 };
  } else {
    switch (field.kind) {
      case "map":
        target[localName2] = /* @__PURE__ */ Object.create(null);
        break;
      case "enum":
        target[localName2] = implicitPresence ? field.T.values[0].no : void 0;
        break;
      case "scalar":
        target[localName2] = implicitPresence ? scalarZeroValue(field.T, field.L) : void 0;
        break;
      case "message":
        target[localName2] = void 0;
        break;
    }
  }
}
function canEmitFieldDefaultValue(field) {
  if (field.repeated || field.kind == "map") {
    return true;
  }
  if (field.oneof) {
    return false;
  }
  if (field.kind == "message") {
    return false;
  }
  if (field.opt || field.req) {
    return false;
  }
  return true;
}
function writeField2(field, value, options) {
  if (field.kind == "map") {
    const jsonObj = /* @__PURE__ */ Object.create(null);
    assert(!value || typeof value === "object");
    const entries2 = value ? Object.entries(value) : [];
    switch (field.V.kind) {
      case "scalar":
        for (const [entryKey, entryValue] of entries2) {
          jsonObj[entryKey.toString()] = writeScalar2(field.V.T, entryValue);
        }
        break;
      case "message":
        for (const [entryKey, entryValue] of entries2) {
          const messageType = resolveMessageType(field.V.T);
          jsonObj[entryKey.toString()] = messageType.toJson(entryValue, options);
        }
        break;
      case "enum": {
        const enumType = field.V.T;
        for (const [entryKey, entryValue] of entries2) {
          jsonObj[entryKey.toString()] = writeEnum(enumType, entryValue, options.enumAsInteger);
        }
        break;
      }
    }
    return options.emitDefaultValues || entries2.length > 0 ? jsonObj : void 0;
  }
  if (field.repeated) {
    assert(!value || Array.isArray(value));
    const jsonArr = [];
    const valueArr = value;
    if (valueArr && valueArr.length) {
      switch (field.kind) {
        case "scalar":
          for (let i = 0; i < valueArr.length; i++) {
            jsonArr.push(writeScalar2(field.T, valueArr[i]));
          }
          break;
        case "enum":
          for (let i = 0; i < valueArr.length; i++) {
            jsonArr.push(writeEnum(field.T, valueArr[i], options.enumAsInteger));
          }
          break;
        case "message": {
          const messageType = resolveMessageType(field.T);
          for (let i = 0; i < valueArr.length; i++) {
            jsonArr.push(messageType.toJson(wrapField(messageType.fieldWrapper, valueArr[i])));
          }
          break;
        }
      }
    }
    return options.emitDefaultValues || jsonArr.length > 0 ? jsonArr : void 0;
  }
  switch (field.kind) {
    case "scalar": {
      const scalarValue = normalizeScalarValue(field.T, value, false);
      if (!options.emitDefaultValues && isScalarZeroValue(field.T, scalarValue)) {
        return void 0;
      }
      return writeScalar2(field.T, value);
    }
    case "enum": {
      const enumValue = normalizeEnumValue(field.T, value);
      if (!options.emitDefaultValues && enumZeroValue(field.T) === enumValue) {
        return void 0;
      }
      return writeEnum(field.T, value, options.enumAsInteger);
    }
    case "message": {
      if (!options.emitDefaultValues && value == null) {
        return void 0;
      }
      const messageType = resolveMessageType(field.T);
      return messageType.toJson(wrapField(messageType.fieldWrapper, value));
    }
  }
}
function writeScalar2(type, value) {
  switch (type) {
    // int32, fixed32, uint32: JSON value will be a decimal number. Either numbers or strings are accepted.
    case ScalarType.INT32:
    case ScalarType.SFIXED32:
    case ScalarType.SINT32:
    case ScalarType.FIXED32:
    case ScalarType.UINT32:
      assert(typeof value == "number");
      return value;
    // float, double: JSON value will be a number or one of the special string values "NaN", "Infinity", and "-Infinity".
    // Either numbers or strings are accepted. Exponent notation is also accepted.
    case ScalarType.FLOAT:
    // assertFloat32(value);
    case ScalarType.DOUBLE:
      assert(typeof value == "number");
      if (Number.isNaN(value))
        return "NaN";
      if (value === Number.POSITIVE_INFINITY)
        return "Infinity";
      if (value === Number.NEGATIVE_INFINITY)
        return "-Infinity";
      return value;
    // string:
    case ScalarType.STRING:
      assert(typeof value == "string");
      return value;
    // bool:
    case ScalarType.BOOL:
      assert(typeof value == "boolean");
      return value;
    // JSON value will be a decimal string. Either numbers or strings are accepted.
    case ScalarType.UINT64:
    case ScalarType.FIXED64:
    case ScalarType.INT64:
    case ScalarType.SFIXED64:
    case ScalarType.SINT64:
      assert(typeof value == "bigint" || typeof value == "string" || typeof value == "number");
      return value.toString();
    // bytes: JSON value will be the data encoded as a string using standard base64 encoding with paddings.
    // Either standard or URL-safe base64 encoding with/without paddings are accepted.
    case ScalarType.BYTES:
      assert(value instanceof Uint8Array);
      return protoBase64.enc(value);
    case ScalarType.DATE:
      throw new Error("cannot write date with writeScalar");
    default:
      throw new Error("unknown scalar type");
  }
}
function writeEnum(type, value, enumAsInteger) {
  assert(typeof value == "number");
  if (type.typeName == "google.protobuf.NullValue") {
    return null;
  }
  if (enumAsInteger) {
    return value;
  }
  const val = type.findNumber(value);
  return val?.name ?? value;
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/message.js
function createMessageType(params, exts) {
  const { fields: fieldsSource, typeName, packedByDefault, delimitedMessageEncoding, fieldWrapper } = params;
  const fields = newFieldList(fieldsSource, packedByDefault);
  const mt = {
    typeName,
    fields,
    fieldWrapper,
    create(partial) {
      const message = /* @__PURE__ */ Object.create(null);
      applyPartialMessage(partial, message, fields);
      return message;
    },
    createComplete(partial) {
      const message = createCompleteMessage(fields);
      applyPartialMessage(partial, message, fields);
      return message;
    },
    equals(a, b) {
      return compareMessages(fields, a, b);
    },
    clone(a) {
      if (a == null) {
        return a;
      }
      return cloneMessage(a, fields);
    },
    fromBinary(bytes, options) {
      const message = {};
      if (bytes && bytes.length) {
        const opt = makeReadOptions(options);
        readMessage(message, fields, opt.readerFactory(bytes), bytes.byteLength, opt, delimitedMessageEncoding ?? false);
      }
      return message;
    },
    fromJson(jsonValue, options) {
      const message = {};
      if (jsonValue != null) {
        const opts = makeReadOptions2(options);
        readMessage2(fields, typeName, jsonValue, opts, message);
      }
      return message;
    },
    fromJsonString(jsonString, options) {
      let json = null;
      if (jsonString) {
        try {
          json = JSON.parse(jsonString);
        } catch (e) {
          throw new Error(`cannot decode ${typeName} from JSON: ${e instanceof Error ? e.message : String(e)}`, { cause: e });
        }
      }
      return mt.fromJson(json, options);
    },
    toBinary(a, options) {
      if (a == null)
        return new Uint8Array(0);
      const opt = makeWriteOptions(options);
      const writer = opt.writerFactory();
      writeMessage(a, fields, writer, opt);
      return writer.finish();
    },
    toJson(a, options) {
      const opt = makeWriteOptions2(options);
      return writeMessage2(a, fields, opt);
    },
    toJsonString(a, options) {
      const value = mt.toJson(a, options);
      return JSON.stringify(value, null, options?.prettySpaces ?? 0);
    },
    ...exts ?? {}
  };
  return mt;
}
function compareMessages(fields, a, b) {
  if (a == null && b == null) {
    return true;
  }
  if (a === b) {
    return true;
  }
  if (!a || !b) {
    return false;
  }
  return fields.byMember().every((m) => {
    const va = a[m.localName];
    const vb = b[m.localName];
    if (m.repeated) {
      if ((va?.length ?? 0) !== (vb?.length ?? 0)) {
        return false;
      }
      if (!va?.length) {
        return true;
      }
      switch (m.kind) {
        case "message": {
          const messageType = resolveMessageType(m.T);
          return va.every((a2, i) => messageType.equals(a2, vb[i]));
        }
        case "scalar":
          return va.every((a2, i) => scalarEquals(m.T, a2, vb[i]));
        case "enum":
          return va.every((a2, i) => scalarEquals(ScalarType.INT32, a2, vb[i]));
      }
      throw new Error(`repeated cannot contain ${m.kind}`);
    }
    switch (m.kind) {
      case "message":
        return resolveMessageType(m.T).equals(va, vb);
      case "enum":
        return scalarEquals(ScalarType.INT32, va, vb);
      case "scalar":
        return scalarEquals(m.T, va, vb);
      case "oneof": {
        if (va?.case !== vb?.case) {
          return false;
        }
        if (va == null) {
          return true;
        }
        const s = m.findField(va.case);
        if (s === void 0) {
          return true;
        }
        switch (s.kind) {
          case "message": {
            const messageType = resolveMessageType(s.T);
            return messageType.equals(va.value, vb.value);
          }
          case "enum":
            return scalarEquals(ScalarType.INT32, va.value, vb.value);
          case "scalar":
            return scalarEquals(s.T, va.value, vb.value);
        }
        throw new Error(`oneof cannot contain ${s.kind}`);
      }
      case "map": {
        const keys = Object.keys(va).concat(Object.keys(vb));
        switch (m.V.kind) {
          case "message": {
            const messageType = resolveMessageType(m.V.T);
            return keys.every((k) => messageType.equals(va[k], vb[k]));
          }
          case "enum":
            return keys.every((k) => scalarEquals(ScalarType.INT32, va[k], vb[k]));
          case "scalar": {
            const scalarType = m.V.T;
            return keys.every((k) => scalarEquals(scalarType, va[k], vb[k]));
          }
        }
      }
    }
  });
}
function cloneMessage(message, fields) {
  if (message == null) {
    return null;
  }
  const clone = /* @__PURE__ */ Object.create(null);
  applyPartialMessage(message, clone, fields, true);
  return clone;
}
function createCompleteMessage(fields) {
  const message = {};
  for (const field of fields.byMember()) {
    const { localName: localName2, kind: fieldKind } = field;
    throwSanitizeKey(localName2);
    switch (fieldKind) {
      case "oneof":
        message[localName2] = /* @__PURE__ */ Object.create(null);
        message[localName2].case = void 0;
        break;
      case "scalar":
        if (field.repeated) {
          message[localName2] = [];
        } else {
          message[localName2] = scalarZeroValue(field.T, LongType.BIGINT);
        }
        break;
      case "enum":
        message[localName2] = field.repeated ? [] : enumZeroValue(field.T);
        break;
      case "message": {
        if (field.oneof) {
          break;
        }
        if (field.repeated) {
          message[localName2] = [];
          break;
        }
        const messageType = resolveMessageType(field.T);
        message[localName2] = messageType.fieldWrapper ? messageType.fieldWrapper.unwrapField(null) : createCompleteMessage(messageType.fields);
        break;
      }
      case "map":
        message[localName2] = /* @__PURE__ */ Object.create(null);
        break;
    }
  }
  return message;
}

// ../../../node_modules/@aptre/protobuf-es-lite/dist/service-type.js
var MethodKind;
(function(MethodKind2) {
  MethodKind2[MethodKind2["Unary"] = 0] = "Unary";
  MethodKind2[MethodKind2["ServerStreaming"] = 1] = "ServerStreaming";
  MethodKind2[MethodKind2["ClientStreaming"] = 2] = "ClientStreaming";
  MethodKind2[MethodKind2["BiDiStreaming"] = 3] = "BiDiStreaming";
})(MethodKind || (MethodKind = {}));
var MethodIdempotency;
(function(MethodIdempotency2) {
  MethodIdempotency2[MethodIdempotency2["NoSideEffects"] = 1] = "NoSideEffects";
  MethodIdempotency2[MethodIdempotency2["Idempotent"] = 2] = "Idempotent";
})(MethodIdempotency || (MethodIdempotency = {}));

// ../../../node_modules/starpc/dist/srpc/rpcproto.pb.js
var CallStart = createMessageType({
  typeName: "srpc.CallStart",
  fields: [
    { no: 1, name: "rpc_service", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "rpc_method", kind: "scalar", T: ScalarType.STRING },
    { no: 3, name: "data", kind: "scalar", T: ScalarType.BYTES },
    { no: 4, name: "data_is_zero", kind: "scalar", T: ScalarType.BOOL }
  ],
  packedByDefault: true
});
var CallData = createMessageType({
  typeName: "srpc.CallData",
  fields: [
    { no: 1, name: "data", kind: "scalar", T: ScalarType.BYTES },
    { no: 2, name: "data_is_zero", kind: "scalar", T: ScalarType.BOOL },
    { no: 3, name: "complete", kind: "scalar", T: ScalarType.BOOL },
    { no: 4, name: "error", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
var Packet = createMessageType({
  typeName: "srpc.Packet",
  fields: [
    {
      no: 1,
      name: "call_start",
      kind: "message",
      T: () => CallStart,
      oneof: "body"
    },
    {
      no: 2,
      name: "call_data",
      kind: "message",
      T: () => CallData,
      oneof: "body"
    },
    {
      no: 3,
      name: "call_cancel",
      kind: "scalar",
      T: ScalarType.BOOL,
      oneof: "body"
    }
  ],
  packedByDefault: true
});

// ../../../node_modules/starpc/dist/srpc/common-rpc.js
var CommonRPC = class {
  // sink is the data sink for incoming messages.
  sink;
  // source is the packet source for outgoing Packets.
  source;
  // rpcDataSource is the source for rpc packets.
  rpcDataSource;
  // _source is used to write to the source.
  _source = pushable({
    objectMode: true
  });
  // _rpcDataSource is used to write to the rpc message source.
  _rpcDataSource = pushable({
    objectMode: true
  });
  // service is the rpc service
  service;
  // method is the rpc method
  method;
  // closed indicates this rpc has been closed already.
  closed;
  constructor() {
    this.sink = this._createSink();
    this.source = this._source;
    this.rpcDataSource = this._rpcDataSource;
  }
  // isClosed returns one of: true (closed w/o error), Error (closed w/ error), or false (not closed).
  get isClosed() {
    return this.closed ?? false;
  }
  // writeCallData writes the call data packet.
  async writeCallData(data, complete, error) {
    const callData = {
      data: data || new Uint8Array(0),
      dataIsZero: !!data && data.length === 0,
      complete: complete || false,
      error: error || ""
    };
    await this.writePacket({
      body: {
        case: "callData",
        value: callData
      }
    });
  }
  // writeCallCancel writes the call cancel packet.
  async writeCallCancel() {
    await this.writePacket({
      body: {
        case: "callCancel",
        value: true
      }
    });
  }
  // writeCallDataFromSource writes all call data from the iterable.
  async writeCallDataFromSource(dataSource) {
    try {
      for await (const data of dataSource) {
        await this.writeCallData(data);
      }
      await this.writeCallData(void 0, true);
    } catch (err) {
      this.close(err);
    }
  }
  // writePacket writes a packet to the stream.
  async writePacket(packet) {
    this._source.push(packet);
  }
  // handleMessage handles an incoming encoded Packet.
  //
  // note: closes the stream if any error is thrown.
  async handleMessage(message) {
    return this.handlePacket(Packet.fromBinary(message));
  }
  // handlePacket handles an incoming packet.
  //
  // note: closes the stream if any error is thrown.
  async handlePacket(packet) {
    try {
      switch (packet?.body?.case) {
        case "callStart":
          await this.handleCallStart(packet.body.value);
          break;
        case "callData":
          await this.handleCallData(packet.body.value);
          break;
        case "callCancel":
          if (packet.body.value) {
            await this.handleCallCancel();
          }
          break;
      }
    } catch (err) {
      let asError = err;
      if (!asError?.message) {
        asError = new Error("error handling packet");
      }
      this.close(asError);
    }
  }
  // handleCallStart handles a CallStart packet.
  async handleCallStart(packet) {
    throw new Error(`unexpected call start: ${packet.rpcService}/${packet.rpcMethod}`);
  }
  // pushRpcData pushes incoming rpc data to the rpc data source.
  pushRpcData(data, dataIsZero) {
    if (dataIsZero) {
      if (!data || data.length !== 0) {
        data = new Uint8Array(0);
      }
    } else if (!data || data.length === 0) {
      return;
    }
    this._rpcDataSource.push(data);
  }
  // handleCallData handles a CallData packet.
  async handleCallData(packet) {
    if (!this.service || !this.method) {
      throw new Error("call start must be sent before call data");
    }
    this.pushRpcData(packet.data, packet.dataIsZero);
    if (packet.error) {
      this._rpcDataSource.end(new Error(packet.error));
    } else if (packet.complete) {
      this._rpcDataSource.end();
    }
  }
  // handleCallCancel handles a CallCancel packet.
  async handleCallCancel() {
    this.close(new Error(ERR_RPC_ABORT));
  }
  // close closes the call, optionally with an error.
  async close(err) {
    if (this.closed) {
      return;
    }
    this.closed = err ?? true;
    if (err && err.message) {
      await this.writeCallData(void 0, true, err.message);
    }
    this._source.end();
    this._rpcDataSource.end(err);
  }
  // _createSink returns a value for the sink field.
  _createSink() {
    return async (source) => {
      try {
        if (Symbol.asyncIterator in source) {
          for await (const msg of source) {
            await this.handlePacket(msg);
          }
        } else {
          for (const msg of source) {
            await this.handlePacket(msg);
          }
        }
      } catch (err) {
        this.close(err);
      }
    };
  }
};

// ../../../node_modules/starpc/dist/srpc/client-rpc.js
var ClientRPC = class extends CommonRPC {
  constructor(service, method) {
    super();
    this.service = service;
    this.method = method;
  }
  // writeCallStart writes the call start packet.
  // if data === undefined and data.length === 0 sends empty data packet.
  async writeCallStart(data) {
    if (!this.service || !this.method) {
      throw new Error("service and method must be set");
    }
    const callStart = {
      rpcService: this.service,
      rpcMethod: this.method,
      data: data || new Uint8Array(0),
      dataIsZero: !!data && data.length === 0
    };
    await this.writePacket({
      body: {
        case: "callStart",
        value: callStart
      }
    });
  }
  // handleCallStart handles a CallStart packet.
  async handleCallStart(packet) {
    throw new Error(`unexpected server to client rpc: ${packet.rpcService || "<empty>"}/${packet.rpcMethod || "<empty>"}`);
  }
};

// ../../../node_modules/starpc/dist/srpc/pushable.js
async function writeToPushable(dataSource, out) {
  try {
    for await (const data of dataSource) {
      out.push(data);
    }
    out.end();
  } catch (err) {
    out.end(err);
  }
}

// ../../../node_modules/uint8arrays/dist/src/alloc.js
function alloc(size = 0) {
  return new Uint8Array(size);
}
function allocUnsafe(size = 0) {
  return new Uint8Array(size);
}

// ../../../node_modules/uint8arrays/dist/src/util/as-uint8array.js
function asUint8Array(buf) {
  return buf;
}

// ../../../node_modules/uint8arrays/dist/src/concat.js
function concat(arrays, length) {
  if (length == null) {
    length = arrays.reduce((acc, curr) => acc + curr.length, 0);
  }
  const output = allocUnsafe(length);
  let offset = 0;
  for (const arr of arrays) {
    output.set(arr, offset);
    offset += arr.length;
  }
  return asUint8Array(output);
}

// ../../../node_modules/uint8arrays/dist/src/equals.js
function equals(a, b) {
  if (a === b) {
    return true;
  }
  if (a.byteLength !== b.byteLength) {
    return false;
  }
  for (let i = 0; i < a.byteLength; i++) {
    if (a[i] !== b[i]) {
      return false;
    }
  }
  return true;
}

// ../../../node_modules/uint8arraylist/dist/src/index.js
var symbol = /* @__PURE__ */ Symbol.for("@achingbrain/uint8arraylist");
function findBufAndOffset(bufs, index) {
  if (index == null || index < 0) {
    throw new RangeError("index is out of bounds");
  }
  let offset = 0;
  for (const buf of bufs) {
    const bufEnd = offset + buf.byteLength;
    if (index < bufEnd) {
      return {
        buf,
        index: index - offset
      };
    }
    offset = bufEnd;
  }
  throw new RangeError("index is out of bounds");
}
function isUint8ArrayList(value) {
  return Boolean(value?.[symbol]);
}
var Uint8ArrayList = class _Uint8ArrayList {
  bufs;
  length;
  [symbol] = true;
  constructor(...data) {
    this.bufs = [];
    this.length = 0;
    if (data.length > 0) {
      this.appendAll(data);
    }
  }
  *[Symbol.iterator]() {
    yield* this.bufs;
  }
  get byteLength() {
    return this.length;
  }
  /**
   * Add one or more `bufs` to the end of this Uint8ArrayList
   */
  append(...bufs) {
    this.appendAll(bufs);
  }
  /**
   * Add all `bufs` to the end of this Uint8ArrayList
   */
  appendAll(bufs) {
    let length = 0;
    for (const buf of bufs) {
      if (buf instanceof Uint8Array) {
        length += buf.byteLength;
        this.bufs.push(buf);
      } else if (isUint8ArrayList(buf)) {
        length += buf.byteLength;
        this.bufs.push(...buf.bufs);
      } else {
        throw new Error("Could not append value, must be an Uint8Array or a Uint8ArrayList");
      }
    }
    this.length += length;
  }
  /**
   * Add one or more `bufs` to the start of this Uint8ArrayList
   */
  prepend(...bufs) {
    this.prependAll(bufs);
  }
  /**
   * Add all `bufs` to the start of this Uint8ArrayList
   */
  prependAll(bufs) {
    let length = 0;
    for (const buf of bufs.reverse()) {
      if (buf instanceof Uint8Array) {
        length += buf.byteLength;
        this.bufs.unshift(buf);
      } else if (isUint8ArrayList(buf)) {
        length += buf.byteLength;
        this.bufs.unshift(...buf.bufs);
      } else {
        throw new Error("Could not prepend value, must be an Uint8Array or a Uint8ArrayList");
      }
    }
    this.length += length;
  }
  /**
   * Read the value at `index`
   */
  get(index) {
    const res = findBufAndOffset(this.bufs, index);
    return res.buf[res.index];
  }
  /**
   * Set the value at `index` to `value`
   */
  set(index, value) {
    const res = findBufAndOffset(this.bufs, index);
    res.buf[res.index] = value;
  }
  /**
   * Copy bytes from `buf` to the index specified by `offset`
   */
  write(buf, offset = 0) {
    if (buf instanceof Uint8Array) {
      for (let i = 0; i < buf.length; i++) {
        this.set(offset + i, buf[i]);
      }
    } else if (isUint8ArrayList(buf)) {
      for (let i = 0; i < buf.length; i++) {
        this.set(offset + i, buf.get(i));
      }
    } else {
      throw new Error("Could not write value, must be an Uint8Array or a Uint8ArrayList");
    }
  }
  /**
   * Remove bytes from the front of the pool
   */
  consume(bytes) {
    bytes = Math.trunc(bytes);
    if (Number.isNaN(bytes) || bytes <= 0) {
      return;
    }
    if (bytes === this.byteLength) {
      this.bufs = [];
      this.length = 0;
      return;
    }
    while (this.bufs.length > 0) {
      if (bytes >= this.bufs[0].byteLength) {
        bytes -= this.bufs[0].byteLength;
        this.length -= this.bufs[0].byteLength;
        this.bufs.shift();
      } else {
        this.bufs[0] = this.bufs[0].subarray(bytes);
        this.length -= bytes;
        break;
      }
    }
  }
  /**
   * Extracts a section of an array and returns a new array.
   *
   * This is a copy operation as it is with Uint8Arrays and Arrays
   * - note this is different to the behaviour of Node Buffers.
   */
  slice(beginInclusive, endExclusive) {
    const { bufs, length } = this._subList(beginInclusive, endExclusive);
    return concat(bufs, length);
  }
  /**
   * Returns a alloc from the given start and end element index.
   *
   * In the best case where the data extracted comes from a single Uint8Array
   * internally this is a no-copy operation otherwise it is a copy operation.
   */
  subarray(beginInclusive, endExclusive) {
    const { bufs, length } = this._subList(beginInclusive, endExclusive);
    if (bufs.length === 1) {
      return bufs[0];
    }
    return concat(bufs, length);
  }
  /**
   * Returns a allocList from the given start and end element index.
   *
   * This is a no-copy operation.
   */
  sublist(beginInclusive, endExclusive) {
    const { bufs, length } = this._subList(beginInclusive, endExclusive);
    const list = new _Uint8ArrayList();
    list.length = length;
    list.bufs = [...bufs];
    return list;
  }
  _subList(beginInclusive, endExclusive) {
    beginInclusive = beginInclusive ?? 0;
    endExclusive = endExclusive ?? this.length;
    if (beginInclusive < 0) {
      beginInclusive = this.length + beginInclusive;
    }
    if (endExclusive < 0) {
      endExclusive = this.length + endExclusive;
    }
    if (beginInclusive < 0 || endExclusive > this.length) {
      throw new RangeError("index is out of bounds");
    }
    if (beginInclusive === endExclusive) {
      return { bufs: [], length: 0 };
    }
    if (beginInclusive === 0 && endExclusive === this.length) {
      return { bufs: this.bufs, length: this.length };
    }
    const bufs = [];
    let offset = 0;
    for (let i = 0; i < this.bufs.length; i++) {
      const buf = this.bufs[i];
      const bufStart = offset;
      const bufEnd = bufStart + buf.byteLength;
      offset = bufEnd;
      if (beginInclusive >= bufEnd) {
        continue;
      }
      const sliceStartInBuf = beginInclusive >= bufStart && beginInclusive < bufEnd;
      const sliceEndsInBuf = endExclusive > bufStart && endExclusive <= bufEnd;
      if (sliceStartInBuf && sliceEndsInBuf) {
        if (beginInclusive === bufStart && endExclusive === bufEnd) {
          bufs.push(buf);
          break;
        }
        const start = beginInclusive - bufStart;
        bufs.push(buf.subarray(start, start + (endExclusive - beginInclusive)));
        break;
      }
      if (sliceStartInBuf) {
        if (beginInclusive === 0) {
          bufs.push(buf);
          continue;
        }
        bufs.push(buf.subarray(beginInclusive - bufStart));
        continue;
      }
      if (sliceEndsInBuf) {
        if (endExclusive === bufEnd) {
          bufs.push(buf);
          break;
        }
        bufs.push(buf.subarray(0, endExclusive - bufStart));
        break;
      }
      bufs.push(buf);
    }
    return { bufs, length: endExclusive - beginInclusive };
  }
  indexOf(search, offset = 0) {
    if (!isUint8ArrayList(search) && !(search instanceof Uint8Array)) {
      throw new TypeError('The "value" argument must be a Uint8ArrayList or Uint8Array');
    }
    const needle = search instanceof Uint8Array ? search : search.subarray();
    offset = Number(offset ?? 0);
    if (isNaN(offset)) {
      offset = 0;
    }
    if (offset < 0) {
      offset = this.length + offset;
    }
    if (offset < 0) {
      offset = 0;
    }
    if (search.length === 0) {
      return offset > this.length ? this.length : offset;
    }
    const M = needle.byteLength;
    if (M === 0) {
      throw new TypeError("search must be at least 1 byte long");
    }
    const radix = 256;
    const rightmostPositions = new Int32Array(radix);
    for (let c = 0; c < radix; c++) {
      rightmostPositions[c] = -1;
    }
    for (let j = 0; j < M; j++) {
      rightmostPositions[needle[j]] = j;
    }
    const right = rightmostPositions;
    const lastIndex = this.byteLength - needle.byteLength;
    const lastPatIndex = needle.byteLength - 1;
    let skip;
    for (let i = offset; i <= lastIndex; i += skip) {
      skip = 0;
      for (let j = lastPatIndex; j >= 0; j--) {
        const char = this.get(i + j);
        if (needle[j] !== char) {
          skip = Math.max(1, j - right[char]);
          break;
        }
      }
      if (skip === 0) {
        return i;
      }
    }
    return -1;
  }
  getInt8(byteOffset) {
    const buf = this.subarray(byteOffset, byteOffset + 1);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getInt8(0);
  }
  setInt8(byteOffset, value) {
    const buf = allocUnsafe(1);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setInt8(0, value);
    this.write(buf, byteOffset);
  }
  getInt16(byteOffset, littleEndian) {
    const buf = this.subarray(byteOffset, byteOffset + 2);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getInt16(0, littleEndian);
  }
  setInt16(byteOffset, value, littleEndian) {
    const buf = alloc(2);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setInt16(0, value, littleEndian);
    this.write(buf, byteOffset);
  }
  getInt32(byteOffset, littleEndian) {
    const buf = this.subarray(byteOffset, byteOffset + 4);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getInt32(0, littleEndian);
  }
  setInt32(byteOffset, value, littleEndian) {
    const buf = alloc(4);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setInt32(0, value, littleEndian);
    this.write(buf, byteOffset);
  }
  getBigInt64(byteOffset, littleEndian) {
    const buf = this.subarray(byteOffset, byteOffset + 8);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getBigInt64(0, littleEndian);
  }
  setBigInt64(byteOffset, value, littleEndian) {
    const buf = alloc(8);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setBigInt64(0, value, littleEndian);
    this.write(buf, byteOffset);
  }
  getUint8(byteOffset) {
    const buf = this.subarray(byteOffset, byteOffset + 1);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getUint8(0);
  }
  setUint8(byteOffset, value) {
    const buf = allocUnsafe(1);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setUint8(0, value);
    this.write(buf, byteOffset);
  }
  getUint16(byteOffset, littleEndian) {
    const buf = this.subarray(byteOffset, byteOffset + 2);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getUint16(0, littleEndian);
  }
  setUint16(byteOffset, value, littleEndian) {
    const buf = alloc(2);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setUint16(0, value, littleEndian);
    this.write(buf, byteOffset);
  }
  getUint32(byteOffset, littleEndian) {
    const buf = this.subarray(byteOffset, byteOffset + 4);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getUint32(0, littleEndian);
  }
  setUint32(byteOffset, value, littleEndian) {
    const buf = alloc(4);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setUint32(0, value, littleEndian);
    this.write(buf, byteOffset);
  }
  getBigUint64(byteOffset, littleEndian) {
    const buf = this.subarray(byteOffset, byteOffset + 8);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getBigUint64(0, littleEndian);
  }
  setBigUint64(byteOffset, value, littleEndian) {
    const buf = alloc(8);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setBigUint64(0, value, littleEndian);
    this.write(buf, byteOffset);
  }
  getFloat32(byteOffset, littleEndian) {
    const buf = this.subarray(byteOffset, byteOffset + 4);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getFloat32(0, littleEndian);
  }
  setFloat32(byteOffset, value, littleEndian) {
    const buf = alloc(4);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setFloat32(0, value, littleEndian);
    this.write(buf, byteOffset);
  }
  getFloat64(byteOffset, littleEndian) {
    const buf = this.subarray(byteOffset, byteOffset + 8);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    return view.getFloat64(0, littleEndian);
  }
  setFloat64(byteOffset, value, littleEndian) {
    const buf = alloc(8);
    const view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    view.setFloat64(0, value, littleEndian);
    this.write(buf, byteOffset);
  }
  equals(other) {
    if (other == null) {
      return false;
    }
    if (!(other instanceof _Uint8ArrayList)) {
      return false;
    }
    if (other.bufs.length !== this.bufs.length) {
      return false;
    }
    for (let i = 0; i < this.bufs.length; i++) {
      if (!equals(this.bufs[i], other.bufs[i])) {
        return false;
      }
    }
    return true;
  }
  /**
   * Create a Uint8ArrayList from a pre-existing list of Uint8Arrays.  Use this
   * method if you know the total size of all the Uint8Arrays ahead of time.
   */
  static fromUint8Arrays(bufs, length) {
    const list = new _Uint8ArrayList();
    list.bufs = bufs;
    if (length == null) {
      length = bufs.reduce((acc, curr) => acc + curr.byteLength, 0);
    }
    list.length = length;
    return list;
  }
};

// ../../../node_modules/starpc/dist/srpc/message.js
function buildDecodeMessageTransform(def) {
  const decode2 = def.fromBinary.bind(def);
  return async function* decodeMessageSource(source) {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield* [decode2(p)];
        }
      } else {
        yield* [decode2(pkt)];
      }
    }
  };
}
function buildEncodeMessageTransform(def) {
  return async function* encodeMessageSource(source) {
    for await (const pkt of source) {
      if (Array.isArray(pkt)) {
        for (const p of pkt) {
          yield def.toBinary(p);
        }
      } else {
        yield def.toBinary(pkt);
      }
    }
  };
}

// ../../../node_modules/starpc/dist/srpc/packet.js
var decodePacketSource = buildDecodeMessageTransform(Packet);
var encodePacketSource = buildEncodeMessageTransform(Packet);
var uint32LEDecode = (data) => {
  if (data.length < 4) {
    throw RangeError("Could not decode int32BE");
  }
  return data.getUint32(0, true);
};
uint32LEDecode.bytes = 4;
var uint32LEEncode = (value) => {
  const data = new Uint8ArrayList(new Uint8Array(4));
  data.setUint32(0, value, true);
  return data;
};
uint32LEEncode.bytes = 4;
async function* lengthPrefixEncode(source, lengthEncoder) {
  for await (const chunk of source) {
    const length = chunk instanceof Uint8Array ? chunk.length : chunk.byteLength;
    const lengthEncoded = lengthEncoder(length);
    yield new Uint8ArrayList(lengthEncoded, chunk);
  }
}
async function* lengthPrefixDecode(source, lengthDecoder) {
  const buffer = new Uint8ArrayList();
  for await (const chunk of source) {
    buffer.append(chunk);
    while (buffer.length >= lengthDecoder.bytes) {
      const messageLength = lengthDecoder(buffer);
      const totalLength = lengthDecoder.bytes + messageLength;
      if (buffer.length < totalLength)
        break;
      const message = buffer.sublist(lengthDecoder.bytes, totalLength);
      yield message;
      buffer.consume(totalLength);
    }
  }
}
function prependLengthPrefixTransform(lengthEncoder = uint32LEEncode) {
  return (source) => {
    return lengthPrefixEncode(source, lengthEncoder);
  };
}
function parseLengthPrefixTransform(lengthDecoder = uint32LEDecode) {
  return (source) => {
    return lengthPrefixDecode(source, lengthDecoder);
  };
}

// ../../../node_modules/starpc/dist/srpc/value-ctr.js
var ValueCtr = class {
  // _value contains the current value.
  _value;
  // _waiters contains the list of waiters.
  // called when the value is set to any value other than undefined.
  _waiters;
  constructor(initialValue) {
    this._value = initialValue || void 0;
    this._waiters = [];
  }
  // value returns the current value.
  get value() {
    return this._value;
  }
  // wait waits for the value to not be undefined.
  async wait() {
    const currVal = this._value;
    if (currVal !== void 0) {
      return currVal;
    }
    return new Promise((resolve) => {
      this.waitWithCb((val) => {
        resolve(val);
      });
    });
  }
  // waitWithCb adds a callback to be called when the value is not undefined.
  waitWithCb(cb) {
    if (cb) {
      this._waiters.push(cb);
    }
  }
  // set sets the value and calls the callbacks.
  set(val) {
    this._value = val;
    if (val === void 0) {
      return;
    }
    const waiters = this._waiters;
    if (waiters.length === 0) {
      return;
    }
    this._waiters = [];
    for (const waiter of waiters) {
      waiter(val);
    }
  }
};

// ../../../node_modules/starpc/dist/srpc/open-stream-ctr.js
var OpenStreamCtr = class extends ValueCtr {
  constructor(openStreamFn) {
    super(openStreamFn);
  }
  // openStreamFunc returns an OpenStreamFunc which waits for the underlying OpenStreamFunc.
  get openStreamFunc() {
    return async () => {
      let openFn = this.value;
      if (!openFn) {
        openFn = await this.wait();
      }
      return openFn();
    };
  }
};

// ../../../node_modules/starpc/dist/srpc/client.js
var Client = class {
  // openStreamCtr contains the OpenStreamFunc.
  openStreamCtr;
  constructor(openStreamFn) {
    this.openStreamCtr = new OpenStreamCtr(openStreamFn || void 0);
  }
  // setOpenStreamFn updates the openStreamFn for the Client.
  setOpenStreamFn(openStreamFn) {
    this.openStreamCtr.set(openStreamFn || void 0);
  }
  // request starts a non-streaming request.
  async request(service, method, data, abortSignal) {
    const call = await this.startRpc(service, method, data, abortSignal);
    for await (const data2 of call.rpcDataSource) {
      call.close();
      return data2;
    }
    const err = new Error("empty response");
    call.close(err);
    throw err;
  }
  // clientStreamingRequest starts a client side streaming request.
  async clientStreamingRequest(service, method, data, abortSignal) {
    const call = await this.startRpc(service, method, null, abortSignal);
    call.writeCallDataFromSource(data).catch((err2) => call.close(err2));
    for await (const data2 of call.rpcDataSource) {
      call.close();
      return data2;
    }
    const err = new Error("empty response");
    call.close(err);
    throw err;
  }
  // serverStreamingRequest starts a server-side streaming request.
  serverStreamingRequest(service, method, data, abortSignal) {
    const serverData = pushable({ objectMode: true });
    this.startRpc(service, method, data, abortSignal).then(async (call) => writeToPushable(call.rpcDataSource, serverData)).catch((err) => serverData.end(err));
    return serverData;
  }
  // bidirectionalStreamingRequest starts a two-way streaming request.
  bidirectionalStreamingRequest(service, method, data, abortSignal) {
    const serverData = pushable({ objectMode: true });
    this.startRpc(service, method, null, abortSignal).then(async (call) => {
      const handleErr = (err) => {
        serverData.end(err);
        call.close(err);
      };
      call.writeCallDataFromSource(data).catch(handleErr);
      try {
        for await (const message of call.rpcDataSource) {
          serverData.push(message);
        }
        serverData.end();
        call.close();
      } catch (err) {
        handleErr(err);
      }
    }).catch((err) => serverData.end(err));
    return serverData;
  }
  // startRpc is a common utility function to begin a rpc call.
  // throws any error starting the rpc call
  // if data == null and data.length == 0, sends a separate data packet.
  async startRpc(rpcService, rpcMethod, data, abortSignal) {
    if (abortSignal?.aborted) {
      throw new Error(ERR_RPC_ABORT);
    }
    const openStreamFn = await this.openStreamCtr.wait();
    const stream = await openStreamFn();
    const call = new ClientRPC(rpcService, rpcMethod);
    abortSignal?.addEventListener("abort", () => {
      call.writeCallCancel();
      call.close(new Error(ERR_RPC_ABORT));
    });
    pipe(stream, decodePacketSource, call, encodePacketSource, stream).catch((err) => call.close(err)).then(() => call.close());
    await call.writeCallStart(data ?? void 0);
    return call;
  }
};

// ../../../node_modules/@libp2p/interface/dist/src/errors.js
var AbortError2 = class extends Error {
  static name = "AbortError";
  constructor(message = "The operation was aborted") {
    super(message);
    this.name = "AbortError";
  }
};
var InvalidParametersError = class extends Error {
  static name = "InvalidParametersError";
  constructor(message = "Invalid parameters") {
    super(message);
    this.name = "InvalidParametersError";
  }
};
var MuxerClosedError = class extends Error {
  static name = "MuxerClosedError";
  constructor(message = "The muxer is closed") {
    super(message);
    this.name = "MuxerClosedError";
  }
};
var StreamResetError = class extends Error {
  static name = "StreamResetError";
  constructor(message = "The stream has been reset") {
    super(message);
    this.name = "StreamResetError";
  }
};
var StreamStateError = class extends Error {
  static name = "StreamStateError";
  constructor(message = "The stream is in an invalid state") {
    super(message);
    this.name = "StreamStateError";
  }
};
var TooManyOutboundProtocolStreamsError = class extends Error {
  static name = "TooManyOutboundProtocolStreamsError";
  constructor(message = "Too many outbound protocol streams") {
    super(message);
    this.name = "TooManyOutboundProtocolStreamsError";
  }
};

// ../../../node_modules/main-event/dist/src/events.browser.js
function setMaxListeners() {
}

// ../../../node_modules/@libp2p/interface/dist/src/index.js
var serviceCapabilities = /* @__PURE__ */ Symbol.for("@libp2p/service-capabilities");

// ../../../node_modules/get-iterator/dist/src/index.js
function getIterator(obj) {
  if (obj != null) {
    if (typeof obj[Symbol.iterator] === "function") {
      return obj[Symbol.iterator]();
    }
    if (typeof obj[Symbol.asyncIterator] === "function") {
      return obj[Symbol.asyncIterator]();
    }
    if (typeof obj.next === "function") {
      return obj;
    }
  }
  throw new Error("argument is not an iterator or iterable");
}

// ../../../node_modules/race-signal/dist/src/index.js
var AbortError3 = class extends Error {
  type;
  code;
  constructor(message, code, name) {
    super(message ?? "The operation was aborted");
    this.type = "aborted";
    this.name = name ?? "AbortError";
    this.code = code ?? "ABORT_ERR";
  }
};
async function raceSignal2(promise, signal, opts) {
  if (signal == null) {
    return promise;
  }
  if (signal.aborted) {
    promise.catch(() => {
    });
    return Promise.reject(new AbortError3(opts?.errorMessage, opts?.errorCode, opts?.errorName));
  }
  let listener;
  const error = new AbortError3(opts?.errorMessage, opts?.errorCode, opts?.errorName);
  try {
    return await Promise.race([
      promise,
      new Promise((resolve, reject) => {
        listener = () => {
          reject(error);
        };
        signal.addEventListener("abort", listener);
      })
    ]);
  } finally {
    if (listener != null) {
      signal.removeEventListener("abort", listener);
    }
  }
}

// ../../../node_modules/@chainsafe/libp2p-yamux/dist/src/errors.js
var InvalidFrameError = class extends Error {
  static name = "InvalidFrameError";
  constructor(message = "The frame was invalid") {
    super(message);
    this.name = "InvalidFrameError";
  }
};
var UnrequestedPingError = class extends Error {
  static name = "UnrequestedPingError";
  constructor(message = "Unrequested ping error") {
    super(message);
    this.name = "UnrequestedPingError";
  }
};
var NotMatchingPingError = class extends Error {
  static name = "NotMatchingPingError";
  constructor(message = "Unrequested ping error") {
    super(message);
    this.name = "NotMatchingPingError";
  }
};
var InvalidStateError = class extends Error {
  static name = "InvalidStateError";
  constructor(message = "Invalid state") {
    super(message);
    this.name = "InvalidStateError";
  }
};
var StreamAlreadyExistsError = class extends Error {
  static name = "StreamAlreadyExistsError";
  constructor(message = "Strean already exists") {
    super(message);
    this.name = "StreamAlreadyExistsError";
  }
};
var DecodeInvalidVersionError = class extends Error {
  static name = "DecodeInvalidVersionError";
  constructor(message = "Decode invalid version") {
    super(message);
    this.name = "DecodeInvalidVersionError";
  }
};
var BothClientsError = class extends Error {
  static name = "BothClientsError";
  constructor(message = "Both clients") {
    super(message);
    this.name = "BothClientsError";
  }
};
var ReceiveWindowExceededError = class extends Error {
  static name = "ReceiveWindowExceededError";
  constructor(message = "Receive window exceeded") {
    super(message);
    this.name = "ReceiveWindowExceededError";
  }
};

// ../../../node_modules/@chainsafe/libp2p-yamux/dist/src/constants.js
var PROTOCOL_ERRORS = /* @__PURE__ */ new Set([
  InvalidFrameError.name,
  UnrequestedPingError.name,
  NotMatchingPingError.name,
  StreamAlreadyExistsError.name,
  DecodeInvalidVersionError.name,
  BothClientsError.name,
  ReceiveWindowExceededError.name
]);
var INITIAL_STREAM_WINDOW = 256 * 1024;
var MAX_STREAM_WINDOW = 16 * 1024 * 1024;

// ../../../node_modules/@chainsafe/libp2p-yamux/dist/src/config.js
var defaultConfig = {
  enableKeepAlive: true,
  keepAliveInterval: 3e4,
  maxInboundStreams: 1e3,
  maxOutboundStreams: 1e3,
  initialStreamWindowSize: INITIAL_STREAM_WINDOW,
  maxStreamWindowSize: MAX_STREAM_WINDOW,
  maxMessageSize: 64 * 1024
};
function verifyConfig(config) {
  if (config.keepAliveInterval <= 0) {
    throw new InvalidParametersError("keep-alive interval must be positive");
  }
  if (config.maxInboundStreams < 0) {
    throw new InvalidParametersError("max inbound streams must be larger or equal 0");
  }
  if (config.maxOutboundStreams < 0) {
    throw new InvalidParametersError("max outbound streams must be larger or equal 0");
  }
  if (config.initialStreamWindowSize < INITIAL_STREAM_WINDOW) {
    throw new InvalidParametersError("InitialStreamWindowSize must be larger or equal 256 kB");
  }
  if (config.maxStreamWindowSize < config.initialStreamWindowSize) {
    throw new InvalidParametersError("MaxStreamWindowSize must be larger than the InitialStreamWindowSize");
  }
  if (config.maxStreamWindowSize > 2 ** 32 - 1) {
    throw new InvalidParametersError("MaxStreamWindowSize must be less than equal MAX_UINT32");
  }
  if (config.maxMessageSize < 1024) {
    throw new InvalidParametersError("MaxMessageSize must be greater than a kilobyte");
  }
}

// ../../../node_modules/@chainsafe/libp2p-yamux/dist/src/frame.js
var FrameType;
(function(FrameType2) {
  FrameType2[FrameType2["Data"] = 0] = "Data";
  FrameType2[FrameType2["WindowUpdate"] = 1] = "WindowUpdate";
  FrameType2[FrameType2["Ping"] = 2] = "Ping";
  FrameType2[FrameType2["GoAway"] = 3] = "GoAway";
})(FrameType || (FrameType = {}));
var Flag;
(function(Flag2) {
  Flag2[Flag2["SYN"] = 1] = "SYN";
  Flag2[Flag2["ACK"] = 2] = "ACK";
  Flag2[Flag2["FIN"] = 4] = "FIN";
  Flag2[Flag2["RST"] = 8] = "RST";
})(Flag || (Flag = {}));
Object.values(Flag).filter((x) => typeof x !== "string");
var YAMUX_VERSION = 0;
var GoAwayCode;
(function(GoAwayCode2) {
  GoAwayCode2[GoAwayCode2["NormalTermination"] = 0] = "NormalTermination";
  GoAwayCode2[GoAwayCode2["ProtocolError"] = 1] = "ProtocolError";
  GoAwayCode2[GoAwayCode2["InternalError"] = 2] = "InternalError";
})(GoAwayCode || (GoAwayCode = {}));
var HEADER_LENGTH = 12;

// ../../../node_modules/@chainsafe/libp2p-yamux/dist/src/decode.js
var twoPow24 = 2 ** 24;
function decodeHeader(data) {
  if (data[0] !== YAMUX_VERSION) {
    throw new InvalidFrameError("Invalid frame version");
  }
  return {
    type: data[1],
    flag: (data[2] << 8) + data[3],
    streamID: data[4] * twoPow24 + (data[5] << 16) + (data[6] << 8) + data[7],
    length: data[8] * twoPow24 + (data[9] << 16) + (data[10] << 8) + data[11]
  };
}
var Decoder = class {
  source;
  /** Buffer for in-progress frames */
  buffer;
  /** Used to sanity check against decoding while in an inconsistent state */
  frameInProgress;
  constructor(source) {
    this.source = returnlessSource(source);
    this.buffer = new Uint8ArrayList();
    this.frameInProgress = false;
  }
  /**
   * Emits frames from the decoder source.
   *
   * Note: If `readData` is emitted, it _must_ be called before the next iteration
   * Otherwise an error is thrown
   */
  async *emitFrames() {
    for await (const chunk of this.source) {
      this.buffer.append(chunk);
      while (true) {
        const header = this.readHeader();
        if (header === void 0) {
          break;
        }
        const { type, length } = header;
        if (type === FrameType.Data) {
          this.frameInProgress = true;
          yield {
            header,
            readData: this.readBytes.bind(this, length)
          };
        } else {
          yield { header };
        }
      }
    }
  }
  readHeader() {
    if (this.frameInProgress) {
      throw new InvalidStateError("decoding frame already in progress");
    }
    if (this.buffer.length < HEADER_LENGTH) {
      return;
    }
    const header = decodeHeader(this.buffer.subarray(0, HEADER_LENGTH));
    this.buffer.consume(HEADER_LENGTH);
    return header;
  }
  async readBytes(length) {
    if (this.buffer.length < length) {
      for await (const chunk of this.source) {
        this.buffer.append(chunk);
        if (this.buffer.length >= length) {
          break;
        }
      }
    }
    const out = this.buffer.sublist(0, length);
    this.buffer.consume(length);
    this.frameInProgress = false;
    return out;
  }
};
function returnlessSource(source) {
  if (source[Symbol.iterator] !== void 0) {
    const iterator = source[Symbol.iterator]();
    iterator.return = void 0;
    return {
      [Symbol.iterator]() {
        return iterator;
      }
    };
  } else if (source[Symbol.asyncIterator] !== void 0) {
    const iterator = source[Symbol.asyncIterator]();
    iterator.return = void 0;
    return {
      [Symbol.asyncIterator]() {
        return iterator;
      }
    };
  } else {
    throw new Error("a source must be either an iterable or an async iterable");
  }
}

// ../../../node_modules/@chainsafe/libp2p-yamux/dist/src/encode.js
function encodeHeader(header) {
  const frame = new Uint8Array(HEADER_LENGTH);
  frame[1] = header.type;
  frame[2] = header.flag >>> 8;
  frame[3] = header.flag;
  frame[4] = header.streamID >>> 24;
  frame[5] = header.streamID >>> 16;
  frame[6] = header.streamID >>> 8;
  frame[7] = header.streamID;
  frame[8] = header.length >>> 24;
  frame[9] = header.length >>> 16;
  frame[10] = header.length >>> 8;
  frame[11] = header.length;
  return frame;
}

// ../../../node_modules/@libp2p/utils/dist/src/is-promise.js
function isPromise(thing) {
  if (thing == null) {
    return false;
  }
  return typeof thing.then === "function" && typeof thing.catch === "function" && typeof thing.finally === "function";
}

// ../../../node_modules/@libp2p/utils/dist/src/close-source.js
function closeSource(source, log) {
  const res = getIterator(source).return?.();
  if (isPromise(res)) {
    res.catch((err) => {
      log.error("could not cause iterator to return", err);
    });
  }
}

// ../../../node_modules/@libp2p/utils/dist/src/abstract-stream.js
var DEFAULT_SEND_CLOSE_WRITE_TIMEOUT = 5e3;
function isPromise2(thing) {
  if (thing == null) {
    return false;
  }
  return typeof thing.then === "function" && typeof thing.catch === "function" && typeof thing.finally === "function";
}
var AbstractStream = class {
  id;
  direction;
  timeline;
  protocol;
  metadata;
  source;
  status;
  readStatus;
  writeStatus;
  log;
  sinkController;
  sinkEnd;
  closed;
  endErr;
  streamSource;
  onEnd;
  onCloseRead;
  onCloseWrite;
  onReset;
  onAbort;
  sendCloseWriteTimeout;
  sendingData;
  constructor(init) {
    this.sinkController = new AbortController();
    this.sinkEnd = pDefer();
    this.closed = pDefer();
    this.log = init.log;
    this.status = "open";
    this.readStatus = "ready";
    this.writeStatus = "ready";
    this.id = init.id;
    this.metadata = init.metadata ?? {};
    this.direction = init.direction;
    this.timeline = {
      open: Date.now()
    };
    this.sendCloseWriteTimeout = init.sendCloseWriteTimeout ?? DEFAULT_SEND_CLOSE_WRITE_TIMEOUT;
    this.onEnd = init.onEnd;
    this.onCloseRead = init.onCloseRead;
    this.onCloseWrite = init.onCloseWrite;
    this.onReset = init.onReset;
    this.onAbort = init.onAbort;
    this.source = this.streamSource = pushable({
      onEnd: (err) => {
        if (err != null) {
          this.log.trace("source ended with error", err);
        } else {
          this.log.trace("source ended");
        }
        this.onSourceEnd(err);
      }
    });
    this.sink = this.sink.bind(this);
  }
  async sink(source) {
    if (this.writeStatus !== "ready") {
      throw new StreamStateError(`writable end state is "${this.writeStatus}" not "ready"`);
    }
    try {
      this.writeStatus = "writing";
      const options = {
        signal: this.sinkController.signal
      };
      if (this.direction === "outbound") {
        const res = this.sendNewStream(options);
        if (isPromise2(res)) {
          await res;
        }
      }
      const abortListener = () => {
        closeSource(source, this.log);
      };
      try {
        this.sinkController.signal.addEventListener("abort", abortListener);
        this.log.trace("sink reading from source");
        for await (let data of source) {
          data = data instanceof Uint8Array ? new Uint8ArrayList(data) : data;
          const res = this.sendData(data, options);
          if (isPromise2(res)) {
            this.sendingData = pDefer();
            await res;
            this.sendingData.resolve();
            this.sendingData = void 0;
          }
        }
      } finally {
        this.sinkController.signal.removeEventListener("abort", abortListener);
      }
      this.log.trace('sink finished reading from source, write status is "%s"', this.writeStatus);
      if (this.writeStatus === "writing") {
        this.writeStatus = "closing";
        this.log.trace("send close write to remote");
        await this.sendCloseWrite({
          signal: AbortSignal.timeout(this.sendCloseWriteTimeout)
        });
        this.writeStatus = "closed";
      }
      this.onSinkEnd();
    } catch (err) {
      this.log.trace("sink ended with error, calling abort with error", err);
      this.abort(err);
      throw err;
    } finally {
      this.log.trace("resolve sink end");
      this.sinkEnd.resolve();
    }
  }
  onSourceEnd(err) {
    if (this.timeline.closeRead != null) {
      return;
    }
    this.timeline.closeRead = Date.now();
    this.readStatus = "closed";
    if (err != null && this.endErr == null) {
      this.endErr = err;
    }
    this.onCloseRead?.();
    if (this.timeline.closeWrite != null) {
      this.log.trace("source and sink ended");
      this.timeline.close = Date.now();
      if (this.status !== "aborted" && this.status !== "reset") {
        this.status = "closed";
      }
      if (this.onEnd != null) {
        this.onEnd(this.endErr);
      }
      this.closed.resolve();
    } else {
      this.log.trace("source ended, waiting for sink to end");
    }
  }
  onSinkEnd(err) {
    if (this.timeline.closeWrite != null) {
      return;
    }
    this.timeline.closeWrite = Date.now();
    this.writeStatus = "closed";
    if (err != null && this.endErr == null) {
      this.endErr = err;
    }
    this.onCloseWrite?.();
    if (this.timeline.closeRead != null) {
      this.log.trace("sink and source ended");
      this.timeline.close = Date.now();
      if (this.status !== "aborted" && this.status !== "reset") {
        this.status = "closed";
      }
      if (this.onEnd != null) {
        this.onEnd(this.endErr);
      }
      this.closed.resolve();
    } else {
      this.log.trace("sink ended, waiting for source to end");
    }
  }
  // Close for both Reading and Writing
  async close(options) {
    if (this.status !== "open") {
      return;
    }
    this.log.trace("closing gracefully");
    this.status = "closing";
    await raceSignal2(Promise.all([
      this.closeWrite(options),
      this.closeRead(options),
      this.closed.promise
    ]), options?.signal);
    this.status = "closed";
    this.log.trace("closed gracefully");
  }
  async closeRead(options = {}) {
    if (this.readStatus === "closing" || this.readStatus === "closed") {
      return;
    }
    this.log.trace('closing readable end of stream with starting read status "%s"', this.readStatus);
    const readStatus = this.readStatus;
    this.readStatus = "closing";
    if (this.status !== "reset" && this.status !== "aborted" && this.timeline.closeRead == null) {
      this.log.trace("send close read to remote");
      await this.sendCloseRead(options);
    }
    if (readStatus === "ready") {
      this.log.trace("ending internal source queue with %d queued bytes", this.streamSource.readableLength);
      this.streamSource.end();
    }
    this.log.trace("closed readable end of stream");
  }
  async closeWrite(options = {}) {
    if (this.writeStatus === "closing" || this.writeStatus === "closed") {
      return;
    }
    this.log.trace('closing writable end of stream with starting write status "%s"', this.writeStatus);
    if (this.writeStatus === "ready") {
      this.log.trace("sink was never sunk, sink an empty array");
      await raceSignal2(this.sink([]), options.signal);
    }
    if (this.writeStatus === "writing") {
      if (this.sendingData != null) {
        await raceSignal2(this.sendingData.promise, options.signal);
      }
      this.log.trace("aborting source passed to .sink");
      this.sinkController.abort();
      await raceSignal2(this.sinkEnd.promise, options.signal);
    }
    this.writeStatus = "closed";
    this.log.trace("closed writable end of stream");
  }
  /**
   * Close immediately for reading and writing and send a reset message (local
   * error)
   */
  abort(err) {
    if (this.status === "closed" || this.status === "aborted" || this.status === "reset") {
      return;
    }
    this.log("abort with error", err);
    this.log("try to send reset to remote");
    const res = this.sendReset();
    if (isPromise2(res)) {
      res.catch((err2) => {
        this.log.error("error sending reset message", err2);
      });
    }
    this.status = "aborted";
    this.timeline.abort = Date.now();
    this._closeSinkAndSource(err);
    this.onAbort?.(err);
  }
  /**
   * Receive a reset message - close immediately for reading and writing (remote
   * error)
   */
  reset() {
    if (this.status === "closed" || this.status === "aborted" || this.status === "reset") {
      return;
    }
    const err = new StreamResetError("stream reset");
    this.status = "reset";
    this.timeline.reset = Date.now();
    this._closeSinkAndSource(err);
    this.onReset?.();
  }
  _closeSinkAndSource(err) {
    this._closeSink(err);
    this._closeSource(err);
  }
  _closeSink(err) {
    if (this.writeStatus === "writing") {
      this.log.trace("end sink source");
      this.sinkController.abort();
    }
    this.onSinkEnd(err);
  }
  _closeSource(err) {
    if (this.readStatus !== "closing" && this.readStatus !== "closed") {
      this.log.trace("ending source with %d bytes to be read by consumer", this.streamSource.readableLength);
      this.readStatus = "closing";
      this.streamSource.end(err);
    }
  }
  /**
   * The remote closed for writing so we should expect to receive no more
   * messages
   */
  remoteCloseWrite() {
    if (this.readStatus === "closing" || this.readStatus === "closed") {
      this.log("received remote close write but local source is already closed");
      return;
    }
    this.log.trace("remote close write");
    this._closeSource();
  }
  /**
   * The remote closed for reading so we should not send any more
   * messages
   */
  remoteCloseRead() {
    if (this.writeStatus === "closing" || this.writeStatus === "closed") {
      this.log("received remote close read but local sink is already closed");
      return;
    }
    this.log.trace("remote close read");
    this._closeSink();
  }
  /**
   * The underlying muxer has closed, no more messages can be sent or will
   * be received, close immediately to free up resources
   */
  destroy() {
    if (this.status === "closed" || this.status === "aborted" || this.status === "reset") {
      this.log("received destroy but we are already closed");
      return;
    }
    this.log.trace("stream destroyed");
    this._closeSinkAndSource();
  }
  /**
   * When an extending class reads data from it's implementation-specific source,
   * call this method to allow the stream consumer to read the data.
   */
  sourcePush(data) {
    this.streamSource.push(data);
  }
  /**
   * Returns the amount of unread data - can be used to prevent large amounts of
   * data building up when the stream consumer is too slow.
   */
  sourceReadableLength() {
    return this.streamSource.readableLength;
  }
};

// ../../../node_modules/it-peekable/dist/src/index.js
function peekable(iterable) {
  const [iterator, symbol2] = iterable[Symbol.asyncIterator] != null ? [iterable[Symbol.asyncIterator](), Symbol.asyncIterator] : [iterable[Symbol.iterator](), Symbol.iterator];
  const queue = [];
  return {
    peek: () => {
      return iterator.next();
    },
    push: (value) => {
      queue.push(value);
    },
    next: () => {
      if (queue.length > 0) {
        return {
          done: false,
          value: queue.shift()
        };
      }
      return iterator.next();
    },
    [symbol2]() {
      return this;
    }
  };
}
var src_default2 = peekable;

// ../../../node_modules/it-foreach/dist/src/index.js
function isAsyncIterable3(thing) {
  return thing[Symbol.asyncIterator] != null;
}
function isPromise3(thing) {
  return thing?.then != null;
}
function forEach(source, fn) {
  let index = 0;
  if (isAsyncIterable3(source)) {
    return (async function* () {
      for await (const val of source) {
        const res2 = fn(val, index++);
        if (isPromise3(res2)) {
          await res2;
        }
        yield val;
      }
    })();
  }
  const peekable2 = src_default2(source);
  const { value, done } = peekable2.next();
  if (done === true) {
    return (function* () {
    })();
  }
  const res = fn(value, index++);
  if (typeof res?.then === "function") {
    return (async function* () {
      await res;
      yield value;
      for (const val of peekable2) {
        const res2 = fn(val, index++);
        if (isPromise3(res2)) {
          await res2;
        }
        yield val;
      }
    })();
  }
  const func = fn;
  return (function* () {
    yield value;
    for (const val of peekable2) {
      func(val, index++);
      yield val;
    }
  })();
}
var src_default3 = forEach;

// ../../../node_modules/@chainsafe/libp2p-yamux/dist/src/stream.js
var StreamState;
(function(StreamState2) {
  StreamState2[StreamState2["Init"] = 0] = "Init";
  StreamState2[StreamState2["SYNSent"] = 1] = "SYNSent";
  StreamState2[StreamState2["SYNReceived"] = 2] = "SYNReceived";
  StreamState2[StreamState2["Established"] = 3] = "Established";
  StreamState2[StreamState2["Finished"] = 4] = "Finished";
})(StreamState || (StreamState = {}));
var YamuxStream = class extends AbstractStream {
  name;
  state;
  config;
  _id;
  /** The number of available bytes to send */
  sendWindowCapacity;
  /** Callback to notify that the sendWindowCapacity has been updated */
  sendWindowCapacityUpdate;
  /** The number of bytes available to receive in a full window */
  recvWindow;
  /** The number of available bytes to receive */
  recvWindowCapacity;
  /**
   * An 'epoch' is the time it takes to process and read data
   *
   * Used in conjunction with RTT to determine whether to increase the recvWindow
   */
  epochStart;
  getRTT;
  sendFrame;
  constructor(init) {
    super({
      ...init,
      onEnd: (err) => {
        this.state = StreamState.Finished;
        init.onEnd?.(err);
      }
    });
    this.config = init.config;
    this._id = parseInt(init.id, 10);
    this.name = init.name;
    this.state = init.state;
    this.sendWindowCapacity = INITIAL_STREAM_WINDOW;
    this.recvWindow = this.config.initialStreamWindowSize;
    this.recvWindowCapacity = this.recvWindow;
    this.epochStart = Date.now();
    this.getRTT = init.getRTT;
    this.sendFrame = init.sendFrame;
    this.source = src_default3(this.source, () => {
      this.sendWindowUpdate();
    });
  }
  /**
   * Send a message to the remote muxer informing them a new stream is being
   * opened.
   *
   * This is a noop for Yamux because the first window update is sent when
   * .newStream is called on the muxer which opens the stream on the remote.
   */
  async sendNewStream() {
  }
  /**
   * Send a data message to the remote muxer
   */
  async sendData(buf, options = {}) {
    buf = buf.sublist();
    while (buf.byteLength !== 0) {
      if (this.sendWindowCapacity === 0) {
        this.log?.trace("wait for send window capacity, status %s", this.status);
        await this.waitForSendWindowCapacity(options);
        if (this.status === "closed" || this.status === "aborted" || this.status === "reset") {
          this.log?.trace("%s while waiting for send window capacity", this.status);
          return;
        }
      }
      const toSend = Math.min(this.sendWindowCapacity, this.config.maxMessageSize - HEADER_LENGTH, buf.length);
      const flags = this.getSendFlags();
      this.sendFrame({
        type: FrameType.Data,
        flag: flags,
        streamID: this._id,
        length: toSend
      }, buf.sublist(0, toSend));
      this.sendWindowCapacity -= toSend;
      buf.consume(toSend);
    }
  }
  /**
   * Send a reset message to the remote muxer
   */
  async sendReset() {
    this.sendFrame({
      type: FrameType.WindowUpdate,
      flag: Flag.RST,
      streamID: this._id,
      length: 0
    });
  }
  /**
   * Send a message to the remote muxer, informing them no more data messages
   * will be sent by this end of the stream
   */
  async sendCloseWrite() {
    const flags = this.getSendFlags() | Flag.FIN;
    this.sendFrame({
      type: FrameType.WindowUpdate,
      flag: flags,
      streamID: this._id,
      length: 0
    });
  }
  /**
   * Send a message to the remote muxer, informing them no more data messages
   * will be read by this end of the stream
   */
  async sendCloseRead() {
  }
  /**
   * Wait for the send window to be non-zero
   *
   * Will throw with ERR_STREAM_ABORT if the stream gets aborted
   */
  async waitForSendWindowCapacity(options = {}) {
    if (this.sendWindowCapacity > 0) {
      return;
    }
    let resolve;
    let reject;
    const abort = () => {
      if (this.status === "open" || this.status === "closing") {
        reject(new AbortError2("Stream aborted"));
      } else {
        resolve();
      }
    };
    options.signal?.addEventListener("abort", abort);
    try {
      await new Promise((_resolve, _reject) => {
        this.sendWindowCapacityUpdate = () => {
          _resolve();
        };
        reject = _reject;
        resolve = _resolve;
      });
    } finally {
      options.signal?.removeEventListener("abort", abort);
    }
  }
  /**
   * handleWindowUpdate is called when the stream receives a window update frame
   */
  handleWindowUpdate(header) {
    this.log?.trace("stream received window update id=%s", this._id);
    this.processFlags(header.flag);
    const available = this.sendWindowCapacity;
    this.sendWindowCapacity += header.length;
    if (available === 0 && header.length > 0) {
      this.sendWindowCapacityUpdate?.();
    }
  }
  /**
   * handleData is called when the stream receives a data frame
   */
  async handleData(header, readData) {
    this.log?.trace("stream received data id=%s", this._id);
    this.processFlags(header.flag);
    if (this.recvWindowCapacity < header.length) {
      throw new ReceiveWindowExceededError("Receive window exceeded");
    }
    const data = await readData();
    this.recvWindowCapacity -= header.length;
    this.sourcePush(data);
  }
  /**
   * processFlags is used to update the state of the stream based on set flags, if any.
   */
  processFlags(flags) {
    if ((flags & Flag.ACK) === Flag.ACK) {
      if (this.state === StreamState.SYNSent) {
        this.state = StreamState.Established;
      }
    }
    if ((flags & Flag.FIN) === Flag.FIN) {
      this.remoteCloseWrite();
    }
    if ((flags & Flag.RST) === Flag.RST) {
      this.reset();
    }
  }
  /**
   * getSendFlags determines any flags that are appropriate
   * based on the current stream state.
   *
   * The state is updated as a side-effect.
   */
  getSendFlags() {
    switch (this.state) {
      case StreamState.Init:
        this.state = StreamState.SYNSent;
        return Flag.SYN;
      case StreamState.SYNReceived:
        this.state = StreamState.Established;
        return Flag.ACK;
      default:
        return 0;
    }
  }
  /**
   * potentially sends a window update enabling further writes to take place.
   */
  sendWindowUpdate() {
    const flags = this.getSendFlags();
    const now = Date.now();
    const rtt = this.getRTT();
    if (flags === 0 && rtt > -1 && now - this.epochStart < rtt * 4) {
      this.recvWindow = Math.min(this.recvWindow * 2, this.config.maxStreamWindowSize);
    }
    if (this.recvWindowCapacity >= this.recvWindow && flags === 0) {
      return;
    }
    const delta = this.recvWindow - this.recvWindowCapacity;
    this.recvWindowCapacity = this.recvWindow;
    this.epochStart = now;
    this.sendFrame({
      type: FrameType.WindowUpdate,
      flag: flags,
      streamID: this._id,
      length: delta
    });
  }
};

// ../../../node_modules/@chainsafe/libp2p-yamux/dist/src/muxer.js
var YAMUX_PROTOCOL_ID = "/yamux/1.0.0";
var CLOSE_TIMEOUT = 500;
var Yamux = class {
  protocol = YAMUX_PROTOCOL_ID;
  _components;
  _init;
  constructor(components, init = {}) {
    this._components = components;
    this._init = init;
  }
  [Symbol.toStringTag] = "@chainsafe/libp2p-yamux";
  [serviceCapabilities] = [
    "@libp2p/stream-multiplexing"
  ];
  createStreamMuxer(init) {
    return new YamuxMuxer(this._components, {
      ...this._init,
      ...init
    });
  }
};
var YamuxMuxer = class {
  protocol = YAMUX_PROTOCOL_ID;
  source;
  sink;
  config;
  log;
  logger;
  /** Used to close the muxer from either the sink or source */
  closeController;
  /** The next stream id to be used when initiating a new stream */
  nextStreamID;
  /** Primary stream mapping, streamID => stream */
  _streams;
  /** The next ping id to be used when pinging */
  nextPingID;
  /** Tracking info for the currently active ping */
  activePing;
  /** Round trip time */
  rtt;
  /** True if client, false if server */
  client;
  localGoAway;
  remoteGoAway;
  /** Number of tracked inbound streams */
  numInboundStreams;
  /** Number of tracked outbound streams */
  numOutboundStreams;
  onIncomingStream;
  onStreamEnd;
  constructor(components, init) {
    this.client = init.direction === "outbound";
    this.config = { ...defaultConfig, ...init };
    this.logger = components.logger;
    this.log = this.logger.forComponent("libp2p:yamux");
    verifyConfig(this.config);
    this.closeController = new AbortController();
    setMaxListeners(Infinity, this.closeController.signal);
    this.onIncomingStream = init.onIncomingStream;
    this.onStreamEnd = init.onStreamEnd;
    this._streams = /* @__PURE__ */ new Map();
    this.source = pushable({
      onEnd: () => {
        this.log?.trace("muxer source ended");
        this._streams.forEach((stream) => {
          stream.destroy();
        });
      }
    });
    this.sink = async (source) => {
      const shutDownListener = () => {
        const iterator = getIterator(source);
        if (iterator.return != null) {
          const res = iterator.return();
          if (isPromise4(res)) {
            res.catch((err) => {
              this.log?.("could not cause sink source to return", err);
            });
          }
        }
      };
      let reason, error;
      try {
        const decoder = new Decoder(source);
        try {
          this.closeController.signal.addEventListener("abort", shutDownListener);
          for await (const frame of decoder.emitFrames()) {
            await this.handleFrame(frame.header, frame.readData);
          }
        } finally {
          this.closeController.signal.removeEventListener("abort", shutDownListener);
        }
        reason = GoAwayCode.NormalTermination;
      } catch (err) {
        if (PROTOCOL_ERRORS.has(err.name)) {
          this.log?.error("protocol error in sink", err);
          reason = GoAwayCode.ProtocolError;
        } else {
          this.log?.error("internal error in sink", err);
          reason = GoAwayCode.InternalError;
        }
        error = err;
      }
      this.log?.trace("muxer sink ended");
      if (error != null) {
        this.abort(error, reason);
      } else {
        await this.close({ reason });
      }
    };
    this.numInboundStreams = 0;
    this.numOutboundStreams = 0;
    this.nextStreamID = this.client ? 1 : 2;
    this.nextPingID = 0;
    this.rtt = -1;
    this.log?.trace("muxer created");
    if (this.config.enableKeepAlive) {
      this.keepAliveLoop().catch((e) => this.log?.error("keepalive error: %s", e));
    }
    this.ping().catch((e) => this.log?.error("ping error: %s", e));
  }
  get streams() {
    return Array.from(this._streams.values());
  }
  newStream(name) {
    if (this.remoteGoAway !== void 0) {
      throw new MuxerClosedError("Muxer closed remotely");
    }
    if (this.localGoAway !== void 0) {
      throw new MuxerClosedError("Muxer closed locally");
    }
    const id = this.nextStreamID;
    this.nextStreamID += 2;
    if (this.numOutboundStreams >= this.config.maxOutboundStreams) {
      throw new TooManyOutboundProtocolStreamsError("max outbound streams exceeded");
    }
    this.log?.trace("new outgoing stream id=%s", id);
    const stream = this._newStream(id, name, StreamState.Init, "outbound");
    this._streams.set(id, stream);
    this.numOutboundStreams++;
    stream.sendWindowUpdate();
    return stream;
  }
  /**
   * Initiate a ping and wait for a response
   *
   * Note: only a single ping will be initiated at a time.
   * If a ping is already in progress, a new ping will not be initiated.
   *
   * @returns the round-trip-time in milliseconds
   */
  async ping() {
    if (this.remoteGoAway !== void 0) {
      throw new MuxerClosedError("Muxer closed remotely");
    }
    if (this.localGoAway !== void 0) {
      throw new MuxerClosedError("Muxer closed locally");
    }
    if (this.activePing === void 0) {
      let _resolve = () => {
      };
      this.activePing = {
        id: this.nextPingID++,
        // this promise awaits resolution or the close controller aborting
        promise: new Promise((resolve, reject) => {
          const closed = () => {
            reject(new MuxerClosedError("Muxer closed locally"));
          };
          this.closeController.signal.addEventListener("abort", closed, { once: true });
          _resolve = () => {
            this.closeController.signal.removeEventListener("abort", closed);
            resolve();
          };
        }),
        resolve: _resolve
      };
      const start = Date.now();
      this.sendPing(this.activePing.id);
      try {
        await this.activePing.promise;
      } finally {
        delete this.activePing;
      }
      const end = Date.now();
      this.rtt = end - start;
    } else {
      await this.activePing.promise;
    }
    return this.rtt;
  }
  /**
   * Get the ping round trip time
   *
   * Note: Will return 0 if no successful ping has yet been completed
   *
   * @returns the round-trip-time in milliseconds
   */
  getRTT() {
    return this.rtt;
  }
  /**
   * Close the muxer
   */
  async close(options = {}) {
    if (this.closeController.signal.aborted) {
      return;
    }
    const reason = options?.reason ?? GoAwayCode.NormalTermination;
    this.log?.trace("muxer close reason=%s", reason);
    if (options.signal == null) {
      const signal = AbortSignal.timeout(CLOSE_TIMEOUT);
      options = {
        ...options,
        signal
      };
    }
    try {
      await Promise.all([...this._streams.values()].map(async (s) => s.close(options)));
      this.sendGoAway(reason);
      this._closeMuxer();
    } catch (err) {
      this.abort(err);
    }
  }
  abort(err, reason) {
    if (this.closeController.signal.aborted) {
      return;
    }
    reason = reason ?? GoAwayCode.InternalError;
    this.log?.error("muxer abort reason=%s error=%s", reason, err);
    for (const stream of this._streams.values()) {
      stream.abort(err);
    }
    this.sendGoAway(reason);
    this._closeMuxer();
  }
  isClosed() {
    return this.closeController.signal.aborted;
  }
  /**
   * Called when either the local or remote shuts down the muxer
   */
  _closeMuxer() {
    this.closeController.abort();
    this.source.end();
  }
  /** Create a new stream */
  _newStream(id, name, state, direction) {
    if (this._streams.get(id) != null) {
      throw new InvalidParametersError("Stream already exists with that id");
    }
    const stream = new YamuxStream({
      id: id.toString(),
      name,
      state,
      direction,
      sendFrame: this.sendFrame.bind(this),
      onEnd: () => {
        this.closeStream(id);
        this.onStreamEnd?.(stream);
      },
      log: this.logger.forComponent(`libp2p:yamux:${direction}:${id}`),
      config: this.config,
      getRTT: this.getRTT.bind(this)
    });
    return stream;
  }
  /**
   * closeStream is used to close a stream once both sides have
   * issued a close.
   */
  closeStream(id) {
    if (this.client === (id % 2 === 0)) {
      this.numInboundStreams--;
    } else {
      this.numOutboundStreams--;
    }
    this._streams.delete(id);
  }
  async keepAliveLoop() {
    this.log?.trace("muxer keepalive enabled interval=%s", this.config.keepAliveInterval);
    while (true) {
      let timeoutId;
      try {
        await raceSignal2(new Promise((resolve) => {
          timeoutId = setTimeout(resolve, this.config.keepAliveInterval);
        }), this.closeController.signal);
        this.ping().catch((e) => this.log?.error("ping error: %s", e));
      } catch (e) {
        clearInterval(timeoutId);
        return;
      }
    }
  }
  async handleFrame(header, readData) {
    const { streamID, type, length } = header;
    this.log?.trace("received frame %o", header);
    if (streamID === 0) {
      switch (type) {
        case FrameType.Ping: {
          this.handlePing(header);
          return;
        }
        case FrameType.GoAway: {
          this.handleGoAway(length);
          return;
        }
        default:
          throw new InvalidFrameError("Invalid frame type");
      }
    } else {
      switch (header.type) {
        case FrameType.Data:
        case FrameType.WindowUpdate: {
          await this.handleStreamMessage(header, readData);
          return;
        }
        default:
          throw new InvalidFrameError("Invalid frame type");
      }
    }
  }
  handlePing(header) {
    if (header.flag === Flag.SYN) {
      this.log?.trace("received ping request pingId=%s", header.length);
      this.sendPing(header.length, Flag.ACK);
    } else if (header.flag === Flag.ACK) {
      this.log?.trace("received ping response pingId=%s", header.length);
      this.handlePingResponse(header.length);
    } else {
      throw new InvalidFrameError("Invalid frame flag");
    }
  }
  handlePingResponse(pingId) {
    if (this.activePing === void 0) {
      throw new UnrequestedPingError("ping not requested");
    }
    if (this.activePing.id !== pingId) {
      throw new NotMatchingPingError("ping doesn't match our id");
    }
    this.activePing.resolve();
  }
  handleGoAway(reason) {
    this.log?.trace("received GoAway reason=%s", GoAwayCode[reason] ?? "unknown");
    this.remoteGoAway = reason;
    for (const stream of this._streams.values()) {
      stream.reset();
    }
    this._closeMuxer();
  }
  async handleStreamMessage(header, readData) {
    const { streamID, flag, type } = header;
    if ((flag & Flag.SYN) === Flag.SYN) {
      this.incomingStream(streamID);
    }
    const stream = this._streams.get(streamID);
    if (stream === void 0) {
      if (type === FrameType.Data) {
        this.log?.("discarding data for stream id=%s", streamID);
        if (readData === void 0) {
          throw new Error("unreachable");
        }
        await readData();
      } else {
        this.log?.trace("frame for missing stream id=%s", streamID);
      }
      return;
    }
    switch (type) {
      case FrameType.WindowUpdate: {
        stream.handleWindowUpdate(header);
        return;
      }
      case FrameType.Data: {
        if (readData === void 0) {
          throw new Error("unreachable");
        }
        await stream.handleData(header, readData);
        return;
      }
      default:
        throw new Error("unreachable");
    }
  }
  incomingStream(id) {
    if (this.client !== (id % 2 === 0)) {
      throw new InvalidParametersError("Both endpoints are clients");
    }
    if (this._streams.has(id)) {
      return;
    }
    this.log?.trace("new incoming stream id=%s", id);
    if (this.localGoAway !== void 0) {
      this.sendFrame({
        type: FrameType.WindowUpdate,
        flag: Flag.RST,
        streamID: id,
        length: 0
      });
      return;
    }
    if (this.numInboundStreams >= this.config.maxInboundStreams) {
      this.log?.("maxIncomingStreams exceeded, forcing stream reset");
      this.sendFrame({
        type: FrameType.WindowUpdate,
        flag: Flag.RST,
        streamID: id,
        length: 0
      });
      return;
    }
    const stream = this._newStream(id, void 0, StreamState.SYNReceived, "inbound");
    this.numInboundStreams++;
    this._streams.set(id, stream);
    this.onIncomingStream?.(stream);
  }
  sendFrame(header, data) {
    this.log?.trace("sending frame %o", header);
    if (header.type === FrameType.Data) {
      if (data === void 0) {
        throw new InvalidFrameError("Invalid frame");
      }
      this.source.push(new Uint8ArrayList(encodeHeader(header), data));
    } else {
      this.source.push(encodeHeader(header));
    }
  }
  sendPing(pingId, flag = Flag.SYN) {
    if (flag === Flag.SYN) {
      this.log?.trace("sending ping request pingId=%s", pingId);
    } else {
      this.log?.trace("sending ping response pingId=%s", pingId);
    }
    this.sendFrame({
      type: FrameType.Ping,
      flag,
      streamID: 0,
      length: pingId
    });
  }
  sendGoAway(reason = GoAwayCode.NormalTermination) {
    this.log?.("sending GoAway reason=%s", GoAwayCode[reason]);
    this.localGoAway = reason;
    this.sendFrame({
      type: FrameType.GoAway,
      flag: 0,
      streamID: 0,
      length: reason
    });
  }
};
function isPromise4(thing) {
  return thing != null && typeof thing.then === "function";
}

// ../../../node_modules/@chainsafe/libp2p-yamux/dist/src/index.js
function yamux(init = {}) {
  return (components) => new Yamux(components, init);
}

// ../../../node_modules/starpc/dist/srpc/array-list.js
function combineUint8ArrayListTransform() {
  return async function* decodeMessageSource(source) {
    for await (const obj of source) {
      if (isUint8ArrayList(obj)) {
        yield obj.subarray();
      } else {
        yield obj;
      }
    }
  };
}

// ../../../node_modules/starpc/dist/srpc/stream.js
function streamToPacketStream(stream) {
  return {
    source: pipe(stream, parseLengthPrefixTransform(), combineUint8ArrayListTransform()),
    sink: async (source) => {
      await pipe(source, prependLengthPrefixTransform(), stream).catch((err) => stream.close(err)).then(() => stream.close());
    }
  };
}

// ../../../node_modules/starpc/dist/srpc/log.js
function createDisabledLogger(namespace) {
  const logger = () => {
  };
  logger.enabled = false;
  logger.color = "";
  logger.diff = 0;
  logger.log = () => {
  };
  logger.namespace = namespace;
  logger.destroy = () => true;
  logger.extend = () => logger;
  logger.debug = logger;
  logger.error = logger;
  logger.trace = logger;
  logger.newScope = () => logger;
  return logger;
}
function createDisabledComponentLogger() {
  return { forComponent: createDisabledLogger };
}

// ../../../node_modules/starpc/dist/srpc/conn.js
var StreamConn = class {
  // muxer is the stream muxer.
  _muxer;
  // server is the server side, if set.
  _server;
  constructor(server, connParams) {
    if (server) {
      this._server = server;
    }
    const muxerFactory = connParams?.muxerFactory ?? yamux({ enableKeepAlive: false, ...connParams?.yamuxParams })({
      logger: connParams?.logger ?? createDisabledComponentLogger()
    });
    this._muxer = muxerFactory.createStreamMuxer({
      onIncomingStream: this.handleIncomingStream.bind(this),
      direction: connParams?.direction || "outbound"
    });
  }
  // sink returns the message sink.
  get sink() {
    return this._muxer.sink;
  }
  // source returns the outgoing message source.
  get source() {
    return this._muxer.source;
  }
  // streams returns the set of all ongoing streams.
  get streams() {
    return this._muxer.streams;
  }
  // muxer returns the muxer
  get muxer() {
    return this._muxer;
  }
  // server returns the server, if any.
  get server() {
    return this._server;
  }
  // buildClient builds a new client from the connection.
  buildClient() {
    return new Client(this.openStream.bind(this));
  }
  // openStream implements the client open stream function.
  async openStream() {
    const strm = await this.muxer.newStream();
    return streamToPacketStream(strm);
  }
  // buildOpenStreamFunc returns openStream bound to this conn.
  buildOpenStreamFunc() {
    return this.openStream.bind(this);
  }
  // handleIncomingStream handles an incoming stream.
  //
  // this is usually called by the muxer when streams arrive.
  handleIncomingStream(strm) {
    const server = this.server;
    if (!server) {
      return strm.abort(new Error("server not implemented"));
    }
    server.handlePacketStream(streamToPacketStream(strm));
  }
  // close closes or aborts the muxer with an optional error.
  close(err) {
    if (err) {
      this.muxer.abort(err);
    } else {
      this.muxer.close();
    }
  }
};

// ../../../node_modules/@aptre/it-ws/dist/src/source.js
__toESM(require_dom());

// ../../../node_modules/starpc/dist/srpc/broadcast-channel.js
__toESM(require_dom());

// ../../../node_modules/starpc/dist/srpc/message-port.js
__toESM(require_dom());

// ../../../node_modules/starpc/dist/srpc/handle-stream-ctr.js
var HandleStreamCtr = class extends ValueCtr {
  constructor(handleStreamFn) {
    super(handleStreamFn);
  }
  // handleStreamFunc returns an HandleStreamFunc which waits for the underlying HandleStreamFunc.
  get handleStreamFunc() {
    return async (stream) => {
      let handleFn = this.value;
      if (!handleFn) {
        handleFn = await this.wait();
      }
      return handleFn(stream);
    };
  }
};

// ../../../node_modules/starpc/dist/rpcstream/rpcstream.js
async function openRpcStream(componentId, caller, waitAck) {
  const packetTx = pushable({
    objectMode: true
  });
  const packetRx = caller(packetTx);
  packetTx.push({
    body: {
      case: "init",
      value: { componentId }
    }
  });
  const packetIt = packetRx[Symbol.asyncIterator]();
  return new RpcStream(packetTx, packetIt);
}
function buildRpcStreamOpenStream(componentId, caller) {
  return async () => {
    return openRpcStream(componentId, caller);
  };
}
var RpcStream = class {
  // source is the source for incoming Uint8Array packets.
  source;
  // sink is the sink for outgoing Uint8Array packets.
  sink;
  // _packetRx receives packets from the remote.
  _packetRx;
  // _packetTx writes packets to the remote.
  _packetTx;
  // packetTx writes packets to the remote.
  // packetRx receives packets from the remote.
  constructor(packetTx, packetRx) {
    this._packetTx = packetTx;
    this._packetRx = packetRx;
    this.sink = this._createSink();
    this.source = this._createSource();
  }
  // _createSink initializes the sink field.
  _createSink() {
    return async (source) => {
      try {
        for await (const arr of source) {
          this._packetTx.push({
            body: { case: "data", value: arr }
          });
        }
        this._packetTx.end();
      } catch (err) {
        this._packetTx.end(err);
      }
    };
  }
  // _createSource initializes the source field.
  _createSource() {
    return (async function* (packetRx) {
      while (true) {
        const msgIt = await packetRx.next();
        if (msgIt.done) {
          return;
        }
        const value = msgIt.value;
        const body = value?.body;
        if (!body) {
          continue;
        }
        switch (body.case) {
          case "ack":
            if (body.value.error?.length) {
              throw new Error(body.value.error);
            }
            break;
          case "data":
            yield body.value;
            break;
        }
      }
    })(this._packetRx);
  }
};

// ../../../node_modules/starpc/dist/rpcstream/rpcstream.pb.js
var RpcStreamInit = createMessageType({
  typeName: "rpcstream.RpcStreamInit",
  fields: [
    { no: 1, name: "component_id", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
var RpcAck = createMessageType({
  typeName: "rpcstream.RpcAck",
  fields: [
    { no: 1, name: "error", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
var RpcStreamPacket = createMessageType({
  typeName: "rpcstream.RpcStreamPacket",
  fields: [
    {
      no: 1,
      name: "init",
      kind: "message",
      T: () => RpcStreamInit,
      oneof: "body"
    },
    { no: 2, name: "ack", kind: "message", T: () => RpcAck, oneof: "body" },
    { no: 3, name: "data", kind: "scalar", T: ScalarType.BYTES, oneof: "body" }
  ],
  packedByDefault: true
});

// ../../../node_modules/starpc/dist/echo/echo.pb.js
createMessageType({
  typeName: "echo.EchoMsg",
  fields: [
    { no: 1, name: "body", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});

// ../../../node_modules/@aptre/protobuf-es-lite/dist/google/protobuf/empty.pb.js
createMessageType({
  typeName: "google.protobuf.Empty",
  fields: [],
  packedByDefault: true
});

// ../../../node_modules/starpc/dist/echo/echo_srpc.pb.js
({
  methods: {
    /**
     * Echo returns the given message.
     *
     * @generated from rpc echo.Echoer.Echo
     */
    Echo: {
      kind: MethodKind.Unary
    },
    /**
     * EchoServerStream is an example of a server -> client one-way stream.
     *
     * @generated from rpc echo.Echoer.EchoServerStream
     */
    EchoServerStream: {
      kind: MethodKind.ServerStreaming
    },
    /**
     * EchoClientStream is an example of client->server one-way stream.
     *
     * @generated from rpc echo.Echoer.EchoClientStream
     */
    EchoClientStream: {
      kind: MethodKind.ClientStreaming
    },
    /**
     * EchoBidiStream is an example of a two-way stream.
     *
     * @generated from rpc echo.Echoer.EchoBidiStream
     */
    EchoBidiStream: {
      kind: MethodKind.BiDiStreaming
    },
    /**
     * RpcStream opens a nested rpc call stream.
     *
     * @generated from rpc echo.Echoer.RpcStream
     */
    RpcStream: {
      kind: MethodKind.BiDiStreaming
    },
    /**
     * DoNothing does nothing.
     *
     * @generated from rpc echo.Echoer.DoNothing
     */
    DoNothing: {
      kind: MethodKind.Unary
    }
  }
});

// quickjs/quickjs.ts
function writeCompleteChunk(os, fd, data) {
  let offset = 0;
  while (offset < data.length) {
    const bytesWritten = os.write(
      fd,
      data.buffer,
      data.byteOffset + offset,
      data.length - offset
    );
    if (bytesWritten < 0) {
      throw new Error(`Write failed with error code: ${bytesWritten}`);
    }
    if (bytesWritten === 0) {
      throw new Error(
        "Write returned 0 bytes, possible full disk or broken pipe"
      );
    }
    offset += bytesWritten;
  }
}
async function writeSourceToFd(os, source, filePath) {
  const flags = os.O_WRONLY | os.O_APPEND | os.O_CREAT;
  const mode = 420;
  let fd = void 0;
  try {
    fd = os.open(filePath, flags, mode);
    if (fd < 0) {
      throw new Error(`Failed to open file ${filePath}. Error code: ${fd}`);
    }
    for await (const chunk of source) {
      if (isUint8ArrayList(chunk)) {
        for (const internalBuf of chunk) {
          writeCompleteChunk(os, fd, internalBuf);
        }
      } else if (chunk instanceof Uint8Array) {
        writeCompleteChunk(os, fd, chunk);
      } else {
        throw new Error(
          `Received unsupported chunk type in stream: ${typeof chunk}`
        );
      }
    }
  } finally {
    if (fd !== void 0 && fd >= 0) {
      os.close(fd);
    }
  }
}

// quickjs/polyfill-event.ts
function createEvent() {
  return globalThis.Event;
}
function createEventTarget() {
  return globalThis.EventTarget;
}
function createCustomEvent() {
  return globalThis.CustomEvent;
}

// quickjs/polyfill-abort-controller.ts
function createAbortController() {
  class AbortSignalImpl {
    static abort(reason) {
      const signal = new AbortSignalImpl();
      signal._abort(reason);
      return signal;
    }
    static timeout(delay) {
      const signal = new AbortSignalImpl();
      setTimeout(() => {
        signal._abort(new Error("TimeoutError"));
      }, delay);
      return signal;
    }
    _aborted = false;
    _reason = void 0;
    _listeners = [];
    _onabort = null;
    get aborted() {
      return this._aborted;
    }
    get reason() {
      return this._reason;
    }
    get onabort() {
      return this._onabort;
    }
    set onabort(handler) {
      this._onabort = handler;
    }
    addEventListener(type, listener, _options) {
      if (type === "abort" && typeof listener === "function") {
        this._listeners.push(listener);
      }
    }
    removeEventListener(type, listener, _options) {
      if (type === "abort" && typeof listener === "function") {
        const index = this._listeners.indexOf(listener);
        if (index !== -1) {
          this._listeners.splice(index, 1);
        }
      }
    }
    dispatchEvent(event) {
      if (event.type === "abort") {
        if (this._onabort) {
          this._onabort(event);
        }
        this._listeners.forEach((listener) => listener(event));
      }
      return true;
    }
    throwIfAborted() {
      if (this._aborted) {
        throw this._reason;
      }
    }
    // Make AbortSignal a proper constructor
    static [Symbol.hasInstance](instance) {
      return instance instanceof AbortSignalImpl;
    }
    // Internal method to trigger abort
    _abort(reason) {
      if (this._aborted) return;
      this._aborted = true;
      this._reason = reason !== void 0 ? reason : new Error("AbortError");
      const EventClass = globalThis.Event;
      const event = new EventClass("abort");
      Object.defineProperty(event, "target", { value: this, writable: false });
      this.dispatchEvent(event);
    }
  }
  class AbortControllerImpl {
    _signal;
    constructor() {
      this._signal = new AbortSignalImpl();
    }
    get signal() {
      return this._signal;
    }
    abort(reason) {
      this._signal._abort(reason);
    }
  }
  Object.defineProperty(AbortControllerImpl, "AbortSignal", {
    value: AbortSignalImpl,
    writable: false,
    enumerable: false,
    configurable: false
  });
  return AbortControllerImpl;
}

// quickjs/polyfill-symbol.ts
function createSymbolPolyfills() {
  if (!Symbol.asyncIterator) {
    Object.defineProperty(Symbol, "asyncIterator", {
      value: /* @__PURE__ */ Symbol("Symbol.asyncIterator"),
      writable: false,
      enumerable: false,
      configurable: false
    });
  }
  if (!Symbol.dispose) {
    Object.defineProperty(Symbol, "dispose", {
      value: /* @__PURE__ */ Symbol("Symbol.dispose"),
      writable: false,
      enumerable: false,
      configurable: false
    });
  }
  if (!Symbol.asyncDispose) {
    Object.defineProperty(Symbol, "asyncDispose", {
      value: /* @__PURE__ */ Symbol("Symbol.asyncDispose"),
      writable: false,
      enumerable: false,
      configurable: false
    });
  }
}

// quickjs/text-encoding.js
function inRange(a, min, max) {
  return min <= a && a <= max;
}
function ToDictionary(o) {
  if (o === void 0) {
    return {};
  }
  if (o === Object(o)) {
    return o;
  }
  throw TypeError("Could not convert argument to dictionary");
}
function stringToCodePoints(string) {
  var s = String(string);
  var n = s.length;
  var i = 0;
  var u = [];
  while (i < n) {
    var c = s.charCodeAt(i);
    if (c < 55296 || c > 57343) {
      u.push(c);
    } else if (56320 <= c && c <= 57343) {
      u.push(65533);
    } else if (55296 <= c && c <= 56319) {
      if (i === n - 1) {
        u.push(65533);
      } else {
        var d = string.charCodeAt(i + 1);
        if (56320 <= d && d <= 57343) {
          var a = c & 1023;
          var b = d & 1023;
          u.push(65536 + (a << 10) + b);
          i += 1;
        } else {
          u.push(65533);
        }
      }
    }
    i += 1;
  }
  return u;
}
function codePointsToString(code_points) {
  var s = "";
  for (var i = 0; i < code_points.length; ++i) {
    var cp = code_points[i];
    if (cp <= 65535) {
      s += String.fromCharCode(cp);
    } else {
      cp -= 65536;
      s += String.fromCharCode((cp >> 10) + 55296, (cp & 1023) + 56320);
    }
  }
  return s;
}
var end_of_stream = -1;
function Stream(tokens) {
  this.tokens = [].slice.call(tokens);
  this.tokens.reverse();
}
Stream.prototype = {
  /**
   * @return {boolean} True if end-of-stream has been hit.
   */
  endOfStream: function() {
    return !this.tokens.length;
  },
  /**
   * When a token is read from a stream, the first token in the
   * stream must be returned and subsequently removed, and
   * end-of-stream must be returned otherwise.
   *
   * @return {number} Get the next token from the stream, or
   * end_of_stream.
   */
  read: function() {
    if (!this.tokens.length) {
      return end_of_stream;
    }
    return this.tokens.pop();
  },
  /**
   * When one or more tokens are prepended to a stream, those tokens
   * must be inserted, in given order, before the first token in the
   * stream.
   *
   * @param {(number|!Array.<number>)} token The token(s) to prepend to the stream.
   */
  prepend: function(token) {
    if (Array.isArray(token)) {
      var tokens = (
        /** @type {!Array.<number>}*/
        token
      );
      while (tokens.length) {
        this.tokens.push(tokens.pop());
      }
    } else {
      this.tokens.push(token);
    }
  },
  /**
   * When one or more tokens are pushed to a stream, those tokens
   * must be inserted, in given order, after the last token in the
   * stream.
   *
   * @param {(number|!Array.<number>)} token The tokens(s) to prepend to the stream.
   */
  push: function(token) {
    if (Array.isArray(token)) {
      var tokens = (
        /** @type {!Array.<number>}*/
        token
      );
      while (tokens.length) {
        this.tokens.unshift(tokens.shift());
      }
    } else {
      this.tokens.unshift(token);
    }
  }
};
var finished = -1;
function decoderError(fatal, opt_code_point) {
  if (fatal) {
    throw TypeError("Decoder error");
  }
  return opt_code_point || 65533;
}
var DEFAULT_ENCODING = "utf-8";
function TextDecoder2(encoding, options) {
  if (!(this instanceof TextDecoder2)) {
    return new TextDecoder2(encoding, options);
  }
  encoding = encoding !== void 0 ? String(encoding).toLowerCase() : DEFAULT_ENCODING;
  if (encoding === "utf8") {
    encoding = DEFAULT_ENCODING;
  }
  if (encoding !== DEFAULT_ENCODING) {
    throw new Error("Encoding not supported. Only utf-8 is supported");
  }
  options = ToDictionary(options);
  this._streaming = false;
  this._BOMseen = false;
  this._decoder = null;
  this._fatal = Boolean(options["fatal"]);
  this._ignoreBOM = Boolean(options["ignoreBOM"]);
  Object.defineProperty(this, "encoding", { value: "utf-8" });
  Object.defineProperty(this, "fatal", { value: this._fatal });
  Object.defineProperty(this, "ignoreBOM", { value: this._ignoreBOM });
}
TextDecoder2.prototype = {
  /**
   * @param {ArrayBufferView=} input The buffer of bytes to decode.
   * @param {Object=} options
   * @return {string} The decoded string.
   */
  decode: function decode(input, options) {
    var bytes;
    if (typeof input === "object" && input instanceof ArrayBuffer) {
      bytes = new Uint8Array(input);
    } else if (typeof input === "object" && "buffer" in input && input.buffer instanceof ArrayBuffer) {
      bytes = new Uint8Array(input.buffer, input.byteOffset, input.byteLength);
    } else {
      bytes = new Uint8Array(0);
    }
    options = ToDictionary(options);
    if (!this._streaming) {
      this._decoder = new UTF8Decoder({ fatal: this._fatal });
      this._BOMseen = false;
    }
    this._streaming = Boolean(options["stream"]);
    var input_stream = new Stream(bytes);
    var code_points = [];
    var result;
    while (!input_stream.endOfStream()) {
      result = this._decoder.handler(input_stream, input_stream.read());
      if (result === finished) {
        break;
      }
      if (result === null) {
        continue;
      }
      if (Array.isArray(result)) {
        code_points.push.apply(
          code_points,
          /** @type {!Array.<number>}*/
          result
        );
      } else {
        code_points.push(result);
      }
    }
    if (!this._streaming) {
      do {
        result = this._decoder.handler(input_stream, input_stream.read());
        if (result === finished) {
          break;
        }
        if (result === null) {
          continue;
        }
        if (Array.isArray(result)) {
          code_points.push.apply(
            code_points,
            /** @type {!Array.<number>}*/
            result
          );
        } else {
          code_points.push(result);
        }
      } while (!input_stream.endOfStream());
      this._decoder = null;
    }
    if (code_points.length) {
      if (["utf-8"].indexOf(this.encoding) !== -1 && !this._ignoreBOM && !this._BOMseen) {
        if (code_points[0] === 65279) {
          this._BOMseen = true;
          code_points.shift();
        } else {
          this._BOMseen = true;
        }
      }
    }
    return codePointsToString(code_points);
  }
};
function TextEncoder2(encoding, options) {
  if (!(this instanceof TextEncoder2)) {
    return new TextEncoder2(encoding, options);
  }
  encoding = encoding !== void 0 ? String(encoding).toLowerCase() : DEFAULT_ENCODING;
  if (encoding === "utf8") {
    encoding = DEFAULT_ENCODING;
  }
  if (encoding !== DEFAULT_ENCODING) {
    throw new Error("Encoding not supported. Only utf-8 is supported");
  }
  options = ToDictionary(options);
  this._streaming = false;
  this._encoder = null;
  this._options = { fatal: Boolean(options["fatal"]) };
  Object.defineProperty(this, "encoding", { value: "utf-8" });
}
TextEncoder2.prototype = {
  /**
   * @param {string=} opt_string The string to encode.
   * @param {Object=} options
   * @return {Uint8Array} Encoded bytes, as a Uint8Array.
   */
  encode: function encode(opt_string, options) {
    opt_string = opt_string ? String(opt_string) : "";
    options = ToDictionary(options);
    if (!this._streaming) {
      this._encoder = new UTF8Encoder(this._options);
    }
    this._streaming = Boolean(options["stream"]);
    var bytes = [];
    var input_stream = new Stream(stringToCodePoints(opt_string));
    var result;
    while (!input_stream.endOfStream()) {
      result = this._encoder.handler(input_stream, input_stream.read());
      if (result === finished) {
        break;
      }
      if (Array.isArray(result)) {
        bytes.push.apply(
          bytes,
          /** @type {!Array.<number>}*/
          result
        );
      } else {
        bytes.push(result);
      }
    }
    if (!this._streaming) {
      while (true) {
        result = this._encoder.handler(input_stream, input_stream.read());
        if (result === finished) {
          break;
        }
        if (Array.isArray(result)) {
          bytes.push.apply(
            bytes,
            /** @type {!Array.<number>}*/
            result
          );
        } else {
          bytes.push(result);
        }
      }
      this._encoder = null;
    }
    return new Uint8Array(bytes);
  }
};
function UTF8Decoder(options) {
  var fatal = options.fatal;
  var utf8_code_point = 0, utf8_bytes_seen = 0, utf8_bytes_needed = 0, utf8_lower_boundary = 128, utf8_upper_boundary = 191;
  this.handler = function(stream, bite) {
    if (bite === end_of_stream && utf8_bytes_needed !== 0) {
      utf8_bytes_needed = 0;
      return decoderError(fatal);
    }
    if (bite === end_of_stream) {
      return finished;
    }
    if (utf8_bytes_needed === 0) {
      if (inRange(bite, 0, 127)) {
        return bite;
      }
      if (inRange(bite, 194, 223)) {
        utf8_bytes_needed = 1;
        utf8_code_point = bite - 192;
      } else if (inRange(bite, 224, 239)) {
        if (bite === 224) {
          utf8_lower_boundary = 160;
        }
        if (bite === 237) {
          utf8_upper_boundary = 159;
        }
        utf8_bytes_needed = 2;
        utf8_code_point = bite - 224;
      } else if (inRange(bite, 240, 244)) {
        if (bite === 240) {
          utf8_lower_boundary = 144;
        }
        if (bite === 244) {
          utf8_upper_boundary = 143;
        }
        utf8_bytes_needed = 3;
        utf8_code_point = bite - 240;
      } else {
        return decoderError(fatal);
      }
      utf8_code_point = utf8_code_point << 6 * utf8_bytes_needed;
      return null;
    }
    if (!inRange(bite, utf8_lower_boundary, utf8_upper_boundary)) {
      utf8_code_point = utf8_bytes_needed = utf8_bytes_seen = 0;
      utf8_lower_boundary = 128;
      utf8_upper_boundary = 191;
      stream.prepend(bite);
      return decoderError(fatal);
    }
    utf8_lower_boundary = 128;
    utf8_upper_boundary = 191;
    utf8_bytes_seen += 1;
    utf8_code_point += bite - 128 << 6 * (utf8_bytes_needed - utf8_bytes_seen);
    if (utf8_bytes_seen !== utf8_bytes_needed) {
      return null;
    }
    var code_point = utf8_code_point;
    utf8_code_point = utf8_bytes_needed = utf8_bytes_seen = 0;
    return code_point;
  };
}
function UTF8Encoder(options) {
  options.fatal;
  this.handler = function(stream, code_point) {
    if (code_point === end_of_stream) {
      return finished;
    }
    if (inRange(code_point, 0, 127)) {
      return code_point;
    }
    var count, offset;
    if (inRange(code_point, 128, 2047)) {
      count = 1;
      offset = 192;
    } else if (inRange(code_point, 2048, 65535)) {
      count = 2;
      offset = 224;
    } else if (inRange(code_point, 65536, 1114111)) {
      count = 3;
      offset = 240;
    }
    var bytes = [(code_point >> 6 * count) + offset];
    while (count > 0) {
      var temp = code_point >> 6 * (count - 1);
      bytes.push(128 | temp & 63);
      count -= 1;
    }
    return bytes;
  };
}

// quickjs/console-util.js
function extend(origin, add) {
  if (!add || !isObject(add)) {
    return origin;
  }
  var keys = Object.keys(add);
  var i = keys.length;
  while (i--) {
    origin[keys[i]] = add[keys[i]];
  }
  return origin;
}
var formatRegExp = /%[sdjif%]/g;
function format(f) {
  if (!isString(f)) {
    var objects = [];
    for (let i2 = 0; i2 < arguments.length; i2++) {
      objects.push(inspect(arguments[i2]));
    }
    return objects.join(" ");
  }
  let i = 1;
  var args = arguments;
  var len = args.length;
  var str = String(f).replace(formatRegExp, function(x2) {
    if (x2 === "%%") {
      return "%";
    }
    if (i >= len) {
      return x2;
    }
    switch (x2) {
      case "%s":
        return String(args[i++]);
      case "%d":
      case "%i": {
        const arg = args[i++];
        return typeof arg === "symbol" ? NaN : parseInt(arg, 10);
      }
      case "%f": {
        const arg = args[i++];
        return typeof arg === "symbol" ? NaN : parseFloat(arg);
      }
      case "%j":
        try {
          return JSON.stringify(args[i++]);
        } catch (_) {
          return "[Circular]";
        }
      default:
        return x2;
    }
  });
  for (var x = args[i]; i < len; x = args[++i]) {
    if (x === null || !["object", "symbol"].includes(typeof x)) {
      str += " " + x;
    } else {
      str += " " + inspect(x);
    }
  }
  return str;
}
function inspect(obj, opts) {
  var ctx = {
    seen: [],
    stylize: stylizeNoColor
  };
  if (arguments.length >= 3) {
    ctx.depth = arguments[2];
  }
  if (arguments.length >= 4) {
    ctx.colors = arguments[3];
  }
  if (opts) {
    extend(ctx, opts);
  }
  if (ctx.showHidden === void 0) {
    ctx.showHidden = false;
  }
  if (ctx.depth === void 0) {
    ctx.depth = 2;
  }
  if (ctx.colors === void 0) {
    ctx.colors = false;
  }
  if (ctx.colors) {
    ctx.stylize = stylizeWithColor;
  }
  return formatValue(ctx, obj, ctx.depth);
}
inspect.colors = {
  bold: [1, 22],
  italic: [3, 23],
  underline: [4, 24],
  inverse: [7, 27],
  white: [37, 39],
  grey: [90, 39],
  black: [30, 39],
  blue: [34, 39],
  cyan: [36, 39],
  green: [32, 39],
  magenta: [35, 39],
  red: [31, 39],
  yellow: [33, 39]
};
inspect.styles = {
  special: "cyan",
  number: "yellow",
  boolean: "yellow",
  undefined: "grey",
  null: "bold",
  string: "green",
  date: "magenta",
  // "name": intentionally not styling
  regexp: "red"
};
function stylizeWithColor(str, styleType) {
  var style = inspect.styles[styleType];
  if (style) {
    return "\x1B[" + inspect.colors[style][0] + "m" + str + "\x1B[" + inspect.colors[style][1] + "m";
  } else {
    return str;
  }
}
function stylizeNoColor(str) {
  return str;
}
function formatValue(ctx, value, recurseTimes) {
  const primitive = formatPrimitive(ctx, value);
  if (primitive) {
    return primitive;
  }
  const descriptors = Object.getOwnPropertyDescriptors(value);
  const descriptorsArr = Reflect.ownKeys(descriptors).map((k) => [
    k,
    descriptors[k]
  ]);
  let keys = descriptorsArr.filter(([_v, desc]) => desc.enumerable).map(([v, _desc]) => v);
  const visibleKeys = new Set(keys);
  if (ctx.showHidden) {
    keys = descriptorsArr.map(([v, _desc]) => v);
  }
  if (keys.length === 0) {
    if (typeof value === "function") {
      const name = value.name ? ": " + value.name : "";
      return ctx.stylize("[Function" + name + "]", "special");
    }
    if (isRegExp(value)) {
      return ctx.stylize(RegExp.prototype.toString.call(value), "regexp");
    }
    if (isDate(value)) {
      return ctx.stylize(Date.prototype.toString.call(value), "date");
    }
    if (isError(value)) {
      return formatError(value);
    }
  }
  var base = "", array = false, braces = ["{", "}"];
  if (Array.isArray(value)) {
    array = true;
    braces = ["[", "]"];
  }
  if (typeof value === "function") {
    var n = value.name ? ": " + value.name : "";
    base = " [Function" + n + "]";
  }
  if (isRegExp(value)) {
    base = " " + RegExp.prototype.toString.call(value);
  }
  if (isDate(value)) {
    base = " " + Date.prototype.toUTCString.call(value);
  }
  if (isError(value)) {
    base = " " + formatError(value);
  }
  if (keys.length === 0 && (!array || value.length === 0)) {
    return braces[0] + base + braces[1];
  }
  if (recurseTimes < 0) {
    if (isRegExp(value)) {
      return ctx.stylize(RegExp.prototype.toString.call(value), "regexp");
    } else {
      return ctx.stylize("[Object]", "special");
    }
  }
  ctx.seen.push(value);
  var output;
  if (array) {
    output = formatArray(ctx, value, recurseTimes, visibleKeys, keys);
  } else {
    output = keys.map(function(key) {
      return formatProperty(ctx, value, recurseTimes, visibleKeys, key, array);
    });
  }
  ctx.seen.pop();
  return reduceToSingleString(output, base, braces);
}
function formatPrimitive(ctx, value) {
  if (value === void 0) {
    return ctx.stylize("undefined", "undefined");
  }
  if (isString(value)) {
    var simple = "'" + JSON.stringify(value).replace(/^"|"$/g, "").replace(/'/g, "\\'").replace(/\\"/g, '"') + "'";
    return ctx.stylize(simple, "string");
  }
  if (isNumber(value)) {
    return ctx.stylize("" + value, "number");
  }
  if (typeof value === "boolean") {
    return ctx.stylize("" + value, "boolean");
  }
  if (value === null) {
    return ctx.stylize("null", "null");
  }
  if (isSymbol(value)) {
    return ctx.stylize(value.toString(), "symbol");
  }
}
function formatError(value) {
  return value.toString() + "\n" + value.stack;
}
function formatArray(ctx, value, recurseTimes, visibleKeys, keys) {
  var output = [];
  for (var i = 0, l = value.length; i < l; ++i) {
    if (Object.prototype.hasOwnProperty.call(value, String(i))) {
      output.push(
        formatProperty(ctx, value, recurseTimes, visibleKeys, String(i), true)
      );
    } else {
      output.push("");
    }
  }
  keys.forEach(function(key) {
    if (!key.match(/^\d+$/)) {
      output.push(
        formatProperty(ctx, value, recurseTimes, visibleKeys, key, true)
      );
    }
  });
  return output;
}
function formatKey(ctx, key, visible) {
  let str = visible ? "" : "[";
  if (typeof key === "symbol") {
    str += ctx.stylize("[" + formatValue(ctx, key, null) + "]", "special");
  } else if (key.match(/^([a-zA-Z_][a-zA-Z_0-9]*)$/)) {
    str += ctx.stylize(key, "name");
  } else {
    str += ctx.stylize(
      "'" + JSON.stringify(key).slice(1, -1).replace(/\\"/g, '"') + "'",
      "string"
    );
  }
  if (!visible) {
    str += "]";
  }
  return str;
}
function formatProperty(ctx, value, recurseTimes, visibleKeys, key, array) {
  var name, str, desc;
  desc = Object.getOwnPropertyDescriptor(value, key) || { value: value[key] };
  if (desc.get) {
    if (desc.set) {
      str = ctx.stylize("[Getter/Setter]", "special");
    } else {
      str = ctx.stylize("[Getter]", "special");
    }
  } else {
    if (desc.set) {
      str = ctx.stylize("[Setter]", "special");
    }
  }
  if (!str) {
    if (ctx.seen.indexOf(desc.value) < 0) {
      if (recurseTimes === null) {
        str = formatValue(ctx, desc.value, null);
      } else {
        str = formatValue(ctx, desc.value, recurseTimes - 1);
      }
      if (str.indexOf("\n") > -1) {
        if (array) {
          str = str.split("\n").map(function(line) {
            return "  " + line;
          }).join("\n").slice(2);
        } else {
          str = "\n" + str.split("\n").map(function(line) {
            return "   " + line;
          }).join("\n");
        }
      }
    } else {
      str = ctx.stylize("[Circular]", "special");
    }
  }
  if (name === void 0) {
    if (array && typeof key === "string" && key.match(/^\d+$/)) {
      return str;
    }
    name = formatKey(ctx, key, visibleKeys.has(key));
  }
  return name + ": " + str;
}
function reduceToSingleString(output, base, braces) {
  var length = output.reduce(function(prev, cur) {
    return prev + cur.replace(/\u001b\[\d\d?m/g, "").length + 1;
  }, 0);
  if (length > 60) {
    return braces[0] + (base === "" ? "\n" : base + "\n ") + " " + output.join(",\n  ") + "\n" + braces[1];
  }
  return braces[0] + base + " " + output.join(", ") + " " + braces[1];
}
function isNumber(arg) {
  return typeof arg === "number";
}
function isString(arg) {
  return typeof arg === "string";
}
function isSymbol(arg) {
  return typeof arg === "symbol";
}
function isRegExp(re) {
  return isObject(re) && Object.prototype.toString.call(re) === "[object RegExp]";
}
function isObject(arg) {
  return typeof arg === "object" && arg !== null;
}
function isDate(d) {
  return isObject(d) && Object.prototype.toString.call(d) === "[object Date]";
}
function isError(e) {
  return isObject(e) && (Object.prototype.toString.call(e) === "[object Error]" || e instanceof Error);
}

// quickjs/console.js
function createConsole({
  logger,
  clearConsole,
  printer,
  formatter = (args) => format(...args),
  inspect: inspect2 = inspect
}) {
  if (!printer) {
    throw new Error("Printer is required");
  }
  const _printer = (logLevel, args, options) => {
    printer(logLevel, args, { ...options, indent: groupCount });
  };
  if (!logger) {
    logger = function Logger(logLevel, args, options) {
      if (args.length === 1) {
        _printer(logLevel, args, options);
      } else if (args.length > 1) {
        _printer(logLevel, [formatter(args)], options);
      }
    };
  }
  let groupCount = 0;
  const countMap = /* @__PURE__ */ new Map();
  const timers = /* @__PURE__ */ new Map();
  const consoleObj = /* @__PURE__ */ Object.create({});
  consoleObj.assert = function(condition = false, ...data) {
    if (condition) {
      return;
    }
    const message = "Assertion failed";
    if (data.length === 0) {
      data.push(message);
    } else if (typeof data[0] !== "string") {
      data.unshift(message);
    } else {
      data[0] = message + ": " + data[0];
    }
    logger("assert", data);
  };
  consoleObj.clear = function() {
    groupCount = 0;
    clearConsole();
  };
  consoleObj.table = function(data, properties) {
    if (properties !== void 0 && !Array.isArray(properties)) {
      throw new Error(
        "The 'properties' argument must be of type Array. Received type string"
      );
    }
    if (data === null || typeof data !== "object") {
      return _printer("table", data);
    }
    function getProperties(data2) {
      const props = [];
      const propsS = /* @__PURE__ */ new Set();
      for (const i in data2) {
        if (typeof data2[i] === "object") {
          for (const key in data2[i]) {
            if (!propsS.has(key)) {
              props.push(key);
              propsS.add(key);
            }
          }
        }
      }
      return props;
    }
    if (!properties) {
      properties = getProperties(data);
    }
    function normalize(data2) {
      const colorRegExp = /\u001b\[\d\d?m/g;
      return inspect2(data2).replace(colorRegExp, "");
    }
    function countBytes(str) {
      return encoder.encode(str).byteLength;
    }
    function getTableData(data2, properties2, addIndex = true) {
      const rows2 = [addIndex ? ["(index)", ...properties2] : [...properties2]];
      for (const i in data2) {
        const row = addIndex ? [i] : [];
        for (const p of properties2) {
          row.push(normalize(data2[i][p] || ""));
        }
        rows2.push(row);
      }
      return rows2;
    }
    const rows = getTableData(data, properties);
    const cols = [];
    for (let ci = 0; ci < rows[0].length; ci++) {
      for (let ri = 0; ri < rows.length; ri++) {
        cols[ci] = {
          width: Math.max(cols[ci]?.width ?? 0, countBytes(rows[ri][ci]))
        };
      }
    }
    function renderTable(rows2, cols2) {
      const tableChars = {
        middleMiddle: "\u2500",
        rowMiddle: "\u253C",
        topRight: "\u2510",
        topLeft: "\u250C",
        leftMiddle: "\u251C",
        topMiddle: "\u252C",
        bottomRight: "\u2518",
        bottomLeft: "\u2514",
        bottomMiddle: "\u2534",
        rightMiddle: "\u2524",
        left: "\u2502",
        right: "\u2502",
        middle: "\u2502"
      };
      let str = "";
      function drawHorizLine(left, right, middle) {
        str += left;
        for (let ci = 0; ci < cols2.length; ci++) {
          if (ci > 0) {
            str += middle;
          }
          str += tableChars.middleMiddle.repeat(cols2[ci].width + 2);
        }
        str += right;
      }
      function drawRow(row) {
        for (let ci = 0; ci < cols2.length; ci++) {
          if (ci === 0) {
            str += tableChars.left;
          } else {
            str += tableChars.middle;
          }
          str += " " + row[ci] + " ".repeat(cols2[ci].width - countBytes(row[ci]) + 1);
        }
        str += tableChars.right + "\n";
      }
      for (let ri = 0; ri < rows2.length; ri++) {
        if (ri === 0) {
          drawHorizLine(
            tableChars.topLeft,
            tableChars.topRight + "\n",
            tableChars.topMiddle
          );
        } else if (ri === 1) {
          drawHorizLine(
            tableChars.leftMiddle,
            tableChars.rightMiddle + "\n",
            tableChars.rowMiddle
          );
        }
        drawRow(rows2[ri]);
      }
      drawHorizLine(
        tableChars.bottomLeft,
        tableChars.bottomRight,
        tableChars.bottomMiddle
      );
      return str;
    }
    _printer("table", [renderTable(rows, cols)]);
  };
  consoleObj.trace = function(...data) {
    const stack = new Error().stack.trim().split("\n").slice(1).join("\n");
    _printer("trace", ["Trace: " + formatter(data) + "\n" + stack]);
  };
  consoleObj.dir = function(item, options) {
    _printer("dir", [inspect2(item)], options);
  };
  consoleObj.dirxml = function(...data) {
    logger("dirxml", data);
  };
  consoleObj.count = function(label = "default") {
    label = String(label);
    let count = countMap.get(label) ?? 0;
    count++;
    countMap.set(label, count);
    _printer("count", [label + ": " + count]);
  };
  consoleObj.countReset = function(label = "default") {
    if (!countMap.delete(label)) {
      logger("countReset", ["countReset: No counter named " + label], {
        isWarn: true
      });
    }
  };
  consoleObj.group = function(...data) {
    if (data.length > 0) {
      logger("group", data);
    }
    groupCount++;
  };
  consoleObj.groupCollapsed = function(...data) {
    consoleObj.group(...data);
  };
  consoleObj.groupEnd = function() {
    groupCount = Math.max(0, groupCount - 1);
  };
  consoleObj.time = function(label = "default") {
    label = String(label);
    if (timers.has(label)) {
      logger("time", ["Timer " + label + " already exists"], { isWarn: true });
    } else {
      timers.set(label, performance.now());
    }
  };
  consoleObj.timeLog = function(label = "default", ...data) {
    label = String(label);
    if (!timers.has(label)) {
      logger("timeLog", ["timeLog: No such timer: " + label], { isWarn: true });
    } else {
      const duration = performance.now() - timers.get(label);
      data.unshift(label + ": " + duration + "ms");
      _printer("timeLog", data);
    }
  };
  consoleObj.timeEnd = function(label = "default") {
    label = String(label);
    if (!timers.has(label)) {
      logger("timeEnd", ["timeEnd: No such timer: " + label], { isWarn: true });
    } else {
      const start = timers.get(label);
      timers.delete(label);
      const duration = performance.now() - start;
      _printer("timeEnd", [label + ": " + duration + "ms"]);
    }
  };
  const loggingFuncs = ["debug", "error", "info", "log", "warn"];
  for (const func of loggingFuncs) {
    consoleObj[func] = function(...args) {
      logger(func, args);
    };
  }
  return consoleObj;
}
var encoder = new TextEncoder2();
function createQuickjsConsole(originalConsole) {
  return createConsole({
    clearConsole() {
      originalConsole.log("\x1B[2J\x1B[0f");
    },
    printer(logLevel, args, { indent = 0, isWarn = false }) {
      const msg = args.map((arg) => {
        if (typeof arg === "string") {
          return arg;
        } else {
          try {
            return JSON.stringify(arg, null, 2);
          } catch {
            return String(arg);
          }
        }
      }).join(" ");
      const indentStr = " ".repeat(indent * 2);
      const output = indentStr + msg;
      if (["error", "trace", "warn"].includes(logLevel) || isWarn) {
        originalConsole.log("ERROR:", output);
      } else {
        originalConsole.log(output);
      }
    }
  });
}

// quickjs/performance.js
var entries = [];
var marksIndex = /* @__PURE__ */ Object.create(null);
function mark(name) {
  const mark2 = {
    name,
    entryType: "mark",
    startTime: globalThis.performance.now(),
    duration: 0
  };
  entries.push(mark2);
  marksIndex[name] = mark2;
  return mark2;
}
function measure(name, startMark, endMark) {
  let startTime;
  let endTime;
  if (endMark !== void 0 && marksIndex[endMark] === void 0) {
    throw new SyntaxError(
      "Failed to execute 'measure' on 'Performance': The mark '" + endMark + "' does not exist."
    );
  }
  if (startMark !== void 0 && marksIndex[startMark] === void 0) {
    throw new SyntaxError(
      "Failed to execute 'measure' on 'Performance': The mark '" + startMark + "' does not exist."
    );
  }
  if (marksIndex[startMark]) {
    startTime = marksIndex[startMark].startTime;
  } else {
    startTime = 0;
  }
  if (marksIndex[endMark]) {
    endTime = marksIndex[endMark].startTime;
  } else {
    endTime = globalThis.performance.now();
  }
  const mark2 = {
    name,
    entryType: "measure",
    startTime,
    duration: endTime - startTime
  };
  entries.push(mark2);
  return mark2;
}
function getEntriesByType(type) {
  return entries.filter((entry) => entry.entryType === type);
}
function getEntriesByName(name) {
  return entries.filter((entry) => entry.name === name);
}
function clearMarks(name) {
  if (typeof name === "undefined") {
    entries = entries.filter((entry) => entry.entryType !== "mark");
  } else {
    const entry = entries.find(
      (e) => e.entryType === "mark" && e.name === name
    );
    entries.splice(entries.indexOf(entry), 1);
    delete marksIndex[name];
  }
}
function clearMeasures(name) {
  if (typeof name === "undefined") {
    entries = entries.filter((entry) => entry.entryType !== "measure");
  } else {
    const entry = entries.find(
      (e) => e.entryType === "measure" && e.name === name
    );
    entries.splice(entries.indexOf(entry), 1);
  }
}
function createQuickjsPerformance(originalPerformance) {
  const enhancedPerformance = Object.create(originalPerformance);
  enhancedPerformance.mark = mark;
  enhancedPerformance.measure = measure;
  enhancedPerformance.getEntriesByType = getEntriesByType;
  enhancedPerformance.getEntriesByName = getEntriesByName;
  enhancedPerformance.clearMarks = clearMarks;
  enhancedPerformance.clearMeasures = clearMeasures;
  return enhancedPerformance;
}

// quickjs/base64.js
var keystr = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
function atob(data) {
  if (arguments.length === 0) {
    throw new TypeError("1 argument required, but only 0 present.");
  }
  data = `${data}`;
  data = data.replace(/[ \t\n\f\r]/g, "");
  if (data.length % 4 === 0) {
    data = data.replace(/==?$/, "");
  }
  if (data.length % 4 === 1 || /[^+/0-9A-Za-z]/.test(data)) {
    throw new DOMException(
      "Failed to decode base64: invalid character",
      "InvalidCharacterError"
    );
  }
  let output = "";
  let buffer = 0;
  let accumulatedBits = 0;
  for (let i = 0; i < data.length; i++) {
    buffer <<= 6;
    buffer |= atobLookup(data[i]);
    accumulatedBits += 6;
    if (accumulatedBits === 24) {
      output += String.fromCharCode((buffer & 16711680) >> 16);
      output += String.fromCharCode((buffer & 65280) >> 8);
      output += String.fromCharCode(buffer & 255);
      buffer = accumulatedBits = 0;
    }
  }
  if (accumulatedBits === 12) {
    buffer >>= 4;
    output += String.fromCharCode(buffer);
  } else if (accumulatedBits === 18) {
    buffer >>= 2;
    output += String.fromCharCode((buffer & 65280) >> 8);
    output += String.fromCharCode(buffer & 255);
  }
  return output;
}
function atobLookup(chr) {
  const index = keystr.indexOf(chr);
  return index < 0 ? void 0 : index;
}
function btoa(s) {
  if (arguments.length === 0) {
    throw new TypeError("1 argument required, but only 0 present.");
  }
  let i;
  s = `${s}`;
  for (i = 0; i < s.length; i++) {
    if (s.charCodeAt(i) > 255) {
      throw new DOMException(
        "The string to be encoded contains characters outside of the Latin1 range.",
        "InvalidCharacterError"
      );
    }
  }
  let out = "";
  for (i = 0; i < s.length; i += 3) {
    const groupsOfSix = [void 0, void 0, void 0, void 0];
    groupsOfSix[0] = s.charCodeAt(i) >> 2;
    groupsOfSix[1] = (s.charCodeAt(i) & 3) << 4;
    if (s.length > i + 1) {
      groupsOfSix[1] |= s.charCodeAt(i + 1) >> 4;
      groupsOfSix[2] = (s.charCodeAt(i + 1) & 15) << 2;
    }
    if (s.length > i + 2) {
      groupsOfSix[2] |= s.charCodeAt(i + 2) >> 6;
      groupsOfSix[3] = s.charCodeAt(i + 2) & 63;
    }
    for (let j = 0; j < groupsOfSix.length; j++) {
      if (typeof groupsOfSix[j] === "undefined") {
        out += "=";
      } else {
        out += btoaLookup(groupsOfSix[j]);
      }
    }
  }
  return out;
}
function btoaLookup(index) {
  if (index >= 0 && index < 64) {
    return keystr[index];
  }
  return void 0;
}

// quickjs/polyfill.ts
function applyPolyfills(to) {
  const target = to;
  const globalRefs = ["global", "window", "self"];
  globalRefs.forEach((name) => {
    Object.defineProperty(to, name, {
      enumerable: true,
      get() {
        return to;
      },
      set() {
      }
    });
  });
  createSymbolPolyfills();
  target.console = createQuickjsConsole(target.console);
  target.performance = createQuickjsPerformance(target.performance);
  target.Event = createEvent();
  target.EventTarget = createEventTarget();
  target.CustomEvent = createCustomEvent();
  target.AbortController = createAbortController();
  target.TextEncoder = TextEncoder2;
  target.TextDecoder = TextDecoder2;
  target.setTimeout = to.os.setTimeout;
  target.clearTimeout = to.os.clearTimeout;
  target.setInterval = to.os.setInterval;
  target.clearInterval = to.os.clearInterval;
  target.atob = atob;
  target.btoa = btoa;
  return target;
}

// ../../../vendor/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.ts
var ControllerConfig = createMessageType({
  typeName: "configset.proto.ControllerConfig",
  fields: [
    { no: 1, name: "id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "rev", kind: "scalar", T: ScalarType.UINT64 },
    { no: 3, name: "config", kind: "scalar", T: ScalarType.BYTES }
  ],
  packedByDefault: true
});
var ConfigSet = createMessageType({
  typeName: "configset.proto.ConfigSet",
  fields: [
    { no: 1, name: "configs", kind: "map", K: ScalarType.STRING, V: { kind: "message", T: () => ControllerConfig } }
  ],
  packedByDefault: true
});

// ../../../vendor/github.com/aperturerobotics/controllerbus/controller/controller.pb.ts
var Info = createMessageType({
  typeName: "controller.Info",
  fields: [
    { no: 1, name: "id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "version", kind: "scalar", T: ScalarType.STRING },
    { no: 3, name: "description", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});

// ../../../vendor/github.com/aperturerobotics/controllerbus/controller/exec/exec.pb.ts
var ControllerStatus_Enum = createEnumType("controller.exec.ControllerStatus", [
  { no: 0, name: "ControllerStatus_UNKNOWN" },
  { no: 1, name: "ControllerStatus_CONFIGURING" },
  { no: 2, name: "ControllerStatus_RUNNING" },
  { no: 3, name: "ControllerStatus_ERROR" }
]);
var ExecControllerRequest = createMessageType({
  typeName: "controller.exec.ExecControllerRequest",
  fields: [
    { no: 1, name: "config_set", kind: "message", T: () => ConfigSet },
    { no: 2, name: "config_set_yaml", kind: "scalar", T: ScalarType.STRING },
    { no: 3, name: "config_set_yaml_overwrite", kind: "scalar", T: ScalarType.BOOL }
  ],
  packedByDefault: true
});
var ExecControllerResponse = createMessageType({
  typeName: "controller.exec.ExecControllerResponse",
  fields: [
    { no: 1, name: "id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "status", kind: "enum", T: ControllerStatus_Enum },
    { no: 3, name: "controller_info", kind: "message", T: () => Info },
    { no: 4, name: "error_info", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});

// ../../../vendor/github.com/aperturerobotics/bifrost/hash/hash.pb.ts
var HashType_Enum = createEnumType("hash.HashType", [
  { no: 0, name: "HashType_UNKNOWN" },
  { no: 1, name: "HashType_SHA256" },
  { no: 2, name: "HashType_SHA1" },
  { no: 3, name: "HashType_BLAKE3" }
]);
var Hash = createMessageType({
  typeName: "hash.Hash",
  fields: [
    { no: 1, name: "hash_type", kind: "enum", T: HashType_Enum },
    { no: 2, name: "hash", kind: "scalar", T: ScalarType.BYTES }
  ],
  packedByDefault: true
});

// ../../../vendor/github.com/aperturerobotics/hydra/block/block.pb.ts
createEnumType("block.OverlayMode", [
  { no: 0, name: "UPPER_ONLY" },
  { no: 1, name: "LOWER_ONLY" },
  { no: 2, name: "UPPER_CACHE" },
  { no: 3, name: "LOWER_CACHE" },
  { no: 4, name: "UPPER_READ_CACHE" },
  { no: 5, name: "LOWER_READ_CACHE" },
  { no: 6, name: "UPPER_WRITE_CACHE" },
  { no: 7, name: "LOWER_WRITE_CACHE" }
]);
var BlockRef = createMessageType({
  typeName: "block.BlockRef",
  fields: [
    { no: 1, name: "hash", kind: "message", T: () => Hash }
  ],
  packedByDefault: true
});
var PutOpts = createMessageType({
  typeName: "block.PutOpts",
  fields: [
    { no: 1, name: "hash_type", kind: "enum", T: HashType_Enum },
    { no: 2, name: "force_block_ref", kind: "message", T: () => BlockRef }
  ],
  packedByDefault: true
});

// ../../../node_modules/@aptre/protobuf-es-lite/dist/google/protobuf/timestamp.pb.js
var Timestamp_Wkt = {
  fromJson(json) {
    if (typeof json !== "string") {
      throw new Error(`cannot decode google.protobuf.Timestamp(json)}`);
    }
    const matches = json.match(/^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:Z|\.([0-9]{3,9})Z|([+-][0-9][0-9]:[0-9][0-9]))$/);
    if (!matches) {
      throw new Error(`cannot decode google.protobuf.Timestamp from JSON: invalid RFC 3339 string`);
    }
    const ms = Date.parse(matches[1] + "-" + matches[2] + "-" + matches[3] + "T" + matches[4] + ":" + matches[5] + ":" + matches[6] + (matches[8] ? matches[8] : "Z"));
    if (Number.isNaN(ms)) {
      throw new Error(`cannot decode google.protobuf.Timestamp from JSON: invalid RFC 3339 string`);
    }
    if (ms < Date.parse("0001-01-01T00:00:00Z") || ms > Date.parse("9999-12-31T23:59:59Z")) {
      throw new Error(`cannot decode message google.protobuf.Timestamp from JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
    }
    return {
      seconds: protoInt64.parse(ms / 1e3),
      nanos: !matches[7] ? 0 : parseInt("1" + matches[7] + "0".repeat(9 - matches[7].length)) - 1e9
    };
  },
  toJson(msg) {
    const ms = Number(msg.seconds) * 1e3;
    if (ms < Date.parse("0001-01-01T00:00:00Z") || ms > Date.parse("9999-12-31T23:59:59Z")) {
      throw new Error(`cannot encode google.protobuf.Timestamp to JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
    }
    if (msg.nanos != null && msg.nanos < 0) {
      throw new Error(`cannot encode google.protobuf.Timestamp to JSON: nanos must not be negative`);
    }
    let z = "Z";
    if (msg.nanos != null && msg.nanos > 0) {
      const nanosStr = (msg.nanos + 1e9).toString().substring(1);
      if (nanosStr.substring(3) === "000000") {
        z = "." + nanosStr.substring(0, 3) + "Z";
      } else if (nanosStr.substring(6) === "000") {
        z = "." + nanosStr.substring(0, 6) + "Z";
      } else {
        z = "." + nanosStr + "Z";
      }
    }
    return new Date(ms).toISOString().replace(".000Z", z);
  },
  toDate(msg) {
    if (!msg?.seconds && !msg?.nanos) {
      return null;
    }
    return new Date(Number(msg.seconds ?? 0) * 1e3 + Math.ceil((msg.nanos ?? 0) / 1e6));
  },
  fromDate(value) {
    if (value == null) {
      return {};
    }
    const ms = value.getTime();
    const seconds = Math.floor(ms / 1e3);
    const nanos = ms % 1e3 * 1e6;
    return { seconds: protoInt64.parse(seconds), nanos };
  },
  equals(a, b) {
    const aDate = a instanceof Date ? a : Timestamp_Wkt.toDate(a);
    const bDate = b instanceof Date ? b : Timestamp_Wkt.toDate(b);
    if (aDate === bDate) {
      return true;
    }
    if (aDate == null || bDate == null) {
      return aDate === bDate;
    }
    return +aDate === +bDate;
  }
};
var Timestamp = createMessageType({
  typeName: "google.protobuf.Timestamp",
  fields: [
    { no: 1, name: "seconds", kind: "scalar", T: ScalarType.INT64 },
    { no: 2, name: "nanos", kind: "scalar", T: ScalarType.INT32 }
  ],
  packedByDefault: true,
  fieldWrapper: {
    wrapField(value) {
      if (value == null || value instanceof Date) {
        return Timestamp_Wkt.fromDate(value);
      }
      return Timestamp.createComplete(value);
    },
    unwrapField(msg) {
      return Timestamp_Wkt.toDate(msg);
    }
  }
}, Timestamp_Wkt);

// ../../../vendor/github.com/aperturerobotics/hydra/block/transform/transform.pb.ts
var StepConfig = createMessageType({
  typeName: "block.transform.StepConfig",
  fields: [
    { no: 1, name: "id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "config", kind: "scalar", T: ScalarType.BYTES }
  ],
  packedByDefault: true
});
var Config = createMessageType({
  typeName: "block.transform.Config",
  fields: [
    {
      no: 1,
      name: "steps",
      kind: "message",
      T: () => StepConfig,
      repeated: true
    }
  ],
  packedByDefault: true
});

// ../../../vendor/github.com/aperturerobotics/hydra/bucket/bucket.pb.ts
var ReconcilerConfig = createMessageType({
  typeName: "bucket.ReconcilerConfig",
  fields: [
    { no: 1, name: "id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "controller", kind: "message", T: () => ControllerConfig },
    { no: 3, name: "filter_put", kind: "scalar", T: ScalarType.BOOL }
  ],
  packedByDefault: true
});
var LookupConfig = createMessageType({
  typeName: "bucket.LookupConfig",
  fields: [
    { no: 1, name: "disable", kind: "scalar", T: ScalarType.BOOL },
    { no: 2, name: "controller", kind: "message", T: () => ControllerConfig }
  ],
  packedByDefault: true
});
var Config2 = createMessageType({
  typeName: "bucket.Config",
  fields: [
    { no: 1, name: "id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "rev", kind: "scalar", T: ScalarType.UINT32 },
    {
      no: 3,
      name: "reconcilers",
      kind: "message",
      T: () => ReconcilerConfig,
      repeated: true
    },
    { no: 4, name: "put_opts", kind: "message", T: () => PutOpts },
    { no: 5, name: "lookup", kind: "message", T: () => LookupConfig }
  ],
  packedByDefault: true
});
var BucketInfo = createMessageType({
  typeName: "bucket.BucketInfo",
  fields: [
    { no: 1, name: "config", kind: "message", T: () => Config2 }
  ],
  packedByDefault: true
});
createMessageType({
  typeName: "bucket.ApplyBucketConfigResult",
  fields: [
    { no: 1, name: "volume_id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "bucket_id", kind: "scalar", T: ScalarType.STRING },
    { no: 3, name: "bucket_conf", kind: "message", T: () => Config2 },
    { no: 4, name: "old_bucket_conf", kind: "message", T: () => Config2 },
    { no: 5, name: "timestamp", kind: "message", T: () => Timestamp },
    { no: 6, name: "updated", kind: "scalar", T: ScalarType.BOOL },
    { no: 7, name: "error", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
var ObjectRef = createMessageType({
  typeName: "bucket.ObjectRef",
  fields: [
    { no: 1, name: "root_ref", kind: "message", T: () => BlockRef },
    { no: 2, name: "bucket_id", kind: "scalar", T: ScalarType.STRING },
    { no: 3, name: "transform_conf_ref", kind: "message", T: () => BlockRef },
    { no: 4, name: "transform_conf", kind: "message", T: () => Config }
  ],
  packedByDefault: true
});
createMessageType({
  typeName: "bucket.BucketOpArgs",
  fields: [
    { no: 1, name: "bucket_id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "volume_id", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});

// ../../../manifest/manifest.pb.ts
var ManifestMeta = createMessageType({
  typeName: "bldr.manifest.ManifestMeta",
  fields: [
    { no: 1, name: "manifest_id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "build_type", kind: "scalar", T: ScalarType.STRING },
    { no: 3, name: "platform_id", kind: "scalar", T: ScalarType.STRING },
    { no: 4, name: "rev", kind: "scalar", T: ScalarType.UINT64 },
    { no: 5, name: "description", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
var Manifest = createMessageType({
  typeName: "bldr.manifest.Manifest",
  fields: [
    { no: 1, name: "meta", kind: "message", T: () => ManifestMeta },
    { no: 2, name: "entrypoint", kind: "scalar", T: ScalarType.STRING },
    { no: 3, name: "dist_fs_ref", kind: "message", T: () => BlockRef },
    { no: 4, name: "assets_fs_ref", kind: "message", T: () => BlockRef }
  ],
  packedByDefault: true
});
var ManifestRef = createMessageType({
  typeName: "bldr.manifest.ManifestRef",
  fields: [
    { no: 1, name: "meta", kind: "message", T: () => ManifestMeta },
    { no: 2, name: "manifest_ref", kind: "message", T: () => ObjectRef }
  ],
  packedByDefault: true
});
createMessageType({
  typeName: "bldr.manifest.ManifestBundle",
  fields: [
    {
      no: 1,
      name: "manifest_refs",
      kind: "message",
      T: () => ManifestRef,
      repeated: true
    },
    { no: 2, name: "timestamp", kind: "message", T: () => Timestamp }
  ],
  packedByDefault: true
});
createMessageType({
  typeName: "bldr.manifest.ManifestSnapshot",
  fields: [
    { no: 1, name: "manifest_ref", kind: "message", T: () => ObjectRef },
    { no: 2, name: "manifest", kind: "message", T: () => Manifest }
  ],
  packedByDefault: true
});
createMessageType({
  typeName: "bldr.manifest.FetchManifestRequest",
  fields: [
    { no: 1, name: "manifest_id", kind: "scalar", T: ScalarType.STRING },
    {
      no: 2,
      name: "build_types",
      kind: "scalar",
      T: ScalarType.STRING,
      repeated: true
    },
    {
      no: 3,
      name: "platform_ids",
      kind: "scalar",
      T: ScalarType.STRING,
      repeated: true
    },
    { no: 4, name: "rev", kind: "scalar", T: ScalarType.UINT64 }
  ],
  packedByDefault: true
});
var FetchManifestValue = createMessageType({
  typeName: "bldr.manifest.FetchManifestValue",
  fields: [
    {
      no: 1,
      name: "manifest_refs",
      kind: "message",
      T: () => ManifestRef,
      repeated: true
    }
  ],
  packedByDefault: true
});
createMessageType({
  typeName: "bldr.manifest.FetchManifestResponse",
  fields: [
    { no: 1, name: "value_id", kind: "scalar", T: ScalarType.UINT32 },
    { no: 2, name: "value", kind: "message", T: () => FetchManifestValue },
    { no: 3, name: "removed", kind: "scalar", T: ScalarType.BOOL },
    { no: 4, name: "idle", kind: "scalar", T: ScalarType.UINT32 }
  ],
  packedByDefault: true
});

// ../../../vendor/github.com/aperturerobotics/hydra/volume/volume.pb.ts
var VolumeInfo = createMessageType({
  typeName: "volume.VolumeInfo",
  fields: [
    { no: 1, name: "volume_id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "peer_id", kind: "scalar", T: ScalarType.STRING },
    { no: 3, name: "peer_pub", kind: "scalar", T: ScalarType.STRING },
    { no: 4, name: "controller_info", kind: "message", T: () => Info },
    { no: 5, name: "hash_type", kind: "enum", T: HashType_Enum }
  ],
  packedByDefault: true
});
createMessageType({
  typeName: "volume.VolumeBucketInfo",
  fields: [
    { no: 1, name: "bucket_info", kind: "message", T: () => BucketInfo },
    { no: 2, name: "volume_info", kind: "message", T: () => VolumeInfo }
  ],
  packedByDefault: true
});
createMessageType({
  typeName: "volume.ListBucketsRequest",
  fields: [
    { no: 1, name: "bucket_id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "volume_id_re", kind: "scalar", T: ScalarType.STRING },
    {
      no: 3,
      name: "volume_id_list",
      kind: "scalar",
      T: ScalarType.STRING,
      repeated: true
    }
  ],
  packedByDefault: true
});

// ../../plugin.pb.ts
var PluginStatus = createMessageType({
  typeName: "bldr.plugin.PluginStatus",
  fields: [
    { no: 1, name: "plugin_id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "running", kind: "scalar", T: ScalarType.BOOL }
  ],
  packedByDefault: true
});
var GetPluginInfoRequest = createMessageType({
  typeName: "bldr.plugin.GetPluginInfoRequest",
  fields: [],
  packedByDefault: true
});
var GetPluginInfoResponse = createMessageType({
  typeName: "bldr.plugin.GetPluginInfoResponse",
  fields: [
    { no: 1, name: "plugin_id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "manifest_ref", kind: "message", T: () => ManifestRef },
    { no: 3, name: "host_volume_info", kind: "message", T: () => VolumeInfo }
  ],
  packedByDefault: true
});
var LoadPluginRequest = createMessageType({
  typeName: "bldr.plugin.LoadPluginRequest",
  fields: [
    { no: 1, name: "plugin_id", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
var LoadPluginResponse = createMessageType({
  typeName: "bldr.plugin.LoadPluginResponse",
  fields: [
    { no: 1, name: "plugin_status", kind: "message", T: () => PluginStatus }
  ],
  packedByDefault: true
});
var PluginMeta = createMessageType({
  typeName: "bldr.plugin.PluginMeta",
  fields: [
    { no: 1, name: "project_id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "plugin_id", kind: "scalar", T: ScalarType.STRING },
    { no: 3, name: "platform_id", kind: "scalar", T: ScalarType.STRING },
    { no: 4, name: "build_type", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
var PluginStartInfo = createMessageType({
  typeName: "bldr.plugin.PluginStartInfo",
  fields: [
    { no: 1, name: "instance_id", kind: "scalar", T: ScalarType.STRING },
    { no: 2, name: "plugin_id", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
createMessageType({
  typeName: "bldr.plugin.PluginContextInfo",
  fields: [
    { no: 1, name: "plugin_meta", kind: "message", T: () => PluginMeta }
  ],
  packedByDefault: true
});

// ../../../vendor/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.ts
var RpcStreamInit2 = createMessageType({
  typeName: "rpcstream.RpcStreamInit",
  fields: [
    { no: 1, name: "component_id", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
var RpcAck2 = createMessageType({
  typeName: "rpcstream.RpcAck",
  fields: [
    { no: 1, name: "error", kind: "scalar", T: ScalarType.STRING }
  ],
  packedByDefault: true
});
var RpcStreamPacket2 = createMessageType({
  typeName: "rpcstream.RpcStreamPacket",
  fields: [
    {
      no: 1,
      name: "init",
      kind: "message",
      T: () => RpcStreamInit2,
      oneof: "body"
    },
    { no: 2, name: "ack", kind: "message", T: () => RpcAck2, oneof: "body" },
    { no: 3, name: "data", kind: "scalar", T: ScalarType.BYTES, oneof: "body" }
  ],
  packedByDefault: true
});

// ../../plugin_srpc.pb.ts
var PluginHostDefinition = {
  typeName: "bldr.plugin.PluginHost",
  methods: {
    /**
     * GetPluginInfo returns the information for the current plugin.
     *
     * @generated from rpc bldr.plugin.PluginHost.GetPluginInfo
     */
    GetPluginInfo: {
      name: "GetPluginInfo",
      kind: MethodKind.Unary
    },
    /**
     * ExecController executes a controller configuration on the bus.
     *
     * @generated from rpc bldr.plugin.PluginHost.ExecController
     */
    ExecController: {
      name: "ExecController",
      kind: MethodKind.ServerStreaming
    },
    /**
     * LoadPlugin requests to load the plugin with the given ID.
     * The plugin will remain loaded as long as the RPC is active.
     * Multiple requests to load the same plugin are de-duplicated.
     *
     * @generated from rpc bldr.plugin.PluginHost.LoadPlugin
     */
    LoadPlugin: {
      name: "LoadPlugin",
      kind: MethodKind.ServerStreaming
    },
    /**
     * PluginRpc forwards an RPC call to a remote plugin.
     * The plugin will remain loaded as long as the RPC is active.
     * Component ID: plugin id
     *
     * @generated from rpc bldr.plugin.PluginHost.PluginRpc
     */
    PluginRpc: {
      name: "PluginRpc",
      kind: MethodKind.BiDiStreaming
    },
    /**
     * PluginFsRpc accesses a FSCursorService to access plugin assets or dist filesystems.
     * The plugin will remain loaded as long as the RPC is active.
     * Component ID: plugin-assets or plugin-dist for current plugin
     * Component ID: plugin-assets/{plugin-id} or plugin-dist/{plugin-id} for remote plugin
     *
     * @generated from rpc bldr.plugin.PluginHost.PluginFsRpc
     */
    PluginFsRpc: {
      name: "PluginFsRpc",
      kind: MethodKind.BiDiStreaming
    }
  }
};
var PluginHostServiceName = PluginHostDefinition.typeName;
var PluginHostClient = class {
  rpc;
  service;
  constructor(rpc, opts) {
    this.service = opts?.service || PluginHostServiceName;
    this.rpc = rpc;
    this.GetPluginInfo = this.GetPluginInfo.bind(this);
    this.ExecController = this.ExecController.bind(this);
    this.LoadPlugin = this.LoadPlugin.bind(this);
    this.PluginRpc = this.PluginRpc.bind(this);
    this.PluginFsRpc = this.PluginFsRpc.bind(this);
  }
  /**
   * GetPluginInfo returns the information for the current plugin.
   *
   * @generated from rpc bldr.plugin.PluginHost.GetPluginInfo
   */
  async GetPluginInfo(request, abortSignal) {
    const requestMsg = GetPluginInfoRequest.create(request);
    const result = await this.rpc.request(
      this.service,
      PluginHostDefinition.methods.GetPluginInfo.name,
      GetPluginInfoRequest.toBinary(requestMsg),
      abortSignal || void 0
    );
    return GetPluginInfoResponse.fromBinary(result);
  }
  /**
   * ExecController executes a controller configuration on the bus.
   *
   * @generated from rpc bldr.plugin.PluginHost.ExecController
   */
  ExecController(request, abortSignal) {
    const requestMsg = ExecControllerRequest.create(request);
    const result = this.rpc.serverStreamingRequest(
      this.service,
      PluginHostDefinition.methods.ExecController.name,
      ExecControllerRequest.toBinary(requestMsg),
      abortSignal || void 0
    );
    return buildDecodeMessageTransform(ExecControllerResponse)(result);
  }
  /**
   * LoadPlugin requests to load the plugin with the given ID.
   * The plugin will remain loaded as long as the RPC is active.
   * Multiple requests to load the same plugin are de-duplicated.
   *
   * @generated from rpc bldr.plugin.PluginHost.LoadPlugin
   */
  LoadPlugin(request, abortSignal) {
    const requestMsg = LoadPluginRequest.create(request);
    const result = this.rpc.serverStreamingRequest(
      this.service,
      PluginHostDefinition.methods.LoadPlugin.name,
      LoadPluginRequest.toBinary(requestMsg),
      abortSignal || void 0
    );
    return buildDecodeMessageTransform(LoadPluginResponse)(result);
  }
  /**
   * PluginRpc forwards an RPC call to a remote plugin.
   * The plugin will remain loaded as long as the RPC is active.
   * Component ID: plugin id
   *
   * @generated from rpc bldr.plugin.PluginHost.PluginRpc
   */
  PluginRpc(request, abortSignal) {
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      PluginHostDefinition.methods.PluginRpc.name,
      buildEncodeMessageTransform(RpcStreamPacket2)(request),
      abortSignal || void 0
    );
    return buildDecodeMessageTransform(RpcStreamPacket2)(result);
  }
  /**
   * PluginFsRpc accesses a FSCursorService to access plugin assets or dist filesystems.
   * The plugin will remain loaded as long as the RPC is active.
   * Component ID: plugin-assets or plugin-dist for current plugin
   * Component ID: plugin-assets/{plugin-id} or plugin-dist/{plugin-id} for remote plugin
   *
   * @generated from rpc bldr.plugin.PluginHost.PluginFsRpc
   */
  PluginFsRpc(request, abortSignal) {
    const result = this.rpc.bidirectionalStreamingRequest(
      this.service,
      PluginHostDefinition.methods.PluginFsRpc.name,
      buildEncodeMessageTransform(RpcStreamPacket2)(request),
      abortSignal || void 0
    );
    return buildDecodeMessageTransform(RpcStreamPacket2)(result);
  }
};
({
  methods: {
    /**
     * PluginRpc handles an RPC call from a remote plugin.
     * Component ID: remote plugin id
     *
     * @generated from rpc bldr.plugin.Plugin.PluginRpc
     */
    PluginRpc: {
      kind: MethodKind.BiDiStreaming
    }
  }
});

// ../../../sdk/impl/backend-api.ts
var BackendApiImpl = class {
  // startInfo is the start information passed during initialization.
  startInfo;
  // openStream is the open stream func for client
  openStream;
  // client is a connection to the Go WebRuntime via. WebWorkerRpc rpcstream.
  client;
  // pluginHost is the plugin host RPC service client.
  pluginHost;
  // handleStreamCtr allows the plugin module to register a function
  // that will be called to handle incoming streams from the WebRuntime.
  handleStreamCtr;
  // protos contains the protobuf objects used by the BackendAPI.
  protos = {
    PluginStartInfo,
    GetPluginInfoRequest,
    GetPluginInfoResponse,
    ExecControllerRequest,
    ExecControllerResponse,
    LoadPluginRequest,
    LoadPluginResponse,
    RpcStreamPacket
  };
  // HTTP prefix constants
  constants = {
    BLDR_HTTP_PREFIX: "/b/",
    PLUGIN_DIST_HTTP_PREFIX: "/b/pd/",
    PLUGIN_ASSETS_HTTP_PREFIX: "/b/pa/",
    PLUGIN_WEB_PKG_HTTP_PREFIX: "/b/pkg/",
    PLUGIN_HTTP_PREFIX: "/p/"
  };
  // HTTP path utility functions
  utils = {
    // pluginHttpPath adds the plugin http prefix to the given path.
    pluginHttpPath: (pluginId, ...httpPaths) => {
      let result = this.constants.PLUGIN_HTTP_PREFIX + pluginId;
      if (httpPaths.length === 0 || !httpPaths[0].startsWith("/")) {
        result += "/";
      }
      for (const httpPath of httpPaths) {
        result += httpPath;
      }
      return result;
    },
    // pluginDistHttpPath adds the plugin distribution file prefix to the given path.
    pluginDistHttpPath: (pluginId, httpPath) => {
      let result = this.constants.PLUGIN_DIST_HTTP_PREFIX + pluginId;
      if (!httpPath.startsWith("/")) {
        result += "/";
      }
      result += httpPath;
      return result;
    },
    // pluginAssetHttpPath adds the plugin asset file prefix to the given path.
    pluginAssetHttpPath: (pluginId, httpPath) => {
      let result = this.constants.PLUGIN_ASSETS_HTTP_PREFIX + pluginId;
      if (!httpPath.startsWith("/")) {
        result += "/";
      }
      result += httpPath;
      return result;
    }
  };
  constructor(startInfo, openStream2, handleStreamCtr) {
    this.startInfo = startInfo;
    this.openStream = openStream2;
    this.client = new Client(openStream2);
    this.handleStreamCtr = handleStreamCtr;
    this.pluginHost = new PluginHostClient(this.client);
  }
  // buildPluginOpenStream builds an OpenStreamFunc for RPCs to a remote plugin.
  buildPluginOpenStream(pluginID) {
    return buildRpcStreamOpenStream(pluginID, this.pluginHost.PluginRpc);
  }
};

// plugin-quickjs.ts
function logError(message, err) {
  console.error(message);
  console.error(
    ("message" in err ? err?.message : null) ?? String(err)
  );
  if (err && typeof err === "object" && "stack" in err) {
    console.error(err.stack);
  }
}
var scriptPath = globalThis.std.getenv("BLDR_SCRIPT_PATH");
if (!scriptPath) {
  globalThis.console.log("BLDR_SCRIPT_PATH must be defined");
  globalThis.std.exit(1);
}
var polyGlobalThis = applyPolyfills(globalThis);
var scriptPromise = import(scriptPath);
scriptPromise.catch((err) => {
  logError("error importing script: " + scriptPath, err);
  globalThis.std.exit(1);
});
var startInfoB64 = globalThis.std.getenv("BLDR_PLUGIN_START_INFO") ?? "";
var handleIncomingStreamCtr = new HandleStreamCtr();
var handleIncomingStream = handleIncomingStreamCtr.handleStreamFunc;
var openStreamCtr = new OpenStreamCtr();
var openStream = openStreamCtr.openStreamFunc;
var stdinFd = 0;
var stdinReadBuffer = new Uint8Array(32 * 1024);
var runtimeConn = new StreamConn(
  { handlePacketStream: handleIncomingStream },
  {
    direction: "inbound",
    yamuxParams: {
      enableKeepAlive: false,
      maxMessageSize: 32 * 1024
    }
  }
);
var stdinStream = pushable({ objectMode: true });
function stdinReadHandler() {
  const bytesRead = globalThis.os.read(
    stdinFd,
    stdinReadBuffer.buffer,
    0,
    stdinReadBuffer.length
  );
  if (bytesRead === 0) {
    return;
  }
  const readData = stdinReadBuffer.slice(0, bytesRead);
  stdinStream.push(readData);
}
globalThis.os.setReadHandler(stdinFd, stdinReadHandler);
pipe(
  stdinStream,
  runtimeConn,
  async (source) => writeSourceToFd(globalThis.os, source, "/dev/out")
).catch((err) => {
  logError("caught error in pipe", err);
  globalThis.std.exit(1);
});
openStreamCtr.set(runtimeConn.buildOpenStreamFunc());
async function startPlugin() {
  const script = await scriptPromise;
  if (typeof script.default !== "function") {
    throw new Error(
      `shared-worker: Imported module "${scriptPath}" does not have a default export function.`
    );
  }
  let startInfo;
  if (startInfoB64) {
    const startInfoJson = polyGlobalThis.atob(startInfoB64);
    startInfo = PluginStartInfo.fromJsonString(startInfoJson);
  } else {
    startInfo = {};
  }
  const backendAPI = new BackendApiImpl(
    startInfo,
    openStream,
    handleIncomingStreamCtr
  );
  globalThis.gc?.();
  const abortController = new AbortController();
  const abortSignal = abortController.signal;
  await script.default(backendAPI, abortSignal);
}
startPlugin().catch((err) => {
  logError("startPlugin exited w/error", err);
  globalThis.std.exit(1);
});
