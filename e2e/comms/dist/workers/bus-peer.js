import { n as SabBusEndpoint } from "../chunks/sab-bus-D8XB9B_y.js";
//#region e2e/comms/fixtures/workers/bus-peer.ts
self.onmessage = async (ev) => {
	const { busSab, pluginId, targetId, payload, readOne } = ev.data;
	const endpoint = new SabBusEndpoint(busSab, pluginId, {
		slotSize: 256,
		numSlots: 32
	});
	endpoint.register();
	self.postMessage({
		type: "registered",
		pluginId
	});
	if (targetId != null && payload) {
		endpoint.write(targetId, new Uint8Array(payload));
		self.postMessage({
			type: "sent",
			pluginId,
			targetId
		});
	}
	if (readOne) {
		const msg = await endpoint.read();
		if (msg) self.postMessage({
			type: "received",
			pluginId,
			sourceId: msg.sourceId,
			targetId: msg.targetId,
			data: Array.from(msg.data)
		});
	}
};
//#endregion

//# sourceMappingURL=bus-peer.js.map