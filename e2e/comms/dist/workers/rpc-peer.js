import { c as createHandler, l as Server, n as EchoerClient, r as EchoerDefinition, s as createMux, t as EchoerServer, u as Client } from "../chunks/dist-CmY9bC3s.js";
import { n as SabBusEndpoint, r as SabBusStream } from "../chunks/sab-bus-9wDhw0vI.js";
//#region e2e/comms/fixtures/workers/rpc-peer.ts
var busOpts = {
	slotSize: 8192,
	numSlots: 64
};
self.onmessage = async (ev) => {
	if ("type" in ev.data && ev.data.type === "start") return;
	const { busSab, pluginId, targetId, role } = ev.data;
	const endpoint = new SabBusEndpoint(busSab, pluginId, busOpts);
	endpoint.register();
	self.postMessage({
		type: "registered",
		pluginId,
		role
	});
	if (role === "server") {
		const stream = new SabBusStream(endpoint, targetId);
		const mux = createMux();
		mux.register(createHandler(EchoerDefinition, new EchoerServer()));
		const server = new Server(mux.lookupMethod);
		self.postMessage({ type: "server-ready" });
		await server.rpcStreamHandler(stream);
		self.postMessage({ type: "server-done" });
	} else {
		await new Promise((resolve) => {
			self.onmessage = (startEv) => {
				if (startEv.data?.type === "start") resolve();
			};
		});
		const stream = new SabBusStream(endpoint, targetId);
		const response = await new EchoerClient(new Client(async () => stream)).Echo({ body: "hello via SAB bus" });
		self.postMessage({
			type: "rpc-result",
			body: response.body
		});
		stream.close();
	}
};
//#endregion

//# sourceMappingURL=rpc-peer.js.map