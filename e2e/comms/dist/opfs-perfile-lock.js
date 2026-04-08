//#region e2e/comms/fixtures/opfs-perfile-lock.ts
function hexEncode(data) {
	return Array.from(data).map((b) => b.toString(16).padStart(2, "0")).join("");
}
function arraysEqual(a, b) {
	if (a.length !== b.length) return false;
	for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
	return true;
}
async function acquireFileExclusive(dir, name, lockName) {
	let releaseLock = null;
	await new Promise((resolve) => {
		navigator.locks.request(lockName, { mode: "exclusive" }, () => {
			return new Promise((relFn) => {
				releaseLock = relFn;
				resolve();
			});
		});
	});
	return {
		writable: await (await dir.getFileHandle(name, { create: true })).createWritable(),
		release: () => {
			releaseLock?.();
		}
	};
}
async function run() {
	const log = document.getElementById("log");
	const errors = [];
	const results = {
		pass: false,
		detail: "",
		perFileLock: false,
		parallelDistinct: false,
		serialSameFile: false,
		blockStorePattern: false,
		objStoreReadWrite: false,
		objStoreAcid: false
	};
	try {
		const opfsRoot = await navigator.storage.getDirectory();
		const testId = `pfl-test-${Date.now()}`;
		const testDir = await opfsRoot.getDirectoryHandle(testId, { create: true });
		{
			const { writable, release } = await acquireFileExclusive(testDir, "file-a", `${testId}/file-a`);
			const enc = new TextEncoder();
			await writable.write(enc.encode("locked-write"));
			await writable.close();
			release();
			const text = await (await (await testDir.getFileHandle("file-a")).getFile()).text();
			if (text !== "locked-write") errors.push(`per-file lock: got ${JSON.stringify(text)}`);
			else results.perFileLock = true;
		}
		{
			const n = 5;
			const promises = [];
			for (let i = 0; i < n; i++) {
				const fname = `par-${i}`;
				const lockName = `${testId}/par-${i}`;
				promises.push((async () => {
					const { writable, release } = await acquireFileExclusive(testDir, fname, lockName);
					const enc = new TextEncoder();
					await writable.write(enc.encode(`data-${i}`));
					await writable.close();
					release();
				})());
			}
			await Promise.all(promises);
			let allOk = true;
			for (let i = 0; i < n; i++) {
				const text = await (await (await testDir.getFileHandle(`par-${i}`)).getFile()).text();
				if (text !== `data-${i}`) {
					errors.push(`parallel distinct ${i}: got ${JSON.stringify(text)}`);
					allOk = false;
				}
			}
			results.parallelDistinct = allOk;
		}
		{
			const w0 = await (await testDir.getFileHandle("counter", { create: true })).createWritable();
			await w0.write("0");
			await w0.close();
			const n = 10;
			const lockName = `${testId}/counter`;
			const promises = [];
			for (let i = 0; i < n; i++) promises.push((async () => {
				let releaseLock = null;
				await new Promise((resolve) => {
					navigator.locks.request(lockName, { mode: "exclusive" }, () => {
						return new Promise((relFn) => {
							releaseLock = relFn;
							resolve();
						});
					});
				});
				const fh = await testDir.getFileHandle("counter");
				const file = await fh.getFile();
				const val = parseInt(await file.text(), 10);
				const w = await fh.createWritable();
				await w.write(String(val + 1));
				await w.close();
				releaseLock?.();
			})());
			await Promise.all(promises);
			const fileFinal = await (await testDir.getFileHandle("counter")).getFile();
			const finalVal = parseInt(await fileFinal.text(), 10);
			if (finalVal !== n) errors.push(`serial same file: counter=${finalVal}, want ${n}`);
			else results.serialSameFile = true;
		}
		{
			const blocksDir = await testDir.getDirectoryHandle("blocks", { create: true });
			const blocks = [
				{
					key: "ab1234",
					data: new Uint8Array([
						1,
						2,
						3
					])
				},
				{
					key: "ab5678",
					data: new Uint8Array([
						4,
						5,
						6
					])
				},
				{
					key: "cd9012",
					data: new Uint8Array([
						7,
						8,
						9
					])
				}
			];
			for (const b of blocks) {
				const shard = b.key.substring(0, 2);
				const shardDir = await blocksDir.getDirectoryHandle(shard, { create: true });
				const lockName = `${testId}/blocks/${shard}/${b.key}`;
				const { writable, release } = await acquireFileExclusive(shardDir, b.key, lockName);
				await writable.write(b.data);
				await writable.close();
				release();
			}
			let allOk = true;
			for (const b of blocks) {
				const shard = b.key.substring(0, 2);
				const ab = await (await (await (await blocksDir.getDirectoryHandle(shard)).getFileHandle(b.key)).getFile()).arrayBuffer();
				if (!arraysEqual(new Uint8Array(ab), b.data)) {
					errors.push(`block ${b.key}: data mismatch`);
					allOk = false;
				}
			}
			const shard0 = blocks[0].key.substring(0, 2);
			const shardDir0 = await blocksDir.getDirectoryHandle(shard0);
			const lockName0 = `${testId}/blocks/${shard0}/${blocks[0].key}`;
			const { writable: w2, release: r2 } = await acquireFileExclusive(shardDir0, blocks[0].key, lockName0);
			await w2.write(blocks[0].data);
			await w2.close();
			r2();
			results.blockStorePattern = allOk;
		}
		{
			const objDir = await testDir.getDirectoryHandle("objects", { create: true });
			const objLock = `${testId}|objstore`;
			await navigator.locks.request(objLock, { mode: "exclusive" }, async () => {
				const entries = [{
					key: new Uint8Array([1, 2]),
					value: new Uint8Array([65, 66])
				}, {
					key: new Uint8Array([3, 4]),
					value: new Uint8Array([
						67,
						68,
						69
					])
				}];
				for (const { key, value } of entries) {
					const hex = hexEncode(key);
					const shard = hex.substring(0, 2);
					const shardDir = await objDir.getDirectoryHandle(shard, { create: true });
					const perFileLock = `${testId}/obj/${shard}/${hex}`;
					await navigator.locks.request(perFileLock, { mode: "exclusive" }, async () => {
						const w = await (await shardDir.getFileHandle(hex, { create: true })).createWritable();
						await w.write(value);
						await w.close();
					});
				}
			});
			let readOk = true;
			await navigator.locks.request(objLock, { mode: "shared" }, async () => {
				const hex1 = hexEncode(new Uint8Array([1, 2]));
				const shard1 = hex1.substring(0, 2);
				const ab1 = await (await (await (await objDir.getDirectoryHandle(shard1)).getFileHandle(hex1)).getFile()).arrayBuffer();
				if (!arraysEqual(new Uint8Array(ab1), new Uint8Array([65, 66]))) {
					errors.push("objstore read key 0102: data mismatch");
					readOk = false;
				}
			});
			results.objStoreReadWrite = readOk;
		}
		{
			const acidLock = `${testId}|acid`;
			const events = [];
			const writeDone = navigator.locks.request(acidLock, { mode: "exclusive" }, async () => {
				events.push("write-start");
				await new Promise((r) => setTimeout(r, 100));
				events.push("write-end");
			});
			await new Promise((r) => setTimeout(r, 20));
			const readDone = navigator.locks.request(acidLock, { mode: "shared" }, async () => {
				events.push("read-start");
			});
			await Promise.all([writeDone, readDone]);
			const writeStartIdx = events.indexOf("write-start");
			const writeEndIdx = events.indexOf("write-end");
			const readStartIdx = events.indexOf("read-start");
			if (writeStartIdx < writeEndIdx && writeEndIdx < readStartIdx) results.objStoreAcid = true;
			else errors.push(`ACID ordering: ${events.join(", ")}`);
		}
		await opfsRoot.removeEntry(testId, { recursive: true });
		results.pass = errors.length === 0;
		results.detail = errors.length > 0 ? errors.join("; ") : "all per-file lock tests passed";
	} catch (err) {
		results.pass = false;
		results.detail = `error: ${err}`;
	}
	window.__results = results;
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=opfs-perfile-lock.js.map