import datetime

from net.peer import peer_pb2 as _peer_pb2
from core.provider import provider_pb2 as _provider_pb2
from db.block.transform import transform_pb2 as _transform_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SharedObjectHealthStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SHARED_OBJECT_HEALTH_STATUS_UNKNOWN: _ClassVar[SharedObjectHealthStatus]
    SHARED_OBJECT_HEALTH_STATUS_LOADING: _ClassVar[SharedObjectHealthStatus]
    SHARED_OBJECT_HEALTH_STATUS_READY: _ClassVar[SharedObjectHealthStatus]
    SHARED_OBJECT_HEALTH_STATUS_DEGRADED: _ClassVar[SharedObjectHealthStatus]
    SHARED_OBJECT_HEALTH_STATUS_CLOSED: _ClassVar[SharedObjectHealthStatus]

class SharedObjectHealthLayer(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SHARED_OBJECT_HEALTH_LAYER_UNKNOWN: _ClassVar[SharedObjectHealthLayer]
    SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT: _ClassVar[SharedObjectHealthLayer]
    SHARED_OBJECT_HEALTH_LAYER_BODY: _ClassVar[SharedObjectHealthLayer]

class SharedObjectHealthCommonReason(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SHARED_OBJECT_HEALTH_COMMON_REASON_UNKNOWN: _ClassVar[SharedObjectHealthCommonReason]
    SHARED_OBJECT_HEALTH_COMMON_REASON_NOT_FOUND: _ClassVar[SharedObjectHealthCommonReason]
    SHARED_OBJECT_HEALTH_COMMON_REASON_ACCESS_REVOKED: _ClassVar[SharedObjectHealthCommonReason]
    SHARED_OBJECT_HEALTH_COMMON_REASON_INITIAL_STATE_REJECTED: _ClassVar[SharedObjectHealthCommonReason]
    SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND: _ClassVar[SharedObjectHealthCommonReason]
    SHARED_OBJECT_HEALTH_COMMON_REASON_TRANSFORM_CONFIG_DECODE_FAILED: _ClassVar[SharedObjectHealthCommonReason]
    SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED: _ClassVar[SharedObjectHealthCommonReason]

class SharedObjectHealthRemediationHint(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SHARED_OBJECT_HEALTH_REMEDIATION_HINT_UNKNOWN: _ClassVar[SharedObjectHealthRemediationHint]
    SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE: _ClassVar[SharedObjectHealthRemediationHint]
    SHARED_OBJECT_HEALTH_REMEDIATION_HINT_RETRY: _ClassVar[SharedObjectHealthRemediationHint]
    SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REQUEST_ACCESS: _ClassVar[SharedObjectHealthRemediationHint]
    SHARED_OBJECT_HEALTH_REMEDIATION_HINT_CONTACT_OWNER: _ClassVar[SharedObjectHealthRemediationHint]
    SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA: _ClassVar[SharedObjectHealthRemediationHint]

class SOParticipantRole(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SOParticipantRole_UNKNOWN: _ClassVar[SOParticipantRole]
    SOParticipantRole_READER: _ClassVar[SOParticipantRole]
    SOParticipantRole_WRITER: _ClassVar[SOParticipantRole]
    SOParticipantRole_OWNER: _ClassVar[SOParticipantRole]

class SOConfigChangeType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_CONFIG_CHANGE_TYPE_UNKNOWN: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_GENESIS: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_ADD_INVITE: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_SET_ROSTER: _ClassVar[SOConfigChangeType]
    SO_CONFIG_CHANGE_TYPE_SET_SEQUENCER: _ClassVar[SOConfigChangeType]

class SORevocationReason(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_REVOCATION_REASON_UNKNOWN: _ClassVar[SORevocationReason]
    SO_REVOCATION_REASON_SESSION_REVOKED: _ClassVar[SORevocationReason]
    SO_REVOCATION_REASON_ORG_REMOVED: _ClassVar[SORevocationReason]
    SO_REVOCATION_REASON_OWNER_REMOVED: _ClassVar[SORevocationReason]
    SO_REVOCATION_REASON_INVITE_REVOKED: _ClassVar[SORevocationReason]
SHARED_OBJECT_HEALTH_STATUS_UNKNOWN: SharedObjectHealthStatus
SHARED_OBJECT_HEALTH_STATUS_LOADING: SharedObjectHealthStatus
SHARED_OBJECT_HEALTH_STATUS_READY: SharedObjectHealthStatus
SHARED_OBJECT_HEALTH_STATUS_DEGRADED: SharedObjectHealthStatus
SHARED_OBJECT_HEALTH_STATUS_CLOSED: SharedObjectHealthStatus
SHARED_OBJECT_HEALTH_LAYER_UNKNOWN: SharedObjectHealthLayer
SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT: SharedObjectHealthLayer
SHARED_OBJECT_HEALTH_LAYER_BODY: SharedObjectHealthLayer
SHARED_OBJECT_HEALTH_COMMON_REASON_UNKNOWN: SharedObjectHealthCommonReason
SHARED_OBJECT_HEALTH_COMMON_REASON_NOT_FOUND: SharedObjectHealthCommonReason
SHARED_OBJECT_HEALTH_COMMON_REASON_ACCESS_REVOKED: SharedObjectHealthCommonReason
SHARED_OBJECT_HEALTH_COMMON_REASON_INITIAL_STATE_REJECTED: SharedObjectHealthCommonReason
SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND: SharedObjectHealthCommonReason
SHARED_OBJECT_HEALTH_COMMON_REASON_TRANSFORM_CONFIG_DECODE_FAILED: SharedObjectHealthCommonReason
SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED: SharedObjectHealthCommonReason
SHARED_OBJECT_HEALTH_REMEDIATION_HINT_UNKNOWN: SharedObjectHealthRemediationHint
SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE: SharedObjectHealthRemediationHint
SHARED_OBJECT_HEALTH_REMEDIATION_HINT_RETRY: SharedObjectHealthRemediationHint
SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REQUEST_ACCESS: SharedObjectHealthRemediationHint
SHARED_OBJECT_HEALTH_REMEDIATION_HINT_CONTACT_OWNER: SharedObjectHealthRemediationHint
SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA: SharedObjectHealthRemediationHint
SOParticipantRole_UNKNOWN: SOParticipantRole
SOParticipantRole_READER: SOParticipantRole
SOParticipantRole_WRITER: SOParticipantRole
SOParticipantRole_OWNER: SOParticipantRole
SO_CONFIG_CHANGE_TYPE_UNKNOWN: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_GENESIS: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_ADD_INVITE: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_SET_ROSTER: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_SET_SEQUENCER: SOConfigChangeType
SO_REVOCATION_REASON_UNKNOWN: SORevocationReason
SO_REVOCATION_REASON_SESSION_REVOKED: SORevocationReason
SO_REVOCATION_REASON_ORG_REMOVED: SORevocationReason
SO_REVOCATION_REASON_OWNER_REMOVED: SORevocationReason
SO_REVOCATION_REASON_INVITE_REVOKED: SORevocationReason

class SharedObjectRef(_message.Message):
    __slots__ = ("provider_resource_ref", "block_store_id")
    PROVIDER_RESOURCE_REF_FIELD_NUMBER: _ClassVar[int]
    BLOCK_STORE_ID_FIELD_NUMBER: _ClassVar[int]
    provider_resource_ref: _provider_pb2.ProviderResourceRef
    block_store_id: str
    def __init__(self, provider_resource_ref: _Optional[_Union[_provider_pb2.ProviderResourceRef, _Mapping]] = ..., block_store_id: _Optional[str] = ...) -> None: ...

class SharedObjectList(_message.Message):
    __slots__ = ("shared_objects",)
    SHARED_OBJECTS_FIELD_NUMBER: _ClassVar[int]
    shared_objects: _containers.RepeatedCompositeFieldContainer[SharedObjectListEntry]
    def __init__(self, shared_objects: _Optional[_Iterable[_Union[SharedObjectListEntry, _Mapping]]] = ...) -> None: ...

class SharedObjectListEntry(_message.Message):
    __slots__ = ("ref", "meta", "source", "transport_peer_id")
    REF_FIELD_NUMBER: _ClassVar[int]
    META_FIELD_NUMBER: _ClassVar[int]
    SOURCE_FIELD_NUMBER: _ClassVar[int]
    TRANSPORT_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    ref: SharedObjectRef
    meta: SharedObjectMeta
    source: str
    transport_peer_id: str
    def __init__(self, ref: _Optional[_Union[SharedObjectRef, _Mapping]] = ..., meta: _Optional[_Union[SharedObjectMeta, _Mapping]] = ..., source: _Optional[str] = ..., transport_peer_id: _Optional[str] = ...) -> None: ...

class SharedObjectMeta(_message.Message):
    __slots__ = ("body_type", "body_meta", "account_private")
    BODY_TYPE_FIELD_NUMBER: _ClassVar[int]
    BODY_META_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_PRIVATE_FIELD_NUMBER: _ClassVar[int]
    body_type: str
    body_meta: bytes
    account_private: bool
    def __init__(self, body_type: _Optional[str] = ..., body_meta: _Optional[bytes] = ..., account_private: _Optional[bool] = ...) -> None: ...

class SharedObjectHealth(_message.Message):
    __slots__ = ("status", "layer", "common_reason", "remediation_hint", "error", "metadata", "sync_denied_peer_ids", "sync_recovery_peer_ids", "rejected_edits", "checkpoint_mismatch")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    LAYER_FIELD_NUMBER: _ClassVar[int]
    COMMON_REASON_FIELD_NUMBER: _ClassVar[int]
    REMEDIATION_HINT_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    METADATA_FIELD_NUMBER: _ClassVar[int]
    SYNC_DENIED_PEER_IDS_FIELD_NUMBER: _ClassVar[int]
    SYNC_RECOVERY_PEER_IDS_FIELD_NUMBER: _ClassVar[int]
    REJECTED_EDITS_FIELD_NUMBER: _ClassVar[int]
    CHECKPOINT_MISMATCH_FIELD_NUMBER: _ClassVar[int]
    status: SharedObjectHealthStatus
    layer: SharedObjectHealthLayer
    common_reason: SharedObjectHealthCommonReason
    remediation_hint: SharedObjectHealthRemediationHint
    error: str
    metadata: bytes
    sync_denied_peer_ids: _containers.RepeatedScalarFieldContainer[str]
    sync_recovery_peer_ids: _containers.RepeatedScalarFieldContainer[str]
    rejected_edits: _containers.RepeatedCompositeFieldContainer[SORejectedEdit]
    checkpoint_mismatch: SOCheckpointMismatch
    def __init__(self, status: _Optional[_Union[SharedObjectHealthStatus, str]] = ..., layer: _Optional[_Union[SharedObjectHealthLayer, str]] = ..., common_reason: _Optional[_Union[SharedObjectHealthCommonReason, str]] = ..., remediation_hint: _Optional[_Union[SharedObjectHealthRemediationHint, str]] = ..., error: _Optional[str] = ..., metadata: _Optional[bytes] = ..., sync_denied_peer_ids: _Optional[_Iterable[str]] = ..., sync_recovery_peer_ids: _Optional[_Iterable[str]] = ..., rejected_edits: _Optional[_Iterable[_Union[SORejectedEdit, _Mapping]]] = ..., checkpoint_mismatch: _Optional[_Union[SOCheckpointMismatch, _Mapping]] = ...) -> None: ...

class SORejectedEdit(_message.Message):
    __slots__ = ("op_hash", "reason", "lost_to_peer_ids")
    OP_HASH_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    LOST_TO_PEER_IDS_FIELD_NUMBER: _ClassVar[int]
    op_hash: bytes
    reason: str
    lost_to_peer_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, op_hash: _Optional[bytes] = ..., reason: _Optional[str] = ..., lost_to_peer_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class SOCheckpointMismatch(_message.Message):
    __slots__ = ("height",)
    HEIGHT_FIELD_NUMBER: _ClassVar[int]
    height: int
    def __init__(self, height: _Optional[int] = ...) -> None: ...

class SharedObjectConfig(_message.Message):
    __slots__ = ("participants", "config_chain_hash", "config_chain_seqno", "removed_authors", "roster_dropped_peer_ids", "sequencer")
    PARTICIPANTS_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_HASH_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_SEQNO_FIELD_NUMBER: _ClassVar[int]
    REMOVED_AUTHORS_FIELD_NUMBER: _ClassVar[int]
    ROSTER_DROPPED_PEER_IDS_FIELD_NUMBER: _ClassVar[int]
    SEQUENCER_FIELD_NUMBER: _ClassVar[int]
    participants: _containers.RepeatedCompositeFieldContainer[SOParticipantConfig]
    config_chain_hash: bytes
    config_chain_seqno: int
    removed_authors: _containers.RepeatedCompositeFieldContainer[SOOperationPosition]
    roster_dropped_peer_ids: _containers.RepeatedScalarFieldContainer[str]
    sequencer: SOSequencer
    def __init__(self, participants: _Optional[_Iterable[_Union[SOParticipantConfig, _Mapping]]] = ..., config_chain_hash: _Optional[bytes] = ..., config_chain_seqno: _Optional[int] = ..., removed_authors: _Optional[_Iterable[_Union[SOOperationPosition, _Mapping]]] = ..., roster_dropped_peer_ids: _Optional[_Iterable[str]] = ..., sequencer: _Optional[_Union[SOSequencer, _Mapping]] = ...) -> None: ...

class SOSequencer(_message.Message):
    __slots__ = ("peer_id", "start")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    START_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    start: SOSequenceHead
    def __init__(self, peer_id: _Optional[str] = ..., start: _Optional[_Union[SOSequenceHead, _Mapping]] = ...) -> None: ...

class SOSequenceHead(_message.Message):
    __slots__ = ("height", "hash")
    HEIGHT_FIELD_NUMBER: _ClassVar[int]
    HASH_FIELD_NUMBER: _ClassVar[int]
    height: int
    hash: bytes
    def __init__(self, height: _Optional[int] = ..., hash: _Optional[bytes] = ...) -> None: ...

class SOSequence(_message.Message):
    __slots__ = ("inner", "signature")
    INNER_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    inner: bytes
    signature: _peer_pb2.Signature
    def __init__(self, inner: _Optional[bytes] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ...) -> None: ...

class SOSequenceInner(_message.Message):
    __slots__ = ("shared_object_id", "height", "prev_hash", "op", "peer_id")
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    HEIGHT_FIELD_NUMBER: _ClassVar[int]
    PREV_HASH_FIELD_NUMBER: _ClassVar[int]
    OP_FIELD_NUMBER: _ClassVar[int]
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    shared_object_id: str
    height: int
    prev_hash: bytes
    op: SOOperationPosition
    peer_id: str
    def __init__(self, shared_object_id: _Optional[str] = ..., height: _Optional[int] = ..., prev_hash: _Optional[bytes] = ..., op: _Optional[_Union[SOOperationPosition, _Mapping]] = ..., peer_id: _Optional[str] = ...) -> None: ...

class SOLeaveRequest(_message.Message):
    __slots__ = ("shared_object_id", "config_hash", "signatures")
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    CONFIG_HASH_FIELD_NUMBER: _ClassVar[int]
    SIGNATURES_FIELD_NUMBER: _ClassVar[int]
    shared_object_id: str
    config_hash: bytes
    signatures: _containers.RepeatedCompositeFieldContainer[_peer_pb2.Signature]
    def __init__(self, shared_object_id: _Optional[str] = ..., config_hash: _Optional[bytes] = ..., signatures: _Optional[_Iterable[_Union[_peer_pb2.Signature, _Mapping]]] = ...) -> None: ...

class SOLeaveResponse(_message.Message):
    __slots__ = ("changes",)
    CHANGES_FIELD_NUMBER: _ClassVar[int]
    changes: _containers.RepeatedCompositeFieldContainer[SOConfigChange]
    def __init__(self, changes: _Optional[_Iterable[_Union[SOConfigChange, _Mapping]]] = ...) -> None: ...

class SORevocationInfo(_message.Message):
    __slots__ = ("reason", "timestamp", "nonce", "leave_request_hash")
    REASON_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    LEAVE_REQUEST_HASH_FIELD_NUMBER: _ClassVar[int]
    reason: SORevocationReason
    timestamp: _timestamp_pb2.Timestamp
    nonce: int
    leave_request_hash: bytes
    def __init__(self, reason: _Optional[_Union[SORevocationReason, str]] = ..., timestamp: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., nonce: _Optional[int] = ..., leave_request_hash: _Optional[bytes] = ...) -> None: ...

class SOConfigChange(_message.Message):
    __slots__ = ("config_seqno", "config", "previous_hash", "change_type", "revocation_info", "leave_request", "shared_object_id", "signatures")
    CONFIG_SEQNO_FIELD_NUMBER: _ClassVar[int]
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    PREVIOUS_HASH_FIELD_NUMBER: _ClassVar[int]
    CHANGE_TYPE_FIELD_NUMBER: _ClassVar[int]
    REVOCATION_INFO_FIELD_NUMBER: _ClassVar[int]
    LEAVE_REQUEST_FIELD_NUMBER: _ClassVar[int]
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    SIGNATURES_FIELD_NUMBER: _ClassVar[int]
    config_seqno: int
    config: SharedObjectConfig
    previous_hash: bytes
    change_type: SOConfigChangeType
    revocation_info: SORevocationInfo
    leave_request: SOLeaveRequest
    shared_object_id: str
    signatures: _containers.RepeatedCompositeFieldContainer[_peer_pb2.Signature]
    def __init__(self, config_seqno: _Optional[int] = ..., config: _Optional[_Union[SharedObjectConfig, _Mapping]] = ..., previous_hash: _Optional[bytes] = ..., change_type: _Optional[_Union[SOConfigChangeType, str]] = ..., revocation_info: _Optional[_Union[SORevocationInfo, _Mapping]] = ..., leave_request: _Optional[_Union[SOLeaveRequest, _Mapping]] = ..., shared_object_id: _Optional[str] = ..., signatures: _Optional[_Iterable[_Union[_peer_pb2.Signature, _Mapping]]] = ...) -> None: ...

class SOParticipantConfig(_message.Message):
    __slots__ = ("peer_id", "role", "entity_id", "username")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    ROLE_FIELD_NUMBER: _ClassVar[int]
    ENTITY_ID_FIELD_NUMBER: _ClassVar[int]
    USERNAME_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    role: SOParticipantRole
    entity_id: str
    username: str
    def __init__(self, peer_id: _Optional[str] = ..., role: _Optional[_Union[SOParticipantRole, str]] = ..., entity_id: _Optional[str] = ..., username: _Optional[str] = ...) -> None: ...

class SOCheckpoint(_message.Message):
    __slots__ = ("inner", "signatures")
    INNER_FIELD_NUMBER: _ClassVar[int]
    SIGNATURES_FIELD_NUMBER: _ClassVar[int]
    inner: bytes
    signatures: _containers.RepeatedCompositeFieldContainer[_peer_pb2.Signature]
    def __init__(self, inner: _Optional[bytes] = ..., signatures: _Optional[_Iterable[_Union[_peer_pb2.Signature, _Mapping]]] = ...) -> None: ...

class SOCheckpointInner(_message.Message):
    __slots__ = ("shared_object_id", "height", "prev_checkpoint_hash", "config_hash", "state_data", "replay_version", "key_epoch", "authors", "sequence")
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    HEIGHT_FIELD_NUMBER: _ClassVar[int]
    PREV_CHECKPOINT_HASH_FIELD_NUMBER: _ClassVar[int]
    CONFIG_HASH_FIELD_NUMBER: _ClassVar[int]
    STATE_DATA_FIELD_NUMBER: _ClassVar[int]
    REPLAY_VERSION_FIELD_NUMBER: _ClassVar[int]
    KEY_EPOCH_FIELD_NUMBER: _ClassVar[int]
    AUTHORS_FIELD_NUMBER: _ClassVar[int]
    SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    shared_object_id: str
    height: int
    prev_checkpoint_hash: bytes
    config_hash: bytes
    state_data: bytes
    replay_version: int
    key_epoch: int
    authors: _containers.RepeatedCompositeFieldContainer[SOOperationPosition]
    sequence: SOSequenceHead
    def __init__(self, shared_object_id: _Optional[str] = ..., height: _Optional[int] = ..., prev_checkpoint_hash: _Optional[bytes] = ..., config_hash: _Optional[bytes] = ..., state_data: _Optional[bytes] = ..., replay_version: _Optional[int] = ..., key_epoch: _Optional[int] = ..., authors: _Optional[_Iterable[_Union[SOOperationPosition, _Mapping]]] = ..., sequence: _Optional[_Union[SOSequenceHead, _Mapping]] = ...) -> None: ...

class SOOperationPosition(_message.Message):
    __slots__ = ("peer_id", "nonce", "op_hash")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    OP_HASH_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    nonce: int
    op_hash: bytes
    def __init__(self, peer_id: _Optional[str] = ..., nonce: _Optional[int] = ..., op_hash: _Optional[bytes] = ...) -> None: ...

class SOOperation(_message.Message):
    __slots__ = ("inner", "signature")
    INNER_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    inner: bytes
    signature: _peer_pb2.Signature
    def __init__(self, inner: _Optional[bytes] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ...) -> None: ...

class SOOperationInner(_message.Message):
    __slots__ = ("peer_id", "local_id", "nonce", "op_data", "shared_object_id", "protocol_version", "prev_op_hash", "parents", "config_hash", "key_epoch")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    LOCAL_ID_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    OP_DATA_FIELD_NUMBER: _ClassVar[int]
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    PROTOCOL_VERSION_FIELD_NUMBER: _ClassVar[int]
    PREV_OP_HASH_FIELD_NUMBER: _ClassVar[int]
    PARENTS_FIELD_NUMBER: _ClassVar[int]
    CONFIG_HASH_FIELD_NUMBER: _ClassVar[int]
    KEY_EPOCH_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    local_id: str
    nonce: int
    op_data: bytes
    shared_object_id: str
    protocol_version: int
    prev_op_hash: bytes
    parents: _containers.RepeatedCompositeFieldContainer[SOOperationPosition]
    config_hash: bytes
    key_epoch: int
    def __init__(self, peer_id: _Optional[str] = ..., local_id: _Optional[str] = ..., nonce: _Optional[int] = ..., op_data: _Optional[bytes] = ..., shared_object_id: _Optional[str] = ..., protocol_version: _Optional[int] = ..., prev_op_hash: _Optional[bytes] = ..., parents: _Optional[_Iterable[_Union[SOOperationPosition, _Mapping]]] = ..., config_hash: _Optional[bytes] = ..., key_epoch: _Optional[int] = ...) -> None: ...

class SOOperationRef(_message.Message):
    __slots__ = ("peer_id", "nonce")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    nonce: int
    def __init__(self, peer_id: _Optional[str] = ..., nonce: _Optional[int] = ...) -> None: ...

class SOOperationResult(_message.Message):
    __slots__ = ("op_ref", "success", "error_details")
    OP_REF_FIELD_NUMBER: _ClassVar[int]
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_DETAILS_FIELD_NUMBER: _ClassVar[int]
    op_ref: SOOperationRef
    success: bool
    error_details: SOOperationRejectionErrorDetails
    def __init__(self, op_ref: _Optional[_Union[SOOperationRef, _Mapping]] = ..., success: _Optional[bool] = ..., error_details: _Optional[_Union[SOOperationRejectionErrorDetails, _Mapping]] = ...) -> None: ...

class SOOperationRejectionErrorDetails(_message.Message):
    __slots__ = ("error_msg", "missing_block")
    ERROR_MSG_FIELD_NUMBER: _ClassVar[int]
    MISSING_BLOCK_FIELD_NUMBER: _ClassVar[int]
    error_msg: str
    missing_block: bool
    def __init__(self, error_msg: _Optional[str] = ..., missing_block: _Optional[bool] = ...) -> None: ...

class SOGrant(_message.Message):
    __slots__ = ("peer_id", "inner_data", "signature")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    INNER_DATA_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    inner_data: bytes
    signature: _peer_pb2.Signature
    def __init__(self, peer_id: _Optional[str] = ..., inner_data: _Optional[bytes] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ...) -> None: ...

class SOGrantInner(_message.Message):
    __slots__ = ("transform_conf",)
    TRANSFORM_CONF_FIELD_NUMBER: _ClassVar[int]
    transform_conf: _transform_pb2.Config
    def __init__(self, transform_conf: _Optional[_Union[_transform_pb2.Config, _Mapping]] = ...) -> None: ...

class SOEntityRecoveryEnvelope(_message.Message):
    __slots__ = ("entity_id", "key_epoch", "config_chain_seqno", "config_chain_hash", "envelope_data")
    ENTITY_ID_FIELD_NUMBER: _ClassVar[int]
    KEY_EPOCH_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_SEQNO_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_HASH_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_DATA_FIELD_NUMBER: _ClassVar[int]
    entity_id: str
    key_epoch: int
    config_chain_seqno: int
    config_chain_hash: bytes
    envelope_data: bytes
    def __init__(self, entity_id: _Optional[str] = ..., key_epoch: _Optional[int] = ..., config_chain_seqno: _Optional[int] = ..., config_chain_hash: _Optional[bytes] = ..., envelope_data: _Optional[bytes] = ...) -> None: ...

class SOEntityRecoveryMaterial(_message.Message):
    __slots__ = ("entity_id", "role", "grant_inner")
    ENTITY_ID_FIELD_NUMBER: _ClassVar[int]
    ROLE_FIELD_NUMBER: _ClassVar[int]
    GRANT_INNER_FIELD_NUMBER: _ClassVar[int]
    entity_id: str
    role: SOParticipantRole
    grant_inner: SOGrantInner
    def __init__(self, entity_id: _Optional[str] = ..., role: _Optional[_Union[SOParticipantRole, str]] = ..., grant_inner: _Optional[_Union[SOGrantInner, _Mapping]] = ...) -> None: ...

class SOInvite(_message.Message):
    __slots__ = ("invite_id", "token_hash", "role", "target_peer_id", "max_uses", "uses", "expires_at", "revoked", "target_account_id", "approval_required", "participant_of")
    INVITE_ID_FIELD_NUMBER: _ClassVar[int]
    TOKEN_HASH_FIELD_NUMBER: _ClassVar[int]
    ROLE_FIELD_NUMBER: _ClassVar[int]
    TARGET_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    MAX_USES_FIELD_NUMBER: _ClassVar[int]
    USES_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    REVOKED_FIELD_NUMBER: _ClassVar[int]
    TARGET_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    APPROVAL_REQUIRED_FIELD_NUMBER: _ClassVar[int]
    PARTICIPANT_OF_FIELD_NUMBER: _ClassVar[int]
    invite_id: str
    token_hash: bytes
    role: SOParticipantRole
    target_peer_id: str
    max_uses: int
    uses: int
    expires_at: _timestamp_pb2.Timestamp
    revoked: bool
    target_account_id: str
    approval_required: bool
    participant_of: SOInviteParticipation
    def __init__(self, invite_id: _Optional[str] = ..., token_hash: _Optional[bytes] = ..., role: _Optional[_Union[SOParticipantRole, str]] = ..., target_peer_id: _Optional[str] = ..., max_uses: _Optional[int] = ..., uses: _Optional[int] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., revoked: _Optional[bool] = ..., target_account_id: _Optional[str] = ..., approval_required: _Optional[bool] = ..., participant_of: _Optional[_Union[SOInviteParticipation, _Mapping]] = ...) -> None: ...

class SOInviteParticipation(_message.Message):
    __slots__ = ("shared_object_ids",)
    SHARED_OBJECT_IDS_FIELD_NUMBER: _ClassVar[int]
    shared_object_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, shared_object_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class SOJoinRequest(_message.Message):
    __slots__ = ("join_response", "created_at")
    JOIN_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    join_response: SOJoinResponse
    created_at: _timestamp_pb2.Timestamp
    def __init__(self, join_response: _Optional[_Union[SOJoinResponse, _Mapping]] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class SOJoinRequestList(_message.Message):
    __slots__ = ("requests",)
    REQUESTS_FIELD_NUMBER: _ClassVar[int]
    requests: _containers.RepeatedCompositeFieldContainer[SOJoinRequest]
    def __init__(self, requests: _Optional[_Iterable[_Union[SOJoinRequest, _Mapping]]] = ...) -> None: ...

class SOState(_message.Message):
    __slots__ = ("config", "checkpoint", "key_epochs", "ops", "invites", "sequence")
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    CHECKPOINT_FIELD_NUMBER: _ClassVar[int]
    KEY_EPOCHS_FIELD_NUMBER: _ClassVar[int]
    OPS_FIELD_NUMBER: _ClassVar[int]
    INVITES_FIELD_NUMBER: _ClassVar[int]
    SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    config: SharedObjectConfig
    checkpoint: SOCheckpoint
    key_epochs: _containers.RepeatedCompositeFieldContainer[SOKeyEpoch]
    ops: _containers.RepeatedCompositeFieldContainer[SOOperation]
    invites: _containers.RepeatedCompositeFieldContainer[SOInvite]
    sequence: _containers.RepeatedCompositeFieldContainer[SOSequence]
    def __init__(self, config: _Optional[_Union[SharedObjectConfig, _Mapping]] = ..., checkpoint: _Optional[_Union[SOCheckpoint, _Mapping]] = ..., key_epochs: _Optional[_Iterable[_Union[SOKeyEpoch, _Mapping]]] = ..., ops: _Optional[_Iterable[_Union[SOOperation, _Mapping]]] = ..., invites: _Optional[_Iterable[_Union[SOInvite, _Mapping]]] = ..., sequence: _Optional[_Iterable[_Union[SOSequence, _Mapping]]] = ...) -> None: ...

class SOKeyEpoch(_message.Message):
    __slots__ = ("epoch", "grants")
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    GRANTS_FIELD_NUMBER: _ClassVar[int]
    epoch: int
    grants: _containers.RepeatedCompositeFieldContainer[SOGrant]
    def __init__(self, epoch: _Optional[int] = ..., grants: _Optional[_Iterable[_Union[SOGrant, _Mapping]]] = ...) -> None: ...

class SOConfigChainResponse(_message.Message):
    __slots__ = ("config_changes", "key_epochs")
    CONFIG_CHANGES_FIELD_NUMBER: _ClassVar[int]
    KEY_EPOCHS_FIELD_NUMBER: _ClassVar[int]
    config_changes: _containers.RepeatedCompositeFieldContainer[SOConfigChange]
    key_epochs: _containers.RepeatedCompositeFieldContainer[SOKeyEpoch]
    def __init__(self, config_changes: _Optional[_Iterable[_Union[SOConfigChange, _Mapping]]] = ..., key_epochs: _Optional[_Iterable[_Union[SOKeyEpoch, _Mapping]]] = ...) -> None: ...

class SOInviteMessage(_message.Message):
    __slots__ = ("invite_id", "shared_object_id", "owner_peer_id", "provider_id", "token", "role", "target_peer_id", "expires_at", "max_uses", "signature", "transport_peer_id")
    INVITE_ID_FIELD_NUMBER: _ClassVar[int]
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    OWNER_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ID_FIELD_NUMBER: _ClassVar[int]
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    ROLE_FIELD_NUMBER: _ClassVar[int]
    TARGET_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    MAX_USES_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    TRANSPORT_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    invite_id: str
    shared_object_id: str
    owner_peer_id: str
    provider_id: str
    token: bytes
    role: SOParticipantRole
    target_peer_id: str
    expires_at: _timestamp_pb2.Timestamp
    max_uses: int
    signature: _peer_pb2.Signature
    transport_peer_id: str
    def __init__(self, invite_id: _Optional[str] = ..., shared_object_id: _Optional[str] = ..., owner_peer_id: _Optional[str] = ..., provider_id: _Optional[str] = ..., token: _Optional[bytes] = ..., role: _Optional[_Union[SOParticipantRole, str]] = ..., target_peer_id: _Optional[str] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., max_uses: _Optional[int] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ..., transport_peer_id: _Optional[str] = ...) -> None: ...

class SOJoinResponse(_message.Message):
    __slots__ = ("invite_id", "responder_peer_id", "responder_pubkey", "signature")
    INVITE_ID_FIELD_NUMBER: _ClassVar[int]
    RESPONDER_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    RESPONDER_PUBKEY_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    invite_id: str
    responder_peer_id: str
    responder_pubkey: bytes
    signature: _peer_pb2.Signature
    def __init__(self, invite_id: _Optional[str] = ..., responder_peer_id: _Optional[str] = ..., responder_pubkey: _Optional[bytes] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ...) -> None: ...
