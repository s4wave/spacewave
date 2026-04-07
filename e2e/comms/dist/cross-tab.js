//#region e2e/comms/fixtures/cross-tab.ts
var peers = /* @__PURE__ */ new Map();
var messagesReceived = [];
function addPeer(peerId, port) {
	const existing = peers.get(peerId);
	if (existing) existing.close();
	peers.set(peerId, port);
	port.onmessage = (ev) => {
		messagesReceived.push(JSON.stringify(ev.data));
		updateResults();
	};
	port.start();
	updateResults();
}
function removePeer(peerId) {
	const port = peers.get(peerId);
	if (port) {
		port.close();
		peers.delete(peerId);
		updateResults();
	}
}
function updateResults() {
	window.__results = {
		pass: true,
		detail: "ok",
		swRegistered: true,
		peerCount: peers.size,
		messagesReceived: [...messagesReceived]
	};
}
window.sendToPeers = (msg) => {
	for (const [, port] of peers) port.postMessage({ text: msg });
};
async function run() {
	const log = document.getElementById("log");
	navigator.serviceWorker.addEventListener("message", (ev) => {
		const data = ev.data;
		if (typeof data !== "object" || !data.crossTab) return;
		if (data.crossTab === "direct-port") {
			const port = ev.ports[0];
			if (port) addPeer(data.peerId, port);
		} else if (data.crossTab === "peer-gone") removePeer(data.peerId);
	});
	const reg = await navigator.serviceWorker.register("/cross-tab-sw.js");
	const sw = reg.active || reg.installing || reg.waiting;
	if (!sw) {
		window.__results = {
			pass: false,
			detail: "no SW instance after registration",
			swRegistered: false,
			peerCount: 0,
			messagesReceived: []
		};
		log.textContent = "DONE";
		return;
	}
	await new Promise((resolve) => {
		if (sw.state === "activated") {
			resolve();
			return;
		}
		sw.addEventListener("statechange", () => {
			if (sw.state === "activated") resolve();
		});
	});
	if (!navigator.serviceWorker.controller) {
		await navigator.serviceWorker.ready;
		await new Promise((resolve) => {
			if (navigator.serviceWorker.controller) {
				resolve();
				return;
			}
			navigator.serviceWorker.addEventListener("controllerchange", () => resolve(), { once: true });
		});
	}
	navigator.serviceWorker.controller.postMessage({ crossTab: "hello" });
	window.__results = {
		pass: true,
		detail: "sw registered, waiting for peers",
		swRegistered: true,
		peerCount: 0,
		messagesReceived: []
	};
	log.textContent = "DONE";
}
run().catch((err) => {
	window.__results = {
		pass: false,
		detail: `error: ${err}`,
		swRegistered: false,
		peerCount: 0,
		messagesReceived: []
	};
	document.getElementById("log").textContent = "DONE";
});
//#endregion

//# sourceMappingURL=cross-tab.js.map