//#region e2e/comms/fixtures/opfs-primitives.ts
async function run() {
	const log = document.getElementById("log");
	const errors = [];
	const results = {
		pass: false,
		detail: "",
		getRoot: false,
		createDir: false,
		nestedDir: false,
		writeRead: false,
		overwrite: false,
		deleteFile: false,
		listDir: false,
		notFoundFile: false,
		notFoundDir: false,
		deleteNotFound: false
	};
	try {
		const prefix = `test-${Date.now()}`;
		const root = await navigator.storage.getDirectory();
		results.getRoot = true;
		const testDir = await root.getDirectoryHandle(prefix, { create: true });
		results.createDir = true;
		await (await (await testDir.getDirectoryHandle("a", { create: true })).getDirectoryHandle("b", { create: true })).getDirectoryHandle("c", { create: true });
		await (await (await testDir.getDirectoryHandle("a", { create: false })).getDirectoryHandle("b", { create: false })).getDirectoryHandle("c", { create: false });
		results.nestedDir = true;
		const testData = new Uint8Array([
			72,
			101,
			108,
			108,
			111
		]);
		const fileDir = await testDir.getDirectoryHandle("files", { create: true });
		const writable = await (await fileDir.getFileHandle("test.bin", { create: true })).createWritable();
		await writable.write(testData);
		await writable.close();
		const ab = await (await (await fileDir.getFileHandle("test.bin")).getFile()).arrayBuffer();
		const readData = new Uint8Array(ab);
		if (readData.length !== testData.length) errors.push(`read length mismatch: got ${readData.length}, want ${testData.length}`);
		else {
			let match = true;
			for (let i = 0; i < testData.length; i++) if (readData[i] !== testData[i]) {
				match = false;
				break;
			}
			if (!match) errors.push("read data mismatch");
		}
		results.writeRead = errors.length === 0;
		const newData = new Uint8Array([
			87,
			111,
			114,
			108,
			100
		]);
		const wOw = await (await fileDir.getFileHandle("test.bin", { create: true })).createWritable();
		await wOw.write(newData);
		await wOw.close();
		const abOr = await (await (await fileDir.getFileHandle("test.bin")).getFile()).arrayBuffer();
		const orData = new Uint8Array(abOr);
		let owMatch = orData.length === newData.length;
		if (owMatch) {
			for (let i = 0; i < newData.length; i++) if (orData[i] !== newData[i]) {
				owMatch = false;
				break;
			}
		}
		if (!owMatch) errors.push("overwrite data mismatch");
		results.overwrite = owMatch;
		await fileDir.removeEntry("test.bin");
		let deletedGone = false;
		try {
			await fileDir.getFileHandle("test.bin");
		} catch (e) {
			if (e.name === "NotFoundError") deletedGone = true;
		}
		if (!deletedGone) errors.push("file still exists after delete");
		results.deleteFile = deletedGone;
		const listDir = await testDir.getDirectoryHandle("listtest", { create: true });
		for (const name of [
			"charlie.txt",
			"alpha.txt",
			"bravo.txt"
		]) {
			const lw = await (await listDir.getFileHandle(name, { create: true })).createWritable();
			await lw.write(new Uint8Array([0]));
			await lw.close();
		}
		await listDir.getDirectoryHandle("delta-dir", { create: true });
		const entries = [];
		for await (const [name] of listDir.entries()) entries.push(name);
		const sorted = [...entries].sort();
		const expected = [
			"alpha.txt",
			"bravo.txt",
			"charlie.txt",
			"delta-dir"
		];
		if (sorted.length !== expected.length) errors.push(`list length: got ${sorted.length}, want ${expected.length}`);
		else for (let i = 0; i < expected.length; i++) if (sorted[i] !== expected[i]) errors.push(`list[${i}]: got ${sorted[i]}, want ${expected[i]}`);
		results.listDir = errors.filter((e) => e.startsWith("list")).length === 0;
		let notFoundFile = false;
		try {
			await fileDir.getFileHandle("nonexistent.bin");
		} catch (e) {
			if (e.name === "NotFoundError") notFoundFile = true;
			else errors.push(`missing file error name: ${e.name}`);
		}
		results.notFoundFile = notFoundFile;
		let notFoundDir = false;
		try {
			await testDir.getDirectoryHandle("nonexistent-dir", { create: false });
		} catch (e) {
			if (e.name === "NotFoundError") notFoundDir = true;
			else errors.push(`missing dir error name: ${e.name}`);
		}
		results.notFoundDir = notFoundDir;
		let deleteNotFound = false;
		try {
			await fileDir.removeEntry("nonexistent.bin");
		} catch (e) {
			if (e.name === "NotFoundError") deleteNotFound = true;
			else errors.push(`delete missing error name: ${e.name}`);
		}
		results.deleteNotFound = deleteNotFound;
		await root.removeEntry(prefix, { recursive: true });
		results.pass = errors.length === 0;
		results.detail = errors.length > 0 ? errors.join("; ") : "all opfs primitives tests passed";
	} catch (err) {
		results.pass = false;
		results.detail = `error: ${err}`;
	}
	window.__results = results;
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=opfs-primitives.js.map