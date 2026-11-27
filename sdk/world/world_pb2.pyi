import datetime

from db.block import block_pb2 as _block_pb2
from db.bucket import bucket_pb2 as _bucket_pb2
from db.block.quad import quad_pb2 as _quad_pb2
from core.sobject import sobject_pb2 as _sobject_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class GraphEdgeBucketDirection(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    GRAPH_EDGE_BUCKET_DIRECTION_UNSPECIFIED: _ClassVar[GraphEdgeBucketDirection]
    GRAPH_EDGE_BUCKET_DIRECTION_OUT: _ClassVar[GraphEdgeBucketDirection]
    GRAPH_EDGE_BUCKET_DIRECTION_IN: _ClassVar[GraphEdgeBucketDirection]
    GRAPH_EDGE_BUCKET_DIRECTION_BOTH: _ClassVar[GraphEdgeBucketDirection]

class GraphPathDirection(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    GRAPH_PATH_DIRECTION_UNSPECIFIED: _ClassVar[GraphPathDirection]
    GRAPH_PATH_DIRECTION_OUT: _ClassVar[GraphPathDirection]
    GRAPH_PATH_DIRECTION_IN: _ClassVar[GraphPathDirection]
    GRAPH_PATH_DIRECTION_BOTH: _ClassVar[GraphPathDirection]

class WorldErrorCode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    WORLD_ERROR_CODE_UNSPECIFIED: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_UNHANDLED_OP: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_TYPED_OBJECT_GRANT_RETIRED: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_OBJECT_NOT_FOUND: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_PLUGIN_UNAVAILABLE: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_PLUGIN_EXECUTABLE_MISSING: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_SUPERSEDED: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_STALE_GENERATION: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_ROOT_NOT_HELD: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_REPLAY_WAITS: _ClassVar[WorldErrorCode]
    WORLD_ERROR_CODE_DURABILITY_UNKNOWN: _ClassVar[WorldErrorCode]
GRAPH_EDGE_BUCKET_DIRECTION_UNSPECIFIED: GraphEdgeBucketDirection
GRAPH_EDGE_BUCKET_DIRECTION_OUT: GraphEdgeBucketDirection
GRAPH_EDGE_BUCKET_DIRECTION_IN: GraphEdgeBucketDirection
GRAPH_EDGE_BUCKET_DIRECTION_BOTH: GraphEdgeBucketDirection
GRAPH_PATH_DIRECTION_UNSPECIFIED: GraphPathDirection
GRAPH_PATH_DIRECTION_OUT: GraphPathDirection
GRAPH_PATH_DIRECTION_IN: GraphPathDirection
GRAPH_PATH_DIRECTION_BOTH: GraphPathDirection
WORLD_ERROR_CODE_UNSPECIFIED: WorldErrorCode
WORLD_ERROR_CODE_UNHANDLED_OP: WorldErrorCode
WORLD_ERROR_CODE_TYPED_OBJECT_GRANT_RETIRED: WorldErrorCode
WORLD_ERROR_CODE_OBJECT_NOT_FOUND: WorldErrorCode
WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT: WorldErrorCode
WORLD_ERROR_CODE_PLUGIN_UNAVAILABLE: WorldErrorCode
WORLD_ERROR_CODE_PLUGIN_EXECUTABLE_MISSING: WorldErrorCode
WORLD_ERROR_CODE_SUPERSEDED: WorldErrorCode
WORLD_ERROR_CODE_STALE_GENERATION: WorldErrorCode
WORLD_ERROR_CODE_ROOT_NOT_HELD: WorldErrorCode
WORLD_ERROR_CODE_REPLAY_WAITS: WorldErrorCode
WORLD_ERROR_CODE_DURABILITY_UNKNOWN: WorldErrorCode

class ReleaseRootsRequest(_message.Message):
    __slots__ = ("root_refs",)
    ROOT_REFS_FIELD_NUMBER: _ClassVar[int]
    root_refs: _containers.RepeatedCompositeFieldContainer[_block_pb2.BlockRef]
    def __init__(self, root_refs: _Optional[_Iterable[_Union[_block_pb2.BlockRef, _Mapping]]] = ...) -> None: ...

class ReleaseRootsResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class StageWorldStateRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class StageWorldStateResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class SyncRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SyncResponse(_message.Message):
    __slots__ = ("fenced",)
    FENCED_FIELD_NUMBER: _ClassVar[int]
    fenced: bool
    def __init__(self, fenced: _Optional[bool] = ...) -> None: ...

class SyncThroughRequest(_message.Message):
    __slots__ = ("seqno",)
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    seqno: int
    def __init__(self, seqno: _Optional[int] = ...) -> None: ...

class SyncThroughResponse(_message.Message):
    __slots__ = ("durable_seqno",)
    DURABLE_SEQNO_FIELD_NUMBER: _ClassVar[int]
    durable_seqno: int
    def __init__(self, durable_seqno: _Optional[int] = ...) -> None: ...

class EngineInfo(_message.Message):
    __slots__ = ("engine_id", "bucket_id")
    ENGINE_ID_FIELD_NUMBER: _ClassVar[int]
    BUCKET_ID_FIELD_NUMBER: _ClassVar[int]
    engine_id: str
    bucket_id: str
    def __init__(self, engine_id: _Optional[str] = ..., bucket_id: _Optional[str] = ...) -> None: ...

class GetEngineInfoRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetEngineInfoResponse(_message.Message):
    __slots__ = ("engine_info", "session_peer_id", "caller_participant")
    ENGINE_INFO_FIELD_NUMBER: _ClassVar[int]
    SESSION_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    CALLER_PARTICIPANT_FIELD_NUMBER: _ClassVar[int]
    engine_info: EngineInfo
    session_peer_id: str
    caller_participant: _sobject_pb2.SOParticipantConfig
    def __init__(self, engine_info: _Optional[_Union[EngineInfo, _Mapping]] = ..., session_peer_id: _Optional[str] = ..., caller_participant: _Optional[_Union[_sobject_pb2.SOParticipantConfig, _Mapping]] = ...) -> None: ...

class WorldRootSnapshot(_message.Message):
    __slots__ = ("root_ref", "seqno", "engine_info", "storage_volume_id")
    ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    ENGINE_INFO_FIELD_NUMBER: _ClassVar[int]
    STORAGE_VOLUME_ID_FIELD_NUMBER: _ClassVar[int]
    root_ref: _bucket_pb2.ObjectRef
    seqno: int
    engine_info: EngineInfo
    storage_volume_id: str
    def __init__(self, root_ref: _Optional[_Union[_bucket_pb2.ObjectRef, _Mapping]] = ..., seqno: _Optional[int] = ..., engine_info: _Optional[_Union[EngineInfo, _Mapping]] = ..., storage_volume_id: _Optional[str] = ...) -> None: ...

class GetWorldRootSnapshotRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchWorldRootSnapshotsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SetRetainedRootRequest(_message.Message):
    __slots__ = ("name", "root_ref")
    NAME_FIELD_NUMBER: _ClassVar[int]
    ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    name: str
    root_ref: _block_pb2.BlockRef
    def __init__(self, name: _Optional[str] = ..., root_ref: _Optional[_Union[_block_pb2.BlockRef, _Mapping]] = ...) -> None: ...

class SetRetainedRootResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class NewTransactionRequest(_message.Message):
    __slots__ = ("write", "known_error_code")
    WRITE_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    write: bool
    known_error_code: WorldErrorCode
    def __init__(self, write: _Optional[bool] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class NewTransactionResponse(_message.Message):
    __slots__ = ("resource_id", "read_only", "error_code")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    READ_ONLY_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    read_only: bool
    error_code: WorldErrorCode
    def __init__(self, resource_id: _Optional[int] = ..., read_only: _Optional[bool] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class GetSeqnoRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetSeqnoResponse(_message.Message):
    __slots__ = ("seqno",)
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    seqno: int
    def __init__(self, seqno: _Optional[int] = ...) -> None: ...

class WaitSeqnoRequest(_message.Message):
    __slots__ = ("seqno",)
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    seqno: int
    def __init__(self, seqno: _Optional[int] = ...) -> None: ...

class WaitSeqnoResponse(_message.Message):
    __slots__ = ("seqno",)
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    seqno: int
    def __init__(self, seqno: _Optional[int] = ...) -> None: ...

class WatchChangesRequest(_message.Message):
    __slots__ = ("after_seqno", "interest")
    AFTER_SEQNO_FIELD_NUMBER: _ClassVar[int]
    INTEREST_FIELD_NUMBER: _ClassVar[int]
    after_seqno: int
    interest: ChangeInterestDelta
    def __init__(self, after_seqno: _Optional[int] = ..., interest: _Optional[_Union[ChangeInterestDelta, _Mapping]] = ...) -> None: ...

class ChangeInterestDelta(_message.Message):
    __slots__ = ("version", "add_keys", "remove_keys", "add_patterns", "remove_patterns")
    VERSION_FIELD_NUMBER: _ClassVar[int]
    ADD_KEYS_FIELD_NUMBER: _ClassVar[int]
    REMOVE_KEYS_FIELD_NUMBER: _ClassVar[int]
    ADD_PATTERNS_FIELD_NUMBER: _ClassVar[int]
    REMOVE_PATTERNS_FIELD_NUMBER: _ClassVar[int]
    version: int
    add_keys: _containers.RepeatedScalarFieldContainer[str]
    remove_keys: _containers.RepeatedScalarFieldContainer[str]
    add_patterns: _containers.RepeatedCompositeFieldContainer[_quad_pb2.Quad]
    remove_patterns: _containers.RepeatedCompositeFieldContainer[_quad_pb2.Quad]
    def __init__(self, version: _Optional[int] = ..., add_keys: _Optional[_Iterable[str]] = ..., remove_keys: _Optional[_Iterable[str]] = ..., add_patterns: _Optional[_Iterable[_Union[_quad_pb2.Quad, _Mapping]]] = ..., remove_patterns: _Optional[_Iterable[_Union[_quad_pb2.Quad, _Mapping]]] = ...) -> None: ...

class WatchChangesResponse(_message.Message):
    __slots__ = ("change", "interest_ack")
    CHANGE_FIELD_NUMBER: _ClassVar[int]
    INTEREST_ACK_FIELD_NUMBER: _ClassVar[int]
    change: WorldChange
    interest_ack: InterestAck
    def __init__(self, change: _Optional[_Union[WorldChange, _Mapping]] = ..., interest_ack: _Optional[_Union[InterestAck, _Mapping]] = ...) -> None: ...

class InterestAck(_message.Message):
    __slots__ = ("version", "after_seqno")
    VERSION_FIELD_NUMBER: _ClassVar[int]
    AFTER_SEQNO_FIELD_NUMBER: _ClassVar[int]
    version: int
    after_seqno: int
    def __init__(self, version: _Optional[int] = ..., after_seqno: _Optional[int] = ...) -> None: ...

class WorldChange(_message.Message):
    __slots__ = ("seqno", "unknown", "keys", "quads")
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    UNKNOWN_FIELD_NUMBER: _ClassVar[int]
    KEYS_FIELD_NUMBER: _ClassVar[int]
    QUADS_FIELD_NUMBER: _ClassVar[int]
    seqno: int
    unknown: bool
    keys: _containers.RepeatedScalarFieldContainer[str]
    quads: _containers.RepeatedCompositeFieldContainer[_quad_pb2.Quad]
    def __init__(self, seqno: _Optional[int] = ..., unknown: _Optional[bool] = ..., keys: _Optional[_Iterable[str]] = ..., quads: _Optional[_Iterable[_Union[_quad_pb2.Quad, _Mapping]]] = ...) -> None: ...

class WaitObjectRevRequest(_message.Message):
    __slots__ = ("key", "rev", "ignore_not_found")
    KEY_FIELD_NUMBER: _ClassVar[int]
    REV_FIELD_NUMBER: _ClassVar[int]
    IGNORE_NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    key: str
    rev: int
    ignore_not_found: bool
    def __init__(self, key: _Optional[str] = ..., rev: _Optional[int] = ..., ignore_not_found: _Optional[bool] = ...) -> None: ...

class WaitObjectRevResponse(_message.Message):
    __slots__ = ("rev", "error_code")
    REV_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    rev: int
    error_code: WorldErrorCode
    def __init__(self, rev: _Optional[int] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class AccessWorldStateRequest(_message.Message):
    __slots__ = ("ref",)
    REF_FIELD_NUMBER: _ClassVar[int]
    ref: _bucket_pb2.ObjectRef
    def __init__(self, ref: _Optional[_Union[_bucket_pb2.ObjectRef, _Mapping]] = ...) -> None: ...

class OpenNestedWorldRequest(_message.Message):
    __slots__ = ("object_key",)
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    def __init__(self, object_key: _Optional[str] = ...) -> None: ...

class OpenNestedWorldResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class OpenOuterWorldRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class OpenOuterWorldResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class AccessWorldStateResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class CommitRequest(_message.Message):
    __slots__ = ("known_error_code",)
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    known_error_code: WorldErrorCode
    def __init__(self, known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class CommitResponse(_message.Message):
    __slots__ = ("error_code", "error_message")
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    error_code: WorldErrorCode
    error_message: str
    def __init__(self, error_code: _Optional[_Union[WorldErrorCode, str]] = ..., error_message: _Optional[str] = ...) -> None: ...

class DiscardRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class DiscardResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetReadOnlyRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetReadOnlyResponse(_message.Message):
    __slots__ = ("read_only",)
    READ_ONLY_FIELD_NUMBER: _ClassVar[int]
    read_only: bool
    def __init__(self, read_only: _Optional[bool] = ...) -> None: ...

class BuildStorageCursorRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class BuildStorageCursorResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class CreateObjectRequest(_message.Message):
    __slots__ = ("object_key", "root_ref", "known_error_code")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    root_ref: _bucket_pb2.ObjectRef
    known_error_code: WorldErrorCode
    def __init__(self, object_key: _Optional[str] = ..., root_ref: _Optional[_Union[_bucket_pb2.ObjectRef, _Mapping]] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class CreateObjectResponse(_message.Message):
    __slots__ = ("resource_id", "object_key", "error_code")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    object_key: str
    error_code: WorldErrorCode
    def __init__(self, resource_id: _Optional[int] = ..., object_key: _Optional[str] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class GetObjectRequest(_message.Message):
    __slots__ = ("object_key",)
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    def __init__(self, object_key: _Optional[str] = ...) -> None: ...

class GetObjectResponse(_message.Message):
    __slots__ = ("found", "resource_id", "object_key")
    FOUND_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    found: bool
    resource_id: int
    object_key: str
    def __init__(self, found: _Optional[bool] = ..., resource_id: _Optional[int] = ..., object_key: _Optional[str] = ...) -> None: ...

class RenameObjectRequest(_message.Message):
    __slots__ = ("old_object_key", "new_object_key", "descendants", "known_error_code")
    OLD_OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    NEW_OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    DESCENDANTS_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    old_object_key: str
    new_object_key: str
    descendants: bool
    known_error_code: WorldErrorCode
    def __init__(self, old_object_key: _Optional[str] = ..., new_object_key: _Optional[str] = ..., descendants: _Optional[bool] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class RenameObjectResponse(_message.Message):
    __slots__ = ("resource_id", "object_key", "error_code")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    object_key: str
    error_code: WorldErrorCode
    def __init__(self, resource_id: _Optional[int] = ..., object_key: _Optional[str] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class DeleteObjectRequest(_message.Message):
    __slots__ = ("object_key", "known_error_code")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    known_error_code: WorldErrorCode
    def __init__(self, object_key: _Optional[str] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class DeleteObjectResponse(_message.Message):
    __slots__ = ("deleted", "error_code")
    DELETED_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    deleted: bool
    error_code: WorldErrorCode
    def __init__(self, deleted: _Optional[bool] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class IterateObjectsRequest(_message.Message):
    __slots__ = ("prefix", "reversed")
    PREFIX_FIELD_NUMBER: _ClassVar[int]
    REVERSED_FIELD_NUMBER: _ClassVar[int]
    prefix: str
    reversed: bool
    def __init__(self, prefix: _Optional[str] = ..., reversed: _Optional[bool] = ...) -> None: ...

class IterateObjectsResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class ListObjectsRequest(_message.Message):
    __slots__ = ("prefix", "start_after", "limit", "delimiter")
    PREFIX_FIELD_NUMBER: _ClassVar[int]
    START_AFTER_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    DELIMITER_FIELD_NUMBER: _ClassVar[int]
    prefix: str
    start_after: str
    limit: int
    delimiter: str
    def __init__(self, prefix: _Optional[str] = ..., start_after: _Optional[str] = ..., limit: _Optional[int] = ..., delimiter: _Optional[str] = ...) -> None: ...

class ListObjectsResponse(_message.Message):
    __slots__ = ("objects", "more", "prefixes")
    OBJECTS_FIELD_NUMBER: _ClassVar[int]
    MORE_FIELD_NUMBER: _ClassVar[int]
    PREFIXES_FIELD_NUMBER: _ClassVar[int]
    objects: _containers.RepeatedCompositeFieldContainer[ObjectMetadata]
    more: bool
    prefixes: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, objects: _Optional[_Iterable[_Union[ObjectMetadata, _Mapping]]] = ..., more: _Optional[bool] = ..., prefixes: _Optional[_Iterable[str]] = ...) -> None: ...

class SetGraphQuadRequest(_message.Message):
    __slots__ = ("quad", "known_error_code")
    QUAD_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    quad: _quad_pb2.Quad
    known_error_code: WorldErrorCode
    def __init__(self, quad: _Optional[_Union[_quad_pb2.Quad, _Mapping]] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class SetGraphQuadResponse(_message.Message):
    __slots__ = ("error_code",)
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    error_code: WorldErrorCode
    def __init__(self, error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class DeleteGraphQuadRequest(_message.Message):
    __slots__ = ("quad", "known_error_code")
    QUAD_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    quad: _quad_pb2.Quad
    known_error_code: WorldErrorCode
    def __init__(self, quad: _Optional[_Union[_quad_pb2.Quad, _Mapping]] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class DeleteGraphQuadResponse(_message.Message):
    __slots__ = ("error_code",)
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    error_code: WorldErrorCode
    def __init__(self, error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class LookupGraphQuadsRequest(_message.Message):
    __slots__ = ("filter", "limit")
    FILTER_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    filter: _quad_pb2.Quad
    limit: int
    def __init__(self, filter: _Optional[_Union[_quad_pb2.Quad, _Mapping]] = ..., limit: _Optional[int] = ...) -> None: ...

class LookupGraphQuadsResponse(_message.Message):
    __slots__ = ("quads",)
    QUADS_FIELD_NUMBER: _ClassVar[int]
    quads: _containers.RepeatedCompositeFieldContainer[_quad_pb2.Quad]
    def __init__(self, quads: _Optional[_Iterable[_Union[_quad_pb2.Quad, _Mapping]]] = ...) -> None: ...

class LookupGraphQuadsBatchRequest(_message.Message):
    __slots__ = ("filters", "limit_per_filter")
    FILTERS_FIELD_NUMBER: _ClassVar[int]
    LIMIT_PER_FILTER_FIELD_NUMBER: _ClassVar[int]
    filters: _containers.RepeatedCompositeFieldContainer[_quad_pb2.Quad]
    limit_per_filter: int
    def __init__(self, filters: _Optional[_Iterable[_Union[_quad_pb2.Quad, _Mapping]]] = ..., limit_per_filter: _Optional[int] = ...) -> None: ...

class LookupGraphQuadsBatchResult(_message.Message):
    __slots__ = ("quads",)
    QUADS_FIELD_NUMBER: _ClassVar[int]
    quads: _containers.RepeatedCompositeFieldContainer[_quad_pb2.Quad]
    def __init__(self, quads: _Optional[_Iterable[_Union[_quad_pb2.Quad, _Mapping]]] = ...) -> None: ...

class LookupGraphQuadsBatchResponse(_message.Message):
    __slots__ = ("results",)
    RESULTS_FIELD_NUMBER: _ClassVar[int]
    results: _containers.RepeatedCompositeFieldContainer[LookupGraphQuadsBatchResult]
    def __init__(self, results: _Optional[_Iterable[_Union[LookupGraphQuadsBatchResult, _Mapping]]] = ...) -> None: ...

class ListGraphEdgeBucketsRequest(_message.Message):
    __slots__ = ("origin_object_keys", "predicate", "limit_per_origin", "direction", "after", "reverse")
    ORIGIN_OBJECT_KEYS_FIELD_NUMBER: _ClassVar[int]
    PREDICATE_FIELD_NUMBER: _ClassVar[int]
    LIMIT_PER_ORIGIN_FIELD_NUMBER: _ClassVar[int]
    DIRECTION_FIELD_NUMBER: _ClassVar[int]
    AFTER_FIELD_NUMBER: _ClassVar[int]
    REVERSE_FIELD_NUMBER: _ClassVar[int]
    origin_object_keys: _containers.RepeatedScalarFieldContainer[str]
    predicate: str
    limit_per_origin: int
    direction: GraphEdgeBucketDirection
    after: str
    reverse: bool
    def __init__(self, origin_object_keys: _Optional[_Iterable[str]] = ..., predicate: _Optional[str] = ..., limit_per_origin: _Optional[int] = ..., direction: _Optional[_Union[GraphEdgeBucketDirection, str]] = ..., after: _Optional[str] = ..., reverse: _Optional[bool] = ...) -> None: ...

class GraphEdgeBucket(_message.Message):
    __slots__ = ("origin_object_key", "outgoing", "incoming", "outgoing_truncated", "incoming_truncated", "creation_order")
    ORIGIN_OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    OUTGOING_FIELD_NUMBER: _ClassVar[int]
    INCOMING_FIELD_NUMBER: _ClassVar[int]
    OUTGOING_TRUNCATED_FIELD_NUMBER: _ClassVar[int]
    INCOMING_TRUNCATED_FIELD_NUMBER: _ClassVar[int]
    CREATION_ORDER_FIELD_NUMBER: _ClassVar[int]
    origin_object_key: str
    outgoing: _containers.RepeatedCompositeFieldContainer[_quad_pb2.Quad]
    incoming: _containers.RepeatedCompositeFieldContainer[_quad_pb2.Quad]
    outgoing_truncated: bool
    incoming_truncated: bool
    creation_order: bool
    def __init__(self, origin_object_key: _Optional[str] = ..., outgoing: _Optional[_Iterable[_Union[_quad_pb2.Quad, _Mapping]]] = ..., incoming: _Optional[_Iterable[_Union[_quad_pb2.Quad, _Mapping]]] = ..., outgoing_truncated: _Optional[bool] = ..., incoming_truncated: _Optional[bool] = ..., creation_order: _Optional[bool] = ...) -> None: ...

class ListGraphEdgeBucketsResponse(_message.Message):
    __slots__ = ("buckets",)
    BUCKETS_FIELD_NUMBER: _ClassVar[int]
    buckets: _containers.RepeatedCompositeFieldContainer[GraphEdgeBucket]
    def __init__(self, buckets: _Optional[_Iterable[_Union[GraphEdgeBucket, _Mapping]]] = ...) -> None: ...

class ListObjectsWithTypeRequest(_message.Message):
    __slots__ = ("type_id", "start_after", "limit", "declared")
    TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    START_AFTER_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    DECLARED_FIELD_NUMBER: _ClassVar[int]
    type_id: str
    start_after: str
    limit: int
    declared: bool
    def __init__(self, type_id: _Optional[str] = ..., start_after: _Optional[str] = ..., limit: _Optional[int] = ..., declared: _Optional[bool] = ...) -> None: ...

class ListObjectsWithTypeResponse(_message.Message):
    __slots__ = ("object_keys", "more", "titles", "states")
    OBJECT_KEYS_FIELD_NUMBER: _ClassVar[int]
    MORE_FIELD_NUMBER: _ClassVar[int]
    TITLES_FIELD_NUMBER: _ClassVar[int]
    STATES_FIELD_NUMBER: _ClassVar[int]
    object_keys: _containers.RepeatedScalarFieldContainer[str]
    more: bool
    titles: _containers.RepeatedScalarFieldContainer[str]
    states: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, object_keys: _Optional[_Iterable[str]] = ..., more: _Optional[bool] = ..., titles: _Optional[_Iterable[str]] = ..., states: _Optional[_Iterable[str]] = ...) -> None: ...

class TypeCountEntry(_message.Message):
    __slots__ = ("type_id", "count", "internal")
    TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    COUNT_FIELD_NUMBER: _ClassVar[int]
    INTERNAL_FIELD_NUMBER: _ClassVar[int]
    type_id: str
    count: int
    internal: bool
    def __init__(self, type_id: _Optional[str] = ..., count: _Optional[int] = ..., internal: _Optional[bool] = ...) -> None: ...

class ListTypeCountsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListTypeCountsResponse(_message.Message):
    __slots__ = ("type_counts",)
    TYPE_COUNTS_FIELD_NUMBER: _ClassVar[int]
    type_counts: _containers.RepeatedCompositeFieldContainer[TypeCountEntry]
    def __init__(self, type_counts: _Optional[_Iterable[_Union[TypeCountEntry, _Mapping]]] = ...) -> None: ...

class WatchTypeCountsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchTypeCountsResponse(_message.Message):
    __slots__ = ("type_counts", "seqno")
    TYPE_COUNTS_FIELD_NUMBER: _ClassVar[int]
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    type_counts: _containers.RepeatedCompositeFieldContainer[TypeCountEntry]
    seqno: int
    def __init__(self, type_counts: _Optional[_Iterable[_Union[TypeCountEntry, _Mapping]]] = ..., seqno: _Optional[int] = ...) -> None: ...

class SearchObjectsWithTypeRequest(_message.Message):
    __slots__ = ("type_id", "query", "page_size")
    TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    QUERY_FIELD_NUMBER: _ClassVar[int]
    PAGE_SIZE_FIELD_NUMBER: _ClassVar[int]
    type_id: str
    query: str
    page_size: int
    def __init__(self, type_id: _Optional[str] = ..., query: _Optional[str] = ..., page_size: _Optional[int] = ...) -> None: ...

class SearchObjectsWithTypeResponse(_message.Message):
    __slots__ = ("object_keys", "titles", "states")
    OBJECT_KEYS_FIELD_NUMBER: _ClassVar[int]
    TITLES_FIELD_NUMBER: _ClassVar[int]
    STATES_FIELD_NUMBER: _ClassVar[int]
    object_keys: _containers.RepeatedScalarFieldContainer[str]
    titles: _containers.RepeatedScalarFieldContainer[str]
    states: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, object_keys: _Optional[_Iterable[str]] = ..., titles: _Optional[_Iterable[str]] = ..., states: _Optional[_Iterable[str]] = ...) -> None: ...

class ObjectRootRef(_message.Message):
    __slots__ = ("object_key", "root_ref", "rev", "exists")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    REV_FIELD_NUMBER: _ClassVar[int]
    EXISTS_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    root_ref: _bucket_pb2.ObjectRef
    rev: int
    exists: bool
    def __init__(self, object_key: _Optional[str] = ..., root_ref: _Optional[_Union[_bucket_pb2.ObjectRef, _Mapping]] = ..., rev: _Optional[int] = ..., exists: _Optional[bool] = ...) -> None: ...

class GetObjectRootRefsBatchRequest(_message.Message):
    __slots__ = ("object_keys",)
    OBJECT_KEYS_FIELD_NUMBER: _ClassVar[int]
    object_keys: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, object_keys: _Optional[_Iterable[str]] = ...) -> None: ...

class GetObjectRootRefsBatchResponse(_message.Message):
    __slots__ = ("root_refs",)
    ROOT_REFS_FIELD_NUMBER: _ClassVar[int]
    root_refs: _containers.RepeatedCompositeFieldContainer[ObjectRootRef]
    def __init__(self, root_refs: _Optional[_Iterable[_Union[ObjectRootRef, _Mapping]]] = ...) -> None: ...

class ObjectMetadata(_message.Message):
    __slots__ = ("object_key", "type_id", "parent_object_key", "declaration")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    PARENT_OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    DECLARATION_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    type_id: str
    parent_object_key: str
    declaration: ObjectDeclaration
    def __init__(self, object_key: _Optional[str] = ..., type_id: _Optional[str] = ..., parent_object_key: _Optional[str] = ..., declaration: _Optional[_Union[ObjectDeclaration, _Mapping]] = ...) -> None: ...

class ObjectDeclaration(_message.Message):
    __slots__ = ("title", "state", "time", "measures")
    TITLE_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    TIME_FIELD_NUMBER: _ClassVar[int]
    MEASURES_FIELD_NUMBER: _ClassVar[int]
    title: str
    state: str
    time: _timestamp_pb2.Timestamp
    measures: _containers.RepeatedCompositeFieldContainer[ObjectMeasure]
    def __init__(self, title: _Optional[str] = ..., state: _Optional[str] = ..., time: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., measures: _Optional[_Iterable[_Union[ObjectMeasure, _Mapping]]] = ...) -> None: ...

class ObjectMeasure(_message.Message):
    __slots__ = ("name", "unit", "value")
    NAME_FIELD_NUMBER: _ClassVar[int]
    UNIT_FIELD_NUMBER: _ClassVar[int]
    VALUE_FIELD_NUMBER: _ClassVar[int]
    name: str
    unit: str
    value: float
    def __init__(self, name: _Optional[str] = ..., unit: _Optional[str] = ..., value: _Optional[float] = ...) -> None: ...

class GetObjectMetadataBatchRequest(_message.Message):
    __slots__ = ("object_keys", "declared")
    OBJECT_KEYS_FIELD_NUMBER: _ClassVar[int]
    DECLARED_FIELD_NUMBER: _ClassVar[int]
    object_keys: _containers.RepeatedScalarFieldContainer[str]
    declared: bool
    def __init__(self, object_keys: _Optional[_Iterable[str]] = ..., declared: _Optional[bool] = ...) -> None: ...

class GetObjectMetadataBatchResponse(_message.Message):
    __slots__ = ("metadata",)
    METADATA_FIELD_NUMBER: _ClassVar[int]
    metadata: _containers.RepeatedCompositeFieldContainer[ObjectMetadata]
    def __init__(self, metadata: _Optional[_Iterable[_Union[ObjectMetadata, _Mapping]]] = ...) -> None: ...

class ObjectBody(_message.Message):
    __slots__ = ("object_key", "body", "exists", "rev", "mod_seqno", "type_id", "body_json", "withheld_size")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    BODY_FIELD_NUMBER: _ClassVar[int]
    EXISTS_FIELD_NUMBER: _ClassVar[int]
    REV_FIELD_NUMBER: _ClassVar[int]
    MOD_SEQNO_FIELD_NUMBER: _ClassVar[int]
    TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    BODY_JSON_FIELD_NUMBER: _ClassVar[int]
    WITHHELD_SIZE_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    body: bytes
    exists: bool
    rev: int
    mod_seqno: int
    type_id: str
    body_json: str
    withheld_size: int
    def __init__(self, object_key: _Optional[str] = ..., body: _Optional[bytes] = ..., exists: _Optional[bool] = ..., rev: _Optional[int] = ..., mod_seqno: _Optional[int] = ..., type_id: _Optional[str] = ..., body_json: _Optional[str] = ..., withheld_size: _Optional[int] = ...) -> None: ...

class GetObjectBodiesBatchRequest(_message.Message):
    __slots__ = ("object_keys", "json", "page_budget")
    OBJECT_KEYS_FIELD_NUMBER: _ClassVar[int]
    JSON_FIELD_NUMBER: _ClassVar[int]
    PAGE_BUDGET_FIELD_NUMBER: _ClassVar[int]
    object_keys: _containers.RepeatedScalarFieldContainer[str]
    json: bool
    page_budget: int
    def __init__(self, object_keys: _Optional[_Iterable[str]] = ..., json: _Optional[bool] = ..., page_budget: _Optional[int] = ...) -> None: ...

class GetObjectBodiesBatchResponse(_message.Message):
    __slots__ = ("bodies", "world_seqno")
    BODIES_FIELD_NUMBER: _ClassVar[int]
    WORLD_SEQNO_FIELD_NUMBER: _ClassVar[int]
    bodies: _containers.RepeatedCompositeFieldContainer[ObjectBody]
    world_seqno: int
    def __init__(self, bodies: _Optional[_Iterable[_Union[ObjectBody, _Mapping]]] = ..., world_seqno: _Optional[int] = ...) -> None: ...

class GraphPathStep(_message.Message):
    __slots__ = ("direction", "predicate", "limit")
    DIRECTION_FIELD_NUMBER: _ClassVar[int]
    PREDICATE_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    direction: GraphPathDirection
    predicate: str
    limit: int
    def __init__(self, direction: _Optional[_Union[GraphPathDirection, str]] = ..., predicate: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class QueryGraphPathRequest(_message.Message):
    __slots__ = ("start_keys", "steps", "result_limit", "include_quads", "page_size")
    START_KEYS_FIELD_NUMBER: _ClassVar[int]
    STEPS_FIELD_NUMBER: _ClassVar[int]
    RESULT_LIMIT_FIELD_NUMBER: _ClassVar[int]
    INCLUDE_QUADS_FIELD_NUMBER: _ClassVar[int]
    PAGE_SIZE_FIELD_NUMBER: _ClassVar[int]
    start_keys: _containers.RepeatedScalarFieldContainer[str]
    steps: _containers.RepeatedCompositeFieldContainer[GraphPathStep]
    result_limit: int
    include_quads: bool
    page_size: int
    def __init__(self, start_keys: _Optional[_Iterable[str]] = ..., steps: _Optional[_Iterable[_Union[GraphPathStep, _Mapping]]] = ..., result_limit: _Optional[int] = ..., include_quads: _Optional[bool] = ..., page_size: _Optional[int] = ...) -> None: ...

class QueryGraphPathResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class DeleteGraphObjectRequest(_message.Message):
    __slots__ = ("object_key", "known_error_code")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    known_error_code: WorldErrorCode
    def __init__(self, object_key: _Optional[str] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class DeleteGraphObjectResponse(_message.Message):
    __slots__ = ("error_code",)
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    error_code: WorldErrorCode
    def __init__(self, error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class ApplyWorldOpRequest(_message.Message):
    __slots__ = ("op_type_id", "op_data", "op_sender", "known_error_code")
    OP_TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    OP_DATA_FIELD_NUMBER: _ClassVar[int]
    OP_SENDER_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    op_type_id: str
    op_data: bytes
    op_sender: str
    known_error_code: WorldErrorCode
    def __init__(self, op_type_id: _Optional[str] = ..., op_data: _Optional[bytes] = ..., op_sender: _Optional[str] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class ApplyWorldOpResponse(_message.Message):
    __slots__ = ("seqno", "sys_err", "error_code", "rejection_code", "rejection_message")
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    SYS_ERR_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    REJECTION_CODE_FIELD_NUMBER: _ClassVar[int]
    REJECTION_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    seqno: int
    sys_err: bool
    error_code: WorldErrorCode
    rejection_code: str
    rejection_message: str
    def __init__(self, seqno: _Optional[int] = ..., sys_err: _Optional[bool] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ..., rejection_code: _Optional[str] = ..., rejection_message: _Optional[str] = ...) -> None: ...

class WorldMutation(_message.Message):
    __slots__ = ("op", "create_object", "set_graph_quad", "delete_object", "set_object_type", "rename_object", "delete_graph_quad", "delete_graph_object", "set_root_ref", "apply_object_op", "increment_rev")
    OP_FIELD_NUMBER: _ClassVar[int]
    CREATE_OBJECT_FIELD_NUMBER: _ClassVar[int]
    SET_GRAPH_QUAD_FIELD_NUMBER: _ClassVar[int]
    DELETE_OBJECT_FIELD_NUMBER: _ClassVar[int]
    SET_OBJECT_TYPE_FIELD_NUMBER: _ClassVar[int]
    RENAME_OBJECT_FIELD_NUMBER: _ClassVar[int]
    DELETE_GRAPH_QUAD_FIELD_NUMBER: _ClassVar[int]
    DELETE_GRAPH_OBJECT_FIELD_NUMBER: _ClassVar[int]
    SET_ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    APPLY_OBJECT_OP_FIELD_NUMBER: _ClassVar[int]
    INCREMENT_REV_FIELD_NUMBER: _ClassVar[int]
    op: ApplyWorldOpRequest
    create_object: CreateObjectRequest
    set_graph_quad: SetGraphQuadRequest
    delete_object: DeleteObjectRequest
    set_object_type: SetObjectTypeMutation
    rename_object: RenameObjectRequest
    delete_graph_quad: DeleteGraphQuadRequest
    delete_graph_object: DeleteGraphObjectRequest
    set_root_ref: SetRootRefMutation
    apply_object_op: ApplyObjectOpMutation
    increment_rev: IncrementRevMutation
    def __init__(self, op: _Optional[_Union[ApplyWorldOpRequest, _Mapping]] = ..., create_object: _Optional[_Union[CreateObjectRequest, _Mapping]] = ..., set_graph_quad: _Optional[_Union[SetGraphQuadRequest, _Mapping]] = ..., delete_object: _Optional[_Union[DeleteObjectRequest, _Mapping]] = ..., set_object_type: _Optional[_Union[SetObjectTypeMutation, _Mapping]] = ..., rename_object: _Optional[_Union[RenameObjectRequest, _Mapping]] = ..., delete_graph_quad: _Optional[_Union[DeleteGraphQuadRequest, _Mapping]] = ..., delete_graph_object: _Optional[_Union[DeleteGraphObjectRequest, _Mapping]] = ..., set_root_ref: _Optional[_Union[SetRootRefMutation, _Mapping]] = ..., apply_object_op: _Optional[_Union[ApplyObjectOpMutation, _Mapping]] = ..., increment_rev: _Optional[_Union[IncrementRevMutation, _Mapping]] = ...) -> None: ...

class SetObjectTypeMutation(_message.Message):
    __slots__ = ("object_key", "type_id")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    type_id: str
    def __init__(self, object_key: _Optional[str] = ..., type_id: _Optional[str] = ...) -> None: ...

class SetRootRefMutation(_message.Message):
    __slots__ = ("object_key", "root_ref")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    root_ref: _bucket_pb2.ObjectRef
    def __init__(self, object_key: _Optional[str] = ..., root_ref: _Optional[_Union[_bucket_pb2.ObjectRef, _Mapping]] = ...) -> None: ...

class ApplyObjectOpMutation(_message.Message):
    __slots__ = ("object_key", "op_type_id", "op_data", "op_sender")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    OP_TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    OP_DATA_FIELD_NUMBER: _ClassVar[int]
    OP_SENDER_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    op_type_id: str
    op_data: bytes
    op_sender: str
    def __init__(self, object_key: _Optional[str] = ..., op_type_id: _Optional[str] = ..., op_data: _Optional[bytes] = ..., op_sender: _Optional[str] = ...) -> None: ...

class IncrementRevMutation(_message.Message):
    __slots__ = ("object_key",)
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    def __init__(self, object_key: _Optional[str] = ...) -> None: ...

class WorldMutationResult(_message.Message):
    __slots__ = ("rev",)
    REV_FIELD_NUMBER: _ClassVar[int]
    rev: int
    def __init__(self, rev: _Optional[int] = ...) -> None: ...

class ExecuteWorldBatchRequest(_message.Message):
    __slots__ = ("mutations", "known_error_code")
    MUTATIONS_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    mutations: _containers.RepeatedCompositeFieldContainer[WorldMutation]
    known_error_code: WorldErrorCode
    def __init__(self, mutations: _Optional[_Iterable[_Union[WorldMutation, _Mapping]]] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class ExecuteWorldBatchResponse(_message.Message):
    __slots__ = ("seqno", "failed_index", "error_code", "rejection_code", "rejection_message", "results")
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    FAILED_INDEX_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    REJECTION_CODE_FIELD_NUMBER: _ClassVar[int]
    REJECTION_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    RESULTS_FIELD_NUMBER: _ClassVar[int]
    seqno: int
    failed_index: int
    error_code: WorldErrorCode
    rejection_code: str
    rejection_message: str
    results: _containers.RepeatedCompositeFieldContainer[WorldMutationResult]
    def __init__(self, seqno: _Optional[int] = ..., failed_index: _Optional[int] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ..., rejection_code: _Optional[str] = ..., rejection_message: _Optional[str] = ..., results: _Optional[_Iterable[_Union[WorldMutationResult, _Mapping]]] = ...) -> None: ...

class WatchWorldStateRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchWorldStateResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class ErrRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ErrResponse(_message.Message):
    __slots__ = ("error",)
    ERROR_FIELD_NUMBER: _ClassVar[int]
    error: str
    def __init__(self, error: _Optional[str] = ...) -> None: ...

class ValidRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ValidResponse(_message.Message):
    __slots__ = ("valid",)
    VALID_FIELD_NUMBER: _ClassVar[int]
    valid: bool
    def __init__(self, valid: _Optional[bool] = ...) -> None: ...

class KeyRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class KeyResponse(_message.Message):
    __slots__ = ("object_key",)
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    def __init__(self, object_key: _Optional[str] = ...) -> None: ...

class NextRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class NextResponse(_message.Message):
    __slots__ = ("valid",)
    VALID_FIELD_NUMBER: _ClassVar[int]
    valid: bool
    def __init__(self, valid: _Optional[bool] = ...) -> None: ...

class SeekRequest(_message.Message):
    __slots__ = ("object_key",)
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    def __init__(self, object_key: _Optional[str] = ...) -> None: ...

class SeekResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class CloseRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class CloseResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class NextGraphPathQueryRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class NextGraphPathQueryResponse(_message.Message):
    __slots__ = ("object_keys", "quads", "done")
    OBJECT_KEYS_FIELD_NUMBER: _ClassVar[int]
    QUADS_FIELD_NUMBER: _ClassVar[int]
    DONE_FIELD_NUMBER: _ClassVar[int]
    object_keys: _containers.RepeatedScalarFieldContainer[str]
    quads: _containers.RepeatedCompositeFieldContainer[_quad_pb2.Quad]
    done: bool
    def __init__(self, object_keys: _Optional[_Iterable[str]] = ..., quads: _Optional[_Iterable[_Union[_quad_pb2.Quad, _Mapping]]] = ..., done: _Optional[bool] = ...) -> None: ...

class CloseGraphPathQueryRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class CloseGraphPathQueryResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetKeyRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetKeyResponse(_message.Message):
    __slots__ = ("object_key",)
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    def __init__(self, object_key: _Optional[str] = ...) -> None: ...

class GetRootRefRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetRootRefResponse(_message.Message):
    __slots__ = ("root_ref", "rev")
    ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    REV_FIELD_NUMBER: _ClassVar[int]
    root_ref: _bucket_pb2.ObjectRef
    rev: int
    def __init__(self, root_ref: _Optional[_Union[_bucket_pb2.ObjectRef, _Mapping]] = ..., rev: _Optional[int] = ...) -> None: ...

class GetModSeqnoRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetModSeqnoResponse(_message.Message):
    __slots__ = ("mod_seqno",)
    MOD_SEQNO_FIELD_NUMBER: _ClassVar[int]
    mod_seqno: int
    def __init__(self, mod_seqno: _Optional[int] = ...) -> None: ...

class SetRootRefRequest(_message.Message):
    __slots__ = ("root_ref", "known_error_code")
    ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    root_ref: _bucket_pb2.ObjectRef
    known_error_code: WorldErrorCode
    def __init__(self, root_ref: _Optional[_Union[_bucket_pb2.ObjectRef, _Mapping]] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class SetRootRefResponse(_message.Message):
    __slots__ = ("rev", "error_code")
    REV_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    rev: int
    error_code: WorldErrorCode
    def __init__(self, rev: _Optional[int] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class ApplyObjectOpRequest(_message.Message):
    __slots__ = ("op_type_id", "op_data", "op_sender", "known_error_code")
    OP_TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    OP_DATA_FIELD_NUMBER: _ClassVar[int]
    OP_SENDER_FIELD_NUMBER: _ClassVar[int]
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    op_type_id: str
    op_data: bytes
    op_sender: str
    known_error_code: WorldErrorCode
    def __init__(self, op_type_id: _Optional[str] = ..., op_data: _Optional[bytes] = ..., op_sender: _Optional[str] = ..., known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class ApplyObjectOpResponse(_message.Message):
    __slots__ = ("rev", "sys_err", "error_code", "rejection_code", "rejection_message")
    REV_FIELD_NUMBER: _ClassVar[int]
    SYS_ERR_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    REJECTION_CODE_FIELD_NUMBER: _ClassVar[int]
    REJECTION_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    rev: int
    sys_err: bool
    error_code: WorldErrorCode
    rejection_code: str
    rejection_message: str
    def __init__(self, rev: _Optional[int] = ..., sys_err: _Optional[bool] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ..., rejection_code: _Optional[str] = ..., rejection_message: _Optional[str] = ...) -> None: ...

class IncrementRevRequest(_message.Message):
    __slots__ = ("known_error_code",)
    KNOWN_ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    known_error_code: WorldErrorCode
    def __init__(self, known_error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class IncrementRevResponse(_message.Message):
    __slots__ = ("rev", "error_code")
    REV_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    rev: int
    error_code: WorldErrorCode
    def __init__(self, rev: _Optional[int] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class WaitRevRequest(_message.Message):
    __slots__ = ("rev", "ignore_not_found")
    REV_FIELD_NUMBER: _ClassVar[int]
    IGNORE_NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    rev: int
    ignore_not_found: bool
    def __init__(self, rev: _Optional[int] = ..., ignore_not_found: _Optional[bool] = ...) -> None: ...

class WaitRevResponse(_message.Message):
    __slots__ = ("rev",)
    REV_FIELD_NUMBER: _ClassVar[int]
    rev: int
    def __init__(self, rev: _Optional[int] = ...) -> None: ...

class AccessTypedObjectRequest(_message.Message):
    __slots__ = ("object_key",)
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    def __init__(self, object_key: _Optional[str] = ...) -> None: ...

class AccessTypedObjectResponse(_message.Message):
    __slots__ = ("resource_id", "type_id", "error_code")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    type_id: str
    error_code: WorldErrorCode
    def __init__(self, resource_id: _Optional[int] = ..., type_id: _Optional[str] = ..., error_code: _Optional[_Union[WorldErrorCode, str]] = ...) -> None: ...

class WatchTypedObjectRequest(_message.Message):
    __slots__ = ("object_key",)
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    def __init__(self, object_key: _Optional[str] = ...) -> None: ...

class WatchTypedObjectResponse(_message.Message):
    __slots__ = ("resource_id", "type_id")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    TYPE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    type_id: str
    def __init__(self, resource_id: _Optional[int] = ..., type_id: _Optional[str] = ...) -> None: ...

class ObjectRecordBase(_message.Message):
    __slots__ = ("object_key", "root_ref")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    root_ref: _bucket_pb2.ObjectRef
    def __init__(self, object_key: _Optional[str] = ..., root_ref: _Optional[_Union[_bucket_pb2.ObjectRef, _Mapping]] = ...) -> None: ...

class CompareObjectRecordsRequest(_message.Message):
    __slots__ = ("bases",)
    BASES_FIELD_NUMBER: _ClassVar[int]
    bases: _containers.RepeatedCompositeFieldContainer[ObjectRecordBase]
    def __init__(self, bases: _Optional[_Iterable[_Union[ObjectRecordBase, _Mapping]]] = ...) -> None: ...

class ObjectRecordChanges(_message.Message):
    __slots__ = ("object_key", "keys", "unknown")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    KEYS_FIELD_NUMBER: _ClassVar[int]
    UNKNOWN_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    keys: _containers.RepeatedScalarFieldContainer[bytes]
    unknown: bool
    def __init__(self, object_key: _Optional[str] = ..., keys: _Optional[_Iterable[bytes]] = ..., unknown: _Optional[bool] = ...) -> None: ...

class CompareObjectRecordsResponse(_message.Message):
    __slots__ = ("changes",)
    CHANGES_FIELD_NUMBER: _ClassVar[int]
    changes: _containers.RepeatedCompositeFieldContainer[ObjectRecordChanges]
    def __init__(self, changes: _Optional[_Iterable[_Union[ObjectRecordChanges, _Mapping]]] = ...) -> None: ...
