//#region web/bldr/worker-comms-detect.ts
function detectCrossOriginIsolated() {
	return typeof self !== "undefined" && !!self.crossOriginIsolated;
}
function detectSabAvailable() {
	try {
		if (typeof SharedArrayBuffer !== "function") return false;
		return new SharedArrayBuffer(8).byteLength === 8;
	} catch {
		return false;
	}
}
async function detectOpfsAvailable() {
	try {
		if (typeof navigator === "undefined") return false;
		if (!navigator.storage?.getDirectory) return false;
		await navigator.storage.getDirectory();
		return true;
	} catch {
		return false;
	}
}
function detectWebLocksAvailable() {
	return typeof navigator !== "undefined" && !!navigator.locks;
}
function detectBroadcastChannelAvailable() {
	return typeof BroadcastChannel === "function";
}
function selectConfig(caps) {
	if (!caps.crossOriginIsolated || !caps.sabAvailable) return "A";
	if (caps.opfsAvailable && caps.webLocksAvailable) return "C";
	return "B";
}
function configDescription(config) {
	switch (config) {
		case "A": return "SharedWorker/MessagePort";
		case "B": return "DedicatedWorker/SAB";
		case "C": return "DedicatedWorker/SAB/OPFS";
		case "F": return "SharedWorker/MessagePort (fallback)";
	}
}
async function detectWorkerCommsConfig() {
	const crossOriginIsolated = detectCrossOriginIsolated();
	const sabAvailable = detectSabAvailable();
	const webLocksAvailable = detectWebLocksAvailable();
	const broadcastChannelAvailable = detectBroadcastChannelAvailable();
	const caps = {
		crossOriginIsolated,
		sabAvailable,
		opfsAvailable: await detectOpfsAvailable(),
		webLocksAvailable,
		broadcastChannelAvailable
	};
	const config = selectConfig(caps);
	console.log("worker-comms: detected config", config, caps);
	return {
		config,
		caps
	};
}
//#endregion
export { detectWorkerCommsConfig as n, configDescription as t };

//# sourceMappingURL=worker-comms-detect-DGF_nj2J.js.map