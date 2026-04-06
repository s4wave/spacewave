//#region web/bldr/snapshot-manager.ts
var OPFS_SNAPSHOT_DIR = ".bldr-snapshots";
var IDB_DB_NAME = "bldr-snapshots";
var IDB_STORE_NAME = "snapshots";
var DEFAULT_SNAPSHOT_INTERVAL_MS = 3e4;
var OpfsSnapshotStorage = class {
	dirHandle = null;
	async init() {
		this.dirHandle = await (await navigator.storage.getDirectory()).getDirectoryHandle(OPFS_SNAPSHOT_DIR, { create: true });
	}
	async write(pluginId, data) {
		if (!this.dirHandle) throw new Error("OpfsSnapshotStorage: not initialized");
		const writable = await (await this.dirHandle.getFileHandle(pluginId, { create: true })).createWritable();
		await writable.write(data);
		await writable.close();
	}
	async read(pluginId) {
		if (!this.dirHandle) throw new Error("OpfsSnapshotStorage: not initialized");
		try {
			const file = await (await this.dirHandle.getFileHandle(pluginId, { create: false })).getFile();
			if (file.size === 0) return null;
			return file.arrayBuffer();
		} catch (err) {
			if (err instanceof DOMException && err.name === "NotFoundError") return null;
			throw err;
		}
	}
	async delete(pluginId) {
		if (!this.dirHandle) throw new Error("OpfsSnapshotStorage: not initialized");
		try {
			await this.dirHandle.removeEntry(pluginId);
		} catch (err) {
			if (err instanceof DOMException && err.name === "NotFoundError") return;
			throw err;
		}
	}
	async list() {
		if (!this.dirHandle) throw new Error("OpfsSnapshotStorage: not initialized");
		const ids = [];
		for await (const key of this.dirHandle.keys()) ids.push(key);
		return ids;
	}
};
var IdbSnapshotStorage = class {
	db = null;
	async init() {
		this.db = await new Promise((resolve, reject) => {
			const req = indexedDB.open(IDB_DB_NAME, 1);
			req.onupgradeneeded = () => {
				req.result.createObjectStore(IDB_STORE_NAME);
			};
			req.onsuccess = () => resolve(req.result);
			req.onerror = () => reject(req.error);
		});
	}
	async write(pluginId, data) {
		if (!this.db) throw new Error("IdbSnapshotStorage: not initialized");
		const store = this.db.transaction(IDB_STORE_NAME, "readwrite").objectStore(IDB_STORE_NAME);
		return new Promise((resolve, reject) => {
			const req = store.put(data, pluginId);
			req.onsuccess = () => resolve();
			req.onerror = () => reject(req.error);
		});
	}
	async read(pluginId) {
		if (!this.db) throw new Error("IdbSnapshotStorage: not initialized");
		const store = this.db.transaction(IDB_STORE_NAME, "readonly").objectStore(IDB_STORE_NAME);
		return new Promise((resolve, reject) => {
			const req = store.get(pluginId);
			req.onsuccess = () => resolve(req.result ?? null);
			req.onerror = () => reject(req.error);
		});
	}
	async delete(pluginId) {
		if (!this.db) throw new Error("IdbSnapshotStorage: not initialized");
		const store = this.db.transaction(IDB_STORE_NAME, "readwrite").objectStore(IDB_STORE_NAME);
		return new Promise((resolve, reject) => {
			const req = store.delete(pluginId);
			req.onsuccess = () => resolve();
			req.onerror = () => reject(req.error);
		});
	}
	async list() {
		if (!this.db) throw new Error("IdbSnapshotStorage: not initialized");
		const store = this.db.transaction(IDB_STORE_NAME, "readonly").objectStore(IDB_STORE_NAME);
		return new Promise((resolve, reject) => {
			const req = store.getAllKeys();
			req.onsuccess = () => resolve(req.result ?? []);
			req.onerror = () => reject(req.error);
		});
	}
};
var SnapshotManager = class {
	storage;
	plugins = /* @__PURE__ */ new Map();
	initialized = false;
	periodicTimer = null;
	snapshotIntervalMs = DEFAULT_SNAPSHOT_INTERVAL_MS;
	constructor(useIdb) {
		this.storage = useIdb ? new IdbSnapshotStorage() : new OpfsSnapshotStorage();
	}
	async init() {
		await this.storage.init();
		this.initialized = true;
	}
	register(pluginId, memory) {
		this.plugins.set(pluginId, {
			pluginId,
			memory,
			generation: 0,
			lastSnapshotGeneration: -1,
			lastSnapshotSize: 0
		});
	}
	unregister(pluginId) {
		this.plugins.delete(pluginId);
		if (this.plugins.size === 0) this.stopPeriodic();
	}
	markDirty(pluginId) {
		const entry = this.plugins.get(pluginId);
		if (entry) entry.generation++;
	}
	isDirty(pluginId) {
		const entry = this.plugins.get(pluginId);
		if (!entry) return false;
		return entry.generation !== entry.lastSnapshotGeneration || entry.memory.buffer.byteLength !== entry.lastSnapshotSize;
	}
	async snapshot(pluginId, force) {
		if (!this.initialized) throw new Error("SnapshotManager: not initialized");
		const entry = this.plugins.get(pluginId);
		if (!entry) throw new Error(`SnapshotManager: plugin ${pluginId} not registered`);
		if (!force && !this.isDirty(pluginId)) return false;
		const copy = entry.memory.buffer.slice(0);
		await this.storage.write(pluginId, copy);
		entry.lastSnapshotGeneration = entry.generation;
		entry.lastSnapshotSize = copy.byteLength;
		return true;
	}
	async snapshotAll(force) {
		const promises = [];
		for (const pluginId of this.plugins.keys()) promises.push(this.snapshot(pluginId, force));
		return (await Promise.all(promises)).filter(Boolean).length;
	}
	startPeriodic(intervalMs) {
		this.stopPeriodic();
		this.snapshotIntervalMs = intervalMs ?? 3e4;
		this.periodicTimer = setInterval(() => {
			this.snapshotAll().catch((err) => {
				console.warn("SnapshotManager: periodic snapshot failed:", err);
			});
		}, this.snapshotIntervalMs);
		console.log("SnapshotManager: periodic snapshots started, interval:", this.snapshotIntervalMs, "ms");
	}
	stopPeriodic() {
		if (this.periodicTimer != null) {
			clearInterval(this.periodicTimer);
			this.periodicTimer = null;
		}
	}
	async restore(pluginId) {
		if (!this.initialized) throw new Error("SnapshotManager: not initialized");
		return this.storage.read(pluginId);
	}
	async deleteSnapshot(pluginId) {
		if (!this.initialized) throw new Error("SnapshotManager: not initialized");
		return this.storage.delete(pluginId);
	}
	async listSnapshots() {
		if (!this.initialized) throw new Error("SnapshotManager: not initialized");
		return this.storage.list();
	}
};
async function createSnapshotManager() {
	let useIdb = false;
	try {
		await navigator.storage.getDirectory();
	} catch {
		useIdb = true;
	}
	const mgr = new SnapshotManager(useIdb);
	await mgr.init();
	return mgr;
}
//#endregion
export { createSnapshotManager as t };

//# sourceMappingURL=snapshot-manager-B5wOl93s.js.map