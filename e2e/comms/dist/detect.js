import { n as detectWorkerCommsConfig, t as configDescription } from "./chunks/worker-comms-detect-DlRfFzjf.js";
//#region e2e/comms/fixtures/detect.ts
async function run() {
	const log = document.getElementById("log");
	try {
		const { config, caps } = await detectWorkerCommsConfig();
		const pass = typeof config === "string" && config.length > 0;
		window.__results = {
			pass,
			config,
			configDesc: configDescription(config),
			caps: {
				crossOriginIsolated: caps.crossOriginIsolated,
				sabAvailable: caps.sabAvailable,
				opfsAvailable: caps.opfsAvailable,
				webLocksAvailable: caps.webLocksAvailable,
				broadcastChannelAvailable: caps.broadcastChannelAvailable
			},
			detail: `config=${config} (${configDescription(config)})`
		};
		log.textContent = "DONE";
	} catch (err) {
		window.__results = {
			pass: false,
			config: "",
			configDesc: "",
			caps: {
				crossOriginIsolated: false,
				sabAvailable: false,
				opfsAvailable: false,
				webLocksAvailable: false,
				broadcastChannelAvailable: false
			},
			detail: `error: ${err}`
		};
		log.textContent = "DONE";
	}
}
run();
//#endregion

//# sourceMappingURL=detect.js.map