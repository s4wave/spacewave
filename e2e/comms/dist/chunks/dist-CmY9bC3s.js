import { n as pDefer, t as pushable } from "./src-DnGF4VQE.js";
//#region \0rolldown/runtime.js
var __commonJSMin = (cb, mod) => () => (mod || cb((mod = { exports: {} }).exports, mod), mod.exports);
//#endregion
//#region node_modules/starpc/dist/srpc/errors.js
var ERR_RPC_ABORT = "ERR_RPC_ABORT";
var ERR_STREAM_IDLE = "ERR_STREAM_IDLE";
function castToError(err, defaultMsg) {
	defaultMsg = defaultMsg || "error";
	if (!err) return new Error(defaultMsg);
	if (typeof err === "string") return new Error(err);
	const asError = err;
	if (asError.message) return asError;
	if (err.toString) {
		const errString = err.toString();
		if (errString) return new Error(errString);
	}
	return new Error(defaultMsg);
}
//#endregion
//#region node_modules/it-queueless-pushable/node_modules/race-signal/dist/src/index.js
/**
* @packageDocumentation
*
* Pass a promise and an abort signal and await the result.
*
* @example Basic usage
*
* ```ts
* import { raceSignal } from 'race-signal'
*
* const controller = new AbortController()
*
* const promise = new Promise((resolve, reject) => {
*   setTimeout(() => {
*     resolve('a value')
*   }, 1000)
* })
*
* setTimeout(() => {
*   controller.abort()
* }, 500)
*
* // throws an AbortError
* const resolve = await raceSignal(promise, controller.signal)
* ```
*
* @example Overriding errors
*
* By default the thrown error is the `.reason` property of the signal but it's
* possible to override this behaviour with the `translateError` option:
*
* ```ts
* import { raceSignal } from 'race-signal'
*
* const controller = new AbortController()
*
* const promise = new Promise((resolve, reject) => {
*   setTimeout(() => {
*     resolve('a value')
*   }, 1000)
* })
*
* setTimeout(() => {
*   controller.abort()
* }, 500)
*
* // throws `Error('Oh no!')`
* const resolve = await raceSignal(promise, controller.signal, {
*   translateError: (signal) => {
*     // use `signal`, or don't
*     return new Error('Oh no!')
*   }
* })
* ```
*/
function defaultTranslate(signal) {
	return signal.reason;
}
/**
* Race a promise against an abort signal
*/
async function raceSignal(promise, signal, opts) {
	if (signal == null) return promise;
	const translateError = opts?.translateError ?? defaultTranslate;
	if (signal.aborted) {
		promise.catch(() => {});
		return Promise.reject(translateError(signal));
	}
	let listener;
	try {
		return await Promise.race([promise, new Promise((resolve, reject) => {
			listener = () => {
				reject(translateError(signal));
			};
			signal.addEventListener("abort", listener);
		})]);
	} finally {
		if (listener != null) signal.removeEventListener("abort", listener);
	}
}
//#endregion
//#region node_modules/it-queueless-pushable/dist/src/index.js
/**
* @packageDocumentation
*
* A pushable async generator that waits until the current value is consumed
* before allowing a new value to be pushed.
*
* Useful for when you don't want to keep memory usage under control and/or
* allow a downstream consumer to dictate how fast data flows through a pipe,
* but you want to be able to apply a transform to that data.
*
* @example
*
* ```typescript
* import { queuelessPushable } from 'it-queueless-pushable'
*
* const pushable = queuelessPushable<string>()
*
* // run asynchronously
* Promise.resolve().then(async () => {
*   // push a value - the returned promise will not resolve until the value is
*   // read from the pushable
*   await pushable.push('hello')
* })
*
* // read a value
* const result = await pushable.next()
* console.info(result) // { done: false, value: 'hello' }
* ```
*/
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
		if (this.nextResult == null) await this.haveNext.promise;
		if (this.nextResult == null) throw new Error("HaveNext promise resolved but nextResult was undefined");
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
			this.haveNext.promise.catch(() => {});
			this.haveNext.reject(err);
		}
		return {
			done: true,
			value: void 0
		};
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
		if (err != null) await this.throw(err);
		else await this._push(void 0, options);
	}
	async _push(value, options) {
		if (value != null && this.ended) throw this.error ?? /* @__PURE__ */ new Error("Cannot push value onto an ended pushable");
		while (this.nextResult != null) await this.readNext.promise;
		if (value != null) this.nextResult = {
			done: false,
			value
		};
		else {
			this.ended = true;
			this.nextResult = {
				done: true,
				value: void 0
			};
		}
		this.haveNext.resolve();
		this.haveNext = pDefer();
		await raceSignal(this.readNext.promise, options?.signal, options);
	}
};
function queuelessPushable() {
	return new QueuelessPushable();
}
//#endregion
//#region node_modules/it-merge/dist/src/index.js
/**
* @packageDocumentation
*
* Merge several (async)iterables into one, yield values as they arrive.
*
* Nb. sources are iterated over in parallel so the order of emitted items is not guaranteed.
*
* @example
*
* ```javascript
* import merge from 'it-merge'
* import all from 'it-all'
*
* // This can also be an iterator, generator, etc
* const values1 = [0, 1, 2, 3, 4]
* const values2 = [5, 6, 7, 8, 9]
*
* const arr = all(merge(values1, values2))
*
* console.info(arr) // 0, 1, 2, 3, 4, 5, 6, 7, 8, 9
* ```
*
* Async sources must be awaited:
*
* ```javascript
* import merge from 'it-merge'
* import all from 'it-all'
*
* // This can also be an iterator, async iterator, generator, etc
* const values1 = async function * () {
*   yield * [0, 1, 2, 3, 4]
* }
* const values2 = async function * () {
*   yield * [5, 6, 7, 8, 9]
* }
*
* const arr = await all(merge(values1(), values2()))
*
* console.info(arr) // 0, 1, 5, 6, 2, 3, 4, 7, 8, 9  <- nb. order is not guaranteed
* ```
*/
function isAsyncIterable$2(thing) {
	return thing[Symbol.asyncIterator] != null;
}
async function addAllToPushable(sources, output, signal) {
	try {
		await Promise.all(sources.map(async (source) => {
			for await (const item of source) {
				await output.push(item, { signal });
				signal.throwIfAborted();
			}
		}));
		await output.end(void 0, { signal });
	} catch (err) {
		await output.end(err, { signal }).catch(() => {});
	}
}
async function* mergeSources(sources) {
	const controller = new AbortController();
	const output = queuelessPushable();
	addAllToPushable(sources, output, controller.signal).catch(() => {});
	try {
		yield* output;
	} finally {
		controller.abort();
	}
}
function* mergeSyncSources(syncSources) {
	for (const source of syncSources) yield* source;
}
function merge(...sources) {
	const syncSources = [];
	for (const source of sources) if (!isAsyncIterable$2(source)) syncSources.push(source);
	if (syncSources.length === sources.length) return mergeSyncSources(syncSources);
	return mergeSources(sources);
}
//#endregion
//#region node_modules/it-pipe/dist/src/index.js
function pipe(first, ...rest) {
	if (first == null) throw new Error("Empty pipeline");
	if (isDuplex(first)) {
		const duplex = first;
		first = () => duplex.source;
	} else if (isIterable(first) || isAsyncIterable$1(first)) {
		const source = first;
		first = () => source;
	}
	const fns = [first, ...rest];
	if (fns.length > 1) {
		if (isDuplex(fns[fns.length - 1])) fns[fns.length - 1] = fns[fns.length - 1].sink;
	}
	if (fns.length > 2) {
		for (let i = 1; i < fns.length - 1; i++) if (isDuplex(fns[i])) fns[i] = duplexPipelineFn(fns[i]);
	}
	return rawPipe(...fns);
}
var rawPipe = (...fns) => {
	let res;
	while (fns.length > 0) res = fns.shift()(res);
	return res;
};
var isAsyncIterable$1 = (obj) => {
	return obj?.[Symbol.asyncIterator] != null;
};
var isIterable = (obj) => {
	return obj?.[Symbol.iterator] != null;
};
var isDuplex = (obj) => {
	if (obj == null) return false;
	return obj.sink != null && obj.source != null;
};
var duplexPipelineFn = (duplex) => {
	return (source) => {
		const p = duplex.sink(source);
		if (p?.then != null) {
			const stream = pushable({ objectMode: true });
			p.then(() => {
				stream.end();
			}, (err) => {
				stream.end(err);
			});
			let sourceWrap;
			const source = duplex.source;
			if (isAsyncIterable$1(source)) sourceWrap = async function* () {
				yield* source;
				stream.end();
			};
			else if (isIterable(source)) sourceWrap = function* () {
				yield* source;
				stream.end();
			};
			else throw new Error("Unknown duplex source type - must be Iterable or AsyncIterable");
			return merge(stream, sourceWrap());
		}
		return duplex.source;
	};
};
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/assert.js
/**
* Assert that condition is truthy or throw error (with message)
*/
function assert(condition, msg) {
	if (!condition) throw new Error(msg);
}
var FLOAT32_MAX = 34028234663852886e22, FLOAT32_MIN = -34028234663852886e22, UINT32_MAX = 4294967295, INT32_MAX = 2147483647, INT32_MIN = -2147483648;
/**
* Assert a valid signed protobuf 32-bit integer.
*/
function assertInt32(arg) {
	if (typeof arg !== "number") throw new Error("invalid int 32: " + typeof arg);
	if (!Number.isInteger(arg) || arg > INT32_MAX || arg < INT32_MIN) throw new Error("invalid int 32: " + arg);
}
/**
* Assert a valid unsigned protobuf 32-bit integer.
*/
function assertUInt32(arg) {
	if (typeof arg !== "number") throw new Error("invalid uint 32: " + typeof arg);
	if (!Number.isInteger(arg) || arg > UINT32_MAX || arg < 0) throw new Error("invalid uint 32: " + arg);
}
/**
* Assert a valid protobuf float value.
*/
function assertFloat32(arg) {
	if (typeof arg !== "number") throw new Error("invalid float 32: " + typeof arg);
	if (!Number.isFinite(arg)) return;
	if (arg > FLOAT32_MAX || arg < FLOAT32_MIN) throw new Error("invalid float 32: " + arg);
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/varint.js
/**
* Read a 64 bit varint as two JS numbers.
*
* Returns tuple:
* [0]: low bits
* [1]: high bits
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf/blob/8a71927d74a4ce34efe2d8769fda198f52d20d12/js/experimental/runtime/kernel/buffer_decoder.js#L175
*/
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
/**
* Write a 64 bit varint, given as two JS numbers, to the given bytes array.
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf/blob/8a71927d74a4ce34efe2d8769fda198f52d20d12/js/experimental/runtime/kernel/writer.js#L344
*/
function varint64write(lo, hi, bytes) {
	for (let i = 0; i < 28; i = i + 7) {
		const shift = lo >>> i;
		const hasNext = !(shift >>> 7 == 0 && hi == 0);
		const byte = (hasNext ? shift | 128 : shift) & 255;
		bytes.push(byte);
		if (!hasNext) return;
	}
	const splitBits = lo >>> 28 & 15 | (hi & 7) << 4;
	const hasMoreBits = !(hi >> 3 == 0);
	bytes.push((hasMoreBits ? splitBits | 128 : splitBits) & 255);
	if (!hasMoreBits) return;
	for (let i = 3; i < 31; i = i + 7) {
		const shift = hi >>> i;
		const hasNext = !(shift >>> 7 == 0);
		const byte = (hasNext ? shift | 128 : shift) & 255;
		bytes.push(byte);
		if (!hasNext) return;
	}
	bytes.push(hi >>> 31 & 1);
}
var TWO_PWR_32_DBL = 4294967296;
/**
* Parse decimal string of 64 bit integer value as two JS numbers.
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf-javascript/blob/a428c58273abad07c66071d9753bc4d1289de426/experimental/runtime/int64.js#L10
*/
function int64FromString(dec) {
	const minus = dec[0] === "-";
	if (minus) dec = dec.slice(1);
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
/**
* Losslessly converts a 64-bit signed integer in 32:32 split representation
* into a decimal string.
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf-javascript/blob/a428c58273abad07c66071d9753bc4d1289de426/experimental/runtime/int64.js#L10
*/
function int64ToString(lo, hi) {
	let bits = newBits(lo, hi);
	const negative = bits.hi & 2147483648;
	if (negative) bits = negate(bits.lo, bits.hi);
	const result = uInt64ToString(bits.lo, bits.hi);
	return negative ? "-" + result : result;
}
/**
* Losslessly converts a 64-bit unsigned integer in 32:32 split representation
* into a decimal string.
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf-javascript/blob/a428c58273abad07c66071d9753bc4d1289de426/experimental/runtime/int64.js#L10
*/
function uInt64ToString(lo, hi) {
	({lo, hi} = toUnsigned(lo, hi));
	if (hi <= 2097151) return String(TWO_PWR_32_DBL * hi + lo);
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
	return {
		lo: lo >>> 0,
		hi: hi >>> 0
	};
}
function newBits(lo, hi) {
	return {
		lo: lo | 0,
		hi: hi | 0
	};
}
/**
* Returns two's compliment negation of input.
* @see https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/Bitwise_Operators#Signed_32-bit_integers
*/
function negate(lowBits, highBits) {
	highBits = ~highBits;
	if (lowBits) lowBits = ~lowBits + 1;
	else highBits += 1;
	return newBits(lowBits, highBits);
}
/**
* Returns decimal representation of digit1e7 with leading zeros.
*/
var decimalFrom1e7WithLeadingZeros = (digit1e7) => {
	const partial = String(digit1e7);
	return "0000000".slice(partial.length) + partial;
};
/**
* Write a 32 bit varint, signed or unsigned. Same as `varint64write(0, value, bytes)`
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf/blob/1b18833f4f2a2f681f4e4a25cdf3b0a43115ec26/js/binary/encoder.js#L144
*/
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
/**
* Read an unsigned 32 bit varint.
*
* See https://github.com/protocolbuffers/protobuf/blob/8a71927d74a4ce34efe2d8769fda198f52d20d12/js/experimental/runtime/kernel/buffer_decoder.js#L220
*/
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
	for (let readBytes = 5; (b & 128) !== 0 && readBytes < 10; readBytes++) b = this.buf[this.pos++];
	if ((b & 128) != 0) throw new Error("invalid varint");
	this.assertBounds();
	return result >>> 0;
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/proto-int64.js
function makeInt64Support() {
	const dv = /* @__PURE__ */ new DataView(/* @__PURE__ */ new ArrayBuffer(8));
	if (typeof BigInt === "function" && typeof dv.getBigInt64 === "function" && typeof dv.getBigUint64 === "function" && typeof dv.setBigInt64 === "function" && typeof dv.setBigUint64 === "function" && (typeof process != "object" || typeof process.env != "object" || process.env.BUF_BIGINT_DISABLE !== "1")) {
		const MIN = BigInt("-9223372036854775808"), MAX = BigInt("9223372036854775807"), UMIN = BigInt("0"), UMAX = BigInt("18446744073709551615");
		return {
			zero: BigInt(0),
			supported: true,
			parse(value) {
				const bi = typeof value == "bigint" ? value : BigInt(value);
				if (bi > MAX || bi < MIN) throw new Error(`int64 invalid: ${value}`);
				return bi;
			},
			uParse(value) {
				const bi = typeof value == "bigint" ? value : BigInt(value);
				if (bi > UMAX || bi < UMIN) throw new Error(`uint64 invalid: ${value}`);
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
			if (typeof value != "string") value = value.toString();
			assertInt64String(value);
			return value;
		},
		uParse(value) {
			if (typeof value != "string") value = value.toString();
			assertUInt64String(value);
			return value;
		},
		enc(value) {
			if (typeof value != "string") value = value.toString();
			assertInt64String(value);
			return int64FromString(value);
		},
		uEnc(value) {
			if (typeof value != "string") value = value.toString();
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
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/scalar.js
/**
* Scalar value types. This is a subset of field types declared by protobuf
* enum google.protobuf.FieldDescriptorProto.Type The types GROUP and MESSAGE
* are omitted, but the numerical values are identical.
*/
var ScalarType;
(function(ScalarType) {
	ScalarType[ScalarType["DOUBLE"] = 1] = "DOUBLE";
	ScalarType[ScalarType["FLOAT"] = 2] = "FLOAT";
	ScalarType[ScalarType["INT64"] = 3] = "INT64";
	ScalarType[ScalarType["UINT64"] = 4] = "UINT64";
	ScalarType[ScalarType["INT32"] = 5] = "INT32";
	ScalarType[ScalarType["FIXED64"] = 6] = "FIXED64";
	ScalarType[ScalarType["FIXED32"] = 7] = "FIXED32";
	ScalarType[ScalarType["BOOL"] = 8] = "BOOL";
	ScalarType[ScalarType["STRING"] = 9] = "STRING";
	ScalarType[ScalarType["BYTES"] = 12] = "BYTES";
	ScalarType[ScalarType["UINT32"] = 13] = "UINT32";
	ScalarType[ScalarType["SFIXED32"] = 15] = "SFIXED32";
	ScalarType[ScalarType["SFIXED64"] = 16] = "SFIXED64";
	ScalarType[ScalarType["SINT32"] = 17] = "SINT32";
	ScalarType[ScalarType["SINT64"] = 18] = "SINT64";
	ScalarType[ScalarType["DATE"] = 100] = "DATE";
})(ScalarType || (ScalarType = {}));
/**
* JavaScript representation of fields with 64 bit integral types (int64, uint64,
* sint64, fixed64, sfixed64).
*
* This is a subset of google.protobuf.FieldOptions.JSType, which defines JS_NORMAL,
* JS_STRING, and JS_NUMBER. Protobuf-ES uses BigInt by default, but will use
* String if `[jstype = JS_STRING]` is specified.
*
* ```protobuf
* uint64 field_a = 1; // BigInt
* uint64 field_b = 2 [jstype = JS_NORMAL]; // BigInt
* uint64 field_b = 2 [jstype = JS_NUMBER]; // BigInt
* uint64 field_b = 2 [jstype = JS_STRING]; // String
* ```
*/
var LongType;
(function(LongType) {
	/**
	* Use JavaScript BigInt.
	*/
	LongType[LongType["BIGINT"] = 0] = "BIGINT";
	/**
	* Use JavaScript String.
	*
	* Field option `[jstype = JS_STRING]`.
	*/
	LongType[LongType["STRING"] = 1] = "STRING";
})(LongType || (LongType = {}));
/**
* Returns true if both scalar values are equal.
*/
function scalarEquals(type, a, b) {
	if (a === b) return true;
	if (a == null || b == null) return a === b;
	if (type == ScalarType.BYTES) {
		if (!(a instanceof Uint8Array) || !(b instanceof Uint8Array)) return false;
		if (a.length !== b.length) return false;
		for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
		return true;
	}
	if (type == ScalarType.DATE) {
		const dateA = toDate(a, false);
		const dateB = toDate(b, false);
		if (dateA == null || dateB == null) return dateA === dateB;
		return dateA != null && dateB != null && +dateA === +dateB;
	}
	switch (type) {
		case ScalarType.UINT64:
		case ScalarType.FIXED64:
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64: return a == b;
	}
	return false;
}
/**
* Returns the zero value for the given scalar type.
*/
function scalarZeroValue(type, longType) {
	switch (type) {
		case ScalarType.BOOL: return false;
		case ScalarType.UINT64:
		case ScalarType.FIXED64:
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64: return longType == 0 ? protoInt64.zero : "0";
		case ScalarType.DOUBLE:
		case ScalarType.FLOAT: return 0;
		case ScalarType.BYTES: return new Uint8Array(0);
		case ScalarType.STRING: return "";
		case ScalarType.DATE: return null;
		default: return 0;
	}
}
var dateZeroValue = +/* @__PURE__ */ new Date(0);
/**
* Returns true for a zero-value. For example, an integer has the zero-value `0`,
* a boolean is `false`, a string is `""`, and bytes is an empty Uint8Array.
*
* In proto3, zero-values are not written to the wire, unless the field is
* optional or repeated.
*/
function isScalarZeroValue(type, value) {
	switch (type) {
		case ScalarType.DATE: return value == null || +value === dateZeroValue;
		case ScalarType.BOOL: return value === false;
		case ScalarType.STRING: return value === "";
		case ScalarType.BYTES: return value instanceof Uint8Array && !value.byteLength;
		default: return value == 0;
	}
}
/**
* Returns the normalized version of the scalar value.
* Zero or null is cast to the zero value.
* Bytes is cast to a Uint8Array.
* The BigInt long type is used.
* If clone is set, Uint8Array will always be copied to a new value.
*/
function normalizeScalarValue(type, value, clone, longType = LongType.BIGINT) {
	if (value == null) return scalarZeroValue(type, longType);
	if (type === ScalarType.BYTES) return toU8Arr(value, clone);
	if (isScalarZeroValue(type, value)) return scalarZeroValue(type, longType);
	if (type === ScalarType.DATE) return toDate(value, clone);
	return value;
}
function toU8Arr(input, clone) {
	return !clone && input instanceof Uint8Array ? input : new Uint8Array(input);
}
function toDate(input, clone) {
	if (input instanceof Date) return clone ? new Date(input.getTime()) : input;
	if (typeof input === "string" || typeof input === "number") {
		const date = new Date(input);
		return isNaN(date.getTime()) ? null : date;
	}
	return null;
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/names.js
/**
* Returns the name of a field in generated code.
*/
function localFieldName(protoName, inOneof) {
	const name = protoCamelCase(protoName);
	if (inOneof) return name;
	return safeObjectProperty(safeMessageProperty(name));
}
/**
* Returns the name of a oneof group in generated code.
*/
function localOneofName(protoName) {
	return localFieldName(protoName, false);
}
/**
* Converts snake_case to protoCamelCase according to the convention
* used by protoc to convert a field name to a JSON name.
*/
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
/**
* Names that cannot be used for object properties because they are reserved
* by built-in JavaScript properties.
*/
var reservedObjectProperties = new Set([
	"constructor",
	"toString",
	"toJSON",
	"valueOf",
	"__proto__",
	"prototype"
]);
/**
* Names that cannot be used for object properties because they are reserved
* by the runtime.
*/
var reservedMessageProperties = new Set(["__proto__"]);
var fallback = (name) => `${name}$`;
/**
* Will wrap names that are Object prototype properties or names reserved
* for `Message`s.
*/
var safeMessageProperty = (name) => {
	if (reservedMessageProperties.has(name)) return fallback(name);
	return name;
};
/**
* Names that cannot be used for object properties because they are reserved
* by built-in JavaScript properties.
*/
var safeObjectProperty = (name) => {
	if (reservedObjectProperties.has(name)) return fallback(name);
	return name;
};
function checkSanitizeKey(key) {
	return typeof key === "string" && !!key.length && !reservedObjectProperties.has(key);
}
function throwSanitizeKey(key) {
	if (typeof key !== "string") throw new Error("illegal non-string object key: " + typeof key);
	if (!checkSanitizeKey(key)) throw new Error("illegal object key: " + key);
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/field.js
/**
* Provides convenient access to field information of a message type.
*/
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
			for (const f of this.list()) t[f.jsonName] = t[f.name] = f;
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
			for (const f of this.list()) t[f.no] = f;
			this.numbers = t;
		}
		return this.numbers[fieldNo];
	}
	/**
	* Return field information in the order they appear in the source.
	*/
	list() {
		if (!this.all) this.all = this._normalizer(this._fields);
		return this.all;
	}
	/**
	* Return field information ordered by field number ascending.
	*/
	byNumber() {
		if (!this.numbersAsc) this.numbersAsc = this.list().concat().sort((a, b) => a.no - b.no);
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
			for (const f of this.list()) if (f.oneof) {
				if (f.oneof !== o) {
					o = f.oneof;
					a.push(o);
				}
			} else a.push(f);
		}
		return this.members;
	}
};
function newFieldList(fields, packedByDefault) {
	return new FieldList(fields, (source) => normalizeFieldInfos(source, packedByDefault));
}
/**
* Returns true if the field is set.
*/
function isFieldSet(field, target) {
	const localName = field.localName;
	if (!target) return false;
	if (field.repeated) return !!target[localName]?.length;
	if (field.oneof) return target[field.oneof.localName]?.case === localName;
	switch (field.kind) {
		case "enum":
		case "scalar":
			if (field.opt || field.req) return target[localName] != null;
			if (field.kind == "enum") return target[localName] !== field.T.values[0].no;
			return !isScalarZeroValue(field.T, target[localName]);
		case "message": return target[localName] != null;
		case "map": return target[localName] != null && !!Object.keys(target[localName]).length;
	}
}
/**
* Returns the JSON name for a protobuf field, exactly like protoc does.
*/
var fieldJsonName = protoCamelCase;
function resolveMessageType(t) {
	if (t instanceof Function) return t();
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
	findField(localName) {
		if (!this._lookup) {
			this._lookup = Object.create(null);
			for (let i = 0; i < this.fields.length; i++) this._lookup[this.fields[i].localName] = this.fields[i];
		}
		return this._lookup[localName];
	}
};
/**
* Convert a collection of field info to an array of normalized FieldInfo.
*
* The argument `packedByDefault` specifies whether fields that do not specify
* `packed` should be packed (proto3) or unpacked (proto2).
*/
function normalizeFieldInfos(fieldInfos, packedByDefault) {
	const r = [];
	let o;
	for (const field of typeof fieldInfos == "function" ? fieldInfos() : fieldInfos) {
		const f = field;
		f.localName = localFieldName(field.name, field.oneof !== void 0);
		f.jsonName = field.jsonName ?? fieldJsonName(field.name);
		f.repeated = field.repeated ?? false;
		if (field.kind == "scalar") f.L = field.L ?? LongType.BIGINT;
		f.delimited = field.delimited ?? false;
		f.req = field.req ?? false;
		f.opt = field.opt ?? false;
		if (field.packed === void 0) if (packedByDefault) f.packed = field.kind == "enum" || field.kind == "scalar" && field.T != ScalarType.BYTES && field.T != ScalarType.STRING;
		else f.packed = false;
		if (field.oneof !== void 0) {
			const ooname = typeof field.oneof == "string" ? field.oneof : field.oneof.name;
			if (!o || o.name != ooname) o = new InternalOneofInfo(ooname);
			f.oneof = o;
			o.addField(f);
		}
		r.push(f);
	}
	return r;
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/enum.js
/**
* Create a new EnumType with the given values.
*/
function createEnumType(typeName, values) {
	const names = Object.create(null);
	const numbers = Object.create(null);
	const normalValues = [];
	for (const value of values) {
		const n = "localName" in value ? value : {
			...value,
			localName: value.name
		};
		normalValues.push(n);
		names[value.name] = n;
		numbers[value.no] = n;
	}
	return {
		typeName,
		values: normalValues,
		findName(name) {
			return names[name];
		},
		findNumber(no) {
			return numbers[no];
		}
	};
}
function enumZeroValue(info) {
	if (info.values.length < 1) throw new Error("invalid enum: missing at least one value");
	return info.values[0].no;
}
/**
* Returns the normalized version of the enum value.
* Null is cast to the default value.
* String names are cast to the number enum.
* If string and the value is unknown, throws an error.
*/
function normalizeEnumValue(info, value) {
	const zeroValue = enumZeroValue(info);
	if (value == null) return zeroValue;
	if (value === "" || value === zeroValue) return zeroValue;
	if (typeof value === "string") {
		const val = info.findName(value);
		if (!val) throw new Error(`enum ${info.typeName}: invalid value: "${value}"`);
		return val.no;
	}
	return value;
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/partial.js
function applyPartialMessage(source, target, fields, clone = false) {
	if (source == null || target == null) return;
	const t = target, s = source;
	for (const member of fields.byMember()) {
		const localName = member.localName;
		throwSanitizeKey(localName);
		if (!(localName in s) || s[localName] === void 0) continue;
		const sourceValue = s[localName];
		if (sourceValue === null) {
			delete t[localName];
			continue;
		}
		switch (member.kind) {
			case "oneof": {
				if (typeof sourceValue !== "object") throw new Error(`field ${localName}: invalid oneof: must be an object with case and value`);
				const { case: sk, value: sv } = sourceValue;
				const sourceField = sk != null ? member.findField(sk) : null;
				let dv = localName in t ? t[localName] : void 0;
				if (typeof dv !== "object") dv = Object.create(null);
				if (sk != null && sourceField == null) throw new Error(`field ${localName}: invalid oneof case: ${sk}`);
				dv.case = sk;
				if (dv.case !== sk || sk == null) delete dv.value;
				t[localName] = dv;
				if (!sourceField) break;
				if (sourceField.kind === "message") {
					let dest = dv.value;
					if (typeof dest !== "object") dest = dv.value = Object.create(null);
					if (sv != null) {
						const sourceFieldMt = resolveMessageType(sourceField.T);
						applyPartialMessage(sv, dest, sourceFieldMt.fields);
					}
				} else if (sourceField.kind === "scalar") dv.value = normalizeScalarValue(sourceField.T, sv, clone);
				else dv.value = sv;
				break;
			}
			case "scalar":
				if (member.repeated) {
					if (!Array.isArray(sourceValue)) throw new Error(`field ${localName}: invalid value: must be array`);
					let dst = localName in t ? t[localName] : null;
					if (dst == null || !Array.isArray(dst)) dst = t[localName] = [];
					dst.push(...sourceValue.map((v) => normalizeScalarValue(member.T, v, clone)));
					break;
				}
				t[localName] = normalizeScalarValue(member.T, sourceValue, clone);
				break;
			case "enum":
				t[localName] = normalizeEnumValue(member.T, sourceValue);
				break;
			case "map": {
				if (typeof sourceValue !== "object") throw new Error(`field ${member.localName}: invalid value: must be object`);
				let tMap = t[localName];
				if (typeof tMap !== "object") tMap = t[localName] = Object.create(null);
				applyPartialMap(sourceValue, tMap, member.V, clone);
				break;
			}
			case "message": {
				const mt = resolveMessageType(member.T);
				if (member.repeated) {
					if (!Array.isArray(sourceValue)) throw new Error(`field ${localName}: invalid value: must be array`);
					let tArr = t[localName];
					if (!Array.isArray(tArr)) tArr = t[localName] = [];
					for (const v of sourceValue) if (v != null) if (mt.fieldWrapper) tArr.push(mt.fieldWrapper.unwrapField(mt.fieldWrapper.wrapField(v)));
					else tArr.push(mt.create(v));
					break;
				}
				if (mt.fieldWrapper) t[localName] = mt.fieldWrapper.unwrapField(mt.fieldWrapper.wrapField(sourceValue));
				else {
					if (typeof sourceValue !== "object") throw new Error(`field ${member.localName}: invalid value: must be object`);
					let destMsg = t[localName];
					if (typeof destMsg !== "object") destMsg = t[localName] = Object.create(null);
					applyPartialMessage(sourceValue, destMsg, mt.fields);
				}
				break;
			}
		}
	}
}
function applyPartialMap(sourceMap, targetMap, value, clone) {
	if (sourceMap == null) return;
	if (typeof sourceMap !== "object") throw new Error(`invalid map: must be object`);
	switch (value.kind) {
		case "scalar":
			for (const [k, v] of Object.entries(sourceMap)) {
				throwSanitizeKey(k);
				if (v !== void 0) targetMap[k] = normalizeScalarValue(value.T, v, clone);
				else delete targetMap[k];
			}
			break;
		case "enum":
			for (const [k, v] of Object.entries(sourceMap)) {
				throwSanitizeKey(k);
				if (v !== void 0) targetMap[k] = normalizeEnumValue(value.T, v);
				else delete targetMap[k];
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
				if (typeof v !== "object") throw new Error(`invalid value: must be object`);
				let val = targetMap[k];
				if (messageType.fieldWrapper) val = targetMap[k] = createCompleteMessage(messageType.fields);
				else if (typeof val !== "object") val = targetMap[k] = Object.create(null);
				applyPartialMessage(v, val, messageType.fields);
			}
			break;
		}
	}
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/unknown.js
var unknownFieldsSymbol = Symbol("@aptre/protobuf-es-lite/unknown-fields");
function handleUnknownField(message, no, wireType, data) {
	if (typeof message !== "object") return;
	const m = message;
	if (!Array.isArray(m[unknownFieldsSymbol])) m[unknownFieldsSymbol] = [];
	m[unknownFieldsSymbol].push({
		no,
		wireType,
		data
	});
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/field-wrapper.js
/**
* Wrap a primitive message field value in its corresponding wrapper
* message. This function is idempotent.
*/
function wrapField(fieldWrapper, value) {
	if (!fieldWrapper) return value;
	return fieldWrapper.wrapField(value);
}
/**
* Wrap a primitive message field value in its corresponding wrapper
* message. This function is idempotent.
*/
function unwrapField(fieldWrapper, value) {
	return fieldWrapper ? fieldWrapper.unwrapField(value) : value;
}
ScalarType.DATE, ScalarType.DOUBLE, ScalarType.FLOAT, ScalarType.INT64, ScalarType.UINT64, ScalarType.INT32, ScalarType.UINT32, ScalarType.BOOL, ScalarType.STRING, ScalarType.BYTES;
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/binary-encoding.js
/**
* Protobuf binary format wire types.
*
* A wire type provides just enough information to find the length of the
* following value.
*
* See https://developers.google.com/protocol-buffers/docs/encoding#structure
*/
var WireType;
(function(WireType) {
	/**
	* Used for int32, int64, uint32, uint64, sint32, sint64, bool, enum
	*/
	WireType[WireType["Varint"] = 0] = "Varint";
	/**
	* Used for fixed64, sfixed64, double.
	* Always 8 bytes with little-endian byte order.
	*/
	WireType[WireType["Bit64"] = 1] = "Bit64";
	/**
	* Used for string, bytes, embedded messages, packed repeated fields
	*
	* Only repeated numeric types (types which use the varint, 32-bit,
	* or 64-bit wire types) can be packed. In proto3, such fields are
	* packed by default.
	*/
	WireType[WireType["LengthDelimited"] = 2] = "LengthDelimited";
	/**
	* Start of a tag-delimited aggregate, such as a proto2 group, or a message
	* in editions with message_encoding = DELIMITED.
	*/
	WireType[WireType["StartGroup"] = 3] = "StartGroup";
	/**
	* End of a tag-delimited aggregate.
	*/
	WireType[WireType["EndGroup"] = 4] = "EndGroup";
	/**
	* Used for fixed32, sfixed32, float.
	* Always 4 bytes with little-endian byte order.
	*/
	WireType[WireType["Bit32"] = 5] = "Bit32";
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
		for (let i = 0; i < this.chunks.length; i++) len += this.chunks[i].length;
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
		this.stack.push({
			chunks: this.chunks,
			buf: this.buf
		});
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
		if (!prev) throw new Error("invalid state, fork stack empty");
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
		let tc = protoInt64.enc(value), sign = tc.hi >> 31;
		varint64write(tc.lo << 1 ^ sign, (tc.hi << 1 | tc.lo >>> 31) ^ sign, this.buf);
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
		if (fieldNo <= 0 || wireType < 0 || wireType > 5) throw new Error("illegal tag: field no " + fieldNo + " wire type " + wireType);
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
				while (this.buf[this.pos++] & 128);
				break;
			case WireType.Bit64: this.pos += 4;
			case WireType.Bit32:
				this.pos += 4;
				break;
			case WireType.LengthDelimited:
				let len = this.uint32();
				this.pos += len;
				break;
			case WireType.StartGroup:
				let t;
				while ((t = this.tag()[1]) !== WireType.EndGroup) this.skip(t);
				break;
			default: throw new Error("cant skip wire type " + wireType);
		}
		this.assertBounds();
		return this.buf.subarray(start, this.pos);
	}
	varint64 = varint64read;
	/**
	* Throws error if position in byte array is out of range.
	*/
	assertBounds() {
		if (this.pos > this.len) throw new RangeError("premature EOF");
	}
	/**
	* Read a `uint32` field, an unsigned 32 bit varint.
	*/
	uint32 = varint32read;
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
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/binary.js
var readDefaults = {
	readUnknownFields: true,
	readerFactory: (bytes) => new BinaryReader(bytes)
};
var writeDefaults = {
	writeUnknownFields: true,
	writerFactory: () => new BinaryWriter()
};
function makeReadOptions$1(options) {
	return options ? {
		...readDefaults,
		...options
	} : readDefaults;
}
function makeWriteOptions$1(options) {
	return options ? {
		...writeDefaults,
		...options
	} : writeDefaults;
}
function readField$1(target, reader, field, wireType, options) {
	const { repeated } = field;
	let { localName } = field;
	if (field.oneof) {
		let oneofMsg = target[field.oneof.localName];
		if (!oneofMsg) oneofMsg = target[field.oneof.localName] = Object.create(null);
		target = oneofMsg;
		if (target.case != localName) delete target.value;
		target.case = localName;
		localName = "value";
	}
	switch (field.kind) {
		case "scalar":
		case "enum": {
			const scalarType = field.kind == "enum" ? ScalarType.INT32 : field.T;
			let read = readScalar$1;
			if (field.kind == "scalar" && field.L > 0) read = readScalarLTString;
			if (repeated) {
				let tgtArr = target[localName];
				if (!Array.isArray(tgtArr)) tgtArr = target[localName] = [];
				if (wireType == WireType.LengthDelimited && scalarType != ScalarType.STRING && scalarType != ScalarType.BYTES) {
					const e = reader.uint32() + reader.pos;
					while (reader.pos < e) tgtArr.push(read(reader, scalarType));
				} else tgtArr.push(read(reader, scalarType));
			} else target[localName] = read(reader, scalarType);
			break;
		}
		case "message": {
			const fieldT = field.T;
			const messageType = fieldT instanceof Function ? fieldT() : fieldT;
			if (repeated) {
				let tgtArr = target[localName];
				if (!Array.isArray(tgtArr)) tgtArr = target[localName] = [];
				tgtArr.push(unwrapField(messageType.fieldWrapper, readMessageField(reader, Object.create(null), messageType.fields, options, field)));
			} else target[localName] = unwrapField(messageType.fieldWrapper, readMessageField(reader, Object.create(null), messageType.fields, options, field));
			break;
		}
		case "map": {
			const [mapKey, mapVal] = readMapEntry(field, reader, options);
			if (typeof target[localName] !== "object") target[localName] = Object.create(null);
			target[localName][mapKey] = mapVal;
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
				key = readScalar$1(reader, field.K);
				break;
			case 2:
				switch (field.V.kind) {
					case "scalar":
						val = readScalar$1(reader, field.V.T);
						break;
					case "enum":
						val = reader.int32();
						break;
					case "message":
						val = readMessageField(reader, Object.create(null), resolveMessageType(field.V.T).fields, options, void 0);
						break;
				}
				break;
		}
	}
	if (key === void 0) key = scalarZeroValue(field.K, LongType.BIGINT);
	if (typeof key !== "string" && typeof key !== "number") key = key?.toString() ?? "";
	if (val === void 0) switch (field.V.kind) {
		case "scalar":
			val = scalarZeroValue(field.V.T, LongType.BIGINT);
			break;
		case "enum":
			val = field.V.T.values[0].no;
			break;
		case "message":
			val = Object.create(null);
			break;
	}
	return [key, val];
}
function readScalar$1(reader, type) {
	switch (type) {
		case ScalarType.STRING: return reader.string();
		case ScalarType.BOOL: return reader.bool();
		case ScalarType.DOUBLE: return reader.double();
		case ScalarType.FLOAT: return reader.float();
		case ScalarType.INT32: return reader.int32();
		case ScalarType.INT64: return reader.int64();
		case ScalarType.UINT64: return reader.uint64();
		case ScalarType.FIXED64: return reader.fixed64();
		case ScalarType.BYTES: return reader.bytes();
		case ScalarType.FIXED32: return reader.fixed32();
		case ScalarType.SFIXED32: return reader.sfixed32();
		case ScalarType.SFIXED64: return reader.sfixed64();
		case ScalarType.SINT64: return reader.sint64();
		case ScalarType.UINT32: return reader.uint32();
		case ScalarType.SINT32: return reader.sint32();
		case ScalarType.DATE: throw new Error("cannot read a date with readScalar");
		default: throw new Error("unknown scalar type");
	}
}
function readScalarLTString(reader, type) {
	const v = readScalar$1(reader, type);
	return typeof v == "bigint" ? v.toString() : v;
}
function readMessageField(reader, message, fields, options, field) {
	readMessage$1(message, fields, reader, field?.delimited ? field.no : reader.uint32(), options, field?.delimited ?? false);
	return message;
}
function readMessage$1(message, fields, reader, lengthOrEndTagFieldNo, options, delimitedMessageEncoding) {
	const end = delimitedMessageEncoding ? reader.len : reader.pos + lengthOrEndTagFieldNo;
	let fieldNo, wireType;
	while (reader.pos < end) {
		[fieldNo, wireType] = reader.tag();
		if (wireType == WireType.EndGroup) break;
		const field = fields.find(fieldNo);
		if (!field) {
			const data = reader.skip(wireType);
			if (options.readUnknownFields) handleUnknownField(message, fieldNo, wireType, data);
			continue;
		}
		readField$1(message, reader, field, wireType, options);
	}
	if (delimitedMessageEncoding && (wireType != WireType.EndGroup || fieldNo !== lengthOrEndTagFieldNo)) throw new Error(`invalid end group tag`);
}
/**
* Serialize a message to binary data.
*/
function writeMessage$1(message, fields, writer, options) {
	for (const field of fields.byNumber()) {
		if (!isFieldSet(field, message)) {
			if (field.req) throw new Error(`cannot encode field ${field.name} to binary: required field not set`);
			continue;
		}
		const value = field.oneof ? message[field.oneof.localName].value : message[field.localName];
		if (value !== void 0) writeField$1(field, value, writer, options);
	}
	if (options.writeUnknownFields) writeUnknownFields(message, writer);
}
function writeField$1(field, value, writer, options) {
	assert(value !== void 0);
	const repeated = field.repeated;
	switch (field.kind) {
		case "scalar":
		case "enum": {
			const scalarType = field.kind == "enum" ? ScalarType.INT32 : field.T;
			if (repeated) {
				assert(Array.isArray(value));
				if (field.packed) writePacked(writer, scalarType, field.no, value);
				else for (const item of value) writeScalar$1(writer, scalarType, field.no, item);
			} else writeScalar$1(writer, scalarType, field.no, value);
			break;
		}
		case "message":
			if (repeated) {
				assert(Array.isArray(value));
				for (const item of value) writeMessageField(writer, options, field, item);
			} else writeMessageField(writer, options, field, value);
			break;
		case "map":
			assert(typeof value == "object" && value != null);
			for (const [key, val] of Object.entries(value)) writeMapEntry(writer, options, field, key, val);
			break;
	}
}
function writeUnknownFields(message, writer) {
	const c = message[unknownFieldsSymbol];
	if (c) for (const f of c) writer.tag(f.no, f.wireType).raw(f.data);
}
function writeMessageField(writer, options, field, value) {
	const messageType = resolveMessageType(field.T);
	const message = wrapField(messageType.fieldWrapper, value);
	if (field.delimited) writer.tag(field.no, WireType.StartGroup).raw(messageType.toBinary(message, options)).tag(field.no, WireType.EndGroup);
	else writer.tag(field.no, WireType.LengthDelimited).bytes(messageType.toBinary(message, options));
}
function writeScalar$1(writer, type, fieldNo, value) {
	assert(value !== void 0);
	const [wireType, method] = scalarTypeInfo(type);
	writer.tag(fieldNo, wireType)[method](value);
}
function writePacked(writer, type, fieldNo, value) {
	if (!value.length) return;
	writer.tag(fieldNo, WireType.LengthDelimited).fork();
	const [, method] = scalarTypeInfo(type);
	for (let i = 0; i < value.length; i++) writer[method](value[i]);
	writer.join();
}
/**
* Get information for writing a scalar value.
*
* Returns tuple:
* [0]: appropriate WireType
* [1]: name of the appropriate method of IBinaryWriter
* [2]: whether the given value is a default value for proto3 semantics
*
* If argument `value` is omitted, [2] is always false.
*/
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
	writeScalar$1(writer, field.K, 1, keyValue);
	switch (field.V.kind) {
		case "scalar":
			writeScalar$1(writer, field.V.T, 2, value);
			break;
		case "enum":
			writeScalar$1(writer, ScalarType.INT32, 2, value);
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
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/proto-base64.js
var encTable = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".split("");
var decTable = [];
for (let i = 0; i < encTable.length; i++) decTable[encTable[i].charCodeAt(0)] = i;
decTable["-".charCodeAt(0)] = encTable.indexOf("+");
decTable["_".charCodeAt(0)] = encTable.indexOf("/");
var protoBase64 = {
	dec(base64Str) {
		let es = base64Str.length * 3 / 4;
		if (base64Str[base64Str.length - 2] == "=") es -= 2;
		else if (base64Str[base64Str.length - 1] == "=") es -= 1;
		let bytes = new Uint8Array(es), bytePos = 0, groupPos = 0, b, p = 0;
		for (let i = 0; i < base64Str.length; i++) {
			b = decTable[base64Str.charCodeAt(i)];
			if (b === void 0) switch (base64Str[i]) {
				case "=": groupPos = 0;
				case "\n":
				case "\r":
				case "	":
				case " ": continue;
				default: throw Error("invalid base64 string.");
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
		if (groupPos == 1) throw Error("invalid base64 string.");
		return bytes.subarray(0, bytePos);
	},
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
			if (groupPos == 1) base64 += "=";
		}
		return base64;
	}
};
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/json.js
var jsonReadDefaults = { ignoreUnknownFields: false };
var jsonWriteDefaults = {
	emitDefaultValues: false,
	enumAsInteger: false,
	useProtoFieldName: false,
	prettySpaces: 0
};
function makeReadOptions(options) {
	return options ? {
		...jsonReadDefaults,
		...options
	} : jsonReadDefaults;
}
function makeWriteOptions(options) {
	return options ? {
		...jsonWriteDefaults,
		...options
	} : jsonWriteDefaults;
}
function jsonDebugValue(json) {
	if (json === null) return "null";
	switch (typeof json) {
		case "object": return Array.isArray(json) ? "array" : "object";
		case "string": return json.length > 100 ? "string" : `"${json.split("\"").join("\\\"")}"`;
		default: return String(json);
	}
}
function readMessage(fields, typeName, json, options, message) {
	if (json == null || Array.isArray(json) || typeof json != "object") throw new Error(`cannot decode message ${typeName} from JSON: ${jsonDebugValue(json)}`);
	const oneofSeen = /* @__PURE__ */ new Map();
	for (const [jsonKey, jsonValue] of Object.entries(json)) {
		const field = fields.findJsonName(jsonKey);
		if (field) {
			if (field.oneof) {
				if (jsonValue === null && field.kind == "scalar") continue;
				const seen = oneofSeen.get(field.oneof);
				if (seen !== void 0) throw new Error(`cannot decode message ${typeName} from JSON: multiple keys for oneof "${field.oneof.name}" present: "${seen}", "${jsonKey}"`);
				oneofSeen.set(field.oneof, jsonKey);
			}
			readField(message, jsonValue, field, options);
		} else if (!options.ignoreUnknownFields) throw new Error(`cannot decode message ${typeName} from JSON: key "${jsonKey}" is unknown`);
	}
	return message;
}
function writeMessage(message, fields, options) {
	const json = Object.create(null);
	let field;
	try {
		for (field of fields.byNumber()) {
			if (!isFieldSet(field, message)) {
				if (field.req) throw `required field not set`;
				if (!options.emitDefaultValues) continue;
				if (!canEmitFieldDefaultValue(field)) continue;
			}
			const value = field.oneof ? message[field.oneof.localName].value : message[field.localName];
			const jsonValue = writeField(field, value, options);
			if (jsonValue !== void 0) json[options.useProtoFieldName ? field.name : field.jsonName] = jsonValue;
		}
	} catch (e) {
		const m = field ? `cannot encode field ${field.name} to JSON` : `cannot encode message to JSON`;
		const r = e instanceof Error ? e.message : String(e);
		throw new Error(m + (r.length > 0 ? `: ${r}` : ""), { cause: e });
	}
	return json;
}
function readField(target, jsonValue, field, options) {
	let localName = field.localName;
	if (field.repeated) {
		assert(field.kind != "map");
		if (jsonValue === null) return;
		if (!Array.isArray(jsonValue)) throw new Error(`cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`);
		let targetArray = target[localName];
		if (!Array.isArray(targetArray)) targetArray = target[localName] = [];
		for (const jsonItem of jsonValue) {
			if (jsonItem === null) throw new Error(`cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonItem)}`);
			switch (field.kind) {
				case "message": {
					const messageType = resolveMessageType(field.T);
					targetArray.push(unwrapField(messageType.fieldWrapper, messageType.fromJson(jsonItem, options)));
					break;
				}
				case "enum": {
					const enumValue = readEnum(field.T, jsonItem, options.ignoreUnknownFields, true);
					if (enumValue !== tokenIgnoredUnknownEnum) targetArray.push(enumValue);
					break;
				}
				case "scalar":
					try {
						targetArray.push(readScalar(field.T, jsonItem, field.L, true));
					} catch (e) {
						let m = `cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonItem)}`;
						if (e instanceof Error && e.message.length > 0) m += `: ${e.message}`;
						throw new Error(m, { cause: e });
					}
					break;
			}
		}
	} else if (field.kind == "map") {
		if (jsonValue === null) return;
		if (typeof jsonValue != "object" || Array.isArray(jsonValue)) throw new Error(`cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`);
		let targetMap = target[localName];
		if (typeof targetMap !== "object") targetMap = target[localName] = Object.create(null);
		for (const [jsonMapKey, jsonMapValue] of Object.entries(jsonValue)) {
			if (jsonMapValue === null) throw new Error(`cannot decode field ${field.name} from JSON: map value null`);
			let key;
			try {
				key = readMapKey(field.K, jsonMapKey);
			} catch (e) {
				let m = `cannot decode map key for field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`;
				if (e instanceof Error && e.message.length > 0) m += `: ${e.message}`;
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
					if (enumValue !== tokenIgnoredUnknownEnum) targetMap[key] = enumValue;
					break;
				}
				case "scalar":
					try {
						targetMap[key] = readScalar(field.V.T, jsonMapValue, LongType.BIGINT, true);
					} catch (e) {
						let m = `cannot decode map value for field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`;
						if (e instanceof Error && e.message.length > 0) m += `: ${e.message}`;
						throw new Error(m, { cause: e });
					}
					break;
			}
		}
	} else {
		if (field.oneof) {
			target = target[field.oneof.localName] = { case: localName };
			localName = "value";
		}
		switch (field.kind) {
			case "message": {
				const messageType = resolveMessageType(field.T);
				if (jsonValue === null && messageType.typeName != "google.protobuf.Value") return;
				target[localName] = unwrapField(messageType.fieldWrapper, messageType.fromJson(jsonValue, options));
				break;
			}
			case "enum": {
				const enumValue = readEnum(field.T, jsonValue, options.ignoreUnknownFields, false);
				switch (enumValue) {
					case tokenNull:
						clearField(field, target);
						break;
					case tokenIgnoredUnknownEnum: break;
					default:
						target[localName] = enumValue;
						break;
				}
				break;
			}
			case "scalar":
				try {
					const scalarValue = readScalar(field.T, jsonValue, field.L, false);
					switch (scalarValue) {
						case tokenNull:
							clearField(field, target);
							break;
						default:
							target[localName] = scalarValue;
							break;
					}
				} catch (e) {
					let m = `cannot decode field ${field.name} from JSON: ${jsonDebugValue(jsonValue)}`;
					if (e instanceof Error && e.message.length > 0) m += `: ${e.message}`;
					throw new Error(m, { cause: e });
				}
				break;
		}
	}
}
var tokenNull = Symbol();
var tokenIgnoredUnknownEnum = Symbol();
function readEnum(type, json, ignoreUnknownFields, nullAsZeroValue) {
	if (json === null) {
		if (type.typeName == "google.protobuf.NullValue") return 0;
		return nullAsZeroValue ? type.values[0].no : tokenNull;
	}
	switch (typeof json) {
		case "number":
			if (Number.isInteger(json)) return json;
			break;
		case "string": {
			const value = type.findName(json);
			if (value !== void 0) return value.no;
			if (ignoreUnknownFields) return tokenIgnoredUnknownEnum;
			break;
		}
	}
	throw new Error(`cannot decode enum ${type.typeName} from JSON: ${jsonDebugValue(json)}`);
}
function readScalar(type, json, longType = LongType.BIGINT, nullAsZeroValue = true) {
	if (json == null) {
		if (nullAsZeroValue) return scalarZeroValue(type, longType);
		return tokenNull;
	}
	switch (type) {
		case ScalarType.DOUBLE:
		case ScalarType.FLOAT: {
			if (json === "NaN") return NaN;
			if (json === "Infinity") return Number.POSITIVE_INFINITY;
			if (json === "-Infinity") return Number.NEGATIVE_INFINITY;
			if (json === "") break;
			if (typeof json == "string" && json.trim().length !== json.length) break;
			if (typeof json != "string" && typeof json != "number") break;
			const float = Number(json);
			if (Number.isNaN(float)) break;
			if (!Number.isFinite(float)) break;
			if (type == ScalarType.FLOAT) assertFloat32(float);
			return float;
		}
		case ScalarType.INT32:
		case ScalarType.FIXED32:
		case ScalarType.SFIXED32:
		case ScalarType.SINT32:
		case ScalarType.UINT32: {
			let int32;
			if (typeof json == "number") int32 = json;
			else if (typeof json == "string" && json.length > 0) {
				if (json.trim().length === json.length) int32 = Number(json);
			}
			if (int32 === void 0) break;
			if (type == ScalarType.UINT32 || type == ScalarType.FIXED32) assertUInt32(int32);
			else assertInt32(int32);
			return int32;
		}
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64: {
			if (typeof json != "number" && typeof json != "string") break;
			const long = protoInt64.parse(json);
			return longType ? long.toString() : long;
		}
		case ScalarType.FIXED64:
		case ScalarType.UINT64: {
			if (typeof json != "number" && typeof json != "string") break;
			const uLong = protoInt64.uParse(json);
			return longType ? uLong.toString() : uLong;
		}
		case ScalarType.BOOL:
			if (typeof json !== "boolean") break;
			return json;
		case ScalarType.STRING:
			if (typeof json !== "string") break;
			return json;
		case ScalarType.BYTES:
			if (json === "") return new Uint8Array(0);
			if (typeof json !== "string") break;
			return protoBase64.dec(json);
	}
	throw new Error();
}
function readMapKey(type, json) {
	if (type === ScalarType.BOOL) switch (json) {
		case "true":
			json = true;
			break;
		case "false":
			json = false;
			break;
	}
	return readScalar(type, json, LongType.BIGINT, true)?.toString() ?? "";
}
/**
* Resets the field, so that isFieldSet() will return false.
*/
function clearField(field, target) {
	const localName = field.localName;
	const implicitPresence = !field.opt && !field.req;
	if (field.repeated) target[localName] = [];
	else if (field.oneof) target[field.oneof.localName] = { case: void 0 };
	else switch (field.kind) {
		case "map":
			target[localName] = Object.create(null);
			break;
		case "enum":
			target[localName] = implicitPresence ? field.T.values[0].no : void 0;
			break;
		case "scalar":
			target[localName] = implicitPresence ? scalarZeroValue(field.T, field.L) : void 0;
			break;
		case "message":
			target[localName] = void 0;
			break;
	}
}
function canEmitFieldDefaultValue(field) {
	if (field.repeated || field.kind == "map") return true;
	if (field.oneof) return false;
	if (field.kind == "message") return false;
	if (field.opt || field.req) return false;
	return true;
}
function writeField(field, value, options) {
	if (field.kind == "map") {
		const jsonObj = Object.create(null);
		assert(!value || typeof value === "object");
		const entries = value ? Object.entries(value) : [];
		switch (field.V.kind) {
			case "scalar":
				for (const [entryKey, entryValue] of entries) jsonObj[entryKey.toString()] = writeScalar(field.V.T, entryValue);
				break;
			case "message":
				for (const [entryKey, entryValue] of entries) {
					const messageType = resolveMessageType(field.V.T);
					jsonObj[entryKey.toString()] = messageType.toJson(entryValue, options);
				}
				break;
			case "enum": {
				const enumType = field.V.T;
				for (const [entryKey, entryValue] of entries) jsonObj[entryKey.toString()] = writeEnum(enumType, entryValue, options.enumAsInteger);
				break;
			}
		}
		return options.emitDefaultValues || entries.length > 0 ? jsonObj : void 0;
	}
	if (field.repeated) {
		assert(!value || Array.isArray(value));
		const jsonArr = [];
		const valueArr = value;
		if (valueArr && valueArr.length) switch (field.kind) {
			case "scalar":
				for (let i = 0; i < valueArr.length; i++) jsonArr.push(writeScalar(field.T, valueArr[i]));
				break;
			case "enum":
				for (let i = 0; i < valueArr.length; i++) jsonArr.push(writeEnum(field.T, valueArr[i], options.enumAsInteger));
				break;
			case "message": {
				const messageType = resolveMessageType(field.T);
				for (let i = 0; i < valueArr.length; i++) jsonArr.push(messageType.toJson(wrapField(messageType.fieldWrapper, valueArr[i])));
				break;
			}
		}
		return options.emitDefaultValues || jsonArr.length > 0 ? jsonArr : void 0;
	}
	switch (field.kind) {
		case "scalar": {
			const scalarValue = normalizeScalarValue(field.T, value, false);
			if (!options.emitDefaultValues && isScalarZeroValue(field.T, scalarValue)) return;
			return writeScalar(field.T, value);
		}
		case "enum": {
			const enumValue = normalizeEnumValue(field.T, value);
			if (!options.emitDefaultValues && enumZeroValue(field.T) === enumValue) return;
			return writeEnum(field.T, value, options.enumAsInteger);
		}
		case "message": {
			if (!options.emitDefaultValues && value == null) return;
			const messageType = resolveMessageType(field.T);
			return messageType.toJson(wrapField(messageType.fieldWrapper, value));
		}
	}
}
function writeScalar(type, value) {
	switch (type) {
		case ScalarType.INT32:
		case ScalarType.SFIXED32:
		case ScalarType.SINT32:
		case ScalarType.FIXED32:
		case ScalarType.UINT32:
			assert(typeof value == "number");
			return value;
		case ScalarType.FLOAT:
		case ScalarType.DOUBLE:
			assert(typeof value == "number");
			if (Number.isNaN(value)) return "NaN";
			if (value === Number.POSITIVE_INFINITY) return "Infinity";
			if (value === Number.NEGATIVE_INFINITY) return "-Infinity";
			return value;
		case ScalarType.STRING:
			assert(typeof value == "string");
			return value;
		case ScalarType.BOOL:
			assert(typeof value == "boolean");
			return value;
		case ScalarType.UINT64:
		case ScalarType.FIXED64:
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64:
			assert(typeof value == "bigint" || typeof value == "string" || typeof value == "number");
			return value.toString();
		case ScalarType.BYTES:
			assert(value instanceof Uint8Array);
			return protoBase64.enc(value);
		case ScalarType.DATE: throw new Error("cannot write date with writeScalar");
		default: throw new Error("unknown scalar type");
	}
}
function writeEnum(type, value, enumAsInteger) {
	assert(typeof value == "number");
	if (type.typeName == "google.protobuf.NullValue") return null;
	if (enumAsInteger) return value;
	return type.findNumber(value)?.name ?? value;
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/message.js
/**
* createMessageType creates a new message type.
*
* The argument `packedByDefault` specifies whether fields that do not specify
* `packed` should be packed (proto3) or unpacked (proto2).
*/
function createMessageType(params, exts) {
	const { fields: fieldsSource, typeName, packedByDefault, delimitedMessageEncoding, fieldWrapper } = params;
	const fields = newFieldList(fieldsSource, packedByDefault);
	const mt = {
		typeName,
		fields,
		fieldWrapper,
		create(partial) {
			const message = Object.create(null);
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
			if (a == null) return a;
			return cloneMessage(a, fields);
		},
		fromBinary(bytes, options) {
			const message = {};
			if (bytes && bytes.length) {
				const opt = makeReadOptions$1(options);
				readMessage$1(message, fields, opt.readerFactory(bytes), bytes.byteLength, opt, delimitedMessageEncoding ?? false);
			}
			return message;
		},
		fromJson(jsonValue, options) {
			const message = {};
			if (jsonValue != null) readMessage(fields, typeName, jsonValue, makeReadOptions(options), message);
			return message;
		},
		fromJsonString(jsonString, options) {
			let json = null;
			if (jsonString) try {
				json = JSON.parse(jsonString);
			} catch (e) {
				throw new Error(`cannot decode ${typeName} from JSON: ${e instanceof Error ? e.message : String(e)}`, { cause: e });
			}
			return mt.fromJson(json, options);
		},
		toBinary(a, options) {
			if (a == null) return new Uint8Array(0);
			const opt = makeWriteOptions$1(options);
			const writer = opt.writerFactory();
			writeMessage$1(a, fields, writer, opt);
			return writer.finish();
		},
		toJson(a, options) {
			return writeMessage(a, fields, makeWriteOptions(options));
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
	if (a == null && b == null) return true;
	if (a === b) return true;
	if (!a || !b) return false;
	return fields.byMember().every((m) => {
		const va = a[m.localName];
		const vb = b[m.localName];
		if (m.repeated) {
			if ((va?.length ?? 0) !== (vb?.length ?? 0)) return false;
			if (!va?.length) return true;
			switch (m.kind) {
				case "message": {
					const messageType = resolveMessageType(m.T);
					return va.every((a, i) => messageType.equals(a, vb[i]));
				}
				case "scalar": return va.every((a, i) => scalarEquals(m.T, a, vb[i]));
				case "enum": return va.every((a, i) => scalarEquals(ScalarType.INT32, a, vb[i]));
			}
			throw new Error(`repeated cannot contain ${m.kind}`);
		}
		switch (m.kind) {
			case "message": return resolveMessageType(m.T).equals(va, vb);
			case "enum": return scalarEquals(ScalarType.INT32, va, vb);
			case "scalar": return scalarEquals(m.T, va, vb);
			case "oneof": {
				if (va?.case !== vb?.case) return false;
				if (va == null) return true;
				const s = m.findField(va.case);
				if (s === void 0) return true;
				switch (s.kind) {
					case "message": return resolveMessageType(s.T).equals(va.value, vb.value);
					case "enum": return scalarEquals(ScalarType.INT32, va.value, vb.value);
					case "scalar": return scalarEquals(s.T, va.value, vb.value);
				}
				throw new Error(`oneof cannot contain ${s.kind}`);
			}
			case "map": {
				const ma = va ?? {};
				const mb = vb ?? {};
				const keys = Object.keys(ma).concat(Object.keys(mb));
				switch (m.V.kind) {
					case "message": {
						const messageType = resolveMessageType(m.V.T);
						return keys.every((k) => messageType.equals(ma[k], mb[k]));
					}
					case "enum": return keys.every((k) => scalarEquals(ScalarType.INT32, ma[k], mb[k]));
					case "scalar": {
						const scalarType = m.V.T;
						return keys.every((k) => scalarEquals(scalarType, ma[k], mb[k]));
					}
				}
			}
		}
	});
}
function cloneMessage(message, fields) {
	if (message == null) return null;
	const clone = Object.create(null);
	applyPartialMessage(message, clone, fields, true);
	return clone;
}
/**
* createCompleteMessage recursively builds a message filled with zero values based on the given FieldList.
*/
function createCompleteMessage(fields) {
	const message = {};
	for (const field of fields.byMember()) {
		const { localName, kind: fieldKind } = field;
		throwSanitizeKey(localName);
		switch (fieldKind) {
			case "oneof":
				message[localName] = Object.create(null);
				message[localName].case = void 0;
				break;
			case "scalar":
				if (field.repeated) message[localName] = [];
				else message[localName] = scalarZeroValue(field.T, LongType.BIGINT);
				break;
			case "enum":
				message[localName] = field.repeated ? [] : enumZeroValue(field.T);
				break;
			case "message": {
				if (field.oneof) break;
				if (field.repeated) {
					message[localName] = [];
					break;
				}
				const messageType = resolveMessageType(field.T);
				message[localName] = messageType.fieldWrapper ? messageType.fieldWrapper.unwrapField(null) : createCompleteMessage(messageType.fields);
				break;
			}
			case "map":
				message[localName] = Object.create(null);
				break;
			default:
		}
	}
	return message;
}
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/service-type.js
/**
* MethodKind represents the four method types that can be declared in
* protobuf with the `stream` keyword:
*
* 1. Unary:           rpc (Input) returns (Output)
* 2. ServerStreaming: rpc (Input) returns (stream Output)
* 3. ClientStreaming: rpc (stream Input) returns (Output)
* 4. BiDiStreaming:   rpc (stream Input) returns (stream Output)
*/
var MethodKind;
(function(MethodKind) {
	MethodKind[MethodKind["Unary"] = 0] = "Unary";
	MethodKind[MethodKind["ServerStreaming"] = 1] = "ServerStreaming";
	MethodKind[MethodKind["ClientStreaming"] = 2] = "ClientStreaming";
	MethodKind[MethodKind["BiDiStreaming"] = 3] = "BiDiStreaming";
})(MethodKind || (MethodKind = {}));
/**
* Is this method side-effect-free (or safe in HTTP parlance), or just
* idempotent, or neither? HTTP based RPC implementation may choose GET verb
* for safe methods, and PUT verb for idempotent methods instead of the
* default POST.
*
* This enum matches the protobuf enum google.protobuf.MethodOptions.IdempotencyLevel,
* defined in the well-known type google/protobuf/descriptor.proto, but
* drops UNKNOWN.
*/
var MethodIdempotency;
(function(MethodIdempotency) {
	/**
	* Idempotent, no side effects.
	*/
	MethodIdempotency[MethodIdempotency["NoSideEffects"] = 1] = "NoSideEffects";
	/**
	* Idempotent, but may have side effects.
	*/
	MethodIdempotency[MethodIdempotency["Idempotent"] = 2] = "Idempotent";
})(MethodIdempotency || (MethodIdempotency = {}));
Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY;
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/codegen-info.js
var packageName = "@aptre/protobuf-es-lite";
var symbolInfo = (typeOnly, privateImportPath) => ({
	typeOnly,
	privateImportPath,
	publicImportPath: packageName
});
symbolInfo(false, "./message.js"), symbolInfo(true, "./field-list.js"), symbolInfo(true, "./field.js"), symbolInfo(true, "./message-type.js"), symbolInfo(true, "./extension.js"), symbolInfo(true, "./type-registry.js"), symbolInfo(true, "./binary-format.js"), symbolInfo(true, "./binary-format.js"), symbolInfo(true, "./json.js"), symbolInfo(true, "./json.js"), symbolInfo(true, "./json.js"), symbolInfo(true, "./json.js"), symbolInfo(false, "./json.js"), symbolInfo(false, "./json.js"), symbolInfo(false, "./json.js"), symbolInfo(false, "./json.js"), symbolInfo(false, "./json.js"), symbolInfo(false, "./proto-double.js"), symbolInfo(false, "./proto-int64.js"), symbolInfo(false, "./partial.js"), symbolInfo(false, "./scalar.js"), symbolInfo(false, "./scalar.js"), symbolInfo(false, "./scalar.js"), symbolInfo(false, "./service-type.js"), symbolInfo(false, "./service-type.js"), symbolInfo(false, "./enum.js"), symbolInfo(false, "./message.js");
//#endregion
//#region node_modules/starpc/dist/srpc/rpcproto.pb.js
var CallStart = createMessageType({
	typeName: "srpc.CallStart",
	fields: [
		{
			no: 1,
			name: "rpc_service",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "rpc_method",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "data",
			kind: "scalar",
			T: ScalarType.BYTES
		},
		{
			no: 4,
			name: "data_is_zero",
			kind: "scalar",
			T: ScalarType.BOOL
		}
	],
	packedByDefault: true
});
var CallData = createMessageType({
	typeName: "srpc.CallData",
	fields: [
		{
			no: 1,
			name: "data",
			kind: "scalar",
			T: ScalarType.BYTES
		},
		{
			no: 2,
			name: "data_is_zero",
			kind: "scalar",
			T: ScalarType.BOOL
		},
		{
			no: 3,
			name: "complete",
			kind: "scalar",
			T: ScalarType.BOOL
		},
		{
			no: 4,
			name: "error",
			kind: "scalar",
			T: ScalarType.STRING
		}
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
//#endregion
//#region node_modules/starpc/dist/srpc/common-rpc.js
var CommonRPC = class {
	sink;
	source;
	rpcDataSource;
	_source = pushable({ objectMode: true });
	_rpcDataSource = pushable({ objectMode: true });
	service;
	method;
	closed;
	constructor() {
		this.sink = this._createSink();
		this.source = this._source;
		this.rpcDataSource = this._rpcDataSource;
	}
	get isClosed() {
		return this.closed ?? false;
	}
	async writeCallData(data, complete, error) {
		const callData = {
			data: data || new Uint8Array(0),
			dataIsZero: !!data && data.length === 0,
			complete: complete || false,
			error: error || ""
		};
		await this.writePacket({ body: {
			case: "callData",
			value: callData
		} });
	}
	async writeCallCancel() {
		await this.writePacket({ body: {
			case: "callCancel",
			value: true
		} });
	}
	async writeCallDataFromSource(dataSource) {
		try {
			for await (const data of dataSource) await this.writeCallData(data);
			await this.writeCallData(void 0, true);
		} catch (err) {
			this.close(err);
		}
	}
	async writePacket(packet) {
		this._source.push(packet);
	}
	async handleMessage(message) {
		return this.handlePacket(Packet.fromBinary(message));
	}
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
					if (packet.body.value) await this.handleCallCancel();
					break;
			}
		} catch (err) {
			let asError = err;
			if (!asError?.message) asError = /* @__PURE__ */ new Error("error handling packet");
			this.close(asError);
		}
	}
	async handleCallStart(packet) {
		throw new Error(`unexpected call start: ${packet.rpcService}/${packet.rpcMethod}`);
	}
	pushRpcData(data, dataIsZero) {
		if (dataIsZero) {
			if (!data || data.length !== 0) data = new Uint8Array(0);
		} else if (!data || data.length === 0) return;
		this._rpcDataSource.push(data);
	}
	async handleCallData(packet) {
		if (!this.service || !this.method) throw new Error("call start must be sent before call data");
		this.pushRpcData(packet.data, packet.dataIsZero);
		if (packet.error) this._rpcDataSource.end(new Error(packet.error));
		else if (packet.complete) this._rpcDataSource.end();
	}
	async handleCallCancel() {
		this.close(new Error(ERR_RPC_ABORT));
	}
	async close(err) {
		if (this.closed) return;
		this.closed = err ?? true;
		if (err && err.message) await this.writeCallData(void 0, true, err.message);
		this._source.end();
		this._rpcDataSource.end(err);
	}
	_createSink() {
		return async (source) => {
			try {
				if (Symbol.asyncIterator in source) for await (const msg of source) await this.handlePacket(msg);
				else for (const msg of source) await this.handlePacket(msg);
			} catch (err) {
				this.close(err);
			}
		};
	}
};
//#endregion
//#region node_modules/starpc/dist/srpc/client-rpc.js
var ClientRPC = class extends CommonRPC {
	constructor(service, method) {
		super();
		this.service = service;
		this.method = method;
	}
	async writeCallStart(data) {
		if (!this.service || !this.method) throw new Error("service and method must be set");
		const callStart = {
			rpcService: this.service,
			rpcMethod: this.method,
			data: data || new Uint8Array(0),
			dataIsZero: !!data && data.length === 0
		};
		await this.writePacket({ body: {
			case: "callStart",
			value: callStart
		} });
	}
	async handleCallStart(packet) {
		throw new Error(`unexpected server to client rpc: ${packet.rpcService || "<empty>"}/${packet.rpcMethod || "<empty>"}`);
	}
};
//#endregion
//#region node_modules/starpc/dist/srpc/pushable.js
function messagePushable() {
	return pushable({ objectMode: true });
}
async function writeToPushable(dataSource, out) {
	try {
		for await (const data of dataSource) out.push(data);
		out.end();
	} catch (err) {
		out.end(err);
	}
}
//#endregion
//#region node_modules/uint8arrays/dist/src/alloc.js
/**
* Returns a `Uint8Array` of the requested size. Referenced memory will
* be initialized to 0.
*/
function alloc(size = 0) {
	return new Uint8Array(size);
}
/**
* Where possible returns a Uint8Array of the requested size that references
* uninitialized memory. Only use if you are certain you will immediately
* overwrite every value in the returned `Uint8Array`.
*/
function allocUnsafe(size = 0) {
	return new Uint8Array(size);
}
//#endregion
//#region node_modules/uint8arrays/dist/src/util/as-uint8array.js
/**
* To guarantee Uint8Array semantics, convert nodejs Buffers
* into vanilla Uint8Arrays
*/
function asUint8Array(buf) {
	return buf;
}
//#endregion
//#region node_modules/uint8arrays/dist/src/concat.js
/**
* Returns a new Uint8Array created by concatenating the passed Uint8Arrays
*/
function concat(arrays, length) {
	if (length == null) length = arrays.reduce((acc, curr) => acc + curr.length, 0);
	const output = allocUnsafe(length);
	let offset = 0;
	for (const arr of arrays) {
		output.set(arr, offset);
		offset += arr.length;
	}
	return asUint8Array(output);
}
//#endregion
//#region node_modules/uint8arrays/dist/src/equals.js
/**
* Returns true if the two passed Uint8Arrays have the same content
*/
function equals(a, b) {
	if (a === b) return true;
	if (a.byteLength !== b.byteLength) return false;
	for (let i = 0; i < a.byteLength; i++) if (a[i] !== b[i]) return false;
	return true;
}
//#endregion
//#region node_modules/uint8arraylist/dist/src/index.js
/**
* @packageDocumentation
*
* A class that lets you do operations over a list of Uint8Arrays without
* copying them.
*
* ```js
* import { Uint8ArrayList } from 'uint8arraylist'
*
* const list = new Uint8ArrayList()
* list.append(Uint8Array.from([0, 1, 2]))
* list.append(Uint8Array.from([3, 4, 5]))
*
* list.subarray()
* // -> Uint8Array([0, 1, 2, 3, 4, 5])
*
* list.consume(3)
* list.subarray()
* // -> Uint8Array([3, 4, 5])
*
* // you can also iterate over the list
* for (const buf of list) {
*   // ..do something with `buf`
* }
*
* list.subarray(0, 1)
* // -> Uint8Array([0])
* ```
*
* ## Converting Uint8ArrayLists to Uint8Arrays
*
* There are two ways to turn a `Uint8ArrayList` into a `Uint8Array` - `.slice` and `.subarray` and one way to turn a `Uint8ArrayList` into a `Uint8ArrayList` with different contents - `.sublist`.
*
* ### slice
*
* Slice follows the same semantics as [Uint8Array.slice](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/TypedArray/slice) in that it creates a new `Uint8Array` and copies bytes into it using an optional offset & length.
*
* ```js
* const list = new Uint8ArrayList()
* list.append(Uint8Array.from([0, 1, 2]))
* list.append(Uint8Array.from([3, 4, 5]))
*
* list.slice(0, 1)
* // -> Uint8Array([0])
* ```
*
* ### subarray
*
* Subarray attempts to follow the same semantics as [Uint8Array.subarray](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/TypedArray/subarray) with one important different - this is a no-copy operation, unless the requested bytes span two internal buffers in which case it is a copy operation.
*
* ```js
* const list = new Uint8ArrayList()
* list.append(Uint8Array.from([0, 1, 2]))
* list.append(Uint8Array.from([3, 4, 5]))
*
* list.subarray(0, 1)
* // -> Uint8Array([0]) - no-copy
*
* list.subarray(2, 5)
* // -> Uint8Array([2, 3, 4]) - copy
* ```
*
* ### sublist
*
* Sublist creates and returns a new `Uint8ArrayList` that shares the underlying buffers with the original so is always a no-copy operation.
*
* ```js
* const list = new Uint8ArrayList()
* list.append(Uint8Array.from([0, 1, 2]))
* list.append(Uint8Array.from([3, 4, 5]))
*
* list.sublist(0, 1)
* // -> Uint8ArrayList([0]) - no-copy
*
* list.sublist(2, 5)
* // -> Uint8ArrayList([2], [3, 4]) - no-copy
* ```
*
* ## Inspiration
*
* Borrows liberally from [bl](https://www.npmjs.com/package/bl) but only uses native JS types.
*/
var symbol = Symbol.for("@achingbrain/uint8arraylist");
function findBufAndOffset(bufs, index) {
	if (index == null || index < 0) throw new RangeError("index is out of bounds");
	let offset = 0;
	for (const buf of bufs) {
		const bufEnd = offset + buf.byteLength;
		if (index < bufEnd) return {
			buf,
			index: index - offset
		};
		offset = bufEnd;
	}
	throw new RangeError("index is out of bounds");
}
/**
* Check if object is a CID instance
*
* @example
*
* ```js
* import { isUint8ArrayList, Uint8ArrayList } from 'uint8arraylist'
*
* isUint8ArrayList(true) // false
* isUint8ArrayList([]) // false
* isUint8ArrayList(new Uint8ArrayList()) // true
* ```
*/
function isUint8ArrayList(value) {
	return Boolean(value?.[symbol]);
}
var Uint8ArrayList = class Uint8ArrayList {
	bufs;
	length;
	[symbol] = true;
	constructor(...data) {
		this.bufs = [];
		this.length = 0;
		if (data.length > 0) this.appendAll(data);
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
		for (const buf of bufs) if (buf instanceof Uint8Array) {
			length += buf.byteLength;
			this.bufs.push(buf);
		} else if (isUint8ArrayList(buf)) {
			length += buf.byteLength;
			this.bufs.push(...buf.bufs);
		} else throw new Error("Could not append value, must be an Uint8Array or a Uint8ArrayList");
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
		for (const buf of bufs.reverse()) if (buf instanceof Uint8Array) {
			length += buf.byteLength;
			this.bufs.unshift(buf);
		} else if (isUint8ArrayList(buf)) {
			length += buf.byteLength;
			this.bufs.unshift(...buf.bufs);
		} else throw new Error("Could not prepend value, must be an Uint8Array or a Uint8ArrayList");
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
		if (buf instanceof Uint8Array) for (let i = 0; i < buf.length; i++) this.set(offset + i, buf[i]);
		else if (isUint8ArrayList(buf)) for (let i = 0; i < buf.length; i++) this.set(offset + i, buf.get(i));
		else throw new Error("Could not write value, must be an Uint8Array or a Uint8ArrayList");
	}
	/**
	* Remove bytes from the front of the pool
	*/
	consume(bytes) {
		bytes = Math.trunc(bytes);
		if (Number.isNaN(bytes) || bytes <= 0) return;
		if (bytes === this.byteLength) {
			this.bufs = [];
			this.length = 0;
			return;
		}
		while (this.bufs.length > 0) if (bytes >= this.bufs[0].byteLength) {
			bytes -= this.bufs[0].byteLength;
			this.length -= this.bufs[0].byteLength;
			this.bufs.shift();
		} else {
			this.bufs[0] = this.bufs[0].subarray(bytes);
			this.length -= bytes;
			break;
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
		if (bufs.length === 1) return bufs[0];
		return concat(bufs, length);
	}
	/**
	* Returns a allocList from the given start and end element index.
	*
	* This is a no-copy operation.
	*/
	sublist(beginInclusive, endExclusive) {
		const { bufs, length } = this._subList(beginInclusive, endExclusive);
		const list = new Uint8ArrayList();
		list.length = length;
		list.bufs = [...bufs];
		return list;
	}
	_subList(beginInclusive, endExclusive) {
		beginInclusive = beginInclusive ?? 0;
		endExclusive = endExclusive ?? this.length;
		if (beginInclusive < 0) beginInclusive = this.length + beginInclusive;
		if (endExclusive < 0) endExclusive = this.length + endExclusive;
		if (beginInclusive < 0 || endExclusive > this.length) throw new RangeError("index is out of bounds");
		if (beginInclusive === endExclusive) return {
			bufs: [],
			length: 0
		};
		if (beginInclusive === 0 && endExclusive === this.length) return {
			bufs: this.bufs,
			length: this.length
		};
		const bufs = [];
		let offset = 0;
		for (let i = 0; i < this.bufs.length; i++) {
			const buf = this.bufs[i];
			const bufStart = offset;
			const bufEnd = bufStart + buf.byteLength;
			offset = bufEnd;
			if (beginInclusive >= bufEnd) continue;
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
		return {
			bufs,
			length: endExclusive - beginInclusive
		};
	}
	indexOf(search, offset = 0) {
		if (!isUint8ArrayList(search) && !(search instanceof Uint8Array)) throw new TypeError("The \"value\" argument must be a Uint8ArrayList or Uint8Array");
		const needle = search instanceof Uint8Array ? search : search.subarray();
		offset = Number(offset ?? 0);
		if (isNaN(offset)) offset = 0;
		if (offset < 0) offset = this.length + offset;
		if (offset < 0) offset = 0;
		if (search.length === 0) return offset > this.length ? this.length : offset;
		const M = needle.byteLength;
		if (M === 0) throw new TypeError("search must be at least 1 byte long");
		const radix = 256;
		const rightmostPositions = new Int32Array(radix);
		for (let c = 0; c < radix; c++) rightmostPositions[c] = -1;
		for (let j = 0; j < M; j++) rightmostPositions[needle[j]] = j;
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
			if (skip === 0) return i;
		}
		return -1;
	}
	getInt8(byteOffset) {
		const buf = this.subarray(byteOffset, byteOffset + 1);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getInt8(0);
	}
	setInt8(byteOffset, value) {
		const buf = allocUnsafe(1);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setInt8(0, value);
		this.write(buf, byteOffset);
	}
	getInt16(byteOffset, littleEndian) {
		const buf = this.subarray(byteOffset, byteOffset + 2);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getInt16(0, littleEndian);
	}
	setInt16(byteOffset, value, littleEndian) {
		const buf = alloc(2);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setInt16(0, value, littleEndian);
		this.write(buf, byteOffset);
	}
	getInt32(byteOffset, littleEndian) {
		const buf = this.subarray(byteOffset, byteOffset + 4);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getInt32(0, littleEndian);
	}
	setInt32(byteOffset, value, littleEndian) {
		const buf = alloc(4);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setInt32(0, value, littleEndian);
		this.write(buf, byteOffset);
	}
	getBigInt64(byteOffset, littleEndian) {
		const buf = this.subarray(byteOffset, byteOffset + 8);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getBigInt64(0, littleEndian);
	}
	setBigInt64(byteOffset, value, littleEndian) {
		const buf = alloc(8);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setBigInt64(0, value, littleEndian);
		this.write(buf, byteOffset);
	}
	getUint8(byteOffset) {
		const buf = this.subarray(byteOffset, byteOffset + 1);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getUint8(0);
	}
	setUint8(byteOffset, value) {
		const buf = allocUnsafe(1);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setUint8(0, value);
		this.write(buf, byteOffset);
	}
	getUint16(byteOffset, littleEndian) {
		const buf = this.subarray(byteOffset, byteOffset + 2);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getUint16(0, littleEndian);
	}
	setUint16(byteOffset, value, littleEndian) {
		const buf = alloc(2);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setUint16(0, value, littleEndian);
		this.write(buf, byteOffset);
	}
	getUint32(byteOffset, littleEndian) {
		const buf = this.subarray(byteOffset, byteOffset + 4);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getUint32(0, littleEndian);
	}
	setUint32(byteOffset, value, littleEndian) {
		const buf = alloc(4);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setUint32(0, value, littleEndian);
		this.write(buf, byteOffset);
	}
	getBigUint64(byteOffset, littleEndian) {
		const buf = this.subarray(byteOffset, byteOffset + 8);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getBigUint64(0, littleEndian);
	}
	setBigUint64(byteOffset, value, littleEndian) {
		const buf = alloc(8);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setBigUint64(0, value, littleEndian);
		this.write(buf, byteOffset);
	}
	getFloat32(byteOffset, littleEndian) {
		const buf = this.subarray(byteOffset, byteOffset + 4);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getFloat32(0, littleEndian);
	}
	setFloat32(byteOffset, value, littleEndian) {
		const buf = alloc(4);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setFloat32(0, value, littleEndian);
		this.write(buf, byteOffset);
	}
	getFloat64(byteOffset, littleEndian) {
		const buf = this.subarray(byteOffset, byteOffset + 8);
		return new DataView(buf.buffer, buf.byteOffset, buf.byteLength).getFloat64(0, littleEndian);
	}
	setFloat64(byteOffset, value, littleEndian) {
		const buf = alloc(8);
		new DataView(buf.buffer, buf.byteOffset, buf.byteLength).setFloat64(0, value, littleEndian);
		this.write(buf, byteOffset);
	}
	equals(other) {
		if (other == null) return false;
		if (!(other instanceof Uint8ArrayList)) return false;
		if (other.bufs.length !== this.bufs.length) return false;
		for (let i = 0; i < this.bufs.length; i++) if (!equals(this.bufs[i], other.bufs[i])) return false;
		return true;
	}
	/**
	* Create a Uint8ArrayList from a pre-existing list of Uint8Arrays.  Use this
	* method if you know the total size of all the Uint8Arrays ahead of time.
	*/
	static fromUint8Arrays(bufs, length) {
		const list = new Uint8ArrayList();
		list.bufs = bufs;
		if (length == null) length = bufs.reduce((acc, curr) => acc + curr.byteLength, 0);
		list.length = length;
		return list;
	}
};
//#endregion
//#region node_modules/starpc/dist/srpc/message.js
function buildDecodeMessageTransform(def) {
	const decode = def.fromBinary.bind(def);
	return async function* decodeMessageSource(source) {
		for await (const pkt of source) if (Array.isArray(pkt)) for (const p of pkt) yield* [decode(p)];
		else yield* [decode(pkt)];
	};
}
function buildEncodeMessageTransform(def) {
	return async function* encodeMessageSource(source) {
		for await (const pkt of source) if (Array.isArray(pkt)) for (const p of pkt) yield def.toBinary(p);
		else yield def.toBinary(pkt);
	};
}
//#endregion
//#region node_modules/starpc/dist/srpc/packet.js
var decodePacketSource = buildDecodeMessageTransform(Packet);
var encodePacketSource = buildEncodeMessageTransform(Packet);
var uint32LEDecode = (data) => {
	if (data.length < 4) throw RangeError("Could not decode int32BE");
	return data.getUint32(0, true);
};
uint32LEDecode.bytes = 4;
var uint32LEEncode = (value) => {
	const data = new Uint8ArrayList(new Uint8Array(4));
	data.setUint32(0, value, true);
	return data;
};
uint32LEEncode.bytes = 4;
//#endregion
//#region node_modules/starpc/dist/srpc/value-ctr.js
var ValueCtr = class {
	_value;
	_waiters;
	constructor(initialValue) {
		this._value = initialValue || void 0;
		this._waiters = [];
	}
	get value() {
		return this._value;
	}
	async wait() {
		const currVal = this._value;
		if (currVal !== void 0) return currVal;
		return new Promise((resolve) => {
			this.waitWithCb((val) => {
				resolve(val);
			});
		});
	}
	waitWithCb(cb) {
		if (cb) this._waiters.push(cb);
	}
	set(val) {
		this._value = val;
		if (val === void 0) return;
		const waiters = this._waiters;
		if (waiters.length === 0) return;
		this._waiters = [];
		for (const waiter of waiters) waiter(val);
	}
};
//#endregion
//#region node_modules/starpc/dist/srpc/open-stream-ctr.js
var OpenStreamCtr = class extends ValueCtr {
	constructor(openStreamFn) {
		super(openStreamFn);
	}
	get openStreamFunc() {
		return async () => {
			let openFn = this.value;
			if (!openFn) openFn = await this.wait();
			return openFn();
		};
	}
};
//#endregion
//#region node_modules/starpc/dist/srpc/client.js
var Client = class {
	openStreamCtr;
	constructor(openStreamFn) {
		this.openStreamCtr = new OpenStreamCtr(openStreamFn || void 0);
	}
	setOpenStreamFn(openStreamFn) {
		this.openStreamCtr.set(openStreamFn || void 0);
	}
	async request(service, method, data, abortSignal) {
		const call = await this.startRpc(service, method, data, abortSignal);
		for await (const data of call.rpcDataSource) {
			call.close();
			return data;
		}
		const err = /* @__PURE__ */ new Error("empty response");
		call.close(err);
		throw err;
	}
	async clientStreamingRequest(service, method, data, abortSignal) {
		const call = await this.startRpc(service, method, null, abortSignal);
		call.writeCallDataFromSource(data).catch((err) => call.close(err));
		for await (const data of call.rpcDataSource) {
			call.close();
			return data;
		}
		const err = /* @__PURE__ */ new Error("empty response");
		call.close(err);
		throw err;
	}
	serverStreamingRequest(service, method, data, abortSignal) {
		const serverData = pushable({ objectMode: true });
		this.startRpc(service, method, data, abortSignal).then(async (call) => writeToPushable(call.rpcDataSource, serverData)).catch((err) => serverData.end(err));
		return serverData;
	}
	bidirectionalStreamingRequest(service, method, data, abortSignal) {
		const serverData = pushable({ objectMode: true });
		this.startRpc(service, method, null, abortSignal).then(async (call) => {
			const handleErr = (err) => {
				serverData.end(err);
				call.close(err);
			};
			call.writeCallDataFromSource(data).catch(handleErr);
			try {
				for await (const message of call.rpcDataSource) serverData.push(message);
				serverData.end();
				call.close();
			} catch (err) {
				handleErr(err);
			}
		}).catch((err) => serverData.end(err));
		return serverData;
	}
	async startRpc(rpcService, rpcMethod, data, abortSignal) {
		if (abortSignal?.aborted) throw new Error(ERR_RPC_ABORT);
		const stream = await (await this.openStreamCtr.wait())();
		const call = new ClientRPC(rpcService, rpcMethod);
		const onAbort = () => {
			call.writeCallCancel();
			call.close(new Error(ERR_RPC_ABORT));
		};
		abortSignal?.addEventListener("abort", onAbort, { once: true });
		pipe(stream, decodePacketSource, call, encodePacketSource, stream).catch((err) => call.close(err)).then(() => call.close()).finally(() => {
			abortSignal?.removeEventListener("abort", onAbort);
		});
		await call.writeCallStart(data ?? void 0);
		return call;
	}
};
//#endregion
//#region node_modules/starpc/dist/srpc/server-rpc.js
var ServerRPC = class extends CommonRPC {
	lookupMethod;
	constructor(lookupMethod) {
		super();
		this.lookupMethod = lookupMethod;
	}
	async handleCallStart(packet) {
		if (this.service || this.method) throw new Error("call start must be sent only once");
		this.service = packet.rpcService;
		this.method = packet.rpcMethod;
		if (!this.service || !this.method) throw new Error("rpcService and rpcMethod cannot be empty");
		if (!this.lookupMethod) throw new Error("LookupMethod is not defined");
		const methodDef = await this.lookupMethod(this.service, this.method);
		if (!methodDef) throw new Error(`not found: ${this.service}/${this.method}`);
		this.pushRpcData(packet.data, packet.dataIsZero);
		this.invokeRPC(methodDef);
	}
	async handleCallData(packet) {
		if (!this.service || !this.method) throw new Error("call start must be sent before call data");
		return super.handleCallData(packet);
	}
	async invokeRPC(invokeFn) {
		const dataSink = this._createDataSink();
		try {
			await invokeFn(this.rpcDataSource, dataSink);
		} catch (err) {
			this.close(err);
		}
	}
	_createDataSink() {
		return async (source) => {
			try {
				for await (const msg of source) await this.writeCallData(msg);
				await this.writeCallData(void 0, true);
				this.close();
			} catch (err) {
				this.close(err);
			}
		};
	}
};
//#endregion
//#region node_modules/starpc/dist/srpc/server.js
var Server = class {
	lookupMethod;
	constructor(lookupMethod) {
		this.lookupMethod = lookupMethod;
	}
	get rpcStreamHandler() {
		return async (stream) => {
			const rpc = this.startRpc();
			return pipe(stream, decodePacketSource, rpc, encodePacketSource, stream).catch((err) => rpc.close(err)).then(() => rpc.close());
		};
	}
	startRpc() {
		return new ServerRPC(this.lookupMethod);
	}
	handlePacketStream(stream) {
		const rpc = this.startRpc();
		pipe(stream, decodePacketSource, rpc, encodePacketSource, stream).catch((err) => rpc.close(err)).then(() => rpc.close());
		return rpc;
	}
};
//#endregion
//#region node_modules/@chainsafe/libp2p-yamux/dist/src/errors.js
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
new Set([
	InvalidFrameError.name,
	UnrequestedPingError.name,
	NotMatchingPingError.name,
	StreamAlreadyExistsError.name,
	DecodeInvalidVersionError.name,
	BothClientsError.name,
	ReceiveWindowExceededError.name
]);
//#endregion
//#region node_modules/@chainsafe/libp2p-yamux/dist/src/frame.js
var FrameType;
(function(FrameType) {
	/** Used to transmit data. May transmit zero length payloads depending on the flags. */
	FrameType[FrameType["Data"] = 0] = "Data";
	/** Used to updated the senders receive window size. This is used to implement per-session flow control. */
	FrameType[FrameType["WindowUpdate"] = 1] = "WindowUpdate";
	/** Used to measure RTT. It can also be used to heart-beat and do keep-alives over TCP. */
	FrameType[FrameType["Ping"] = 2] = "Ping";
	/** Used to close a session. */
	FrameType[FrameType["GoAway"] = 3] = "GoAway";
})(FrameType || (FrameType = {}));
var Flag;
(function(Flag) {
	/** Signals the start of a new stream. May be sent with a data or window update message. Also sent with a ping to indicate outbound. */
	Flag[Flag["SYN"] = 1] = "SYN";
	/** Acknowledges the start of a new stream. May be sent with a data or window update message. Also sent with a ping to indicate response. */
	Flag[Flag["ACK"] = 2] = "ACK";
	/** Performs a half-close of a stream. May be sent with a data message or window update. */
	Flag[Flag["FIN"] = 4] = "FIN";
	/** Reset a stream immediately. May be sent with a data or window update message. */
	Flag[Flag["RST"] = 8] = "RST";
})(Flag || (Flag = {}));
Object.values(Flag).filter((x) => typeof x !== "string");
var GoAwayCode;
(function(GoAwayCode) {
	GoAwayCode[GoAwayCode["NormalTermination"] = 0] = "NormalTermination";
	GoAwayCode[GoAwayCode["ProtocolError"] = 1] = "ProtocolError";
	GoAwayCode[GoAwayCode["InternalError"] = 2] = "InternalError";
})(GoAwayCode || (GoAwayCode = {}));
//#endregion
//#region node_modules/@chainsafe/libp2p-yamux/dist/src/stream.js
var StreamState;
(function(StreamState) {
	StreamState[StreamState["Init"] = 0] = "Init";
	StreamState[StreamState["SYNSent"] = 1] = "SYNSent";
	StreamState[StreamState["SYNReceived"] = 2] = "SYNReceived";
	StreamState[StreamState["Established"] = 3] = "Established";
	StreamState[StreamState["Finished"] = 4] = "Finished";
})(StreamState || (StreamState = {}));
//#endregion
//#region node_modules/event-iterator/lib/event-iterator.js
var require_event_iterator = /* @__PURE__ */ __commonJSMin(((exports) => {
	Object.defineProperty(exports, "__esModule", { value: true });
	var EventQueue = class {
		constructor() {
			this.pullQueue = [];
			this.pushQueue = [];
			this.eventHandlers = {};
			this.isPaused = false;
			this.isStopped = false;
		}
		push(value) {
			if (this.isStopped) return;
			const resolution = {
				value,
				done: false
			};
			if (this.pullQueue.length) {
				const placeholder = this.pullQueue.shift();
				if (placeholder) placeholder.resolve(resolution);
			} else {
				this.pushQueue.push(Promise.resolve(resolution));
				if (this.highWaterMark !== void 0 && this.pushQueue.length >= this.highWaterMark && !this.isPaused) {
					this.isPaused = true;
					if (this.eventHandlers.highWater) this.eventHandlers.highWater();
					else if (console) console.warn(`EventIterator queue reached ${this.pushQueue.length} items`);
				}
			}
		}
		stop() {
			if (this.isStopped) return;
			this.isStopped = true;
			this.remove();
			for (const placeholder of this.pullQueue) placeholder.resolve({
				value: void 0,
				done: true
			});
			this.pullQueue.length = 0;
		}
		fail(error) {
			if (this.isStopped) return;
			this.isStopped = true;
			this.remove();
			if (this.pullQueue.length) {
				for (const placeholder of this.pullQueue) placeholder.reject(error);
				this.pullQueue.length = 0;
			} else {
				const rejection = Promise.reject(error);
				rejection.catch(() => {});
				this.pushQueue.push(rejection);
			}
		}
		remove() {
			Promise.resolve().then(() => {
				if (this.removeCallback) this.removeCallback();
			});
		}
		[Symbol.asyncIterator]() {
			return {
				next: (value) => {
					const result = this.pushQueue.shift();
					if (result) {
						if (this.lowWaterMark !== void 0 && this.pushQueue.length <= this.lowWaterMark && this.isPaused) {
							this.isPaused = false;
							if (this.eventHandlers.lowWater) this.eventHandlers.lowWater();
						}
						return result;
					} else if (this.isStopped) return Promise.resolve({
						value: void 0,
						done: true
					});
					else return new Promise((resolve, reject) => {
						this.pullQueue.push({
							resolve,
							reject
						});
					});
				},
				return: () => {
					this.isStopped = true;
					this.pushQueue.length = 0;
					this.remove();
					return Promise.resolve({
						value: void 0,
						done: true
					});
				}
			};
		}
	};
	var EventIterator = class {
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
			}) || (() => {});
			this[Symbol.asyncIterator] = () => queue[Symbol.asyncIterator]();
			Object.freeze(this);
		}
	};
	exports.EventIterator = EventIterator;
	exports.default = EventIterator;
}));
(/* @__PURE__ */ __commonJSMin(((exports) => {
	Object.defineProperty(exports, "__esModule", { value: true });
	var event_iterator_1 = require_event_iterator();
	exports.EventIterator = event_iterator_1.EventIterator;
	exports.default = event_iterator_1.EventIterator;
})))();
//#endregion
//#region node_modules/starpc/dist/srpc/invoker.js
function createInvokeFn(methodInfo, methodProto) {
	const requestDecode = buildDecodeMessageTransform(methodInfo.I);
	return async (dataSource, dataSink) => {
		const responseSink = pushable({ objectMode: true });
		pipe(responseSink, buildEncodeMessageTransform(methodInfo.O), dataSink);
		const requestSource = pipe(dataSource, requestDecode);
		let requestArg;
		if (methodInfo.kind === MethodKind.ClientStreaming || methodInfo.kind === MethodKind.BiDiStreaming) requestArg = requestSource;
		else for await (const msg of requestSource) {
			requestArg = msg;
			break;
		}
		if (!requestArg) throw new Error("request object was empty");
		try {
			const responseObj = methodProto(requestArg);
			if (!responseObj) throw new Error("return value was undefined");
			if (methodInfo.kind === MethodKind.ServerStreaming || methodInfo.kind === MethodKind.BiDiStreaming) return writeToPushable(responseObj, responseSink);
			else {
				const responsePromise = responseObj;
				if (!responsePromise.then) throw new Error("expected return value to be a Promise");
				const responseMsg = await responsePromise;
				if (!responseMsg) throw new Error("expected non-empty response object");
				responseSink.push(responseMsg);
				responseSink.end();
			}
		} catch (err) {
			let asError = err;
			if (!asError?.message) asError = /* @__PURE__ */ new Error("error calling implementation: " + err);
			responseSink.end();
			throw asError;
		}
	};
}
//#endregion
//#region node_modules/starpc/dist/srpc/handler.js
var StaticHandler = class {
	service;
	methods;
	constructor(serviceID, methods) {
		this.service = serviceID;
		this.methods = methods;
	}
	getServiceID() {
		return this.service;
	}
	getMethodIDs() {
		return Object.keys(this.methods);
	}
	async lookupMethod(serviceID, methodID) {
		if (serviceID && serviceID !== this.service) return null;
		return this.methods[methodID] || null;
	}
};
function createHandler(definition, impl, serviceID) {
	serviceID = serviceID || definition.typeName;
	const methodMap = {};
	for (const methodInfo of Object.values(definition.methods)) {
		const methodName = methodInfo.name;
		let methodProto = impl[methodName];
		if (!methodProto) continue;
		methodProto = methodProto.bind(impl);
		methodMap[methodName] = createInvokeFn(methodInfo, methodProto);
	}
	return new StaticHandler(serviceID, methodMap);
}
//#endregion
//#region node_modules/starpc/dist/srpc/mux.js
function createMux() {
	return new StaticMux();
}
var StaticMux = class {
	services = {};
	lookups = [];
	get lookupMethod() {
		return this._lookupMethod.bind(this);
	}
	register(handler) {
		const serviceID = handler?.getServiceID();
		if (!serviceID) throw new Error("service id cannot be empty");
		const serviceMethods = this.services[serviceID] || {};
		const methodIDs = handler.getMethodIDs();
		for (const methodID of methodIDs) serviceMethods[methodID] = handler;
		this.services[serviceID] = serviceMethods;
	}
	registerLookupMethod(lookupMethod) {
		this.lookups.push(lookupMethod);
	}
	async _lookupMethod(serviceID, methodID) {
		if (serviceID) {
			const invokeFn = await this.lookupViaMap(serviceID, methodID);
			if (invokeFn) return invokeFn;
		}
		return await this.lookupViaLookups(serviceID, methodID);
	}
	async lookupViaMap(serviceID, methodID) {
		const serviceMethods = this.services[serviceID];
		if (!serviceMethods) return null;
		const handler = serviceMethods[methodID];
		if (!handler) return null;
		return await handler.lookupMethod(serviceID, methodID);
	}
	async lookupViaLookups(serviceID, methodID) {
		for (const lookupMethod of this.lookups) {
			const invokeFn = await lookupMethod(serviceID, methodID);
			if (invokeFn) return invokeFn;
		}
		return null;
	}
};
//#endregion
//#region node_modules/starpc/dist/srpc/watchdog.js
var Watchdog = class {
	timeoutDuration;
	expiredCallback;
	timerId = null;
	lastFeedTimestamp = null;
	paused = false;
	pausedTimestamp = null;
	/**
	* Constructs a Watchdog instance.
	* The Watchdog will not start ticking until feed() is called.
	* @param timeoutDuration The duration in milliseconds after which the watchdog should expire if not fed.
	* @param expiredCallback The callback function to be called when the watchdog expires.
	*/
	constructor(timeoutDuration, expiredCallback) {
		this.timeoutDuration = timeoutDuration;
		this.expiredCallback = expiredCallback;
	}
	/**
	* Returns whether the watchdog is currently paused.
	*/
	get isPaused() {
		return this.paused;
	}
	/**
	* Pauses the watchdog, preventing it from expiring until resumed.
	* The time spent paused does not count towards the timeout.
	*/
	pause() {
		if (this.paused) return;
		this.paused = true;
		this.pausedTimestamp = Date.now();
		if (this.timerId != null) {
			clearTimeout(this.timerId);
			this.timerId = null;
		}
	}
	/**
	* Resumes the watchdog after being paused.
	* The timeout continues from where it left off, not counting the paused duration.
	*/
	resume() {
		if (!this.paused) return;
		this.paused = false;
		if (this.lastFeedTimestamp != null && this.pausedTimestamp != null) {
			const pausedDuration = Date.now() - this.pausedTimestamp;
			this.lastFeedTimestamp += pausedDuration;
		}
		this.pausedTimestamp = null;
		if (this.lastFeedTimestamp != null) {
			const elapsed = Date.now() - this.lastFeedTimestamp;
			const remaining = Math.max(0, this.timeoutDuration - elapsed);
			this.scheduleTickWatchdog(remaining);
		}
	}
	/**
	* Feeds the watchdog, preventing it from expiring.
	* This resets the timeout and reschedules the next tick.
	*/
	feed() {
		this.lastFeedTimestamp = Date.now();
		this.scheduleTickWatchdog(this.timeoutDuration);
	}
	/**
	* Clears the current timeout, effectively stopping the watchdog.
	* This prevents the expired callback from being called until the watchdog is fed again.
	*/
	clear() {
		if (this.timerId != null) {
			clearTimeout(this.timerId);
			this.timerId = null;
		}
		this.lastFeedTimestamp = null;
	}
	/**
	* Schedules the next tick of the watchdog.
	* This method calculates the delay for the next tick based on the last feed time
	* and schedules a call to tickWatchdog after that delay.
	*/
	scheduleTickWatchdog(delay) {
		if (this.timerId != null) clearTimeout(this.timerId);
		this.timerId = setTimeout(() => this.tickWatchdog(), delay);
	}
	/**
	* Handler for the watchdog tick.
	* Checks if the time since the last feed is greater than the timeout duration.
	* If so, it calls the expired callback. Otherwise, it reschedules the tick.
	*/
	tickWatchdog() {
		this.timerId = null;
		if (this.paused) return;
		if (this.lastFeedTimestamp == null) {
			this.expiredCallback();
			return;
		}
		const elapsedSinceLastFeed = Date.now() - this.lastFeedTimestamp;
		if (elapsedSinceLastFeed >= this.timeoutDuration) this.expiredCallback();
		else this.scheduleTickWatchdog(this.timeoutDuration - elapsedSinceLastFeed);
	}
};
//#endregion
//#region node_modules/starpc/dist/srpc/channel.js
var ChannelStream = class {
	channel;
	sink;
	source;
	_source;
	localId;
	localOpen;
	remoteOpen;
	waitRemoteOpen;
	_remoteOpen;
	remoteAck;
	waitRemoteAck;
	_remoteAck;
	keepAlive;
	idleWatchdog;
	closed = false;
	get isAcked() {
		return this.remoteAck ?? false;
	}
	get isOpen() {
		return this.remoteOpen ?? false;
	}
	get isIdlePaused() {
		return this.idleWatchdog?.isPaused ?? false;
	}
	constructor(localId, channel, opts) {
		this.localId = localId;
		this.channel = channel;
		this.localOpen = false;
		this.remoteOpen = opts?.remoteOpen ?? false;
		this.remoteAck = this.remoteOpen;
		if (this.remoteOpen) {
			this.waitRemoteOpen = Promise.resolve();
			this.waitRemoteAck = Promise.resolve();
		} else {
			this.waitRemoteOpen = new Promise((resolve, reject) => {
				this._remoteOpen = (err) => {
					if (err) reject(err);
					else resolve();
				};
			});
			this.waitRemoteOpen.catch(() => {});
			this.waitRemoteAck = new Promise((resolve, reject) => {
				this._remoteAck = (err) => {
					if (err) reject(err);
					else resolve();
				};
			});
			this.waitRemoteAck.catch(() => {});
		}
		this.sink = this._createSink();
		const source = pushable({ objectMode: true });
		this.source = source;
		this._source = source;
		const onMessage = this.onMessage.bind(this);
		if (channel instanceof MessagePort) {
			channel.onmessage = onMessage;
			channel.start();
		} else channel.rx.onmessage = onMessage;
		if (opts?.idleTimeoutMs != null) this.idleWatchdog = new Watchdog(opts.idleTimeoutMs, () => this.idleElapsed());
		if (opts?.keepAliveMs != null) this.keepAlive = new Watchdog(opts.keepAliveMs, () => this.keepAliveElapsed());
		this.postMessage({ ack: true });
	}
	postMessage(msg) {
		if (this.closed) return;
		msg.from = this.localId;
		if (this.channel instanceof MessagePort) this.channel.postMessage(msg);
		else this.channel.tx.postMessage(msg);
		if (!msg.closed) this.keepAlive?.feed();
	}
	idleElapsed() {
		if (this.idleWatchdog) {
			delete this.idleWatchdog;
			this.close(new Error(ERR_STREAM_IDLE));
		}
	}
	keepAliveElapsed() {
		if (this.keepAlive) this.postMessage({});
	}
	finish(error, notifyRemote) {
		if (this.closed) return;
		if (notifyRemote) try {
			this.postMessage({
				closed: true,
				error
			});
		} catch {}
		this.closed = true;
		if (this.channel instanceof MessagePort) {
			this.channel.onmessage = null;
			this.channel.close();
		} else {
			this.channel.rx.onmessage = null;
			this.channel.tx.close();
			this.channel.rx.close();
		}
		if (!this.remoteOpen && this._remoteOpen) {
			this._remoteOpen(error || /* @__PURE__ */ new Error("closed"));
			delete this._remoteOpen;
		}
		if (!this.remoteAck && this._remoteAck) {
			this._remoteAck(error || /* @__PURE__ */ new Error("closed"));
			delete this._remoteAck;
		}
		if (this.idleWatchdog) {
			this.idleWatchdog.clear();
			delete this.idleWatchdog;
		}
		if (this.keepAlive) {
			this.keepAlive.clear();
			delete this.keepAlive;
		}
		this._source.end(error);
	}
	close(error) {
		this.finish(error, true);
	}
	pauseIdle() {
		this.idleWatchdog?.pause();
	}
	resumeIdle() {
		this.idleWatchdog?.resume();
	}
	onLocalOpened() {
		if (!this.localOpen) {
			this.localOpen = true;
			this.postMessage({ opened: true });
		}
	}
	onRemoteAcked() {
		if (!this.remoteAck) {
			this.remoteAck = true;
			if (this._remoteAck) this._remoteAck();
		}
	}
	onRemoteOpened() {
		if (!this.remoteOpen) {
			this.remoteOpen = true;
			if (this._remoteOpen) this._remoteOpen();
		}
	}
	_createSink() {
		return async (source) => {
			await this.waitRemoteAck;
			this.onLocalOpened();
			await this.waitRemoteOpen;
			try {
				for await (const msg of source) this.postMessage({ data: msg });
				this.postMessage({ closed: true });
			} catch (error) {
				this.postMessage({
					closed: true,
					error
				});
			}
		};
	}
	onMessage(ev) {
		const msg = ev.data;
		if (!msg || msg.from === this.localId || !msg.from) return;
		this.idleWatchdog?.feed();
		if (msg.ack || msg.opened) this.onRemoteAcked();
		if (msg.opened) this.onRemoteOpened();
		const { data, closed, error: err } = msg;
		if (data) this._source.push(data);
		if (err) {
			this.finish(err, false);
			return;
		}
		if (closed) this.finish(void 0, false);
	}
};
//#endregion
//#region node_modules/starpc/dist/rpcstream/rpcstream.js
async function openRpcStream(componentId, caller, waitAck) {
	const packetTx = pushable({ objectMode: true });
	const packetRx = caller(packetTx);
	packetTx.push({ body: {
		case: "init",
		value: { componentId }
	} });
	const packetIt = packetRx[Symbol.asyncIterator]();
	if (waitAck) {
		const ackPacketIt = await packetIt.next();
		if (ackPacketIt.done) throw new Error(`rpcstream: closed before ack packet`);
		const ackBody = ackPacketIt.value?.body;
		if (!ackBody || ackBody.case !== "ack") {
			const msgType = ackBody?.case || "none";
			throw new Error(`rpcstream: expected ack packet but got ${msgType}`);
		}
		const errStr = ackBody.value?.error;
		if (errStr) throw new Error(`rpcstream: remote: ${errStr}`);
	}
	return new RpcStream(packetTx, packetIt);
}
async function* handleRpcStream(packetRx, getter) {
	const initRpcStreamIt = await packetRx.next();
	if (initRpcStreamIt.done) throw new Error("closed before init received");
	const initRpcStreamPacket = initRpcStreamIt.value;
	if (initRpcStreamPacket?.body?.case !== "init") throw new Error("expected init packet");
	let handler = null;
	let err;
	try {
		handler = await getter(initRpcStreamPacket.body.value.componentId ?? "");
	} catch (errAny) {
		err = errAny;
		if (!err) err = /* @__PURE__ */ new Error(`rpc getter failed`);
		else if (!err.message) err = /* @__PURE__ */ new Error(`rpc getter failed: ${err}`);
	}
	if (!handler && !err) err = /* @__PURE__ */ new Error("not implemented");
	yield* [{ body: {
		case: "ack",
		value: { error: err?.message || "" }
	} }];
	if (err) throw err;
	const packetTx = pushable({ objectMode: true });
	const rpcStream = new RpcStream(packetTx, packetRx);
	handler(rpcStream).catch((err) => packetTx.end(err)).then(() => packetTx.end());
	for await (const packet of packetTx) yield* [packet];
}
var RpcStream = class {
	source;
	sink;
	_packetRx;
	_packetTx;
	constructor(packetTx, packetRx) {
		this._packetTx = packetTx;
		this._packetRx = packetRx;
		this.sink = this._createSink();
		this.source = this._createSource();
	}
	_createSink() {
		return async (source) => {
			try {
				for await (const arr of source) this._packetTx.push({ body: {
					case: "data",
					value: arr
				} });
				this._packetTx.end();
			} catch (err) {
				this._packetTx.end(err);
			}
		};
	}
	_createSource() {
		return (async function* (packetRx) {
			while (true) {
				const msgIt = await packetRx.next();
				if (msgIt.done) return;
				const body = msgIt.value?.body;
				if (!body) continue;
				switch (body.case) {
					case "ack":
						if (body.value.error?.length) throw new Error(body.value.error);
						break;
					case "data":
						yield body.value;
						break;
				}
			}
		})(this._packetRx);
	}
};
//#endregion
//#region node_modules/starpc/dist/rpcstream/rpcstream.pb.js
var RpcStreamInit = createMessageType({
	typeName: "rpcstream.RpcStreamInit",
	fields: [{
		no: 1,
		name: "component_id",
		kind: "scalar",
		T: ScalarType.STRING
	}],
	packedByDefault: true
});
var RpcAck = createMessageType({
	typeName: "rpcstream.RpcAck",
	fields: [{
		no: 1,
		name: "error",
		kind: "scalar",
		T: ScalarType.STRING
	}],
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
		{
			no: 2,
			name: "ack",
			kind: "message",
			T: () => RpcAck,
			oneof: "body"
		},
		{
			no: 3,
			name: "data",
			kind: "scalar",
			T: ScalarType.BYTES,
			oneof: "body"
		}
	],
	packedByDefault: true
});
//#endregion
//#region node_modules/starpc/dist/echo/echo.pb.js
var EchoMsg = createMessageType({
	typeName: "echo.EchoMsg",
	fields: [{
		no: 1,
		name: "body",
		kind: "scalar",
		T: ScalarType.STRING
	}],
	packedByDefault: true
});
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/google/protobuf/empty.pb.js
var Empty = createMessageType({
	typeName: "google.protobuf.Empty",
	fields: [],
	packedByDefault: true
});
//#endregion
//#region node_modules/starpc/dist/echo/echo_srpc.pb.js
/**
* Echoer service returns the given message.
*
* @generated from service echo.Echoer
*/
var EchoerDefinition = {
	typeName: "echo.Echoer",
	methods: {
		Echo: {
			name: "Echo",
			I: EchoMsg,
			O: EchoMsg,
			kind: MethodKind.Unary
		},
		EchoServerStream: {
			name: "EchoServerStream",
			I: EchoMsg,
			O: EchoMsg,
			kind: MethodKind.ServerStreaming
		},
		EchoClientStream: {
			name: "EchoClientStream",
			I: EchoMsg,
			O: EchoMsg,
			kind: MethodKind.ClientStreaming
		},
		EchoBidiStream: {
			name: "EchoBidiStream",
			I: EchoMsg,
			O: EchoMsg,
			kind: MethodKind.BiDiStreaming
		},
		RpcStream: {
			name: "RpcStream",
			I: RpcStreamPacket,
			O: RpcStreamPacket,
			kind: MethodKind.BiDiStreaming
		},
		DoNothing: {
			name: "DoNothing",
			I: Empty,
			O: Empty,
			kind: MethodKind.Unary
		}
	}
};
var EchoerServiceName = EchoerDefinition.typeName;
var EchoerClient = class {
	rpc;
	service;
	constructor(rpc, opts) {
		this.service = opts?.service || EchoerServiceName;
		this.rpc = rpc;
		this.Echo = this.Echo.bind(this);
		this.EchoServerStream = this.EchoServerStream.bind(this);
		this.EchoClientStream = this.EchoClientStream.bind(this);
		this.EchoBidiStream = this.EchoBidiStream.bind(this);
		this.RpcStream = this.RpcStream.bind(this);
		this.DoNothing = this.DoNothing.bind(this);
	}
	/**
	* Echo returns the given message.
	*
	* @generated from rpc echo.Echoer.Echo
	*/
	async Echo(request, abortSignal) {
		const requestMsg = EchoMsg.create(request);
		const result = await this.rpc.request(this.service, EchoerDefinition.methods.Echo.name, EchoMsg.toBinary(requestMsg), abortSignal || void 0);
		return EchoMsg.fromBinary(result);
	}
	/**
	* EchoServerStream is an example of a server -> client one-way stream.
	*
	* @generated from rpc echo.Echoer.EchoServerStream
	*/
	EchoServerStream(request, abortSignal) {
		const requestMsg = EchoMsg.create(request);
		const result = this.rpc.serverStreamingRequest(this.service, EchoerDefinition.methods.EchoServerStream.name, EchoMsg.toBinary(requestMsg), abortSignal || void 0);
		return buildDecodeMessageTransform(EchoMsg)(result);
	}
	/**
	* EchoClientStream is an example of client->server one-way stream.
	*
	* @generated from rpc echo.Echoer.EchoClientStream
	*/
	async EchoClientStream(request, abortSignal) {
		const result = await this.rpc.clientStreamingRequest(this.service, EchoerDefinition.methods.EchoClientStream.name, buildEncodeMessageTransform(EchoMsg)(request), abortSignal || void 0);
		return EchoMsg.fromBinary(result);
	}
	/**
	* EchoBidiStream is an example of a two-way stream.
	*
	* @generated from rpc echo.Echoer.EchoBidiStream
	*/
	EchoBidiStream(request, abortSignal) {
		const result = this.rpc.bidirectionalStreamingRequest(this.service, EchoerDefinition.methods.EchoBidiStream.name, buildEncodeMessageTransform(EchoMsg)(request), abortSignal || void 0);
		return buildDecodeMessageTransform(EchoMsg)(result);
	}
	/**
	* RpcStream opens a nested rpc call stream.
	*
	* @generated from rpc echo.Echoer.RpcStream
	*/
	RpcStream(request, abortSignal) {
		const result = this.rpc.bidirectionalStreamingRequest(this.service, EchoerDefinition.methods.RpcStream.name, buildEncodeMessageTransform(RpcStreamPacket)(request), abortSignal || void 0);
		return buildDecodeMessageTransform(RpcStreamPacket)(result);
	}
	/**
	* DoNothing does nothing.
	*
	* @generated from rpc echo.Echoer.DoNothing
	*/
	async DoNothing(request, abortSignal) {
		const requestMsg = Empty.create(request);
		const result = await this.rpc.request(this.service, EchoerDefinition.methods.DoNothing.name, Empty.toBinary(requestMsg), abortSignal || void 0);
		return Empty.fromBinary(result);
	}
};
//#endregion
//#region node_modules/it-first/dist/src/index.js
/**
* @packageDocumentation
*
* Return the first value in an (async)iterable
*
* @example
*
* ```javascript
* import first from 'it-first'
*
* // This can also be an iterator, generator, etc
* const values = [0, 1, 2, 3, 4]
*
* const res = first(values)
*
* console.info(res) // 0
* ```
*
* Async sources must be awaited:
*
* ```javascript
* import first from 'it-first'
*
* const values = async function * () {
*   yield * [0, 1, 2, 3, 4]
* }
*
* const res = await first(values())
*
* console.info(res) // 0
* ```
*/
function isAsyncIterable(thing) {
	return thing[Symbol.asyncIterator] != null;
}
function first(source) {
	if (isAsyncIterable(source)) return (async () => {
		for await (const entry of source) return entry;
	})();
	for (const entry of source) return entry;
}
//#endregion
//#region node_modules/starpc/dist/echo/server.js
var EchoerServer = class {
	proxyServer;
	constructor(proxyServer) {
		this.proxyServer = proxyServer;
	}
	async Echo(request) {
		return request;
	}
	async *EchoServerStream(request) {
		for (let i = 0; i < 5; i++) {
			yield request;
			await new Promise((resolve) => setTimeout(resolve, 200));
		}
	}
	async EchoClientStream(request) {
		const message = await first(request);
		if (!message) throw new Error("received no messages");
		return message;
	}
	EchoBidiStream(request) {
		const result = messagePushable();
		result.push({ body: "hello from server" });
		writeToPushable(request, result);
		return result;
	}
	RpcStream(request) {
		return handleRpcStream(request[Symbol.asyncIterator](), async () => {
			if (!this.proxyServer) throw new Error("rpc stream proxy server not set");
			return this.proxyServer.rpcStreamHandler;
		});
	}
	async DoNothing() {
		return {};
	}
};
//#endregion
export { protoInt64 as _, openRpcStream as a, createHandler as c, buildDecodeMessageTransform as d, buildEncodeMessageTransform as f, ScalarType as g, createEnumType as h, handleRpcStream as i, Server as l, createMessageType as m, EchoerClient as n, ChannelStream as o, MethodKind as p, EchoerDefinition as r, createMux as s, EchoerServer as t, Client as u, pipe as v, castToError as y };

//# sourceMappingURL=dist-CmY9bC3s.js.map