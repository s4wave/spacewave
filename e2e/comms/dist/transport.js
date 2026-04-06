import { t as pushable } from "./chunks/src-BuuWItrw.js";
import { i as createBusSab, n as SabBusEndpoint, r as SabBusStream } from "./chunks/sab-bus-D8XB9B_y.js";
import { n as detectWorkerCommsConfig } from "./chunks/worker-comms-detect-DlRfFzjf.js";
import { a as COMMS_BROADCAST_CHANNEL, i as initCommsSchema, n as CommsReader, r as CommsWriter } from "./chunks/comms-table-yK5IO-r0.js";
//#region web/bldr/comms-stream.ts
var SqliteCommsStream = class {
	source;
	sink;
	_source;
	writer;
	reader;
	asyncDb;
	sourcePluginId;
	targetPluginId;
	channel;
	closed = false;
	constructor(opts) {
		this.sourcePluginId = opts.sourcePluginId;
		this.targetPluginId = opts.targetPluginId;
		this.asyncDb = opts.asyncDb;
		this.reader = new CommsReader();
		if (opts.writeDb) {
			initCommsSchema(opts.writeDb);
			this.writer = new CommsWriter(opts.writeDb);
		} else this.writer = null;
		const source = pushable({ objectMode: true });
		this._source = source;
		this.source = source;
		this.sink = this._createSink();
		this.channel = new BroadcastChannel(COMMS_BROADCAST_CHANNEL);
		this.channel.onmessage = (ev) => {
			if (this.closed) return;
			if (ev.data?.table === "messages") this._handleNotification();
		};
	}
	_handleNotification() {
		this.asyncDb.refresh().then(() => {
			const db = this.asyncDb.getDb();
			if (!db || this.closed) return;
			const messages = this.reader.readNew(db, this.sourcePluginId);
			for (const msg of messages) if (msg.sourcePluginId === this.targetPluginId) this._source.push(msg.payload);
		}).catch((err) => {
			if (!this.closed) console.warn("comms-stream: refresh failed:", err);
		});
	}
	_createSink() {
		return async (source) => {
			try {
				for await (const msg of source) {
					if (!this.writer) throw new Error("comms-stream: no writable database");
					this.writer.write(this.sourcePluginId, this.targetPluginId, msg instanceof Uint8Array ? msg : new Uint8Array(msg));
				}
			} catch (err) {
				this.close(err instanceof Error ? err : new Error(String(err)));
			}
		};
	}
	close(error) {
		if (this.closed) return;
		this.closed = true;
		if (this.channel) {
			this.channel.close();
			this.channel = null;
		}
		if (this.writer) this.writer.close();
		this._source.end(error);
	}
};
//#endregion
//#region web/bldr/plugin-transport.ts
function createTransportFactory(detect, opts) {
	const factory = {
		openStream: opts.openStream,
		handleIncomingStream: opts.handleIncomingStream,
		config: detect.config
	};
	if (opts.busEndpoint) {
		factory.busEndpoint = opts.busEndpoint;
		factory.openBusStream = async (targetPluginId) => {
			return new SabBusStream(opts.busEndpoint, targetPluginId);
		};
		console.log("worker-comms: SAB bus transport available for intra-tab IPC");
	}
	if (opts.commsSqlite && opts.pluginId != null) {
		factory.commsSqlite = opts.commsSqlite;
		factory.openCrossTabStream = async (targetPluginId) => {
			return new SqliteCommsStream({
				writeDb: null,
				asyncDb: opts.commsSqlite.getDb(),
				sourcePluginId: opts.pluginId,
				targetPluginId
			});
		};
		console.log("worker-comms: sqlite cross-tab transport available");
	}
	return factory;
}
//#endregion
//#region e2e/comms/fixtures/transport.ts
async function run() {
	const log = document.getElementById("log");
	const errors = [];
	try {
		const detect = await detectWorkerCommsConfig();
		const config = detect.config;
		const noopOpen = async () => {
			throw new Error("not implemented");
		};
		const noopHandle = async () => {};
		let factory;
		let hasBusStream = false;
		if (config === "B" || config === "C") {
			const busOpts = {
				slotSize: 256,
				numSlots: 16
			};
			const endpoint = new SabBusEndpoint(createBusSab(busOpts), 0, busOpts);
			endpoint.register();
			factory = createTransportFactory(detect, {
				openStream: noopOpen,
				handleIncomingStream: noopHandle,
				busEndpoint: endpoint,
				pluginId: 0
			});
			hasBusStream = factory.openBusStream != null;
			endpoint.close();
		} else {
			factory = createTransportFactory(detect, {
				openStream: noopOpen,
				handleIncomingStream: noopHandle
			});
			hasBusStream = factory.openBusStream != null;
		}
		const factoryCreated = factory.config === config;
		if (config === "B" || config === "C") {
			if (!hasBusStream) errors.push("expected openBusStream on config " + config);
		} else if (hasBusStream) errors.push("unexpected openBusStream on config " + config);
		if (!factoryCreated) errors.push(`factory config mismatch: ${factory.config} vs ${config}`);
		const pass = errors.length === 0 && factoryCreated;
		window.__results = {
			pass,
			detail: errors.length > 0 ? errors.join("; ") : "all tests passed",
			config,
			hasBusStream,
			factoryCreated
		};
	} catch (err) {
		window.__results = {
			pass: false,
			detail: `error: ${err}`,
			config: "",
			hasBusStream: false,
			factoryCreated: false
		};
	}
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=transport.js.map