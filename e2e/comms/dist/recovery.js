import { t as createSnapshotManager } from "./chunks/snapshot-manager-B5wOl93s.js";
//#region web/bldr/snapshot-recovery.ts
var PLUGIN_LOCK_PREFIX = "bldr-plugin-";
function acquirePluginLock(pluginId) {
	if (typeof navigator === "undefined" || !navigator.locks) return Promise.resolve(() => {});
	const lockName = PLUGIN_LOCK_PREFIX + pluginId;
	let releaseFn = () => {};
	return new Promise((resolveOuter) => {
		navigator.locks.request(lockName, { mode: "exclusive" }, () => {
			return new Promise((resolveHold) => {
				releaseFn = resolveHold;
				resolveOuter(releaseFn);
			});
		}).catch(() => {
			resolveOuter(() => {});
		});
	});
}
async function isPluginLockHeld(pluginId) {
	if (typeof navigator === "undefined" || !navigator.locks) return false;
	const lockName = PLUGIN_LOCK_PREFIX + pluginId;
	return ((await navigator.locks.query()).held ?? []).some((lock) => lock.name === lockName);
}
async function findOrphanedSnapshots(mgr) {
	const snapshots = await mgr.listSnapshots();
	const orphaned = [];
	for (const pluginId of snapshots) if (!await isPluginLockHeld(pluginId)) orphaned.push(pluginId);
	return orphaned;
}
async function recoverOrphanedPlugins(opts) {
	let mgr;
	try {
		mgr = await createSnapshotManager();
	} catch (err) {
		console.warn("snapshot-recovery: unable to init snapshot manager:", err);
		return 0;
	}
	const orphaned = await findOrphanedSnapshots(mgr);
	if (orphaned.length === 0) return 0;
	console.log("snapshot-recovery: found", orphaned.length, "orphaned plugins:", orphaned);
	let recovered = 0;
	for (const pluginId of orphaned) try {
		const snapshot = await mgr.restore(pluginId);
		if (!snapshot) {
			console.warn("snapshot-recovery: empty snapshot for", pluginId);
			await mgr.deleteSnapshot(pluginId);
			continue;
		}
		await opts.restorePlugin(pluginId, snapshot);
		await mgr.deleteSnapshot(pluginId);
		recovered++;
		console.log("snapshot-recovery: restored plugin", pluginId);
	} catch (err) {
		console.error("snapshot-recovery: failed to restore", pluginId, err);
		await mgr.deleteSnapshot(pluginId).catch(() => {});
	}
	return recovered;
}
//#endregion
//#region e2e/comms/fixtures/recovery.ts
var PLUGIN_ID = "test-recovery-plugin";
var PATTERN = [
	222,
	173,
	192,
	222
];
async function runSetup() {
	const mgr = await createSnapshotManager();
	const memory = new WebAssembly.Memory({ initial: 1 });
	const view = new Uint8Array(memory.buffer);
	view[0] = PATTERN[0];
	view[1] = PATTERN[1];
	view[2] = PATTERN[2];
	view[3] = PATTERN[3];
	mgr.register(PLUGIN_ID, memory);
	mgr.markDirty(PLUGIN_ID);
	await mgr.snapshot(PLUGIN_ID);
	await acquirePluginLock(PLUGIN_ID);
	window.__results = {
		pass: true,
		detail: "setup: lock acquired and snapshot written",
		lockAcquired: true,
		snapshotWritten: true
	};
}
async function runRecover() {
	const orphans = await findOrphanedSnapshots(await createSnapshotManager());
	const orphanDetected = orphans.includes(PLUGIN_ID);
	if (!orphanDetected) {
		window.__results = {
			pass: false,
			detail: `recover: orphan not found, got: ${JSON.stringify(orphans)}`,
			orphanDetected: false,
			recovered: false
		};
		return;
	}
	let recoveredData = null;
	const count = await recoverOrphanedPlugins({ restorePlugin: async (_pluginId, snapshot) => {
		recoveredData = snapshot;
	} });
	let recovered = false;
	if (count === 1 && recoveredData) {
		const view = new Uint8Array(recoveredData);
		recovered = view[0] === PATTERN[0] && view[1] === PATTERN[1] && view[2] === PATTERN[2] && view[3] === PATTERN[3];
	}
	window.__results = {
		pass: orphanDetected && recovered,
		detail: orphanDetected && recovered ? "recover: orphan detected and snapshot restored" : `recover: orphan=${orphanDetected} recovered=${recovered} count=${count}`,
		orphanDetected,
		recovered
	};
}
async function run() {
	const log = document.getElementById("log");
	const mode = new URLSearchParams(location.search).get("mode") || "setup";
	try {
		if (mode === "recover") await runRecover();
		else await runSetup();
	} catch (err) {
		window.__results = {
			pass: false,
			detail: `error: ${err}`
		};
	}
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=recovery.js.map