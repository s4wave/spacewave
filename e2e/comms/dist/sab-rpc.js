import { i as createBusSab } from "./chunks/sab-bus-9wDhw0vI.js";
//#region e2e/comms/fixtures/sab-rpc.ts
function waitWorkerMsg(worker, type, timeoutMs = 5e3) {
	return new Promise((resolve, reject) => {
		const timer = setTimeout(() => reject(/* @__PURE__ */ new Error(`timeout waiting for worker message: ${type}`)), timeoutMs);
		const handler = (ev) => {
			if (ev.data?.type === type) {
				clearTimeout(timer);
				worker.removeEventListener("message", handler);
				resolve(ev);
			}
		};
		worker.addEventListener("message", handler);
	});
}
async function run() {
	const log = document.getElementById("log");
	const busSab = createBusSab({
		slotSize: 8192,
		numSlots: 64
	});
	const serverWorker = new Worker("/workers/rpc-peer.js", { type: "module" });
	const serverRegistered = waitWorkerMsg(serverWorker, "registered");
	const serverReady = waitWorkerMsg(serverWorker, "server-ready");
	serverWorker.postMessage({
		busSab,
		pluginId: 1,
		targetId: 2,
		role: "server"
	});
	await serverRegistered;
	await serverReady;
	const clientWorker = new Worker("/workers/rpc-peer.js", { type: "module" });
	const clientRegistered = waitWorkerMsg(clientWorker, "registered");
	const rpcResult = waitWorkerMsg(clientWorker, "rpc-result");
	clientWorker.postMessage({
		busSab,
		pluginId: 2,
		targetId: 1,
		role: "client"
	});
	await clientRegistered;
	clientWorker.postMessage({ type: "start" });
	const body = (await rpcResult).data?.body ?? "";
	window.__results = {
		pass: body === "hello via SAB bus",
		detail: body === "hello via SAB bus" ? "echo round-trip ok" : `unexpected: ${body}`,
		echoBody: body
	};
	serverWorker.terminate();
	clientWorker.terminate();
	log.textContent = "DONE";
}
run().catch((err) => {
	window.__results = {
		pass: false,
		detail: `error: ${err}`,
		echoBody: ""
	};
	document.getElementById("log").textContent = "DONE";
});
//#endregion

//# sourceMappingURL=sab-rpc.js.map