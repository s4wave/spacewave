import { a as createMux, c as Client, i as ChannelStream, n as EchoerClient, o as createHandler, r as EchoerDefinition, s as Server, t as EchoerServer } from "./chunks/dist-BYT-zYEp.js";
//#region e2e/comms/fixtures/cross-tab-rpc.ts
var streamOpts = {
	keepAliveMs: 5e3,
	idleTimeoutMs: 1e4
};
var peers = /* @__PURE__ */ new Map();
window.__peers = peers;
var mux = createMux();
mux.register(createHandler(EchoerDefinition, new EchoerServer()));
var server = new Server(mux.lookupMethod);
function addPeer(peerId, port) {
	const existing = peers.get(peerId);
	if (existing) existing.close();
	peers.set(peerId, port);
	port.onmessage = (ev) => {
		if (ev.data?.type === "relay" && ev.ports?.[0]) {
			const subPort = ev.ports[0];
			const stream = new ChannelStream(peerId, subPort, streamOpts);
			server.rpcStreamHandler(stream).catch(() => {});
		}
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
		...window.__results,
		peerCount: peers.size
	};
}
window.callEcho = async (peerId, body) => {
	const port = peers.get(peerId);
	if (!port) throw new Error(`no peer: ${peerId}`);
	const { port1, port2 } = new MessageChannel();
	port.postMessage({
		type: "relay",
		port: port2
	}, [port2]);
	const stream = new ChannelStream("local", port1, streamOpts);
	return (await new EchoerClient(new Client(async () => stream)).Echo({ body })).body ?? "";
};
async function run() {
	const log = document.getElementById("log");
	window.__results = {
		pass: false,
		detail: "initializing",
		swRegistered: false,
		peerCount: 0,
		echoBody: ""
	};
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
			detail: "no SW",
			swRegistered: false,
			peerCount: 0,
			echoBody: ""
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
		echoBody: ""
	};
	log.textContent = "DONE";
}
run().catch((err) => {
	window.__results = {
		pass: false,
		detail: `error: ${err}`,
		swRegistered: false,
		peerCount: 0,
		echoBody: ""
	};
	document.getElementById("log").textContent = "DONE";
});
//#endregion

//# sourceMappingURL=cross-tab-rpc.js.map