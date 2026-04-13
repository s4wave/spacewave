import { c as Client, d as createEnumType, f as ScalarType, i as ChannelStream, l as MethodKind, m as castToError, p as protoInt64, u as createMessageType } from "./dist-C0KRw9Ez.js";
//#region vendor/github.com/aperturerobotics/bifrost/hash/hash.pb.ts
var HashType_Enum = createEnumType("hash.HashType", [
	{
		no: 0,
		name: "HashType_UNKNOWN"
	},
	{
		no: 1,
		name: "HashType_SHA256"
	},
	{
		no: 2,
		name: "HashType_SHA1"
	},
	{
		no: 3,
		name: "HashType_BLAKE3"
	}
]);
var Hash = createMessageType({
	typeName: "hash.Hash",
	fields: [{
		no: 1,
		name: "hash_type",
		kind: "enum",
		T: HashType_Enum
	}, {
		no: 2,
		name: "hash",
		kind: "scalar",
		T: ScalarType.BYTES
	}],
	packedByDefault: true
});
createEnumType("block.OverlayMode", [
	{
		no: 0,
		name: "UPPER_ONLY"
	},
	{
		no: 1,
		name: "LOWER_ONLY"
	},
	{
		no: 2,
		name: "UPPER_CACHE"
	},
	{
		no: 3,
		name: "LOWER_CACHE"
	},
	{
		no: 4,
		name: "UPPER_READ_CACHE"
	},
	{
		no: 5,
		name: "LOWER_READ_CACHE"
	},
	{
		no: 6,
		name: "UPPER_WRITE_CACHE"
	},
	{
		no: 7,
		name: "LOWER_WRITE_CACHE"
	}
]);
var BlockRef = createMessageType({
	typeName: "block.BlockRef",
	fields: [{
		no: 1,
		name: "hash",
		kind: "message",
		T: () => Hash
	}],
	packedByDefault: true
});
var PutOpts = createMessageType({
	typeName: "block.PutOpts",
	fields: [{
		no: 1,
		name: "hash_type",
		kind: "enum",
		T: HashType_Enum
	}, {
		no: 2,
		name: "force_block_ref",
		kind: "message",
		T: () => BlockRef
	}],
	packedByDefault: true
});
//#endregion
//#region vendor/github.com/aperturerobotics/controllerbus/controller/configset/proto/configset.pb.ts
var ControllerConfig = createMessageType({
	typeName: "configset.proto.ControllerConfig",
	fields: [
		{
			no: 1,
			name: "id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "rev",
			kind: "scalar",
			T: ScalarType.UINT64
		},
		{
			no: 3,
			name: "config",
			kind: "scalar",
			T: ScalarType.BYTES
		}
	],
	packedByDefault: true
});
createMessageType({
	typeName: "configset.proto.ConfigSet",
	fields: [{
		no: 1,
		name: "configs",
		kind: "map",
		K: ScalarType.STRING,
		V: {
			kind: "message",
			T: () => ControllerConfig
		}
	}],
	packedByDefault: true
});
//#endregion
//#region node_modules/@aptre/protobuf-es-lite/dist/google/protobuf/timestamp.pb.js
var Timestamp_Wkt = {
	fromJson(json) {
		if (typeof json !== "string") throw new Error(`cannot decode google.protobuf.Timestamp(json)}`);
		const matches = json.match(/^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:Z|\.([0-9]{3,9})Z|([+-][0-9][0-9]:[0-9][0-9]))$/);
		if (!matches) throw new Error(`cannot decode google.protobuf.Timestamp from JSON: invalid RFC 3339 string`);
		const ms = Date.parse(matches[1] + "-" + matches[2] + "-" + matches[3] + "T" + matches[4] + ":" + matches[5] + ":" + matches[6] + (matches[8] ? matches[8] : "Z"));
		if (Number.isNaN(ms)) throw new Error(`cannot decode google.protobuf.Timestamp from JSON: invalid RFC 3339 string`);
		if (ms < Date.parse("0001-01-01T00:00:00Z") || ms > Date.parse("9999-12-31T23:59:59Z")) throw new Error(`cannot decode message google.protobuf.Timestamp from JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
		return {
			seconds: protoInt64.parse(ms / 1e3),
			nanos: !matches[7] ? 0 : parseInt("1" + matches[7] + "0".repeat(9 - matches[7].length)) - 1e9
		};
	},
	toJson(msg) {
		const ms = Number(msg.seconds) * 1e3;
		if (ms < Date.parse("0001-01-01T00:00:00Z") || ms > Date.parse("9999-12-31T23:59:59Z")) throw new Error(`cannot encode google.protobuf.Timestamp to JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
		if (msg.nanos != null && msg.nanos < 0) throw new Error(`cannot encode google.protobuf.Timestamp to JSON: nanos must not be negative`);
		let z = "Z";
		if (msg.nanos != null && msg.nanos > 0) {
			const nanosStr = (msg.nanos + 1e9).toString().substring(1);
			if (nanosStr.substring(3) === "000000") z = "." + nanosStr.substring(0, 3) + "Z";
			else if (nanosStr.substring(6) === "000") z = "." + nanosStr.substring(0, 6) + "Z";
			else z = "." + nanosStr + "Z";
		}
		return new Date(ms).toISOString().replace(".000Z", z);
	},
	toDate(msg) {
		if (!msg?.seconds && !msg?.nanos) return null;
		return new Date(Number(msg.seconds ?? 0) * 1e3 + Math.ceil((msg.nanos ?? 0) / 1e6));
	},
	fromDate(value) {
		if (value == null) return {};
		const ms = value.getTime();
		const seconds = Math.floor(ms / 1e3);
		const nanos = ms % 1e3 * 1e6;
		return {
			seconds: protoInt64.parse(seconds),
			nanos
		};
	},
	equals(a, b) {
		const aDate = a instanceof Date ? a : Timestamp_Wkt.toDate(a);
		const bDate = b instanceof Date ? b : Timestamp_Wkt.toDate(b);
		if (aDate === bDate) return true;
		if (aDate == null || bDate == null) return aDate === bDate;
		return +aDate === +bDate;
	}
};
var Timestamp = createMessageType({
	typeName: "google.protobuf.Timestamp",
	fields: [{
		no: 1,
		name: "seconds",
		kind: "scalar",
		T: ScalarType.INT64
	}, {
		no: 2,
		name: "nanos",
		kind: "scalar",
		T: ScalarType.INT32
	}],
	packedByDefault: true,
	fieldWrapper: {
		wrapField(value) {
			if (value == null || value instanceof Date) return Timestamp_Wkt.fromDate(value);
			return Timestamp.createComplete(value);
		},
		unwrapField(msg) {
			return Timestamp_Wkt.toDate(msg);
		}
	}
}, Timestamp_Wkt);
//#endregion
//#region vendor/github.com/aperturerobotics/hydra/block/transform/transform.pb.ts
var StepConfig = createMessageType({
	typeName: "block.transform.StepConfig",
	fields: [{
		no: 1,
		name: "id",
		kind: "scalar",
		T: ScalarType.STRING
	}, {
		no: 2,
		name: "config",
		kind: "scalar",
		T: ScalarType.BYTES
	}],
	packedByDefault: true
});
var Config$1 = createMessageType({
	typeName: "block.transform.Config",
	fields: [{
		no: 1,
		name: "steps",
		kind: "message",
		T: () => StepConfig,
		repeated: true
	}],
	packedByDefault: true
});
//#endregion
//#region vendor/github.com/aperturerobotics/hydra/bucket/bucket.pb.ts
var ReconcilerConfig = createMessageType({
	typeName: "bucket.ReconcilerConfig",
	fields: [
		{
			no: 1,
			name: "id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "controller",
			kind: "message",
			T: () => ControllerConfig
		},
		{
			no: 3,
			name: "filter_put",
			kind: "scalar",
			T: ScalarType.BOOL
		}
	],
	packedByDefault: true
});
var LookupConfig = createMessageType({
	typeName: "bucket.LookupConfig",
	fields: [{
		no: 1,
		name: "disable",
		kind: "scalar",
		T: ScalarType.BOOL
	}, {
		no: 2,
		name: "controller",
		kind: "message",
		T: () => ControllerConfig
	}],
	packedByDefault: true
});
var Config = createMessageType({
	typeName: "bucket.Config",
	fields: [
		{
			no: 1,
			name: "id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "rev",
			kind: "scalar",
			T: ScalarType.UINT32
		},
		{
			no: 3,
			name: "reconcilers",
			kind: "message",
			T: () => ReconcilerConfig,
			repeated: true
		},
		{
			no: 4,
			name: "put_opts",
			kind: "message",
			T: () => PutOpts
		},
		{
			no: 5,
			name: "lookup",
			kind: "message",
			T: () => LookupConfig
		}
	],
	packedByDefault: true
});
var BucketInfo = createMessageType({
	typeName: "bucket.BucketInfo",
	fields: [{
		no: 1,
		name: "config",
		kind: "message",
		T: () => Config
	}],
	packedByDefault: true
});
createMessageType({
	typeName: "bucket.ApplyBucketConfigResult",
	fields: [
		{
			no: 1,
			name: "volume_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "bucket_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "bucket_conf",
			kind: "message",
			T: () => Config
		},
		{
			no: 4,
			name: "old_bucket_conf",
			kind: "message",
			T: () => Config
		},
		{
			no: 5,
			name: "timestamp",
			kind: "message",
			T: () => Timestamp
		},
		{
			no: 6,
			name: "updated",
			kind: "scalar",
			T: ScalarType.BOOL
		},
		{
			no: 7,
			name: "error",
			kind: "scalar",
			T: ScalarType.STRING
		}
	],
	packedByDefault: true
});
var ObjectRef = createMessageType({
	typeName: "bucket.ObjectRef",
	fields: [
		{
			no: 1,
			name: "root_ref",
			kind: "message",
			T: () => BlockRef
		},
		{
			no: 2,
			name: "bucket_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "transform_conf_ref",
			kind: "message",
			T: () => BlockRef
		},
		{
			no: 4,
			name: "transform_conf",
			kind: "message",
			T: () => Config$1
		}
	],
	packedByDefault: true
});
createMessageType({
	typeName: "bucket.BucketOpArgs",
	fields: [{
		no: 1,
		name: "bucket_id",
		kind: "scalar",
		T: ScalarType.STRING
	}, {
		no: 2,
		name: "volume_id",
		kind: "scalar",
		T: ScalarType.STRING
	}],
	packedByDefault: true
});
//#endregion
//#region manifest/manifest.pb.ts
var ManifestMeta = createMessageType({
	typeName: "bldr.manifest.ManifestMeta",
	fields: [
		{
			no: 1,
			name: "manifest_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "build_type",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "platform_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 4,
			name: "rev",
			kind: "scalar",
			T: ScalarType.UINT64
		},
		{
			no: 5,
			name: "description",
			kind: "scalar",
			T: ScalarType.STRING
		}
	],
	packedByDefault: true
});
var Manifest = createMessageType({
	typeName: "bldr.manifest.Manifest",
	fields: [
		{
			no: 1,
			name: "meta",
			kind: "message",
			T: () => ManifestMeta
		},
		{
			no: 2,
			name: "entrypoint",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "dist_fs_ref",
			kind: "message",
			T: () => BlockRef
		},
		{
			no: 4,
			name: "assets_fs_ref",
			kind: "message",
			T: () => BlockRef
		}
	],
	packedByDefault: true
});
var ManifestRef = createMessageType({
	typeName: "bldr.manifest.ManifestRef",
	fields: [{
		no: 1,
		name: "meta",
		kind: "message",
		T: () => ManifestMeta
	}, {
		no: 2,
		name: "manifest_ref",
		kind: "message",
		T: () => ObjectRef
	}],
	packedByDefault: true
});
createMessageType({
	typeName: "bldr.manifest.ManifestBundle",
	fields: [{
		no: 1,
		name: "manifest_refs",
		kind: "message",
		T: () => ManifestRef,
		repeated: true
	}, {
		no: 2,
		name: "timestamp",
		kind: "message",
		T: () => Timestamp
	}],
	packedByDefault: true
});
createMessageType({
	typeName: "bldr.manifest.ManifestSnapshot",
	fields: [{
		no: 1,
		name: "manifest_ref",
		kind: "message",
		T: () => ObjectRef
	}, {
		no: 2,
		name: "manifest",
		kind: "message",
		T: () => Manifest
	}],
	packedByDefault: true
});
createMessageType({
	typeName: "bldr.manifest.FetchManifestRequest",
	fields: [
		{
			no: 1,
			name: "manifest_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "build_types",
			kind: "scalar",
			T: ScalarType.STRING,
			repeated: true
		},
		{
			no: 3,
			name: "platform_ids",
			kind: "scalar",
			T: ScalarType.STRING,
			repeated: true
		},
		{
			no: 4,
			name: "rev",
			kind: "scalar",
			T: ScalarType.UINT64
		}
	],
	packedByDefault: true
});
var FetchManifestValue = createMessageType({
	typeName: "bldr.manifest.FetchManifestValue",
	fields: [{
		no: 1,
		name: "manifest_refs",
		kind: "message",
		T: () => ManifestRef,
		repeated: true
	}],
	packedByDefault: true
});
createMessageType({
	typeName: "bldr.manifest.FetchManifestResponse",
	fields: [
		{
			no: 1,
			name: "value_id",
			kind: "scalar",
			T: ScalarType.UINT32
		},
		{
			no: 2,
			name: "value",
			kind: "message",
			T: () => FetchManifestValue
		},
		{
			no: 3,
			name: "removed",
			kind: "scalar",
			T: ScalarType.BOOL
		},
		{
			no: 4,
			name: "idle",
			kind: "scalar",
			T: ScalarType.UINT32
		}
	],
	packedByDefault: true
});
//#endregion
//#region vendor/github.com/aperturerobotics/controllerbus/controller/controller.pb.ts
var Info = createMessageType({
	typeName: "controller.Info",
	fields: [
		{
			no: 1,
			name: "id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "version",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "description",
			kind: "scalar",
			T: ScalarType.STRING
		}
	],
	packedByDefault: true
});
//#endregion
//#region vendor/github.com/aperturerobotics/hydra/volume/volume.pb.ts
var VolumeInfo = createMessageType({
	typeName: "volume.VolumeInfo",
	fields: [
		{
			no: 1,
			name: "volume_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "peer_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "peer_pub",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 4,
			name: "controller_info",
			kind: "message",
			T: () => Info
		},
		{
			no: 5,
			name: "hash_type",
			kind: "enum",
			T: HashType_Enum
		}
	],
	packedByDefault: true
});
createMessageType({
	typeName: "volume.StorageStats",
	fields: [{
		no: 1,
		name: "total_bytes",
		kind: "scalar",
		T: ScalarType.UINT64
	}, {
		no: 2,
		name: "block_count",
		kind: "scalar",
		T: ScalarType.UINT64
	}],
	packedByDefault: true
});
createMessageType({
	typeName: "volume.VolumeBucketInfo",
	fields: [{
		no: 1,
		name: "bucket_info",
		kind: "message",
		T: () => BucketInfo
	}, {
		no: 2,
		name: "volume_info",
		kind: "message",
		T: () => VolumeInfo
	}],
	packedByDefault: true
});
createMessageType({
	typeName: "volume.ListBucketsRequest",
	fields: [
		{
			no: 1,
			name: "bucket_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "volume_id_re",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "volume_id_list",
			kind: "scalar",
			T: ScalarType.STRING,
			repeated: true
		}
	],
	packedByDefault: true
});
//#endregion
//#region plugin/plugin.pb.ts
var PluginStatus = createMessageType({
	typeName: "bldr.plugin.PluginStatus",
	fields: [{
		no: 1,
		name: "plugin_id",
		kind: "scalar",
		T: ScalarType.STRING
	}, {
		no: 2,
		name: "running",
		kind: "scalar",
		T: ScalarType.BOOL
	}],
	packedByDefault: true
});
createMessageType({
	typeName: "bldr.plugin.GetPluginInfoRequest",
	fields: [],
	packedByDefault: true
});
createMessageType({
	typeName: "bldr.plugin.GetPluginInfoResponse",
	fields: [
		{
			no: 1,
			name: "plugin_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "manifest_ref",
			kind: "message",
			T: () => ManifestRef
		},
		{
			no: 3,
			name: "host_volume_info",
			kind: "message",
			T: () => VolumeInfo
		}
	],
	packedByDefault: true
});
createMessageType({
	typeName: "bldr.plugin.LoadPluginRequest",
	fields: [{
		no: 1,
		name: "plugin_id",
		kind: "scalar",
		T: ScalarType.STRING
	}, {
		no: 2,
		name: "instance_key",
		kind: "scalar",
		T: ScalarType.STRING
	}],
	packedByDefault: true
});
createMessageType({
	typeName: "bldr.plugin.LoadPluginResponse",
	fields: [{
		no: 1,
		name: "plugin_status",
		kind: "message",
		T: () => PluginStatus
	}],
	packedByDefault: true
});
var PluginMeta = createMessageType({
	typeName: "bldr.plugin.PluginMeta",
	fields: [
		{
			no: 1,
			name: "project_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "plugin_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "platform_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 4,
			name: "build_type",
			kind: "scalar",
			T: ScalarType.STRING
		}
	],
	packedByDefault: true
});
var PluginStartInfo = createMessageType({
	typeName: "bldr.plugin.PluginStartInfo",
	fields: [
		{
			no: 1,
			name: "instance_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "plugin_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "instance_key",
			kind: "scalar",
			T: ScalarType.STRING
		}
	],
	packedByDefault: true
});
createMessageType({
	typeName: "bldr.plugin.PluginContextInfo",
	fields: [{
		no: 1,
		name: "plugin_meta",
		kind: "message",
		T: () => PluginMeta
	}],
	packedByDefault: true
});
//#endregion
//#region web/runtime/runtime.pb.ts
/**
* WebRuntimeClientType is the set of client types for a WebRuntime.
*
* @generated from enum web.runtime.WebRuntimeClientType
*/
var WebRuntimeClientType = /* @__PURE__ */ function(WebRuntimeClientType) {
	/**
	* WebRuntimeClientType_UNKNOWN is the unknown type.
	*
	* @generated from enum value: WebRuntimeClientType_UNKNOWN = 0;
	*/
	WebRuntimeClientType[WebRuntimeClientType["WebRuntimeClientType_UNKNOWN"] = 0] = "WebRuntimeClientType_UNKNOWN";
	/**
	* WebRuntimeClientType_WEB_DOCUMENT is the WebDocument type.
	*
	* @generated from enum value: WebRuntimeClientType_WEB_DOCUMENT = 1;
	*/
	WebRuntimeClientType[WebRuntimeClientType["WebRuntimeClientType_WEB_DOCUMENT"] = 1] = "WebRuntimeClientType_WEB_DOCUMENT";
	/**
	* WebRuntimeClientType_SERVICE_WORKER is the ServiceWorker type.
	*
	* @generated from enum value: WebRuntimeClientType_SERVICE_WORKER = 2;
	*/
	WebRuntimeClientType[WebRuntimeClientType["WebRuntimeClientType_SERVICE_WORKER"] = 2] = "WebRuntimeClientType_SERVICE_WORKER";
	/**
	* WebRuntimeClientType_WEB_WORKER is the WebWorker type.
	*
	* @generated from enum value: WebRuntimeClientType_WEB_WORKER = 3;
	*/
	WebRuntimeClientType[WebRuntimeClientType["WebRuntimeClientType_WEB_WORKER"] = 3] = "WebRuntimeClientType_WEB_WORKER";
	return WebRuntimeClientType;
}({});
var WebRuntimeClientType_Enum = createEnumType("web.runtime.WebRuntimeClientType", [
	{
		no: 0,
		name: "WebRuntimeClientType_UNKNOWN"
	},
	{
		no: 1,
		name: "WebRuntimeClientType_WEB_DOCUMENT"
	},
	{
		no: 2,
		name: "WebRuntimeClientType_SERVICE_WORKER"
	},
	{
		no: 3,
		name: "WebRuntimeClientType_WEB_WORKER"
	}
]);
createEnumType("web.runtime.WebRenderer", [
	{
		no: 0,
		name: "WEB_RENDERER_DEFAULT"
	},
	{
		no: 1,
		name: "WEB_RENDERER_ELECTRON"
	},
	{
		no: 2,
		name: "WEB_RENDERER_SAUCER"
	}
]);
createMessageType({
	typeName: "web.runtime.WebRuntimeHostInit",
	fields: [{
		no: 1,
		name: "web_runtime_id",
		kind: "scalar",
		T: ScalarType.STRING
	}],
	packedByDefault: true
});
var WatchWebRuntimeStatusRequest = createMessageType({
	typeName: "web.runtime.WatchWebRuntimeStatusRequest",
	fields: [],
	packedByDefault: true
});
var WebDocumentStatus = createMessageType({
	typeName: "web.runtime.WebDocumentStatus",
	fields: [
		{
			no: 1,
			name: "id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "deleted",
			kind: "scalar",
			T: ScalarType.BOOL
		},
		{
			no: 3,
			name: "permanent",
			kind: "scalar",
			T: ScalarType.BOOL
		}
	],
	packedByDefault: true
});
var WebRuntimeStatus = createMessageType({
	typeName: "web.runtime.WebRuntimeStatus",
	fields: [
		{
			no: 1,
			name: "snapshot",
			kind: "scalar",
			T: ScalarType.BOOL
		},
		{
			no: 2,
			name: "web_documents",
			kind: "message",
			T: () => WebDocumentStatus,
			repeated: true
		},
		{
			no: 3,
			name: "closed",
			kind: "scalar",
			T: ScalarType.BOOL
		}
	],
	packedByDefault: true
});
var CreateWebDocumentRequest = createMessageType({
	typeName: "web.runtime.CreateWebDocumentRequest",
	fields: [{
		no: 1,
		name: "id",
		kind: "scalar",
		T: ScalarType.STRING
	}],
	packedByDefault: true
});
var CreateWebDocumentResponse = createMessageType({
	typeName: "web.runtime.CreateWebDocumentResponse",
	fields: [{
		no: 1,
		name: "created",
		kind: "scalar",
		T: ScalarType.BOOL
	}],
	packedByDefault: true
});
var RemoveWebDocumentRequest = createMessageType({
	typeName: "web.runtime.RemoveWebDocumentRequest",
	fields: [{
		no: 1,
		name: "id",
		kind: "scalar",
		T: ScalarType.STRING
	}],
	packedByDefault: true
});
var RemoveWebDocumentResponse = createMessageType({
	typeName: "web.runtime.RemoveWebDocumentResponse",
	fields: [{
		no: 1,
		name: "removed",
		kind: "scalar",
		T: ScalarType.BOOL
	}],
	packedByDefault: true
});
var WebRuntimeClientInit = createMessageType({
	typeName: "web.runtime.WebRuntimeClientInit",
	fields: [
		{
			no: 1,
			name: "web_runtime_id",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 2,
			name: "client_uuid",
			kind: "scalar",
			T: ScalarType.STRING
		},
		{
			no: 3,
			name: "client_type",
			kind: "enum",
			T: WebRuntimeClientType_Enum
		},
		{
			no: 4,
			name: "disable_web_locks",
			kind: "scalar",
			T: ScalarType.BOOL
		}
	],
	packedByDefault: true
});
//#endregion
//#region web/bldr/timeout.ts
function timeoutPromise(dur) {
	return new Promise((resolve) => {
		setTimeout(resolve, dur);
	});
}
//#endregion
//#region vendor/github.com/aperturerobotics/starpc/rpcstream/rpcstream.pb.ts
var RpcStreamInit = createMessageType({
	typeName: "rpcstream.RpcStreamInit",
	fields: [{
		no: 1,
		name: "component_id",
		kind: "scalar",
		T: ScalarType.STRING
	}],
	packedByDefault: true
});
var RpcAck = createMessageType({
	typeName: "rpcstream.RpcAck",
	fields: [{
		no: 1,
		name: "error",
		kind: "scalar",
		T: ScalarType.STRING
	}],
	packedByDefault: true
});
var RpcStreamPacket = createMessageType({
	typeName: "rpcstream.RpcStreamPacket",
	fields: [
		{
			no: 1,
			name: "init",
			kind: "message",
			T: () => RpcStreamInit,
			oneof: "body"
		},
		{
			no: 2,
			name: "ack",
			kind: "message",
			T: () => RpcAck,
			oneof: "body"
		},
		{
			no: 3,
			name: "data",
			kind: "scalar",
			T: ScalarType.BYTES,
			oneof: "body"
		}
	],
	packedByDefault: true
});
({
	typeName: "web.runtime.WebRuntimeHost",
	methods: {
		WebDocumentRpc: {
			name: "WebDocumentRpc",
			I: RpcStreamPacket,
			O: RpcStreamPacket,
			kind: MethodKind.BiDiStreaming
		},
		ServiceWorkerRpc: {
			name: "ServiceWorkerRpc",
			I: RpcStreamPacket,
			O: RpcStreamPacket,
			kind: MethodKind.BiDiStreaming
		},
		WebWorkerRpc: {
			name: "WebWorkerRpc",
			I: RpcStreamPacket,
			O: RpcStreamPacket,
			kind: MethodKind.BiDiStreaming
		}
	}
}).typeName;
({
	typeName: "web.runtime.WebRuntime",
	methods: {
		WatchWebRuntimeStatus: {
			name: "WatchWebRuntimeStatus",
			I: WatchWebRuntimeStatusRequest,
			O: WebRuntimeStatus,
			kind: MethodKind.ServerStreaming
		},
		CreateWebDocument: {
			name: "CreateWebDocument",
			I: CreateWebDocumentRequest,
			O: CreateWebDocumentResponse,
			kind: MethodKind.Unary
		},
		RemoveWebDocument: {
			name: "RemoveWebDocument",
			I: RemoveWebDocumentRequest,
			O: RemoveWebDocumentResponse,
			kind: MethodKind.Unary
		},
		WebDocumentRpc: {
			name: "WebDocumentRpc",
			I: RpcStreamPacket,
			O: RpcStreamPacket,
			kind: MethodKind.BiDiStreaming
		},
		WebWorkerRpc: {
			name: "WebWorkerRpc",
			I: RpcStreamPacket,
			O: RpcStreamPacket,
			kind: MethodKind.BiDiStreaming
		}
	}
}).typeName;
//#endregion
//#region web/bldr/web-runtime.ts
var WebRuntimeClientChannelStreamOpts = {
	keepAliveMs: 12420,
	idleTimeoutMs: 60500
};
//#endregion
//#region web/bldr/web-runtime-client.ts
var WebRuntimeClient = class {
	rpcClient;
	clientChannel;
	constructor(webRuntimeId, clientId, clientType, openClientCh, handleIncomingStream, handleDisconnected, disableWebLocks) {
		this.webRuntimeId = webRuntimeId;
		this.clientId = clientId;
		this.clientType = clientType;
		this.openClientCh = openClientCh;
		this.handleIncomingStream = handleIncomingStream;
		this.handleDisconnected = handleDisconnected;
		this.disableWebLocks = disableWebLocks;
		this.rpcClient = new Client(this.openStream.bind(this));
	}
	async waitConn() {
		await this.openClientChannel();
	}
	async openStream() {
		let err;
		for (let attempt = 0; attempt < 3; attempt++) {
			const clientPort = await this.openClientChannel();
			const streamChannel = new MessageChannel();
			const streamConn = new ChannelStream(this.clientId, streamChannel.port1, WebRuntimeClientChannelStreamOpts);
			clientPort.postMessage({ openStream: true }, [streamChannel.port2]);
			await Promise.race([streamConn.waitRemoteOpen, timeoutPromise(1500)]);
			if (!streamConn.isOpen) {
				streamConn.close();
				const msg = `WebRuntimeClient: ${this.clientId}: timeout opening stream with host`;
				err = new Error(msg);
				console.warn(msg);
				if (this.clientChannel === clientPort) {
					this.clientChannel.close();
					this.clientChannel = void 0;
					if (this.handleDisconnected) await this.handleDisconnected(err);
				}
				await timeoutPromise(100);
				continue;
			}
			return streamConn;
		}
		err = /* @__PURE__ */ new Error(`WebRuntimeClient: ${this.clientId}: unable to open stream with host${err ? ": " + err : ""}`);
		console.warn(err.message);
		throw err;
	}
	close() {
		if (this.clientChannel) {
			this.clientChannel.postMessage({ close: true });
			this.clientChannel.close();
			this.clientChannel = void 0;
			if (this.handleDisconnected) this.handleDisconnected().catch(() => {});
		}
	}
	async openClientChannel() {
		if (this.clientChannel) return this.clientChannel;
		const port = await this.openClientCh({
			webRuntimeId: this.webRuntimeId,
			clientUuid: this.clientId,
			clientType: this.clientType,
			disableWebLocks: this.disableWebLocks
		});
		if (!await Promise.race([new Promise((resolve) => {
			port.onmessage = (ev) => {
				const data = ev.data;
				if (typeof data === "object" && data.connected) resolve(true);
			};
			port.start();
		}), timeoutPromise(3e3).then(() => false)])) {
			port.close();
			throw new Error(`WebRuntimeClient: ${this.clientId}: timeout waiting for runtime connected ack`);
		}
		port.onmessage = (ev) => {
			const data = ev.data;
			if (typeof data !== "object") return;
			this.handleMessage(data, ev.ports);
		};
		this.clientChannel = port;
		if (!this.disableWebLocks) port.postMessage({ armWebLock: true });
		return port;
	}
	async handleMessage(msg, ports) {
		if (msg.openStream && ports && ports.length) await this.handleWebRuntimeOpenStream(ports[0]);
	}
	async handleWebRuntimeOpenStream(remoteMsgPort) {
		const channel = new ChannelStream(this.clientId, remoteMsgPort, {
			...WebRuntimeClientChannelStreamOpts,
			remoteOpen: true
		});
		let err;
		if (!this.handleIncomingStream) err = /* @__PURE__ */ new Error(`${this.clientType.toString()}: handle stream: not implemented`);
		else try {
			await this.handleIncomingStream(channel);
		} catch (e) {
			err = castToError(e, `${this.clientType.toString()}: handle stream: unknown error`);
		}
		if (err) {
			console.error(err.message);
			channel.close(err);
			return;
		}
	}
};
//#endregion
//#region web/bldr/web-document-tracker.ts
var openViaWebDocumentTimeoutMs = 1e3;
var waitForNextWebDocumentTimeoutMs = 3e3;
var WebDocumentTracker = class {
	clientUuid;
	clientType;
	webRuntimeClient;
	webDocuments = {};
	webDocumentWaiters = [];
	lastWebDocumentIdx = 0;
	lastWebDocumentId;
	constructor(clientUuid, clientType, onWebDocumentsExhausted, handleIncomingStream, onAllWebDocumentsClosed) {
		this.onWebDocumentsExhausted = onWebDocumentsExhausted;
		this.onAllWebDocumentsClosed = onAllWebDocumentsClosed;
		this.clientUuid = clientUuid;
		this.clientType = clientType;
		this.webRuntimeClient = new WebRuntimeClient("", clientUuid, clientType, this.openWebRuntimeClient.bind(this), handleIncomingStream, null);
	}
	async waitConn() {
		return this.webRuntimeClient.waitConn();
	}
	handleWebDocumentMessage(msg) {
		if (typeof msg !== "object" || !msg.from || !msg.initPort) return;
		const { from: webDocumentId, initPort: port } = msg;
		console.log(`WebDocumentTracker: ${this.clientUuid}: added WebDocument: ${webDocumentId}`);
		this.webDocuments[webDocumentId] = port;
		port.onmessage = (ev) => {
			const data = ev.data;
			if (typeof data !== "object") return;
			if (data.close) (async () => {
				const closePort = this.webDocuments[webDocumentId];
				if (closePort) {
					closePort.close();
					console.log(`WebDocumentTracker: ${this.clientUuid}: removed WebDocument: ${webDocumentId}`);
					delete this.webDocuments[webDocumentId];
					if (this.lastWebDocumentId === webDocumentId) {
						this.lastWebDocumentId = void 0;
						this.lastWebDocumentIdx = 0;
						this.webRuntimeClient.close();
					}
					if (!Object.keys(this.webDocuments).length && this.onAllWebDocumentsClosed) await this.onAllWebDocumentsClosed();
				}
			})().catch((err) => {
				console.error(`WebDocumentTracker: ${this.clientUuid}: error handling WebDocument close:`, err);
			});
		};
		const waiters = this.webDocumentWaiters.splice(0);
		for (const waiter of waiters) waiter.resume();
		port.start();
	}
	close() {
		const msg = {
			from: this.clientUuid,
			close: true
		};
		for (const docID in this.webDocuments) {
			this.webDocuments[docID].postMessage(msg);
			delete this.webDocuments[docID];
		}
		delete this.lastWebDocumentId;
		this.rejectWaiters(/* @__PURE__ */ new Error(`WebDocumentTracker: ${this.clientUuid}: closed while waiting for WebDocument`));
	}
	postMessage(msg) {
		for (const docID in this.webDocuments) this.webDocuments[docID]?.postMessage(msg);
	}
	async openWebRuntimeClient(initMsg) {
		const init = WebRuntimeClientInit.toBinary(initMsg);
		const webDocumentIds = Object.keys(this.webDocuments);
		for (let i = 0; i < webDocumentIds.length; i++) {
			const x = (i + this.lastWebDocumentIdx + 1) % webDocumentIds.length;
			const webDocumentId = webDocumentIds[x];
			const webDocumentPort = this.webDocuments[webDocumentId];
			if (!webDocumentPort) {
				delete this.webDocuments[webDocumentId];
				continue;
			}
			const ackChannel = new MessageChannel();
			const ackPromise = new Promise((resolve) => {
				const ackPort = ackChannel.port1;
				ackPort.onmessage = (ev) => {
					const data = ev.data;
					if (!data || !data.from) return;
					resolve(data);
				};
				ackPort.start();
			});
			const lockAbortController = new AbortController();
			const disconnectedPromise = this.waitForWebDocumentDisconnect(webDocumentId, lockAbortController.signal);
			try {
				console.log(`WebDocumentTracker: ${this.clientUuid}: connecting via WebDocument: ${webDocumentId}`);
				const connectMsg = {
					from: this.clientUuid,
					connectWebRuntime: {
						init,
						port: ackChannel.port2
					}
				};
				webDocumentPort.postMessage(connectMsg, [ackChannel.port2]);
				const result = await Promise.race([
					ackPromise,
					disconnectedPromise,
					timeoutPromise(openViaWebDocumentTimeoutMs)
				]);
				if (!result) throw new Error("timed out waiting for ack from WebDocument");
				if (result instanceof Error) throw result;
				console.log(`WebDocumentTracker: ${this.clientUuid}: opened port with WebRuntime via WebDocument: ${webDocumentId}`);
				this.lastWebDocumentIdx = x;
				this.lastWebDocumentId = webDocumentId;
				return result.webRuntimePort;
			} catch (err) {
				console.error(`ServiceWorker: connecting via WebDocument failed: ${webDocumentId}`, err);
				delete this.webDocuments[webDocumentId];
				continue;
			} finally {
				lockAbortController.abort();
			}
		}
		const waitPromise = new Promise((resolve, reject) => {
			this.webDocumentWaiters.push({
				resume: () => {
					resolve(this.openWebRuntimeClient(initMsg));
				},
				reject
			});
		});
		await this.onWebDocumentsExhausted();
		console.log("ServiceWorker: waiting for next WebDocument to proxy conn");
		return Promise.race([waitPromise, timeoutPromise(waitForNextWebDocumentTimeoutMs).then(() => {
			throw new Error("timed out waiting for next WebDocument to proxy conn");
		})]);
	}
	waitForWebDocumentDisconnect(webDocumentId, signal) {
		if (typeof navigator === "undefined" || !("locks" in navigator)) return new Promise(() => {});
		return navigator.locks.request(`bldr-doc-${webDocumentId}`, { signal }, () => {
			return /* @__PURE__ */ new Error(`WebDocumentTracker: ${this.clientUuid}: WebDocument ${webDocumentId} disconnected before ack`);
		}).catch(() => void 0);
	}
	rejectWaiters(err) {
		const waiters = this.webDocumentWaiters.splice(0);
		for (const waiter of waiters) waiter.reject(err);
	}
};
//#endregion
export { PluginStartInfo as i, timeoutPromise as n, WebRuntimeClientType as r, WebDocumentTracker as t };

//# sourceMappingURL=web-document-tracker-KAfasl-R.js.map