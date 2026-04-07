//#region e2e/comms/fixtures/opfs-kvtx.ts
function hexEncode(data) {
	return Array.from(data).map((b) => b.toString(16).padStart(2, "0")).join("");
}
function shardPrefix(encoded) {
	if (encoded.length < 2) return "00";
	return encoded.substring(0, 2);
}
async function writeEntry(root, key, value) {
	const encoded = hexEncode(key);
	const shard = shardPrefix(encoded);
	const w = await (await (await root.getDirectoryHandle(shard, { create: true })).getFileHandle(encoded, { create: true })).createWritable();
	await w.write(value);
	await w.close();
}
async function readEntry(root, key) {
	const encoded = hexEncode(key);
	const shard = shardPrefix(encoded);
	try {
		const ab = await (await (await (await root.getDirectoryHandle(shard, { create: false })).getFileHandle(encoded)).getFile()).arrayBuffer();
		return new Uint8Array(ab);
	} catch (e) {
		if (e.name === "NotFoundError") return null;
		throw e;
	}
}
async function deleteEntry(root, key) {
	const encoded = hexEncode(key);
	const shard = shardPrefix(encoded);
	try {
		await (await root.getDirectoryHandle(shard, { create: false })).removeEntry(encoded);
	} catch (e) {
		if (e.name === "NotFoundError") return;
		throw e;
	}
}
async function listAllKeys(root) {
	const keys = [];
	for await (const [name, handle] of root.entries()) {
		if (handle.kind !== "directory" || name.length !== 2) continue;
		const shardDir = await root.getDirectoryHandle(name);
		for await (const [fname] of shardDir.entries()) keys.push(fname);
	}
	return keys.sort();
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
		readTx: false,
		writeTx: false,
		deleteTx: false,
		scanPrefix: false,
		scanPrefixKeys: false,
		iterate: false,
		size: false,
		crashRecovery: false
	};
	try {
		const opfsRoot = await navigator.storage.getDirectory();
		const testDir = await opfsRoot.getDirectoryHandle(`kvtx-test-${Date.now()}`, { create: true });
		const key1 = new Uint8Array([
			1,
			2,
			3
		]);
		const val1 = new Uint8Array([
			65,
			66,
			67
		]);
		const key2 = new Uint8Array([
			1,
			2,
			4
		]);
		const val2 = new Uint8Array([
			68,
			69,
			70
		]);
		await navigator.locks.request("kvtx-test", { mode: "exclusive" }, async () => {
			await writeEntry(testDir, key1, val1);
			await writeEntry(testDir, key2, val2);
		});
		results.writeTx = true;
		let readOk = true;
		await navigator.locks.request("kvtx-test", { mode: "shared" }, async () => {
			const r1 = await readEntry(testDir, key1);
			if (!r1 || !arraysEqual(r1, val1)) {
				errors.push("read key1 mismatch");
				readOk = false;
			}
			const r2 = await readEntry(testDir, key2);
			if (!r2 || !arraysEqual(r2, val2)) {
				errors.push("read key2 mismatch");
				readOk = false;
			}
			if (await readEntry(testDir, new Uint8Array([255])) !== null) {
				errors.push("read missing key should be null");
				readOk = false;
			}
		});
		results.readTx = readOk;
		let deleteOk = true;
		await navigator.locks.request("kvtx-test", { mode: "exclusive" }, async () => {
			await deleteEntry(testDir, key1);
		});
		await navigator.locks.request("kvtx-test", { mode: "shared" }, async () => {
			if (await readEntry(testDir, key1) !== null) {
				errors.push("key1 should be deleted");
				deleteOk = false;
			}
			const r2 = await readEntry(testDir, key2);
			if (!r2 || !arraysEqual(r2, val2)) {
				errors.push("key2 should survive delete of key1");
				deleteOk = false;
			}
		});
		results.deleteTx = deleteOk;
		const scanDir = await opfsRoot.getDirectoryHandle(`kvtx-scan-${Date.now()}`, { create: true });
		const scanKeys = [
			{
				key: new Uint8Array([170, 1]),
				val: new Uint8Array([1])
			},
			{
				key: new Uint8Array([170, 2]),
				val: new Uint8Array([2])
			},
			{
				key: new Uint8Array([170, 3]),
				val: new Uint8Array([3])
			},
			{
				key: new Uint8Array([187, 1]),
				val: new Uint8Array([4])
			},
			{
				key: new Uint8Array([187, 2]),
				val: new Uint8Array([5])
			}
		];
		for (const { key, val } of scanKeys) await writeEntry(scanDir, key, val);
		const aaPrefix = hexEncode(new Uint8Array([170]));
		const allKeys = await listAllKeys(scanDir);
		const aaKeys = allKeys.filter((k) => k.startsWith(aaPrefix));
		if (aaKeys.length !== 3) errors.push(`scanPrefix 0xaa: got ${aaKeys.length} entries, want 3`);
		results.scanPrefix = aaKeys.length === 3;
		const bbPrefix = hexEncode(new Uint8Array([187]));
		const bbKeys = allKeys.filter((k) => k.startsWith(bbPrefix));
		if (bbKeys.length !== 2) errors.push(`scanPrefixKeys 0xbb: got ${bbKeys.length} entries, want 2`);
		results.scanPrefixKeys = bbKeys.length === 2;
		const sortedKeys = [...allKeys];
		const expectedOrder = [
			"aa01",
			"aa02",
			"aa03",
			"bb01",
			"bb02"
		];
		let iterateOk = sortedKeys.length === expectedOrder.length;
		if (iterateOk) {
			for (let i = 0; i < expectedOrder.length; i++) if (sortedKeys[i] !== expectedOrder[i]) {
				errors.push(`iterate order: [${i}] got ${sortedKeys[i]}, want ${expectedOrder[i]}`);
				iterateOk = false;
			}
		} else errors.push(`iterate count: got ${sortedKeys.length}, want ${expectedOrder.length}`);
		results.iterate = iterateOk;
		results.size = allKeys.length === 5;
		const crashDir = await opfsRoot.getDirectoryHandle(`kvtx-crash-${Date.now()}`, { create: true });
		const pw = await (await crashDir.getFileHandle(".pending", { create: true })).createWritable();
		await pw.write(new Uint8Array([49]));
		await pw.close();
		await writeEntry(crashDir, new Uint8Array([204, 1]), new Uint8Array([99]));
		let pendingExists = false;
		try {
			await crashDir.getFileHandle(".pending");
			pendingExists = true;
		} catch {
			pendingExists = false;
		}
		if (!pendingExists) errors.push("pending marker should exist before cleanup");
		try {
			await crashDir.removeEntry(".pending");
		} catch {}
		let markerGone = false;
		try {
			await crashDir.getFileHandle(".pending");
		} catch (e) {
			if (e.name === "NotFoundError") markerGone = true;
		}
		const partial = await readEntry(crashDir, new Uint8Array([204, 1]));
		results.crashRecovery = pendingExists && markerGone && partial !== null;
		const testPrefix = testDir.name;
		const scanPrefix2 = scanDir.name;
		const crashPrefix = crashDir.name;
		await opfsRoot.removeEntry(testPrefix, { recursive: true });
		await opfsRoot.removeEntry(scanPrefix2, { recursive: true });
		await opfsRoot.removeEntry(crashPrefix, { recursive: true });
		results.pass = errors.length === 0;
		results.detail = errors.length > 0 ? errors.join("; ") : "all kvtx tests passed";
	} catch (err) {
		results.pass = false;
		results.detail = `error: ${err}`;
	}
	window.__results = results;
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=opfs-kvtx.js.map