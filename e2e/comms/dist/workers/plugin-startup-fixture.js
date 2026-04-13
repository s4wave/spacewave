import { a as WebRuntimeClientType, i as buildWebWorkerLockName, o as PluginStartInfo, t as WebDocumentTracker } from "../chunks/web-document-tracker-BxuL8ox8.js";
//#region web/runtime/plugin-worker.ts
function checkSharedWorker(scope) {
	return typeof SharedWorkerGlobalScope !== "undefined" && scope instanceof SharedWorkerGlobalScope;
}
var PluginWorker = class {
	webDocumentTracker;
	get isSharedWorker() {
		return checkSharedWorker(this.global);
	}
	get workerId() {
		return this.global.name;
	}
	get webRuntimeClient() {
		return this.webDocumentTracker.webRuntimeClient;
	}
	get started() {
		return this.pluginStarted ?? false;
	}
	pluginStarted;
	startPluginPromise;
	lockAbortController;
	onSnapshotNow;
	constructor(global, startPlugin, handleIncomingStream) {
		this.global = global;
		this.startPlugin = startPlugin;
		this.webDocumentTracker = new WebDocumentTracker(this.workerId, WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER, this.onWebDocumentsExhausted.bind(this), handleIncomingStream);
		this.armWorkerLock();
		if (checkSharedWorker(global)) global.addEventListener("connect", (ev) => {
			const ports = ev.ports;
			if (!ports || !ports.length) return;
			const port = ev.ports[0];
			if (!port) return;
			port.onmessage = this.handleWorkerMessage.bind(this);
			port.start();
		});
		else global.addEventListener("message", this.handleWorkerMessage.bind(this));
	}
	async onWebDocumentsExhausted() {
		console.log(`PluginWorker: ${this.workerId}: no WebDocument available, exiting!`);
		this.shutdown();
	}
	armWorkerLock() {
		if (typeof navigator === "undefined" || !("locks" in navigator) || this.lockAbortController) return;
		this.lockAbortController = new AbortController();
		navigator.locks.request(buildWebWorkerLockName(this.workerId), { signal: this.lockAbortController.signal }, () => {
			return new Promise(() => {});
		}).catch((err) => {
			if (isAbortError(err)) return;
			console.warn(`PluginWorker: ${this.workerId}: worker liveness lock failed`, err);
		});
	}
	shutdown() {
		this.lockAbortController?.abort();
		this.lockAbortController = void 0;
		this.webDocumentTracker.close();
		this.global.close();
	}
	async handleStartPlugin(startInfoBin, busSab, busPluginId, workerCommsDetect) {
		if (this.startPluginPromise) {
			await this.startPluginPromise;
			this.notifyReady();
			return;
		}
		this.startPluginPromise = this.startPluginImpl(startInfoBin, busSab, busPluginId, workerCommsDetect).catch((err) => {
			this.startPluginPromise = void 0;
			throw err;
		});
		await this.startPluginPromise;
		this.notifyReady();
	}
	async startPluginImpl(startInfoBin, busSab, busPluginId, workerCommsDetect) {
		const startInfoJsonB64 = new TextDecoder().decode(startInfoBin);
		const startInfoJson = atob(startInfoJsonB64);
		const startInfo = PluginStartInfo.fromJsonString(startInfoJson);
		await this.webDocumentTracker.waitConn();
		await this.startPlugin({
			startInfo,
			busSab,
			busPluginId,
			workerCommsDetect
		});
		this.pluginStarted = true;
	}
	notifyReady() {
		const msg = {
			from: this.workerId,
			ready: true
		};
		this.webDocumentTracker.postMessage(msg);
	}
	handleWorkerMessage(msgEvent) {
		const data = msgEvent.data;
		this.webDocumentTracker.handleWebDocumentMessage(data);
		if (data.snapshotNow && this.onSnapshotNow) {
			console.log(`PluginWorker: ${this.workerId}: received snapshotNow`);
			this.onSnapshotNow();
			return;
		}
		if (data.initData) this.handleStartPlugin(data.initData, data.busSab, data.busPluginId, data.workerCommsDetect).catch((err) => {
			console.warn(`PluginWorker: ${this.workerId}: startup failed, exiting!`, err);
			this.shutdown();
		});
	}
};
function isAbortError(err) {
	return typeof err === "object" && err !== null && "name" in err && err.name === "AbortError";
}
//#endregion
//#region e2e/comms/fixtures/workers/plugin-startup-fixture.ts
function readMode() {
	return new URL(self.location.href).searchParams.get("mode") ?? "import-fail";
}
new PluginWorker(self, async () => {
	const mode = readMode();
	if (mode === "idle") return;
	if (mode === "import-fail") {
		await import("/workers/does-not-exist.js");
		return;
	}
	throw new Error(`unknown startup fixture mode: ${mode}`);
}, null);
self.postMessage({ type: "booted" });
//#endregion

//# sourceMappingURL=plugin-startup-fixture.js.map