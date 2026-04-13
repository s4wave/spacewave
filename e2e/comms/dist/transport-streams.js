import { i as createBusSab, n as SabBusEndpoint } from "./chunks/sab-bus-9wDhw0vI.js";
import { n as detectWorkerCommsConfig } from "./chunks/worker-comms-detect-DGF_nj2J.js";
import { t as createTransportFactory } from "./chunks/plugin-transport-Zq4ID6iL.js";
//#region e2e/comms/fixtures/transport-streams.ts
async function run() {
	const log = document.getElementById("log");
	const errors = [];
	const detect = await detectWorkerCommsConfig();
	const config = detect.config;
	let hasBusStream = false;
	let busStreamRoundTrip = false;
	let busUnavailableOnFallback = false;
	const noopOpen = async () => {
		throw new Error("not implemented");
	};
	const noopHandle = async () => {};
	if (config === "B" || config === "C") {
		const busOpts = {
			slotSize: 8192,
			numSlots: 64
		};
		const busSab = createBusSab(busOpts);
		const endpoint1 = new SabBusEndpoint(busSab, 1, busOpts);
		endpoint1.register();
		const endpoint2 = new SabBusEndpoint(busSab, 2, busOpts);
		endpoint2.register();
		const factory = createTransportFactory(detect, {
			openStream: noopOpen,
			handleIncomingStream: noopHandle,
			busEndpoint: endpoint1
		});
		hasBusStream = factory.openBusStream != null;
		if (factory.openBusStream) {
			const stream = await factory.openBusStream(2);
			if (!stream.source) errors.push("bus stream missing source");
			if (!stream.sink) errors.push("bus stream missing sink");
			const testPayload = new TextEncoder().encode("transport-factory-test");
			const writePromise = stream.sink((async function* () {
				yield testPayload;
			})());
			const msg = await endpoint2.read();
			if (msg) {
				const received = new TextDecoder().decode(msg.data);
				busStreamRoundTrip = received === "transport-factory-test";
				if (!busStreamRoundTrip) errors.push(`bus round-trip mismatch: got ${received}`);
			} else errors.push("bus endpoint 2 received no data");
			if (stream.close) stream.close();
			await writePromise.catch(() => {});
		} else errors.push("expected openBusStream on config " + config);
		endpoint1.close();
		endpoint2.close();
	} else {
		hasBusStream = createTransportFactory(detect, {
			openStream: noopOpen,
			handleIncomingStream: noopHandle
		}).openBusStream != null;
		busUnavailableOnFallback = !hasBusStream;
		if (hasBusStream) errors.push("unexpected openBusStream on config " + config);
	}
	const pass = errors.length === 0;
	window.__results = {
		pass,
		detail: errors.length > 0 ? errors.join("; ") : "ok",
		config,
		hasBusStream,
		busStreamRoundTrip,
		busUnavailableOnFallback
	};
	log.textContent = "DONE";
}
run().catch((err) => {
	window.__results = {
		pass: false,
		detail: `error: ${err}`,
		config: "",
		hasBusStream: false,
		busStreamRoundTrip: false,
		busUnavailableOnFallback: false
	};
	document.getElementById("log").textContent = "DONE";
});
//#endregion

//# sourceMappingURL=transport-streams.js.map