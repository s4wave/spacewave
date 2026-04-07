import { t as pushable } from "./chunks/src-DnGF4VQE.js";
//#region web/bldr/sab-ring-stream.ts
var CTRL_WRITE_IDX = 0;
var CTRL_READ_IDX = 1;
var CTRL_STATE = 2;
var CTRL_INT32S = 4;
var CTRL_BYTES = CTRL_INT32S * 4;
var STATE_OPEN = 0;
var STATE_CLOSED = 1;
var DEFAULT_SLOT_SIZE = 8192;
var DEFAULT_NUM_SLOTS = 32;
function createSabPair(opts) {
	const slotSize = opts?.slotSize ?? DEFAULT_SLOT_SIZE;
	const size = CTRL_BYTES + (opts?.numSlots ?? DEFAULT_NUM_SLOTS) * slotSize;
	return {
		aSab: new SharedArrayBuffer(size),
		bSab: new SharedArrayBuffer(size)
	};
}
async function waitForChange(arr, index, expected) {
	const atomics = Atomics;
	if (typeof atomics.waitAsync === "function") {
		const result = atomics.waitAsync(arr, index, expected);
		if (result.async) await result.value;
		return;
	}
	while (Atomics.load(arr, index) === expected) await new Promise((r) => setTimeout(r, 1));
}
var SabRingStream = class {
	source;
	sink;
	_source;
	txCtrl;
	rxCtrl;
	txSab;
	rxSab;
	slotSize;
	numSlots;
	closed = false;
	constructor(txSab, rxSab, opts) {
		this.txSab = txSab;
		this.rxSab = rxSab;
		this.slotSize = opts?.slotSize ?? DEFAULT_SLOT_SIZE;
		this.numSlots = opts?.numSlots ?? DEFAULT_NUM_SLOTS;
		this.txCtrl = new Int32Array(txSab, 0, CTRL_INT32S);
		this.rxCtrl = new Int32Array(rxSab, 0, CTRL_INT32S);
		const source = pushable({ objectMode: true });
		this._source = source;
		this.source = source;
		this.sink = this._createSink();
		this._readLoop().catch((err) => {
			if (!this.closed) this._source.end(err instanceof Error ? err : new Error(String(err)));
		});
	}
	async _readLoop() {
		while (true) {
			const readIdx = Atomics.load(this.rxCtrl, CTRL_READ_IDX);
			const writeIdx = Atomics.load(this.rxCtrl, CTRL_WRITE_IDX);
			if (readIdx < writeIdx) {
				const slotOff = CTRL_BYTES + readIdx % this.numSlots * this.slotSize;
				const len = new DataView(this.rxSab, slotOff, 4).getUint32(0, true);
				const data = new Uint8Array(len);
				data.set(new Uint8Array(this.rxSab, slotOff + 4, len));
				Atomics.add(this.rxCtrl, CTRL_READ_IDX, 1);
				Atomics.notify(this.rxCtrl, CTRL_READ_IDX);
				this._source.push(data);
				continue;
			}
			if (this.closed) break;
			if (Atomics.load(this.rxCtrl, CTRL_STATE) !== STATE_OPEN) break;
			await waitForChange(this.rxCtrl, CTRL_WRITE_IDX, writeIdx);
		}
		if (!this.closed) this._source.end();
	}
	async _write(data) {
		const maxPayload = this.slotSize - 4;
		if (data.byteLength > maxPayload) throw new Error(`SabRingStream: message ${data.byteLength} bytes exceeds slot max ${maxPayload}`);
		while (!this.closed) {
			const writeIdx = Atomics.load(this.txCtrl, CTRL_WRITE_IDX);
			const readIdx = Atomics.load(this.txCtrl, CTRL_READ_IDX);
			if (writeIdx - readIdx < this.numSlots) break;
			await waitForChange(this.txCtrl, CTRL_READ_IDX, readIdx);
		}
		if (this.closed) return;
		const slotOff = CTRL_BYTES + Atomics.load(this.txCtrl, CTRL_WRITE_IDX) % this.numSlots * this.slotSize;
		new DataView(this.txSab, slotOff, 4).setUint32(0, data.byteLength, true);
		new Uint8Array(this.txSab, slotOff + 4, data.byteLength).set(data);
		Atomics.add(this.txCtrl, CTRL_WRITE_IDX, 1);
		Atomics.notify(this.txCtrl, CTRL_WRITE_IDX);
	}
	_closeTx() {
		Atomics.store(this.txCtrl, CTRL_STATE, STATE_CLOSED);
		Atomics.notify(this.txCtrl, CTRL_WRITE_IDX);
	}
	_createSink() {
		return async (source) => {
			try {
				for await (const msg of source) await this._write(msg);
			} catch (err) {
				this.close(err instanceof Error ? err : new Error(String(err)));
				return;
			}
			this._closeTx();
		};
	}
	close(error) {
		if (this.closed) return;
		this.closed = true;
		this._closeTx();
		this._source.end(error);
	}
};
//#endregion
//#region e2e/comms/fixtures/sab-ring.ts
async function collectN(source, n, timeoutMs) {
	const msgs = [];
	const deadline = Date.now() + timeoutMs;
	for await (const chunk of source) {
		msgs.push(new Uint8Array(chunk));
		if (msgs.length >= n) break;
		if (Date.now() > deadline) break;
	}
	return msgs;
}
async function run() {
	const log = document.getElementById("log");
	const errors = [];
	try {
		const opts = {
			slotSize: 256,
			numSlots: 32
		};
		let sendRecv = false;
		{
			const { aSab, bSab } = createSabPair(opts);
			const streamA = new SabRingStream(aSab, bSab, opts);
			const streamB = new SabRingStream(bSab, aSab, opts);
			const count = 10;
			const recvPromise = collectN(streamB.source, count, 5e3);
			for (let i = 0; i < count; i++) {
				const data = new Uint8Array([i]);
				await streamA.sink((async function* () {
					yield data;
				})());
			}
			const received = await recvPromise;
			if (received.length !== count) errors.push(`sendRecv: got ${received.length} msgs, want ${count}`);
			else {
				let ok = true;
				for (let i = 0; i < count; i++) if (received[i][0] !== i) {
					errors.push(`sendRecv: msg[${i}]=${received[i][0]}, want ${i}`);
					ok = false;
					break;
				}
				sendRecv = ok;
			}
			streamA.close();
			streamB.close();
		}
		let bidirectional = false;
		{
			const { aSab, bSab } = createSabPair(opts);
			const streamA = new SabRingStream(aSab, bSab, opts);
			const streamB = new SabRingStream(bSab, aSab, opts);
			const count = 5;
			const recvA = collectN(streamA.source, count, 5e3);
			const recvB = collectN(streamB.source, count, 5e3);
			for (let i = 0; i < count; i++) {
				await streamA.sink((async function* () {
					yield new Uint8Array([170, i]);
				})());
				await streamB.sink((async function* () {
					yield new Uint8Array([187, i]);
				})());
			}
			const msgsA = await recvA;
			const msgsB = await recvB;
			if (msgsA.length === count && msgsB.length === count) {
				let ok = true;
				for (let i = 0; i < count; i++) {
					if (msgsB[i][0] !== 170 || msgsB[i][1] !== i) {
						errors.push(`bidir: B got wrong msg at ${i}`);
						ok = false;
						break;
					}
					if (msgsA[i][0] !== 187 || msgsA[i][1] !== i) {
						errors.push(`bidir: A got wrong msg at ${i}`);
						ok = false;
						break;
					}
				}
				bidirectional = ok;
			} else errors.push(`bidir: A got ${msgsA.length}, B got ${msgsB.length}, want ${count}`);
			streamA.close();
			streamB.close();
		}
		let closeOk = false;
		{
			const { aSab, bSab } = createSabPair(opts);
			const streamA = new SabRingStream(aSab, bSab, opts);
			const streamB = new SabRingStream(bSab, aSab, opts);
			await streamA.sink((async function* () {
				yield new Uint8Array([42]);
			})());
			streamA.close();
			const msgs = [];
			const deadline = Date.now() + 3e3;
			for await (const chunk of streamB.source) {
				msgs.push(new Uint8Array(chunk));
				if (Date.now() > deadline) break;
			}
			if (msgs.length >= 1 && msgs[0][0] === 42) closeOk = true;
			else errors.push(`close: got ${msgs.length} msgs, first=${msgs[0]?.[0]}`);
			streamB.close();
		}
		const pass = sendRecv && bidirectional && closeOk && errors.length === 0;
		window.__results = {
			pass,
			detail: errors.length > 0 ? errors.join("; ") : "all tests passed",
			sendRecv,
			bidirectional,
			close: closeOk,
			messageCount: 10
		};
	} catch (err) {
		window.__results = {
			pass: false,
			detail: `error: ${err}`,
			sendRecv: false,
			bidirectional: false,
			close: false,
			messageCount: 0
		};
	}
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=sab-ring.js.map