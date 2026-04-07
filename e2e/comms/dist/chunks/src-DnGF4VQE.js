//#region node_modules/p-defer/index.js
function pDefer() {
	const deferred = {};
	deferred.promise = new Promise((resolve, reject) => {
		deferred.resolve = resolve;
		deferred.reject = reject;
	});
	return deferred;
}
//#endregion
//#region node_modules/it-pushable/dist/src/fifo.js
var FixedFIFO = class {
	buffer;
	mask;
	top;
	btm;
	next;
	constructor(hwm) {
		if (!(hwm > 0) || (hwm - 1 & hwm) !== 0) throw new Error("Max size for a FixedFIFO should be a power of two");
		this.buffer = new Array(hwm);
		this.mask = hwm - 1;
		this.top = 0;
		this.btm = 0;
		this.next = null;
	}
	push(data) {
		if (this.buffer[this.top] !== void 0) return false;
		this.buffer[this.top] = data;
		this.top = this.top + 1 & this.mask;
		return true;
	}
	shift() {
		const last = this.buffer[this.btm];
		if (last === void 0) return;
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
		if (obj?.byteLength != null) return obj.byteLength;
		return 1;
	}
	push(val) {
		if (val?.value != null) this.size += this.calculateSize(val.value);
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
		if (val?.value != null) this.size -= this.calculateSize(val.value);
		return val;
	}
	isEmpty() {
		return this.head.isEmpty();
	}
};
//#endregion
//#region node_modules/it-pushable/dist/src/index.js
/**
* @packageDocumentation
*
* An iterable that you can push values into.
*
* @example
*
* ```js
* import { pushable } from 'it-pushable'
*
* const source = pushable()
*
* setTimeout(() => source.push('hello'), 100)
* setTimeout(() => source.push('world'), 200)
* setTimeout(() => source.end(), 300)
*
* const start = Date.now()
*
* for await (const value of source) {
*   console.log(`got "${value}" after ${Date.now() - start}ms`)
* }
* console.log(`done after ${Date.now() - start}ms`)
*
* // Output:
* // got "hello" after 105ms
* // got "world" after 207ms
* // done after 309ms
* ```
*
* @example
*
* ```js
* import { pushableV } from 'it-pushable'
* import all from 'it-all'
*
* const source = pushableV()
*
* source.push(1)
* source.push(2)
* source.push(3)
* source.end()
*
* console.info(await all(source))
*
* // Output:
* // [ [1, 2, 3] ]
* ```
*/
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
		if (next == null) return { done: true };
		if (next.error != null) throw next.error;
		return {
			done: next.done === true,
			value: next.value
		};
	};
	return _pushable(getNext, options);
}
function _pushable(getNext, options) {
	options = options ?? {};
	let onEnd = options.onEnd;
	let buffer = new FIFO();
	let pushable;
	let onNext;
	let ended;
	let drain = pDefer();
	const waitNext = async () => {
		try {
			if (!buffer.isEmpty()) return getNext(buffer);
			if (ended) return { done: true };
			return await new Promise((resolve, reject) => {
				onNext = (next) => {
					onNext = null;
					buffer.push(next);
					try {
						resolve(getNext(buffer));
					} catch (err) {
						reject(err);
					}
					return pushable;
				};
			});
		} finally {
			if (buffer.isEmpty()) queueMicrotask(() => {
				drain.resolve();
				drain = pDefer();
			});
		}
	};
	const bufferNext = (next) => {
		if (onNext != null) return onNext(next);
		buffer.push(next);
		return pushable;
	};
	const bufferError = (err) => {
		buffer = new FIFO();
		if (onNext != null) return onNext({ error: err });
		buffer.push({ error: err });
		return pushable;
	};
	const push = (value) => {
		if (ended) return pushable;
		if (options?.objectMode !== true && value?.byteLength == null) throw new Error("objectMode was not true but tried to push non-Uint8Array value");
		return bufferNext({
			done: false,
			value
		});
	};
	const end = (err) => {
		if (ended) return pushable;
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
	pushable = {
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
		onEmpty: async (options) => {
			const signal = options?.signal;
			signal?.throwIfAborted();
			if (buffer.isEmpty()) return;
			let cancel;
			let listener;
			if (signal != null) cancel = new Promise((resolve, reject) => {
				listener = () => {
					reject(new AbortError());
				};
				signal.addEventListener("abort", listener);
			});
			try {
				await Promise.race([drain.promise, cancel]);
			} finally {
				if (listener != null && signal != null) signal?.removeEventListener("abort", listener);
			}
		}
	};
	if (onEnd == null) return pushable;
	const _pushable = pushable;
	pushable = {
		[Symbol.asyncIterator]() {
			return this;
		},
		next() {
			return _pushable.next();
		},
		throw(err) {
			_pushable.throw(err);
			if (onEnd != null) {
				onEnd(err);
				onEnd = void 0;
			}
			return { done: true };
		},
		return() {
			_pushable.return();
			if (onEnd != null) {
				onEnd();
				onEnd = void 0;
			}
			return { done: true };
		},
		push,
		end(err) {
			_pushable.end(err);
			if (onEnd != null) {
				onEnd(err);
				onEnd = void 0;
			}
			return pushable;
		},
		get readableLength() {
			return _pushable.readableLength;
		},
		onEmpty: (opts) => {
			return _pushable.onEmpty(opts);
		}
	};
	return pushable;
}
//#endregion
export { pDefer as n, pushable as t };

//# sourceMappingURL=src-DnGF4VQE.js.map