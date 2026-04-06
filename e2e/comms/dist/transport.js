import { i as createBusSab, n as SabBusEndpoint, r as SabBusStream } from "./chunks/sab-bus-D8XB9B_y.js";
import { n as detectWorkerCommsConfig } from "./chunks/worker-comms-detect-DlRfFzjf.js";
//#region web/bldr/plugin-transport.ts
function createTransportFactory(detect, opts) {
	const factory = {
		openStream: opts.openStream,
		handleIncomingStream: opts.handleIncomingStream,
		config: detect.config
	};
	if (opts.busEndpoint) {
		factory.busEndpoint = opts.busEndpoint;
		factory.openBusStream = async (targetPluginId) => {
			return new SabBusStream(opts.busEndpoint, targetPluginId);
		};
		console.log("worker-comms: SAB bus transport available for intra-tab IPC");
	}
	return factory;
}
//#endregion
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