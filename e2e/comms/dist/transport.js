import { i as createBusSab, n as SabBusEndpoint } from "./chunks/sab-bus-9wDhw0vI.js";
import { n as detectWorkerCommsConfig } from "./chunks/worker-comms-detect-DGF_nj2J.js";
import { t as createTransportFactory } from "./chunks/plugin-transport-Zq4ID6iL.js";
//#region e2e/comms/fixtures/transport.ts
async function run() {
	const log = document.getElementById("log");
	const errors = [];
	try {
		const detect = await detectWorkerCommsConfig();
		const config = detect.config;
		const noopOpen = async () => {
			throw new Error("not implemented");
		};
		const noopHandle = async () => {};
		let factory;
		let hasBusStream = false;
		if (config === "B" || config === "C") {
			const busOpts = {
				slotSize: 256,
				numSlots: 16
			};
			const endpoint = new SabBusEndpoint(createBusSab(busOpts), 0, busOpts);
			endpoint.register();
			factory = createTransportFactory(detect, {
				openStream: noopOpen,
				handleIncomingStream: noopHandle,
				busEndpoint: endpoint,
				pluginId: 0
			});
			hasBusStream = factory.openBusStream != null;
			endpoint.close();
		} else {
			factory = createTransportFactory(detect, {
				openStream: noopOpen,
				handleIncomingStream: noopHandle
			});
			hasBusStream = factory.openBusStream != null;
		}
		const factoryCreated = factory.config === config;
		if (config === "B" || config === "C") {
			if (!hasBusStream) errors.push("expected openBusStream on config " + config);
		} else if (hasBusStream) errors.push("unexpected openBusStream on config " + config);
		if (!factoryCreated) errors.push(`factory config mismatch: ${factory.config} vs ${config}`);
		const pass = errors.length === 0 && factoryCreated;
		window.__results = {
			pass,
			detail: errors.length > 0 ? errors.join("; ") : "all tests passed",
			config,
			hasBusStream,
			factoryCreated
		};
	} catch (err) {
		window.__results = {
			pass: false,
			detail: `error: ${err}`,
			config: "",
			hasBusStream: false,
			factoryCreated: false
		};
	}
	log.textContent = "DONE";
}
run();
//#endregion

//# sourceMappingURL=transport.js.map