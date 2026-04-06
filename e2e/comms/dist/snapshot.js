import { t as createSnapshotManager } from "./chunks/snapshot-manager-B5wOl93s.js";
//#region e2e/comms/fixtures/snapshot.ts
var PLUGIN_ID = "test-plugin-snapshot";
var PATTERN = [
	202,
	254,
	186,
	190
];
async function run() {
	const log = document.getElementById("log");
	const errors = [];
	try {
		const mgr = await createSnapshotManager();
		let snapshotRestore = false;
		{
			const memory = new WebAssembly.Memory({ initial: 1 });
			const view = new Uint8Array(memory.buffer);
			view[0] = PATTERN[0];
			view[1] = PATTERN[1];
			view[2] = PATTERN[2];
			view[3] = PATTERN[3];
			mgr.register(PLUGIN_ID, memory);
			mgr.markDirty(PLUGIN_ID);
			if (!await mgr.snapshot(PLUGIN_ID)) errors.push("snapshot: snapshot() returned false");
			view.fill(0);
			if (view[0] !== 0) errors.push("snapshot: memory not cleared");
			const restored = await mgr.restore(PLUGIN_ID);
			if (!restored) errors.push("snapshot: restore returned null");
			else {
				const restoredView = new Uint8Array(restored);
				if (restoredView[0] === PATTERN[0] && restoredView[1] === PATTERN[1] && restoredView[2] === PATTERN[2] && restoredView[3] === PATTERN[3]) snapshotRestore = true;
				else errors.push(`snapshot: pattern mismatch: ${restoredView[0]},${restoredView[1]},${restoredView[2]},${restoredView[3]}`);
			}
		}
		let dirtyTracking = false;
		if (mgr.isDirty(PLUGIN_ID)) errors.push("dirty: still dirty after snapshot");
		else {
			mgr.markDirty(PLUGIN_ID);
			if (!mgr.isDirty(PLUGIN_ID)) errors.push("dirty: not dirty after markDirty");
			else {
				await mgr.snapshot(PLUGIN_ID);
				if (mgr.isDirty(PLUGIN_ID)) errors.push("dirty: still dirty after second snapshot");
				else dirtyTracking = true;
			}
		}
		let listSnapshots = false;
		{
			const ids = await mgr.listSnapshots();
			if (ids.includes(PLUGIN_ID)) listSnapshots = true;
			else errors.push(`list: ${PLUGIN_ID} not in ${JSON.stringify(ids)}`);
		}
		await mgr.deleteSnapshot(PLUGIN_ID);
		mgr.unregister(PLUGIN_ID);
		const pass = snapshotRestore && dirtyTracking && listSnapshots && errors.length === 0;
		window.__results = {
			pass,
			detail: errors.length > 0 ? errors.join("; ") : "all tests passed",
			snapshotRestore,
			dirtyTracking,
			listSnapshots
		};
	} catch (err) {
		window.__results = {
			pass: false,
			detail: `error: ${err}`,
			snapshotRestore: false,
			dirtyTracking: false,
			listSnapshots: false
		};
	}
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=snapshot.js.map