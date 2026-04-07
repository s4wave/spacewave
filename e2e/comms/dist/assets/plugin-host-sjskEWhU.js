(function() {
	//#region web/bldr/sab-bus.ts
	const CTRL_WRITE_IDX = 0;
	const CTRL_STATE = 1;
	const CTRL_READER_COUNT = 2;
	const READER_IDX_START = 4;
	const MAX_READERS = 16;
	const CTRL_INT32S = READER_IDX_START + MAX_READERS;
	const CTRL_BYTES = CTRL_INT32S * 4;
	const MSG_HEADER = 8;
	const STATE_OPEN = 0;
	const STATE_CLOSED = 1;
	const DEFAULT_SLOT_SIZE = 8192;
	const DEFAULT_NUM_SLOTS = 64;
	var SabBusEndpoint = class {
		ctrl;
		sab;
		slotSize;
		numSlots;
		readerSlot = -1;
		pluginId;
		closed = false;
		constructor(sab, pluginId, opts) {
			this.sab = sab;
			this.pluginId = pluginId;
			this.slotSize = opts?.slotSize ?? DEFAULT_SLOT_SIZE;
			this.numSlots = opts?.numSlots ?? DEFAULT_NUM_SLOTS;
			this.ctrl = new Int32Array(sab, 0, CTRL_INT32S);
		}
		register() {
			const slot = Atomics.add(this.ctrl, CTRL_READER_COUNT, 1);
			if (slot >= MAX_READERS) throw new Error(`SabBus: max readers (${MAX_READERS}) exceeded`);
			this.readerSlot = slot;
			Atomics.store(this.ctrl, READER_IDX_START + slot, Atomics.load(this.ctrl, CTRL_WRITE_IDX));
		}
		async write(targetId, data) {
			const maxPayload = this.slotSize - MSG_HEADER;
			if (data.byteLength > maxPayload) throw new Error(`SabBus: message ${data.byteLength} bytes exceeds max ${maxPayload}`);
			let claimedIdx;
			while (!this.closed) {
				const writeIdx = Atomics.load(this.ctrl, CTRL_WRITE_IDX);
				const readerCount = Atomics.load(this.ctrl, CTRL_READER_COUNT);
				let minRead = writeIdx;
				for (let i = 0; i < readerCount; i++) {
					const r = Atomics.load(this.ctrl, READER_IDX_START + i);
					if (r < minRead) minRead = r;
				}
				if (writeIdx - minRead >= this.numSlots) {
					await new Promise((r) => setTimeout(r, 1));
					continue;
				}
				if (Atomics.compareExchange(this.ctrl, CTRL_WRITE_IDX, writeIdx, writeIdx + 1) === writeIdx) {
					claimedIdx = writeIdx;
					break;
				}
			}
			if (this.closed) return;
			const slotOff = CTRL_BYTES + claimedIdx % this.numSlots * this.slotSize;
			const hdr = new DataView(this.sab, slotOff, MSG_HEADER);
			hdr.setUint16(0, targetId, true);
			hdr.setUint16(2, this.pluginId, true);
			hdr.setUint32(4, data.byteLength, true);
			new Uint8Array(this.sab, slotOff + MSG_HEADER, data.byteLength).set(data);
			Atomics.notify(this.ctrl, CTRL_WRITE_IDX);
		}
		async read() {
			if (this.readerSlot < 0) throw new Error("SabBus: not registered, call register() first");
			const readerIdx = READER_IDX_START + this.readerSlot;
			while (!this.closed) {
				const readPos = Atomics.load(this.ctrl, readerIdx);
				const writePos = Atomics.load(this.ctrl, CTRL_WRITE_IDX);
				if (readPos < writePos) {
					const slotOff = CTRL_BYTES + readPos % this.numSlots * this.slotSize;
					const hdr = new DataView(this.sab, slotOff, MSG_HEADER);
					const targetId = hdr.getUint16(0, true);
					const sourceId = hdr.getUint16(2, true);
					const length = hdr.getUint32(4, true);
					Atomics.add(this.ctrl, readerIdx, 1);
					Atomics.notify(this.ctrl, readerIdx);
					if (targetId === this.pluginId || targetId === 65535) {
						const data = new Uint8Array(length);
						data.set(new Uint8Array(this.sab, slotOff + MSG_HEADER, length));
						return {
							targetId,
							sourceId,
							data
						};
					}
					continue;
				}
				if (Atomics.load(this.ctrl, CTRL_STATE) !== STATE_OPEN) return null;
				const atomics = Atomics;
				if (typeof atomics.waitAsync === "function") {
					const result = atomics.waitAsync(this.ctrl, CTRL_WRITE_IDX, writePos);
					if (result.async) await result.value;
				} else await new Promise((r) => setTimeout(r, 1));
			}
			return null;
		}
		close() {
			this.closed = true;
		}
		closeAll() {
			Atomics.store(this.ctrl, CTRL_STATE, STATE_CLOSED);
			Atomics.notify(this.ctrl, CTRL_WRITE_IDX);
			this.closed = true;
		}
	};
	//#endregion
	//#region e2e/comms/fixtures/workers/plugin-host.ts
	const ac = new AbortController();
	self.onmessage = async (ev) => {
		const { busSab, busPluginId, scriptUrl, workerCommsDetect } = ev.data;
		const endpoint = new SabBusEndpoint(busSab, busPluginId, {
			slotSize: 256,
			numSlots: 32
		});
		endpoint.register();
		self.postMessage({
			type: "registered",
			busPluginId
		});
		if (workerCommsDetect) self.postMessage({
			type: "config-received",
			config: workerCommsDetect.config
		});
		const pluginModule = await import(
			/* @vite-ignore */
			scriptUrl
);
		if (typeof pluginModule.default !== "function") {
			self.postMessage({
				type: "error",
				detail: "plugin script has no default export function"
			});
			return;
		}
		pluginModule.default(endpoint, ac.signal);
	};
	//#endregion
})();

//# sourceMappingURL=plugin-host-sjskEWhU.js.map