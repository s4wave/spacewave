import { i as createBusSab, n as SabBusEndpoint } from "./chunks/sab-bus-9wDhw0vI.js";
import { n as detectWorkerCommsConfig } from "./chunks/worker-comms-detect-DGF_nj2J.js";
//#region e2e/comms/fixtures/dedicated.ts
function waitWorkerMsg(worker, type, timeoutMs) {
	return new Promise((resolve, reject) => {
		const timer = setTimeout(() => reject(/* @__PURE__ */ new Error(`timeout waiting for ${type}`)), timeoutMs);
		const handler = (ev) => {
			if (ev.data.type === type) {
				clearTimeout(timer);
				worker.removeEventListener("message", handler);
				resolve(ev.data);
			}
		};
		worker.addEventListener("message", handler);
	});
}
async function run() {
	const log = document.getElementById("log");
	const errors = [];
	try {
		const busOpts = {
			slotSize: 256,
			numSlots: 32
		};
		const busSab = createBusSab(busOpts);
		const mainEndpoint = new SabBusEndpoint(busSab, 0, busOpts);
		mainEndpoint.register();
		const detect = await detectWorkerCommsConfig();
		const worker = new Worker(new URL(
			/* @vite-ignore */
			"/assets/plugin-host-sjskEWhU.js",
			"" + import.meta.url
		), { type: "module" });
		const pluginUrl = "/workers/plugin-stub.js";
		const registeredP = waitWorkerMsg(worker, "registered", 5e3);
		const configReceivedP = waitWorkerMsg(worker, "config-received", 5e3);
		const pluginStartedP = waitWorkerMsg(worker, "plugin-started", 5e3);
		worker.postMessage({
			busSab,
			busPluginId: 1,
			scriptUrl: pluginUrl,
			workerCommsDetect: detect
		});
		let registered = false;
		{
			const msg = await registeredP;
			if (msg.busPluginId === 1) registered = true;
			else errors.push(`registered: unexpected busPluginId ${msg.busPluginId}`);
		}
		let configReceived = false;
		{
			const msg = await configReceivedP;
			if (msg.config === detect.config) configReceived = true;
			else errors.push(`config: expected ${detect.config}, got ${msg.config}`);
		}
		let pluginStarted = false;
		await pluginStartedP;
		pluginStarted = true;
		let pluginReceived = false;
		{
			mainEndpoint.write(1, new Uint8Array([255, 66]));
			const msg = await waitWorkerMsg(worker, "plugin-received", 5e3);
			if (msg.sourceId === 0 && msg.data[0] === 255 && msg.data[1] === 66) pluginReceived = true;
			else errors.push(`received: unexpected msg ${JSON.stringify(msg)}`);
		}
		worker.terminate();
		mainEndpoint.close();
		const pass = registered && pluginStarted && pluginReceived && configReceived && errors.length === 0;
		window.__results = {
			pass,
			detail: errors.length > 0 ? errors.join("; ") : "all tests passed",
			registered,
			pluginStarted,
			pluginReceived,
			configReceived
		};
	} catch (err) {
		window.__results = {
			pass: false,
			detail: `error: ${err}`,
			registered: false,
			pluginStarted: false,
			pluginReceived: false
		};
	}
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=dedicated.js.map