import { r as SabBusStream } from "./sab-bus-9wDhw0vI.js";
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
	if (opts.openCrossTabStream) {
		factory.openCrossTabStream = opts.openCrossTabStream;
		console.log("worker-comms: cross-tab transport available");
	}
	return factory;
}
//#endregion
export { createTransportFactory as t };

//# sourceMappingURL=plugin-transport-Zq4ID6iL.js.map