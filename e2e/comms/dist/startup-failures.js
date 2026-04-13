import { a as WebRuntimeClientType, i as buildWebWorkerLockName, n as WebRuntime, o as PluginStartInfo, r as timeoutPromise, t as WebDocumentTracker } from "./chunks/web-document-tracker-BxuL8ox8.js";
//#region e2e/comms/fixtures/startup-failures.ts
function encodeStartInfo() {
	const json = PluginStartInfo.toJsonString({});
	return new TextEncoder().encode(btoa(json));
}
async function holdWebDocumentLock(name) {
	let releaseLock;
	const waitReleased = new Promise((resolve) => {
		releaseLock = resolve;
	});
	await new Promise((resolve, reject) => {
		navigator.locks.request(name, async () => {
			resolve();
			await waitReleased;
		}).catch(reject);
	});
	return () => releaseLock?.();
}
function waitForPortMessage(port, predicate, timeoutMs) {
	return new Promise((resolve, reject) => {
		const timer = globalThis.setTimeout(() => {
			cleanup();
			reject(/* @__PURE__ */ new Error(`timeout waiting for port message after ${timeoutMs}ms`));
		}, timeoutMs);
		const handler = (ev) => {
			if (!predicate(ev.data, ev.ports)) return;
			cleanup();
			resolve(ev.data);
		};
		const cleanup = () => {
			globalThis.clearTimeout(timer);
			port.removeEventListener("message", handler);
		};
		port.addEventListener("message", handler);
		port.start();
	});
}
function waitWorkerMsg(worker, type, timeoutMs) {
	return new Promise((resolve, reject) => {
		const timer = globalThis.setTimeout(() => {
			cleanup();
			reject(/* @__PURE__ */ new Error(`timeout waiting for ${type}`));
		}, timeoutMs);
		const handler = (ev) => {
			if (ev.data?.type !== type) return;
			cleanup();
			resolve(ev.data);
		};
		const cleanup = () => {
			globalThis.clearTimeout(timer);
			worker.removeEventListener("message", handler);
		};
		worker.addEventListener("message", handler);
	});
}
async function runSlowRegistrationScenario() {
	const webDocumentId = "startup-failures-slow-doc";
	const releaseLock = await holdWebDocumentLock(`bldr-doc-${webDocumentId}`);
	let tracker;
	tracker = new WebDocumentTracker("startup-failures-slow-worker", WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER, async () => {
		tracker.close();
	}, null);
	const { port1, port2 } = new MessageChannel();
	tracker.handleWebDocumentMessage({
		from: webDocumentId,
		initPort: port1
	});
	port2.start();
	const start = performance.now();
	try {
		await tracker.waitConn();
		return {
			ok: false,
			detail: "slow registration unexpectedly connected"
		};
	} catch (err) {
		const elapsed = performance.now() - start;
		if (elapsed > 2500) return {
			ok: false,
			detail: `slow registration rejection was not bounded (${Math.round(elapsed)}ms)`
		};
		return {
			ok: true,
			detail: `slow registration rejected in ${Math.round(elapsed)}ms: ${String(err)}`
		};
	} finally {
		tracker.close();
		port2.close();
		releaseLock();
	}
}
async function runCloseDuringStartupScenario() {
	const webDocumentId = "startup-failures-close-doc";
	const releaseLock = await holdWebDocumentLock(`bldr-doc-${webDocumentId}`);
	let tracker;
	tracker = new WebDocumentTracker("startup-failures-close-worker", WebRuntimeClientType.WebRuntimeClientType_WEB_WORKER, async () => {
		tracker.close();
	}, null);
	const { port1, port2 } = new MessageChannel();
	tracker.handleWebDocumentMessage({
		from: webDocumentId,
		initPort: port1
	});
	port2.addEventListener("message", (ev) => {
		const data = ev.data;
		if (typeof data !== "object" || !data?.connectWebRuntime) return;
		port2.postMessage({
			from: "startup-failures-close-doc",
			close: true
		});
		releaseLock();
	});
	port2.start();
	const start = performance.now();
	try {
		await tracker.waitConn();
		return {
			ok: false,
			detail: "close during startup unexpectedly connected"
		};
	} catch (err) {
		const elapsed = performance.now() - start;
		if (elapsed > 1500) return {
			ok: false,
			detail: `close during startup took too long to reject (${Math.round(elapsed)}ms)`
		};
		return {
			ok: true,
			detail: `close during startup rejected in ${Math.round(elapsed)}ms: ${String(err)}`
		};
	} finally {
		tracker.close();
		port2.close();
	}
}
async function runImportFailureScenario() {
	const releaseLock = await holdWebDocumentLock(`bldr-doc-startup-failures-import-doc`);
	const worker = new Worker(new URL(
		/* @vite-ignore */
		"/assets/plugin-startup-fixture-WawstLCr.js",
		"" + import.meta.url
	), {
		type: "module",
		name: "startup-failures-import-worker"
	});
	const { port1, port2 } = new MessageChannel();
	let ready = false;
	port2.addEventListener("message", (ev) => {
		const data = ev.data;
		if (typeof data !== "object" || !data?.connectWebRuntime) {
			if (typeof data === "object" && data?.ready) ready = true;
			return;
		}
		const ackPort = data.connectWebRuntime.port ?? ev.ports[0];
		if (!ackPort) return;
		ackPort.start();
		const { port1: runtimePort1, port2: runtimePort2 } = new MessageChannel();
		ackPort.postMessage({
			from: "startup-failures-import-doc",
			webRuntimePort: runtimePort1
		}, [runtimePort1]);
		runtimePort2.start();
		runtimePort2.postMessage({ connected: true });
	});
	port2.start();
	worker.postMessage({
		from: "startup-failures-import-doc",
		initData: encodeStartInfo(),
		initPort: port1
	}, [port1]);
	try {
		await waitForPortMessage(port2, (data) => {
			return typeof data === "object" && !!data?.close;
		}, 3e3);
		await timeoutPromise(50);
		return {
			ok: !ready,
			detail: ready ? "worker published ready before import failure closed it" : "worker closed after import failure",
			ready
		};
	} catch (err) {
		return {
			ok: false,
			detail: `import failure did not close cleanly: ${String(err)}`,
			ready
		};
	} finally {
		releaseLock();
		worker.terminate();
		port2.close();
	}
}
async function runWorkerPreRegistrationScenario() {
	const workerId = "startup-failures-pre-register-worker";
	const runtime = new WebRuntime("startup-failures-runtime", async () => {
		throw new Error("unexpected runtime host open stream");
	}, null, null);
	const worker = new Worker(new URL(
		/* @vite-ignore */
		"/assets/plugin-startup-fixture-WawstLCr.js",
		"" + import.meta.url
	), {
		type: "module",
		name: workerId
	});
	try {
		const booted = await waitWorkerMsg(worker, "booted", 2e3);
		if (booted.type !== "booted") return {
			ok: false,
			detail: `worker booted with unexpected message ${JSON.stringify(booted)}`
		};
		const start = performance.now();
		const waitClient = runtime.waitForClient(workerId, buildWebWorkerLockName(workerId));
		worker.terminate();
		try {
			await waitClient;
			return {
				ok: false,
				detail: "worker pre-registration unexpectedly connected"
			};
		} catch (err) {
			const elapsed = performance.now() - start;
			if (elapsed > 1500) return {
				ok: false,
				detail: `worker pre-registration rejection was not bounded (${Math.round(elapsed)}ms)`
			};
			return {
				ok: true,
				detail: `worker pre-registration rejected in ${Math.round(elapsed)}ms: ${String(err)}`
			};
		}
	} catch (err) {
		return {
			ok: false,
			detail: `worker pre-registration setup failed: ${String(err)}`
		};
	} finally {
		worker.terminate();
	}
}
async function run() {
	const log = document.getElementById("log");
	const details = [];
	try {
		const slow = await runSlowRegistrationScenario();
		details.push(slow.detail);
		const close = await runCloseDuringStartupScenario();
		details.push(close.detail);
		const workerPreRegistration = await runWorkerPreRegistrationScenario();
		details.push(workerPreRegistration.detail);
		const importFailure = await runImportFailureScenario();
		details.push(importFailure.detail);
		const pass = slow.ok && close.ok && workerPreRegistration.ok && importFailure.ok;
		window.__results = {
			pass,
			detail: details.join("; "),
			slowRegistrationRejected: slow.ok,
			closeDuringStartupRejected: close.ok,
			workerPreRegistrationRejected: workerPreRegistration.ok,
			importFailureClosed: importFailure.ok,
			importFailureReady: importFailure.ready
		};
	} catch (err) {
		window.__results = {
			pass: false,
			detail: `startup failures fixture crashed: ${String(err)}`,
			slowRegistrationRejected: false,
			closeDuringStartupRejected: false,
			workerPreRegistrationRejected: false,
			importFailureClosed: false,
			importFailureReady: false
		};
	}
	if (log) log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=startup-failures.js.map