//#region web/bldr/cross-tab-broker.ts
function isCrossTabMessage(data) {
	if (typeof data !== "object" || data === null) return false;
	const msg = data;
	return msg.crossTab === "hello" || msg.crossTab === "goodbye";
}
async function handleCrossTabMessage(clients, senderId, msg) {
	if (msg.crossTab === "hello") {
		const allClients = await clients.matchAll({ type: "window" });
		for (const client of allClients) {
			if (client.id === senderId) continue;
			const channel = new MessageChannel();
			client.postMessage({
				crossTab: "direct-port",
				peerId: senderId
			}, [channel.port1]);
			const sender = allClients.find((c) => c.id === senderId);
			if (sender) sender.postMessage({
				crossTab: "direct-port",
				peerId: client.id
			}, [channel.port2]);
		}
	} else if (msg.crossTab === "goodbye") {
		const allClients = await clients.matchAll({ type: "window" });
		for (const client of allClients) {
			if (client.id === senderId) continue;
			client.postMessage({
				crossTab: "peer-gone",
				peerId: senderId
			});
		}
	}
}
//#endregion
//#region e2e/comms/fixtures/cross-tab-sw.ts
self.addEventListener("install", () => {
	self.skipWaiting();
});
self.addEventListener("activate", (ev) => {
	ev.waitUntil(self.clients.claim());
});
self.addEventListener("message", (ev) => {
	if (isCrossTabMessage(ev.data)) {
		const senderId = ev.source?.id;
		if (senderId) ev.waitUntil(handleCrossTabMessage(self.clients, senderId, ev.data));
	}
});
//#endregion

//# sourceMappingURL=cross-tab-sw.js.map