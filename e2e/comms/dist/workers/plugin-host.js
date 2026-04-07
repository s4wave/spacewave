import { n as SabBusEndpoint } from "../chunks/sab-bus-9wDhw0vI.js";
//#region e2e/comms/fixtures/workers/plugin-host.ts
var ac = new AbortController();
self.onmessage = async (ev) => {
	const { busSab, busPluginId, scriptUrl, workerCommsDetect } = ev.data;
	const endpoint = new SabBusEndpoint(busSab, busPluginId, {
		slotSize: 256,
		numSlots: 32
	});
	endpoint.register();
	self.postMessage({
		type: "registered",
		busPluginId
	});
	if (workerCommsDetect) self.postMessage({
		type: "config-received",
		config: workerCommsDetect.config
	});
	const pluginModule = await import(
		/* @vite-ignore */
		scriptUrl
);
	if (typeof pluginModule.default !== "function") {
		self.postMessage({
			type: "error",
			detail: "plugin script has no default export function"
		});
		return;
	}
	pluginModule.default(endpoint, ac.signal);
};
//#endregion

//# sourceMappingURL=plugin-host.js.map