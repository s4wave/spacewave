//#region e2e/comms/fixtures/workers/plugin-stub.ts
function main(busEndpoint, signal) {
	self.postMessage({ type: "plugin-started" });
	busEndpoint.read().then((msg) => {
		if (msg && !signal.aborted) self.postMessage({
			type: "plugin-received",
			sourceId: msg.sourceId,
			data: Array.from(msg.data)
		});
	}).catch(() => {});
}
//#endregion
export { main as default };

//# sourceMappingURL=plugin-stub.js.map