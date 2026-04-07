//#region e2e/comms/fixtures/opfs-volume.ts
function hexEncode(data) {
	return Array.from(data).map((b) => b.toString(16).padStart(2, "0")).join("");
}
function arraysEqual(a, b) {
	if (a.length !== b.length) return false;
	for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
	return true;
}
async function run() {
	const log = document.getElementById("log");
	const errors = [];
	const results = {
		pass: false,
		detail: "",
		createVolume: false,
		writeEntries: false,
		readEntries: false,
		persistence: false,
		deleteVolume: false,
		webLockIsolation: false
	};
	try {
		const opfsRoot = await navigator.storage.getDirectory();
		const volId = `vol-test-${Date.now()}`;
		const volDir = await opfsRoot.getDirectoryHandle(volId, { create: true });
		results.createVolume = true;
		const entries = [
			{
				key: new Uint8Array([1, 35]),
				value: new Uint8Array([
					65,
					66,
					67
				])
			},
			{
				key: new Uint8Array([1, 69]),
				value: new Uint8Array([68, 69])
			},
			{
				key: new Uint8Array([171, 205]),
				value: new Uint8Array([
					70,
					71,
					72,
					73
				])
			},
			{
				key: new Uint8Array([171, 239]),
				value: new Uint8Array([80])
			},
			{
				key: new Uint8Array([255, 1]),
				value: new Uint8Array([
					81,
					82,
					83,
					84,
					85
				])
			}
		];
		await navigator.locks.request(`${volId}|kvtx`, { mode: "exclusive" }, async () => {
			for (const { key, value } of entries) {
				const hex = hexEncode(key);
				const shard = hex.substring(0, 2);
				const w = await (await (await volDir.getDirectoryHandle(shard, { create: true })).getFileHandle(hex, { create: true })).createWritable();
				await w.write(value);
				await w.close();
			}
		});
		results.writeEntries = true;
		let readOk = true;
		await navigator.locks.request(`${volId}|kvtx`, { mode: "shared" }, async () => {
			for (const { key, value } of entries) {
				const hex = hexEncode(key);
				const shard = hex.substring(0, 2);
				const ab = await (await (await (await volDir.getDirectoryHandle(shard, { create: false })).getFileHandle(hex)).getFile()).arrayBuffer();
				if (!arraysEqual(new Uint8Array(ab), value)) {
					errors.push(`read mismatch for key ${hex}`);
					readOk = false;
				}
			}
		});
		results.readEntries = readOk;
		const volDir2 = await opfsRoot.getDirectoryHandle(volId, { create: false });
		let persistOk = true;
		for (const { key, value } of entries) {
			const hex = hexEncode(key);
			const shard = hex.substring(0, 2);
			try {
				const ab = await (await (await (await volDir2.getDirectoryHandle(shard, { create: false })).getFileHandle(hex)).getFile()).arrayBuffer();
				if (!arraysEqual(new Uint8Array(ab), value)) {
					errors.push(`persistence mismatch for key ${hex}`);
					persistOk = false;
				}
			} catch (e) {
				errors.push(`persistence error for key ${hex}: ${e.message}`);
				persistOk = false;
			}
		}
		results.persistence = persistOk;
		let lockOk = true;
		const sharedResults = [];
		await Promise.all([navigator.locks.request(`${volId}|lock-test`, { mode: "shared" }, async () => {
			sharedResults.push(true);
			await new Promise((r) => setTimeout(r, 50));
		}), navigator.locks.request(`${volId}|lock-test`, { mode: "shared" }, async () => {
			sharedResults.push(true);
			await new Promise((r) => setTimeout(r, 50));
		})]);
		if (sharedResults.length !== 2) {
			errors.push("expected 2 shared locks to acquire concurrently");
			lockOk = false;
		}
		let exclusiveHeld = false;
		let sharedAfterExclusive = false;
		const exclusiveDone = navigator.locks.request(`${volId}|lock-test2`, { mode: "exclusive" }, async () => {
			exclusiveHeld = true;
			await new Promise((r) => setTimeout(r, 100));
			exclusiveHeld = false;
		});
		await new Promise((r) => setTimeout(r, 20));
		await Promise.all([exclusiveDone, navigator.locks.request(`${volId}|lock-test2`, { mode: "shared" }, async () => {
			sharedAfterExclusive = !exclusiveHeld;
		})]);
		if (!sharedAfterExclusive) {
			errors.push("shared lock acquired while exclusive was held");
			lockOk = false;
		}
		results.webLockIsolation = lockOk;
		await opfsRoot.removeEntry(volId, { recursive: true });
		let deleteOk = false;
		try {
			await opfsRoot.getDirectoryHandle(volId, { create: false });
		} catch (e) {
			if (e.name === "NotFoundError") deleteOk = true;
		}
		results.deleteVolume = deleteOk;
		results.pass = errors.length === 0;
		results.detail = errors.length > 0 ? errors.join("; ") : "all volume integration tests passed";
	} catch (err) {
		results.pass = false;
		results.detail = `error: ${err}`;
	}
	window.__results = results;
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=opfs-volume.js.map