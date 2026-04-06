import { i as createBusSab, n as SabBusEndpoint, t as BROADCAST_ID } from "./chunks/sab-bus-D8XB9B_y.js";
//#region e2e/comms/fixtures/sab-bus.ts
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
		const workerA = new Worker(new URL(
			/* @vite-ignore */
			"/assets/bus-peer-B2g56qEO.js",
			"" + import.meta.url
		), { type: "module" });
		const workerB = new Worker(new URL(
			/* @vite-ignore */
			"/assets/bus-peer-B2g56qEO.js",
			"" + import.meta.url
		), { type: "module" });
		workerA.postMessage({
			busSab,
			pluginId: 1,
			readOne: true
		});
		await waitWorkerMsg(workerA, "registered", 5e3);
		workerB.postMessage({
			busSab,
			pluginId: 2,
			readOne: true
		});
		await waitWorkerMsg(workerB, "registered", 5e3);
		let unicast = false;
		{
			mainEndpoint.write(1, new Uint8Array([170, 1]));
			const msg = await waitWorkerMsg(workerA, "received", 5e3);
			if (msg.sourceId === 0 && msg.data[0] === 170 && msg.data[1] === 1) unicast = true;
			else errors.push(`unicast: unexpected msg ${JSON.stringify(msg)}`);
		}
		let relay = false;
		{
			const workerA2 = new Worker(new URL(
				/* @vite-ignore */
				"/assets/bus-peer-B2g56qEO.js",
				"" + import.meta.url
			), { type: "module" });
			const workerB2 = new Worker(new URL(
				/* @vite-ignore */
				"/assets/bus-peer-B2g56qEO.js",
				"" + import.meta.url
			), { type: "module" });
			workerB2.postMessage({
				busSab,
				pluginId: 12,
				readOne: true
			});
			await waitWorkerMsg(workerB2, "registered", 5e3);
			workerA2.postMessage({
				busSab,
				pluginId: 11,
				targetId: 12,
				payload: [187, 2]
			});
			await waitWorkerMsg(workerA2, "registered", 5e3);
			await waitWorkerMsg(workerA2, "sent", 5e3);
			const msg = await waitWorkerMsg(workerB2, "received", 5e3);
			if (msg.sourceId === 11 && msg.data[0] === 187 && msg.data[1] === 2) relay = true;
			else errors.push(`relay: unexpected msg ${JSON.stringify(msg)}`);
			workerA2.terminate();
			workerB2.terminate();
		}
		let broadcast = false;
		{
			const workerC = new Worker(new URL(
				/* @vite-ignore */
				"/assets/bus-peer-B2g56qEO.js",
				"" + import.meta.url
			), { type: "module" });
			workerC.postMessage({
				busSab,
				pluginId: 20,
				targetId: BROADCAST_ID,
				payload: [204, 3]
			});
			await waitWorkerMsg(workerC, "registered", 5e3);
			await waitWorkerMsg(workerC, "sent", 5e3);
			const msg = await mainEndpoint.read();
			if (msg && msg.sourceId === 20 && msg.targetId === 65535 && msg.data[0] === 204) broadcast = true;
			else errors.push(`broadcast: unexpected msg ${JSON.stringify(msg)}`);
			workerC.terminate();
		}
		workerA.terminate();
		workerB.terminate();
		mainEndpoint.close();
		const pass = unicast && relay && broadcast && errors.length === 0;
		window.__results = {
			pass,
			detail: errors.length > 0 ? errors.join("; ") : "all tests passed",
			unicast,
			relay,
			broadcast
		};
	} catch (err) {
		window.__results = {
			pass: false,
			detail: `error: ${err}`,
			unicast: false,
			relay: false,
			broadcast: false
		};
	}
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=sab-bus.js.map