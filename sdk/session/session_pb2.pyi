import datetime

from core.account.settings import settings_pb2 as _settings_pb2
from core.provider.transfer import transfer_pb2 as _transfer_pb2
from core.session import session_pb2 as _session_pb2
from core.sobject import sobject_pb2 as _sobject_pb2
from core.space import space_pb2 as _space_pb2
from net.hash import hash_pb2 as _hash_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SyncStatusState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SyncStatusState_SYNCED: _ClassVar[SyncStatusState]
    SyncStatusState_ACTIVE: _ClassVar[SyncStatusState]
    SyncStatusState_ERROR: _ClassVar[SyncStatusState]

class SyncActivityDirection(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SyncActivityDirection_NONE: _ClassVar[SyncActivityDirection]
    SyncActivityDirection_UPLOAD: _ClassVar[SyncActivityDirection]
    SyncActivityDirection_DOWNLOAD: _ClassVar[SyncActivityDirection]
    SyncActivityDirection_UPLOAD_DOWNLOAD: _ClassVar[SyncActivityDirection]

class SyncTransportState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SyncTransportState_UNKNOWN: _ClassVar[SyncTransportState]
    SyncTransportState_UNAVAILABLE: _ClassVar[SyncTransportState]
    SyncTransportState_CONNECTING: _ClassVar[SyncTransportState]
    SyncTransportState_ONLINE: _ClassVar[SyncTransportState]
    SyncTransportState_ERROR: _ClassVar[SyncTransportState]

class SyncP2PState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SyncP2PState_UNKNOWN: _ClassVar[SyncP2PState]
    SyncP2PState_NO_PEERS: _ClassVar[SyncP2PState]
    SyncP2PState_IDLE: _ClassVar[SyncP2PState]
    SyncP2PState_ACTIVE: _ClassVar[SyncP2PState]
    SyncP2PState_ERROR: _ClassVar[SyncP2PState]
    SyncP2PState_DISABLED: _ClassVar[SyncP2PState]
    SyncP2PState_STARTING: _ClassVar[SyncP2PState]
    SyncP2PState_FALLBACK_NO_PEER: _ClassVar[SyncP2PState]

class SyncBlockSource(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SyncBlockSource_UNKNOWN: _ClassVar[SyncBlockSource]
    SyncBlockSource_CACHE: _ClassVar[SyncBlockSource]
    SyncBlockSource_DIRECT: _ClassVar[SyncBlockSource]
    SyncBlockSource_CLOUD: _ClassVar[SyncBlockSource]

class PairingStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    PairingStatus_IDLE: _ClassVar[PairingStatus]
    PairingStatus_CODE_GENERATED: _ClassVar[PairingStatus]
    PairingStatus_WAITING_FOR_PEER: _ClassVar[PairingStatus]
    PairingStatus_PEER_CONNECTED: _ClassVar[PairingStatus]
    PairingStatus_VERIFYING_EMOJI: _ClassVar[PairingStatus]
    PairingStatus_VERIFIED: _ClassVar[PairingStatus]
    PairingStatus_FAILED: _ClassVar[PairingStatus]
    PairingStatus_SIGNALING_FAILED: _ClassVar[PairingStatus]
    PairingStatus_CONNECTION_TIMEOUT: _ClassVar[PairingStatus]
    PairingStatus_WAITING_FOR_REMOTE_CONFIRM: _ClassVar[PairingStatus]
    PairingStatus_BOTH_CONFIRMED: _ClassVar[PairingStatus]
    PairingStatus_PAIRING_REJECTED: _ClassVar[PairingStatus]
    PairingStatus_CONFIRMATION_TIMEOUT: _ClassVar[PairingStatus]

class JoinSpaceViaInviteResult(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    JoinSpaceViaInviteResult_UNSPECIFIED: _ClassVar[JoinSpaceViaInviteResult]
    JoinSpaceViaInviteResult_ACCEPTED: _ClassVar[JoinSpaceViaInviteResult]
    JoinSpaceViaInviteResult_PENDING_OWNER_APPROVAL: _ClassVar[JoinSpaceViaInviteResult]
    JoinSpaceViaInviteResult_REJECTED: _ClassVar[JoinSpaceViaInviteResult]
    JoinSpaceViaInviteResult_OWNER_MUST_BE_ONLINE: _ClassVar[JoinSpaceViaInviteResult]
SyncStatusState_SYNCED: SyncStatusState
SyncStatusState_ACTIVE: SyncStatusState
SyncStatusState_ERROR: SyncStatusState
SyncActivityDirection_NONE: SyncActivityDirection
SyncActivityDirection_UPLOAD: SyncActivityDirection
SyncActivityDirection_DOWNLOAD: SyncActivityDirection
SyncActivityDirection_UPLOAD_DOWNLOAD: SyncActivityDirection
SyncTransportState_UNKNOWN: SyncTransportState
SyncTransportState_UNAVAILABLE: SyncTransportState
SyncTransportState_CONNECTING: SyncTransportState
SyncTransportState_ONLINE: SyncTransportState
SyncTransportState_ERROR: SyncTransportState
SyncP2PState_UNKNOWN: SyncP2PState
SyncP2PState_NO_PEERS: SyncP2PState
SyncP2PState_IDLE: SyncP2PState
SyncP2PState_ACTIVE: SyncP2PState
SyncP2PState_ERROR: SyncP2PState
SyncP2PState_DISABLED: SyncP2PState
SyncP2PState_STARTING: SyncP2PState
SyncP2PState_FALLBACK_NO_PEER: SyncP2PState
SyncBlockSource_UNKNOWN: SyncBlockSource
SyncBlockSource_CACHE: SyncBlockSource
SyncBlockSource_DIRECT: SyncBlockSource
SyncBlockSource_CLOUD: SyncBlockSource
PairingStatus_IDLE: PairingStatus
PairingStatus_CODE_GENERATED: PairingStatus
PairingStatus_WAITING_FOR_PEER: PairingStatus
PairingStatus_PEER_CONNECTED: PairingStatus
PairingStatus_VERIFYING_EMOJI: PairingStatus
PairingStatus_VERIFIED: PairingStatus
PairingStatus_FAILED: PairingStatus
PairingStatus_SIGNALING_FAILED: PairingStatus
PairingStatus_CONNECTION_TIMEOUT: PairingStatus
PairingStatus_WAITING_FOR_REMOTE_CONFIRM: PairingStatus
PairingStatus_BOTH_CONFIRMED: PairingStatus
PairingStatus_PAIRING_REJECTED: PairingStatus
PairingStatus_CONFIRMATION_TIMEOUT: PairingStatus
JoinSpaceViaInviteResult_UNSPECIFIED: JoinSpaceViaInviteResult
JoinSpaceViaInviteResult_ACCEPTED: JoinSpaceViaInviteResult
JoinSpaceViaInviteResult_PENDING_OWNER_APPROVAL: JoinSpaceViaInviteResult
JoinSpaceViaInviteResult_REJECTED: JoinSpaceViaInviteResult
JoinSpaceViaInviteResult_OWNER_MUST_BE_ONLINE: JoinSpaceViaInviteResult

class GetSessionInfoRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetSessionInfoResponse(_message.Message):
    __slots__ = ("session_ref", "peer_id", "crypto_info")
    SESSION_REF_FIELD_NUMBER: _ClassVar[int]
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    CRYPTO_INFO_FIELD_NUMBER: _ClassVar[int]
    session_ref: _session_pb2.SessionRef
    peer_id: str
    crypto_info: SessionCryptoInfo
    def __init__(self, session_ref: _Optional[_Union[_session_pb2.SessionRef, _Mapping]] = ..., peer_id: _Optional[str] = ..., crypto_info: _Optional[_Union[SessionCryptoInfo, _Mapping]] = ...) -> None: ...

class SessionCryptoInfo(_message.Message):
    __slots__ = ("key_type", "public_key_base58", "space_count", "total_storage_bytes", "public_key_pem")
    KEY_TYPE_FIELD_NUMBER: _ClassVar[int]
    PUBLIC_KEY_BASE58_FIELD_NUMBER: _ClassVar[int]
    SPACE_COUNT_FIELD_NUMBER: _ClassVar[int]
    TOTAL_STORAGE_BYTES_FIELD_NUMBER: _ClassVar[int]
    PUBLIC_KEY_PEM_FIELD_NUMBER: _ClassVar[int]
    key_type: str
    public_key_base58: str
    space_count: int
    total_storage_bytes: int
    public_key_pem: str
    def __init__(self, key_type: _Optional[str] = ..., public_key_base58: _Optional[str] = ..., space_count: _Optional[int] = ..., total_storage_bytes: _Optional[int] = ..., public_key_pem: _Optional[str] = ...) -> None: ...

class WatchResourcesListRequest(_message.Message):
    __slots__ = ("include_index_object_types",)
    INCLUDE_INDEX_OBJECT_TYPES_FIELD_NUMBER: _ClassVar[int]
    include_index_object_types: bool
    def __init__(self, include_index_object_types: _Optional[bool] = ...) -> None: ...

class WatchResourcesListResponse(_message.Message):
    __slots__ = ("spaces_list",)
    SPACES_LIST_FIELD_NUMBER: _ClassVar[int]
    spaces_list: _containers.RepeatedCompositeFieldContainer[_space_pb2.SpaceSoListEntry]
    def __init__(self, spaces_list: _Optional[_Iterable[_Union[_space_pb2.SpaceSoListEntry, _Mapping]]] = ...) -> None: ...

class CreateSpaceRequest(_message.Message):
    __slots__ = ("space_name", "owner_type", "owner_id")
    SPACE_NAME_FIELD_NUMBER: _ClassVar[int]
    OWNER_TYPE_FIELD_NUMBER: _ClassVar[int]
    OWNER_ID_FIELD_NUMBER: _ClassVar[int]
    space_name: str
    owner_type: str
    owner_id: str
    def __init__(self, space_name: _Optional[str] = ..., owner_type: _Optional[str] = ..., owner_id: _Optional[str] = ...) -> None: ...

class CreateSpaceResponse(_message.Message):
    __slots__ = ("shared_object_ref", "shared_object_meta", "mounted_shared_object", "shared_object_body_resource_id", "space_world_resource_id")
    SHARED_OBJECT_REF_FIELD_NUMBER: _ClassVar[int]
    SHARED_OBJECT_META_FIELD_NUMBER: _ClassVar[int]
    MOUNTED_SHARED_OBJECT_FIELD_NUMBER: _ClassVar[int]
    SHARED_OBJECT_BODY_RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    SPACE_WORLD_RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    shared_object_ref: _sobject_pb2.SharedObjectRef
    shared_object_meta: _sobject_pb2.SharedObjectMeta
    mounted_shared_object: MountSharedObjectResponse
    shared_object_body_resource_id: int
    space_world_resource_id: int
    def __init__(self, shared_object_ref: _Optional[_Union[_sobject_pb2.SharedObjectRef, _Mapping]] = ..., shared_object_meta: _Optional[_Union[_sobject_pb2.SharedObjectMeta, _Mapping]] = ..., mounted_shared_object: _Optional[_Union[MountSharedObjectResponse, _Mapping]] = ..., shared_object_body_resource_id: _Optional[int] = ..., space_world_resource_id: _Optional[int] = ...) -> None: ...

class DeleteSpaceRequest(_message.Message):
    __slots__ = ("shared_object_id",)
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    shared_object_id: str
    def __init__(self, shared_object_id: _Optional[str] = ...) -> None: ...

class DeleteSpaceResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class RenameSpaceRequest(_message.Message):
    __slots__ = ("shared_object_id", "display_name")
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    shared_object_id: str
    display_name: str
    def __init__(self, shared_object_id: _Optional[str] = ..., display_name: _Optional[str] = ...) -> None: ...

class RenameSpaceResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class MountSharedObjectRequest(_message.Message):
    __slots__ = ("shared_object_id",)
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    shared_object_id: str
    def __init__(self, shared_object_id: _Optional[str] = ...) -> None: ...

class MountSharedObjectResponse(_message.Message):
    __slots__ = ("resource_id", "shared_object_meta", "peer_id", "shared_object_id", "block_store_id", "hash_type", "transport_peer_id")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    SHARED_OBJECT_META_FIELD_NUMBER: _ClassVar[int]
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    BLOCK_STORE_ID_FIELD_NUMBER: _ClassVar[int]
    HASH_TYPE_FIELD_NUMBER: _ClassVar[int]
    TRANSPORT_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    shared_object_meta: _sobject_pb2.SharedObjectMeta
    peer_id: str
    shared_object_id: str
    block_store_id: str
    hash_type: _hash_pb2.HashType
    transport_peer_id: str
    def __init__(self, resource_id: _Optional[int] = ..., shared_object_meta: _Optional[_Union[_sobject_pb2.SharedObjectMeta, _Mapping]] = ..., peer_id: _Optional[str] = ..., shared_object_id: _Optional[str] = ..., block_store_id: _Optional[str] = ..., hash_type: _Optional[_Union[_hash_pb2.HashType, str]] = ..., transport_peer_id: _Optional[str] = ...) -> None: ...

class WatchSharedObjectHealthRequest(_message.Message):
    __slots__ = ("shared_object_id",)
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    shared_object_id: str
    def __init__(self, shared_object_id: _Optional[str] = ...) -> None: ...

class WatchSharedObjectHealthResponse(_message.Message):
    __slots__ = ("health",)
    HEALTH_FIELD_NUMBER: _ClassVar[int]
    health: _sobject_pb2.SharedObjectHealth
    def __init__(self, health: _Optional[_Union[_sobject_pb2.SharedObjectHealth, _Mapping]] = ...) -> None: ...

class WatchSyncStatusRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchStorageStatsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SyncBlockStoreStatus(_message.Message):
    __slots__ = ("block_store_id", "direct_hit_count", "cloud_hit_count", "cache_hit_count", "last_source", "accepted_root_inner_sequence", "cloud_remote_sequence", "shared_object_id")
    BLOCK_STORE_ID_FIELD_NUMBER: _ClassVar[int]
    DIRECT_HIT_COUNT_FIELD_NUMBER: _ClassVar[int]
    CLOUD_HIT_COUNT_FIELD_NUMBER: _ClassVar[int]
    CACHE_HIT_COUNT_FIELD_NUMBER: _ClassVar[int]
    LAST_SOURCE_FIELD_NUMBER: _ClassVar[int]
    ACCEPTED_ROOT_INNER_SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    CLOUD_REMOTE_SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    block_store_id: str
    direct_hit_count: int
    cloud_hit_count: int
    cache_hit_count: int
    last_source: SyncBlockSource
    accepted_root_inner_sequence: int
    cloud_remote_sequence: int
    shared_object_id: str
    def __init__(self, block_store_id: _Optional[str] = ..., direct_hit_count: _Optional[int] = ..., cloud_hit_count: _Optional[int] = ..., cache_hit_count: _Optional[int] = ..., last_source: _Optional[_Union[SyncBlockSource, str]] = ..., accepted_root_inner_sequence: _Optional[int] = ..., cloud_remote_sequence: _Optional[int] = ..., shared_object_id: _Optional[str] = ...) -> None: ...

class WatchSyncStatusResponse(_message.Message):
    __slots__ = ("state", "direction", "transport_state", "p2p_state", "pending_upload_bytes", "pending_download_bytes", "pending_upload_count", "pending_download_count", "upload_bytes_per_second", "download_bytes_per_second", "active_upload_bytes", "active_upload_transferred_bytes", "in_flight_upload_count", "active_store_count", "active_peer_count", "last_error", "last_activity_at", "pack_range_request_count", "pack_range_response_bytes", "pack_full_response_fallback_count", "pack_full_response_fallback_bytes", "pack_last_full_response_fallback_bytes", "pack_manifest_entries", "pack_block_count_total", "pack_block_count_min", "pack_block_count_max", "pack_size_bytes_total", "pack_size_bytes_min", "pack_size_bytes_max", "pack_bloom_filter_count", "pack_bloom_missing_count", "pack_bloom_invalid_count", "pack_bloom_parameter_shape_count", "pack_bloom_max_false_positive_rate", "pack_bloom_risk_pack_count", "pack_lookup_count", "pack_candidate_packs", "pack_opened_packs", "pack_negative_packs", "pack_target_hits", "pack_last_candidate_packs", "pack_last_opened_packs", "pack_last_negative_packs", "pack_last_target_hit", "pack_index_cache_hits", "pack_index_cache_misses", "pack_index_cache_read_errors", "pack_index_cache_write_errors", "pack_remote_index_loads", "pack_remote_index_bytes", "pack_last_remote_index_bytes", "pack_index_tail_fetch_count", "pack_index_tail_fetch_bytes", "pack_index_tail_response_bytes", "direct_p2p_disabled", "block_stores")
    STATE_FIELD_NUMBER: _ClassVar[int]
    DIRECTION_FIELD_NUMBER: _ClassVar[int]
    TRANSPORT_STATE_FIELD_NUMBER: _ClassVar[int]
    P2P_STATE_FIELD_NUMBER: _ClassVar[int]
    PENDING_UPLOAD_BYTES_FIELD_NUMBER: _ClassVar[int]
    PENDING_DOWNLOAD_BYTES_FIELD_NUMBER: _ClassVar[int]
    PENDING_UPLOAD_COUNT_FIELD_NUMBER: _ClassVar[int]
    PENDING_DOWNLOAD_COUNT_FIELD_NUMBER: _ClassVar[int]
    UPLOAD_BYTES_PER_SECOND_FIELD_NUMBER: _ClassVar[int]
    DOWNLOAD_BYTES_PER_SECOND_FIELD_NUMBER: _ClassVar[int]
    ACTIVE_UPLOAD_BYTES_FIELD_NUMBER: _ClassVar[int]
    ACTIVE_UPLOAD_TRANSFERRED_BYTES_FIELD_NUMBER: _ClassVar[int]
    IN_FLIGHT_UPLOAD_COUNT_FIELD_NUMBER: _ClassVar[int]
    ACTIVE_STORE_COUNT_FIELD_NUMBER: _ClassVar[int]
    ACTIVE_PEER_COUNT_FIELD_NUMBER: _ClassVar[int]
    LAST_ERROR_FIELD_NUMBER: _ClassVar[int]
    LAST_ACTIVITY_AT_FIELD_NUMBER: _ClassVar[int]
    PACK_RANGE_REQUEST_COUNT_FIELD_NUMBER: _ClassVar[int]
    PACK_RANGE_RESPONSE_BYTES_FIELD_NUMBER: _ClassVar[int]
    PACK_FULL_RESPONSE_FALLBACK_COUNT_FIELD_NUMBER: _ClassVar[int]
    PACK_FULL_RESPONSE_FALLBACK_BYTES_FIELD_NUMBER: _ClassVar[int]
    PACK_LAST_FULL_RESPONSE_FALLBACK_BYTES_FIELD_NUMBER: _ClassVar[int]
    PACK_MANIFEST_ENTRIES_FIELD_NUMBER: _ClassVar[int]
    PACK_BLOCK_COUNT_TOTAL_FIELD_NUMBER: _ClassVar[int]
    PACK_BLOCK_COUNT_MIN_FIELD_NUMBER: _ClassVar[int]
    PACK_BLOCK_COUNT_MAX_FIELD_NUMBER: _ClassVar[int]
    PACK_SIZE_BYTES_TOTAL_FIELD_NUMBER: _ClassVar[int]
    PACK_SIZE_BYTES_MIN_FIELD_NUMBER: _ClassVar[int]
    PACK_SIZE_BYTES_MAX_FIELD_NUMBER: _ClassVar[int]
    PACK_BLOOM_FILTER_COUNT_FIELD_NUMBER: _ClassVar[int]
    PACK_BLOOM_MISSING_COUNT_FIELD_NUMBER: _ClassVar[int]
    PACK_BLOOM_INVALID_COUNT_FIELD_NUMBER: _ClassVar[int]
    PACK_BLOOM_PARAMETER_SHAPE_COUNT_FIELD_NUMBER: _ClassVar[int]
    PACK_BLOOM_MAX_FALSE_POSITIVE_RATE_FIELD_NUMBER: _ClassVar[int]
    PACK_BLOOM_RISK_PACK_COUNT_FIELD_NUMBER: _ClassVar[int]
    PACK_LOOKUP_COUNT_FIELD_NUMBER: _ClassVar[int]
    PACK_CANDIDATE_PACKS_FIELD_NUMBER: _ClassVar[int]
    PACK_OPENED_PACKS_FIELD_NUMBER: _ClassVar[int]
    PACK_NEGATIVE_PACKS_FIELD_NUMBER: _ClassVar[int]
    PACK_TARGET_HITS_FIELD_NUMBER: _ClassVar[int]
    PACK_LAST_CANDIDATE_PACKS_FIELD_NUMBER: _ClassVar[int]
    PACK_LAST_OPENED_PACKS_FIELD_NUMBER: _ClassVar[int]
    PACK_LAST_NEGATIVE_PACKS_FIELD_NUMBER: _ClassVar[int]
    PACK_LAST_TARGET_HIT_FIELD_NUMBER: _ClassVar[int]
    PACK_INDEX_CACHE_HITS_FIELD_NUMBER: _ClassVar[int]
    PACK_INDEX_CACHE_MISSES_FIELD_NUMBER: _ClassVar[int]
    PACK_INDEX_CACHE_READ_ERRORS_FIELD_NUMBER: _ClassVar[int]
    PACK_INDEX_CACHE_WRITE_ERRORS_FIELD_NUMBER: _ClassVar[int]
    PACK_REMOTE_INDEX_LOADS_FIELD_NUMBER: _ClassVar[int]
    PACK_REMOTE_INDEX_BYTES_FIELD_NUMBER: _ClassVar[int]
    PACK_LAST_REMOTE_INDEX_BYTES_FIELD_NUMBER: _ClassVar[int]
    PACK_INDEX_TAIL_FETCH_COUNT_FIELD_NUMBER: _ClassVar[int]
    PACK_INDEX_TAIL_FETCH_BYTES_FIELD_NUMBER: _ClassVar[int]
    PACK_INDEX_TAIL_RESPONSE_BYTES_FIELD_NUMBER: _ClassVar[int]
    DIRECT_P2P_DISABLED_FIELD_NUMBER: _ClassVar[int]
    BLOCK_STORES_FIELD_NUMBER: _ClassVar[int]
    state: SyncStatusState
    direction: SyncActivityDirection
    transport_state: SyncTransportState
    p2p_state: SyncP2PState
    pending_upload_bytes: int
    pending_download_bytes: int
    pending_upload_count: int
    pending_download_count: int
    upload_bytes_per_second: int
    download_bytes_per_second: int
    active_upload_bytes: int
    active_upload_transferred_bytes: int
    in_flight_upload_count: int
    active_store_count: int
    active_peer_count: int
    last_error: str
    last_activity_at: _timestamp_pb2.Timestamp
    pack_range_request_count: int
    pack_range_response_bytes: int
    pack_full_response_fallback_count: int
    pack_full_response_fallback_bytes: int
    pack_last_full_response_fallback_bytes: int
    pack_manifest_entries: int
    pack_block_count_total: int
    pack_block_count_min: int
    pack_block_count_max: int
    pack_size_bytes_total: int
    pack_size_bytes_min: int
    pack_size_bytes_max: int
    pack_bloom_filter_count: int
    pack_bloom_missing_count: int
    pack_bloom_invalid_count: int
    pack_bloom_parameter_shape_count: int
    pack_bloom_max_false_positive_rate: float
    pack_bloom_risk_pack_count: int
    pack_lookup_count: int
    pack_candidate_packs: int
    pack_opened_packs: int
    pack_negative_packs: int
    pack_target_hits: int
    pack_last_candidate_packs: int
    pack_last_opened_packs: int
    pack_last_negative_packs: int
    pack_last_target_hit: bool
    pack_index_cache_hits: int
    pack_index_cache_misses: int
    pack_index_cache_read_errors: int
    pack_index_cache_write_errors: int
    pack_remote_index_loads: int
    pack_remote_index_bytes: int
    pack_last_remote_index_bytes: int
    pack_index_tail_fetch_count: int
    pack_index_tail_fetch_bytes: int
    pack_index_tail_response_bytes: int
    direct_p2p_disabled: bool
    block_stores: _containers.RepeatedCompositeFieldContainer[SyncBlockStoreStatus]
    def __init__(self, state: _Optional[_Union[SyncStatusState, str]] = ..., direction: _Optional[_Union[SyncActivityDirection, str]] = ..., transport_state: _Optional[_Union[SyncTransportState, str]] = ..., p2p_state: _Optional[_Union[SyncP2PState, str]] = ..., pending_upload_bytes: _Optional[int] = ..., pending_download_bytes: _Optional[int] = ..., pending_upload_count: _Optional[int] = ..., pending_download_count: _Optional[int] = ..., upload_bytes_per_second: _Optional[int] = ..., download_bytes_per_second: _Optional[int] = ..., active_upload_bytes: _Optional[int] = ..., active_upload_transferred_bytes: _Optional[int] = ..., in_flight_upload_count: _Optional[int] = ..., active_store_count: _Optional[int] = ..., active_peer_count: _Optional[int] = ..., last_error: _Optional[str] = ..., last_activity_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., pack_range_request_count: _Optional[int] = ..., pack_range_response_bytes: _Optional[int] = ..., pack_full_response_fallback_count: _Optional[int] = ..., pack_full_response_fallback_bytes: _Optional[int] = ..., pack_last_full_response_fallback_bytes: _Optional[int] = ..., pack_manifest_entries: _Optional[int] = ..., pack_block_count_total: _Optional[int] = ..., pack_block_count_min: _Optional[int] = ..., pack_block_count_max: _Optional[int] = ..., pack_size_bytes_total: _Optional[int] = ..., pack_size_bytes_min: _Optional[int] = ..., pack_size_bytes_max: _Optional[int] = ..., pack_bloom_filter_count: _Optional[int] = ..., pack_bloom_missing_count: _Optional[int] = ..., pack_bloom_invalid_count: _Optional[int] = ..., pack_bloom_parameter_shape_count: _Optional[int] = ..., pack_bloom_max_false_positive_rate: _Optional[float] = ..., pack_bloom_risk_pack_count: _Optional[int] = ..., pack_lookup_count: _Optional[int] = ..., pack_candidate_packs: _Optional[int] = ..., pack_opened_packs: _Optional[int] = ..., pack_negative_packs: _Optional[int] = ..., pack_target_hits: _Optional[int] = ..., pack_last_candidate_packs: _Optional[int] = ..., pack_last_opened_packs: _Optional[int] = ..., pack_last_negative_packs: _Optional[int] = ..., pack_last_target_hit: _Optional[bool] = ..., pack_index_cache_hits: _Optional[int] = ..., pack_index_cache_misses: _Optional[int] = ..., pack_index_cache_read_errors: _Optional[int] = ..., pack_index_cache_write_errors: _Optional[int] = ..., pack_remote_index_loads: _Optional[int] = ..., pack_remote_index_bytes: _Optional[int] = ..., pack_last_remote_index_bytes: _Optional[int] = ..., pack_index_tail_fetch_count: _Optional[int] = ..., pack_index_tail_fetch_bytes: _Optional[int] = ..., pack_index_tail_response_bytes: _Optional[int] = ..., direct_p2p_disabled: _Optional[bool] = ..., block_stores: _Optional[_Iterable[_Union[SyncBlockStoreStatus, _Mapping]]] = ...) -> None: ...

class WatchStorageStatsResponse(_message.Message):
    __slots__ = ("supported", "total_bytes", "block_count")
    SUPPORTED_FIELD_NUMBER: _ClassVar[int]
    TOTAL_BYTES_FIELD_NUMBER: _ClassVar[int]
    BLOCK_COUNT_FIELD_NUMBER: _ClassVar[int]
    supported: bool
    total_bytes: int
    block_count: int
    def __init__(self, supported: _Optional[bool] = ..., total_bytes: _Optional[int] = ..., block_count: _Optional[int] = ...) -> None: ...

class WatchLockStateRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchLockStateResponse(_message.Message):
    __slots__ = ("mode", "locked")
    MODE_FIELD_NUMBER: _ClassVar[int]
    LOCKED_FIELD_NUMBER: _ClassVar[int]
    mode: _session_pb2.SessionLockMode
    locked: bool
    def __init__(self, mode: _Optional[_Union[_session_pb2.SessionLockMode, str]] = ..., locked: _Optional[bool] = ...) -> None: ...

class SetLockModeRequest(_message.Message):
    __slots__ = ("mode", "pin")
    MODE_FIELD_NUMBER: _ClassVar[int]
    PIN_FIELD_NUMBER: _ClassVar[int]
    mode: _session_pb2.SessionLockMode
    pin: bytes
    def __init__(self, mode: _Optional[_Union[_session_pb2.SessionLockMode, str]] = ..., pin: _Optional[bytes] = ...) -> None: ...

class SetLockModeResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SetDirectP2PEnabledRequest(_message.Message):
    __slots__ = ("enabled",)
    ENABLED_FIELD_NUMBER: _ClassVar[int]
    enabled: bool
    def __init__(self, enabled: _Optional[bool] = ...) -> None: ...

class SetDirectP2PEnabledResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class UnlockSessionRequest(_message.Message):
    __slots__ = ("pin",)
    PIN_FIELD_NUMBER: _ClassVar[int]
    pin: bytes
    def __init__(self, pin: _Optional[bytes] = ...) -> None: ...

class UnlockSessionResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class LockSessionRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class LockSessionResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GeneratePairingCodeRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GeneratePairingCodeResponse(_message.Message):
    __slots__ = ("code",)
    CODE_FIELD_NUMBER: _ClassVar[int]
    code: str
    def __init__(self, code: _Optional[str] = ...) -> None: ...

class CompletePairingRequest(_message.Message):
    __slots__ = ("code",)
    CODE_FIELD_NUMBER: _ClassVar[int]
    code: str
    def __init__(self, code: _Optional[str] = ...) -> None: ...

class CompletePairingResponse(_message.Message):
    __slots__ = ("remote_peer_id",)
    REMOTE_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    remote_peer_id: str
    def __init__(self, remote_peer_id: _Optional[str] = ...) -> None: ...

class GetSASEmojiRequest(_message.Message):
    __slots__ = ("remote_peer_id",)
    REMOTE_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    remote_peer_id: str
    def __init__(self, remote_peer_id: _Optional[str] = ...) -> None: ...

class GetSASEmojiResponse(_message.Message):
    __slots__ = ("emoji",)
    EMOJI_FIELD_NUMBER: _ClassVar[int]
    emoji: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, emoji: _Optional[_Iterable[str]] = ...) -> None: ...

class ConfirmSASMatchRequest(_message.Message):
    __slots__ = ("confirmed",)
    CONFIRMED_FIELD_NUMBER: _ClassVar[int]
    confirmed: bool
    def __init__(self, confirmed: _Optional[bool] = ...) -> None: ...

class ConfirmSASMatchResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ConfirmPairingRequest(_message.Message):
    __slots__ = ("remote_peer_id", "display_name")
    REMOTE_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    remote_peer_id: str
    display_name: str
    def __init__(self, remote_peer_id: _Optional[str] = ..., display_name: _Optional[str] = ...) -> None: ...

class ConfirmPairingResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class DeleteAccountRequest(_message.Message):
    __slots__ = ("session_idx",)
    SESSION_IDX_FIELD_NUMBER: _ClassVar[int]
    session_idx: int
    def __init__(self, session_idx: _Optional[int] = ...) -> None: ...

class DeleteAccountResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class AccessSessionStateAtomRequest(_message.Message):
    __slots__ = ("store_id",)
    STORE_ID_FIELD_NUMBER: _ClassVar[int]
    store_id: str
    def __init__(self, store_id: _Optional[str] = ...) -> None: ...

class AccessSessionStateAtomResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class AccessPeerTransportRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class AccessPeerTransportResponse(_message.Message):
    __slots__ = ("resource_id", "peer_id")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    peer_id: str
    def __init__(self, resource_id: _Optional[int] = ..., peer_id: _Optional[str] = ...) -> None: ...

class WatchSessionStateAtomsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchSessionStateAtomsResponse(_message.Message):
    __slots__ = ("store_ids", "store_count")
    STORE_IDS_FIELD_NUMBER: _ClassVar[int]
    STORE_COUNT_FIELD_NUMBER: _ClassVar[int]
    store_ids: _containers.RepeatedScalarFieldContainer[str]
    store_count: int
    def __init__(self, store_ids: _Optional[_Iterable[str]] = ..., store_count: _Optional[int] = ...) -> None: ...

class GetTransferInventoryRequest(_message.Message):
    __slots__ = ("session_index",)
    SESSION_INDEX_FIELD_NUMBER: _ClassVar[int]
    session_index: int
    def __init__(self, session_index: _Optional[int] = ...) -> None: ...

class GetTransferInventoryResponse(_message.Message):
    __slots__ = ("spaces",)
    SPACES_FIELD_NUMBER: _ClassVar[int]
    spaces: _containers.RepeatedCompositeFieldContainer[_space_pb2.SpaceSoListEntry]
    def __init__(self, spaces: _Optional[_Iterable[_Union[_space_pb2.SpaceSoListEntry, _Mapping]]] = ...) -> None: ...

class StartTransferRequest(_message.Message):
    __slots__ = ("source_session_index", "target_session_index", "mode", "space_ids")
    SOURCE_SESSION_INDEX_FIELD_NUMBER: _ClassVar[int]
    TARGET_SESSION_INDEX_FIELD_NUMBER: _ClassVar[int]
    MODE_FIELD_NUMBER: _ClassVar[int]
    SPACE_IDS_FIELD_NUMBER: _ClassVar[int]
    source_session_index: int
    target_session_index: int
    mode: _transfer_pb2.TransferMode
    space_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, source_session_index: _Optional[int] = ..., target_session_index: _Optional[int] = ..., mode: _Optional[_Union[_transfer_pb2.TransferMode, str]] = ..., space_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class StartTransferResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchTransferProgressRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchTransferProgressResponse(_message.Message):
    __slots__ = ("state",)
    STATE_FIELD_NUMBER: _ClassVar[int]
    state: _transfer_pb2.TransferState
    def __init__(self, state: _Optional[_Union[_transfer_pb2.TransferState, _Mapping]] = ...) -> None: ...

class CancelTransferRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class CancelTransferResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchPairedDevicesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchPairedDevicesResponse(_message.Message):
    __slots__ = ("paired_devices", "online_peer_ids")
    PAIRED_DEVICES_FIELD_NUMBER: _ClassVar[int]
    ONLINE_PEER_IDS_FIELD_NUMBER: _ClassVar[int]
    paired_devices: _containers.RepeatedCompositeFieldContainer[_settings_pb2.PairedDevice]
    online_peer_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, paired_devices: _Optional[_Iterable[_Union[_settings_pb2.PairedDevice, _Mapping]]] = ..., online_peer_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class UnlinkDeviceRequest(_message.Message):
    __slots__ = ("peer_id",)
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    def __init__(self, peer_id: _Optional[str] = ...) -> None: ...

class UnlinkDeviceResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchPairingStatusRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchPairingStatusResponse(_message.Message):
    __slots__ = ("status", "remote_peer_id", "code", "emoji", "error_message")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    REMOTE_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    CODE_FIELD_NUMBER: _ClassVar[int]
    EMOJI_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    status: PairingStatus
    remote_peer_id: str
    code: str
    emoji: _containers.RepeatedScalarFieldContainer[str]
    error_message: str
    def __init__(self, status: _Optional[_Union[PairingStatus, str]] = ..., remote_peer_id: _Optional[str] = ..., code: _Optional[str] = ..., emoji: _Optional[_Iterable[str]] = ..., error_message: _Optional[str] = ...) -> None: ...

class CreateSpaceInviteRequest(_message.Message):
    __slots__ = ("space_id", "role", "target_peer_id", "max_uses", "expires_at")
    SPACE_ID_FIELD_NUMBER: _ClassVar[int]
    ROLE_FIELD_NUMBER: _ClassVar[int]
    TARGET_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    MAX_USES_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    space_id: str
    role: _sobject_pb2.SOParticipantRole
    target_peer_id: str
    max_uses: int
    expires_at: _timestamp_pb2.Timestamp
    def __init__(self, space_id: _Optional[str] = ..., role: _Optional[_Union[_sobject_pb2.SOParticipantRole, str]] = ..., target_peer_id: _Optional[str] = ..., max_uses: _Optional[int] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class CreateSpaceInviteResponse(_message.Message):
    __slots__ = ("invite_message", "short_code")
    INVITE_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    SHORT_CODE_FIELD_NUMBER: _ClassVar[int]
    invite_message: _sobject_pb2.SOInviteMessage
    short_code: str
    def __init__(self, invite_message: _Optional[_Union[_sobject_pb2.SOInviteMessage, _Mapping]] = ..., short_code: _Optional[str] = ...) -> None: ...

class ListSpaceInvitesRequest(_message.Message):
    __slots__ = ("space_id",)
    SPACE_ID_FIELD_NUMBER: _ClassVar[int]
    space_id: str
    def __init__(self, space_id: _Optional[str] = ...) -> None: ...

class ListSpaceInvitesResponse(_message.Message):
    __slots__ = ("invites",)
    INVITES_FIELD_NUMBER: _ClassVar[int]
    invites: _containers.RepeatedCompositeFieldContainer[_sobject_pb2.SOInvite]
    def __init__(self, invites: _Optional[_Iterable[_Union[_sobject_pb2.SOInvite, _Mapping]]] = ...) -> None: ...

class ListSpaceParticipantsRequest(_message.Message):
    __slots__ = ("space_id",)
    SPACE_ID_FIELD_NUMBER: _ClassVar[int]
    space_id: str
    def __init__(self, space_id: _Optional[str] = ...) -> None: ...

class ListSpaceParticipantsResponse(_message.Message):
    __slots__ = ("participants",)
    PARTICIPANTS_FIELD_NUMBER: _ClassVar[int]
    participants: _containers.RepeatedCompositeFieldContainer[_sobject_pb2.SOParticipantConfig]
    def __init__(self, participants: _Optional[_Iterable[_Union[_sobject_pb2.SOParticipantConfig, _Mapping]]] = ...) -> None: ...

class RemoveSpaceParticipantRequest(_message.Message):
    __slots__ = ("space_id", "peer_id")
    SPACE_ID_FIELD_NUMBER: _ClassVar[int]
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    space_id: str
    peer_id: str
    def __init__(self, space_id: _Optional[str] = ..., peer_id: _Optional[str] = ...) -> None: ...

class RemoveSpaceParticipantResponse(_message.Message):
    __slots__ = ("removed",)
    REMOVED_FIELD_NUMBER: _ClassVar[int]
    removed: bool
    def __init__(self, removed: _Optional[bool] = ...) -> None: ...

class RevokeSpaceInviteRequest(_message.Message):
    __slots__ = ("space_id", "invite_id")
    SPACE_ID_FIELD_NUMBER: _ClassVar[int]
    INVITE_ID_FIELD_NUMBER: _ClassVar[int]
    space_id: str
    invite_id: str
    def __init__(self, space_id: _Optional[str] = ..., invite_id: _Optional[str] = ...) -> None: ...

class RevokeSpaceInviteResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class JoinSpaceViaInviteRequest(_message.Message):
    __slots__ = ("invite_message", "targeted_invitation_envelope")
    INVITE_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    TARGETED_INVITATION_ENVELOPE_FIELD_NUMBER: _ClassVar[int]
    invite_message: _sobject_pb2.SOInviteMessage
    targeted_invitation_envelope: bytes
    def __init__(self, invite_message: _Optional[_Union[_sobject_pb2.SOInviteMessage, _Mapping]] = ..., targeted_invitation_envelope: _Optional[bytes] = ...) -> None: ...

class JoinSpaceViaInviteResponse(_message.Message):
    __slots__ = ("shared_object_id", "result")
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    RESULT_FIELD_NUMBER: _ClassVar[int]
    shared_object_id: str
    result: JoinSpaceViaInviteResult
    def __init__(self, shared_object_id: _Optional[str] = ..., result: _Optional[_Union[JoinSpaceViaInviteResult, str]] = ...) -> None: ...

class GetTransferStatusRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetTransferStatusResponse(_message.Message):
    __slots__ = ("active", "has_checkpoint", "state")
    ACTIVE_FIELD_NUMBER: _ClassVar[int]
    HAS_CHECKPOINT_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    active: bool
    has_checkpoint: bool
    state: _transfer_pb2.TransferState
    def __init__(self, active: _Optional[bool] = ..., has_checkpoint: _Optional[bool] = ..., state: _Optional[_Union[_transfer_pb2.TransferState, _Mapping]] = ...) -> None: ...

class LocalPairingOffer(_message.Message):
    __slots__ = ("sdp", "peer_id")
    SDP_FIELD_NUMBER: _ClassVar[int]
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    sdp: str
    peer_id: str
    def __init__(self, sdp: _Optional[str] = ..., peer_id: _Optional[str] = ...) -> None: ...

class LocalPairingAnswer(_message.Message):
    __slots__ = ("sdp", "peer_id")
    SDP_FIELD_NUMBER: _ClassVar[int]
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    sdp: str
    peer_id: str
    def __init__(self, sdp: _Optional[str] = ..., peer_id: _Optional[str] = ...) -> None: ...

class CreateLocalPairingOfferRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class CreateLocalPairingOfferResponse(_message.Message):
    __slots__ = ("offer_payload",)
    OFFER_PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    offer_payload: str
    def __init__(self, offer_payload: _Optional[str] = ...) -> None: ...

class AcceptLocalPairingOfferRequest(_message.Message):
    __slots__ = ("offer_payload",)
    OFFER_PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    offer_payload: str
    def __init__(self, offer_payload: _Optional[str] = ...) -> None: ...

class AcceptLocalPairingOfferResponse(_message.Message):
    __slots__ = ("answer_payload",)
    ANSWER_PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    answer_payload: str
    def __init__(self, answer_payload: _Optional[str] = ...) -> None: ...

class AcceptLocalPairingAnswerRequest(_message.Message):
    __slots__ = ("answer_payload",)
    ANSWER_PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    answer_payload: str
    def __init__(self, answer_payload: _Optional[str] = ...) -> None: ...

class AcceptLocalPairingAnswerResponse(_message.Message):
    __slots__ = ("remote_peer_id",)
    REMOTE_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    remote_peer_id: str
    def __init__(self, remote_peer_id: _Optional[str] = ...) -> None: ...
