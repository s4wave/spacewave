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
    SOParticipantRole_VALIDATOR: _ClassVar[SOParticipantRole]
    SOParticipantRole_OWNER: _ClassVar[SOParticipantRole]

class SOConsensusMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_CONSENSUS_MODE_SINGLE_VALIDATOR: _ClassVar[SOConsensusMode]

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
SOParticipantRole_VALIDATOR: SOParticipantRole
SOParticipantRole_OWNER: SOParticipantRole
SO_CONSENSUS_MODE_SINGLE_VALIDATOR: SOConsensusMode
SO_CONFIG_CHANGE_TYPE_UNKNOWN: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_GENESIS: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_ADD_INVITE: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_REVOKE_INVITE: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_INCREMENT_INVITE_USES: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER: SOConfigChangeType
SO_CONFIG_CHANGE_TYPE_TRANSFER_OWNERSHIP: SOConfigChangeType
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
    __slots__ = ("status", "layer", "common_reason", "remediation_hint", "error", "metadata", "sync_denied_peer_ids", "sync_recovery_peer_ids")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    LAYER_FIELD_NUMBER: _ClassVar[int]
    COMMON_REASON_FIELD_NUMBER: _ClassVar[int]
    REMEDIATION_HINT_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    METADATA_FIELD_NUMBER: _ClassVar[int]
    SYNC_DENIED_PEER_IDS_FIELD_NUMBER: _ClassVar[int]
    SYNC_RECOVERY_PEER_IDS_FIELD_NUMBER: _ClassVar[int]
    status: SharedObjectHealthStatus
    layer: SharedObjectHealthLayer
    common_reason: SharedObjectHealthCommonReason
    remediation_hint: SharedObjectHealthRemediationHint
    error: str
    metadata: bytes
    sync_denied_peer_ids: _containers.RepeatedScalarFieldContainer[str]
    sync_recovery_peer_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, status: _Optional[_Union[SharedObjectHealthStatus, str]] = ..., layer: _Optional[_Union[SharedObjectHealthLayer, str]] = ..., common_reason: _Optional[_Union[SharedObjectHealthCommonReason, str]] = ..., remediation_hint: _Optional[_Union[SharedObjectHealthRemediationHint, str]] = ..., error: _Optional[str] = ..., metadata: _Optional[bytes] = ..., sync_denied_peer_ids: _Optional[_Iterable[str]] = ..., sync_recovery_peer_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class SharedObjectConfig(_message.Message):
    __slots__ = ("participants", "consensus_mode", "config_chain_hash", "config_chain_seqno")
    PARTICIPANTS_FIELD_NUMBER: _ClassVar[int]
    CONSENSUS_MODE_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_HASH_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_SEQNO_FIELD_NUMBER: _ClassVar[int]
    participants: _containers.RepeatedCompositeFieldContainer[SOParticipantConfig]
    consensus_mode: SOConsensusMode
    config_chain_hash: bytes
    config_chain_seqno: int
    def __init__(self, participants: _Optional[_Iterable[_Union[SOParticipantConfig, _Mapping]]] = ..., consensus_mode: _Optional[_Union[SOConsensusMode, str]] = ..., config_chain_hash: _Optional[bytes] = ..., config_chain_seqno: _Optional[int] = ...) -> None: ...

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
    __slots__ = ("config_seqno", "config", "signed_by", "signature", "previous_hash", "change_type", "revocation_info", "leave_request")
    CONFIG_SEQNO_FIELD_NUMBER: _ClassVar[int]
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    SIGNED_BY_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    PREVIOUS_HASH_FIELD_NUMBER: _ClassVar[int]
    CHANGE_TYPE_FIELD_NUMBER: _ClassVar[int]
    REVOCATION_INFO_FIELD_NUMBER: _ClassVar[int]
    LEAVE_REQUEST_FIELD_NUMBER: _ClassVar[int]
    config_seqno: int
    config: SharedObjectConfig
    signed_by: bytes
    signature: _peer_pb2.Signature
    previous_hash: bytes
    change_type: SOConfigChangeType
    revocation_info: SORevocationInfo
    leave_request: SOLeaveRequest
    def __init__(self, config_seqno: _Optional[int] = ..., config: _Optional[_Union[SharedObjectConfig, _Mapping]] = ..., signed_by: _Optional[bytes] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ..., previous_hash: _Optional[bytes] = ..., change_type: _Optional[_Union[SOConfigChangeType, str]] = ..., revocation_info: _Optional[_Union[SORevocationInfo, _Mapping]] = ..., leave_request: _Optional[_Union[SOLeaveRequest, _Mapping]] = ...) -> None: ...

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

class SORoot(_message.Message):
    __slots__ = ("inner", "inner_seqno", "account_nonces", "validator_signatures")
    INNER_FIELD_NUMBER: _ClassVar[int]
    INNER_SEQNO_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_NONCES_FIELD_NUMBER: _ClassVar[int]
    VALIDATOR_SIGNATURES_FIELD_NUMBER: _ClassVar[int]
    inner: bytes
    inner_seqno: int
    account_nonces: _containers.RepeatedCompositeFieldContainer[SOAccountNonce]
    validator_signatures: _containers.RepeatedCompositeFieldContainer[_peer_pb2.Signature]
    def __init__(self, inner: _Optional[bytes] = ..., inner_seqno: _Optional[int] = ..., account_nonces: _Optional[_Iterable[_Union[SOAccountNonce, _Mapping]]] = ..., validator_signatures: _Optional[_Iterable[_Union[_peer_pb2.Signature, _Mapping]]] = ...) -> None: ...

class SOAccountNonce(_message.Message):
    __slots__ = ("peer_id", "nonce")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    nonce: int
    def __init__(self, peer_id: _Optional[str] = ..., nonce: _Optional[int] = ...) -> None: ...

class SORootInner(_message.Message):
    __slots__ = ("seqno", "state_data")
    SEQNO_FIELD_NUMBER: _ClassVar[int]
    STATE_DATA_FIELD_NUMBER: _ClassVar[int]
    seqno: int
    state_data: bytes
    def __init__(self, seqno: _Optional[int] = ..., state_data: _Optional[bytes] = ...) -> None: ...

class SOOperation(_message.Message):
    __slots__ = ("inner", "signature")
    INNER_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    inner: bytes
    signature: _peer_pb2.Signature
    def __init__(self, inner: _Optional[bytes] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ...) -> None: ...

class SOOperationInner(_message.Message):
    __slots__ = ("peer_id", "local_id", "nonce", "op_data")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    LOCAL_ID_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    OP_DATA_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    local_id: str
    nonce: int
    op_data: bytes
    def __init__(self, peer_id: _Optional[str] = ..., local_id: _Optional[str] = ..., nonce: _Optional[int] = ..., op_data: _Optional[bytes] = ...) -> None: ...

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

class SOOperationRejection(_message.Message):
    __slots__ = ("inner", "signature")
    INNER_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    inner: bytes
    signature: _peer_pb2.Signature
    def __init__(self, inner: _Optional[bytes] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ...) -> None: ...

class SOOperationRejectionInner(_message.Message):
    __slots__ = ("peer_id", "op_nonce", "local_id", "error_details")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    OP_NONCE_FIELD_NUMBER: _ClassVar[int]
    LOCAL_ID_FIELD_NUMBER: _ClassVar[int]
    ERROR_DETAILS_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    op_nonce: int
    local_id: str
    error_details: bytes
    def __init__(self, peer_id: _Optional[str] = ..., op_nonce: _Optional[int] = ..., local_id: _Optional[str] = ..., error_details: _Optional[bytes] = ...) -> None: ...

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
    __slots__ = ("invite_id", "token_hash", "role", "target_peer_id", "max_uses", "uses", "expires_at", "revoked", "target_account_id")
    INVITE_ID_FIELD_NUMBER: _ClassVar[int]
    TOKEN_HASH_FIELD_NUMBER: _ClassVar[int]
    ROLE_FIELD_NUMBER: _ClassVar[int]
    TARGET_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    MAX_USES_FIELD_NUMBER: _ClassVar[int]
    USES_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    REVOKED_FIELD_NUMBER: _ClassVar[int]
    TARGET_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    invite_id: str
    token_hash: bytes
    role: SOParticipantRole
    target_peer_id: str
    max_uses: int
    uses: int
    expires_at: _timestamp_pb2.Timestamp
    revoked: bool
    target_account_id: str
    def __init__(self, invite_id: _Optional[str] = ..., token_hash: _Optional[bytes] = ..., role: _Optional[_Union[SOParticipantRole, str]] = ..., target_peer_id: _Optional[str] = ..., max_uses: _Optional[int] = ..., uses: _Optional[int] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., revoked: _Optional[bool] = ..., target_account_id: _Optional[str] = ...) -> None: ...

class SOState(_message.Message):
    __slots__ = ("config", "root", "root_grants", "ops", "op_rejections", "queued_account_nonces", "invites")
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    ROOT_FIELD_NUMBER: _ClassVar[int]
    ROOT_GRANTS_FIELD_NUMBER: _ClassVar[int]
    OPS_FIELD_NUMBER: _ClassVar[int]
    OP_REJECTIONS_FIELD_NUMBER: _ClassVar[int]
    QUEUED_ACCOUNT_NONCES_FIELD_NUMBER: _ClassVar[int]
    INVITES_FIELD_NUMBER: _ClassVar[int]
    config: SharedObjectConfig
    root: SORoot
    root_grants: _containers.RepeatedCompositeFieldContainer[SOGrant]
    ops: _containers.RepeatedCompositeFieldContainer[SOOperation]
    op_rejections: _containers.RepeatedCompositeFieldContainer[SOPeerOpRejections]
    queued_account_nonces: _containers.RepeatedCompositeFieldContainer[SOAccountNonce]
    invites: _containers.RepeatedCompositeFieldContainer[SOInvite]
    def __init__(self, config: _Optional[_Union[SharedObjectConfig, _Mapping]] = ..., root: _Optional[_Union[SORoot, _Mapping]] = ..., root_grants: _Optional[_Iterable[_Union[SOGrant, _Mapping]]] = ..., ops: _Optional[_Iterable[_Union[SOOperation, _Mapping]]] = ..., op_rejections: _Optional[_Iterable[_Union[SOPeerOpRejections, _Mapping]]] = ..., queued_account_nonces: _Optional[_Iterable[_Union[SOAccountNonce, _Mapping]]] = ..., invites: _Optional[_Iterable[_Union[SOInvite, _Mapping]]] = ...) -> None: ...

class SOPeerOpRejections(_message.Message):
    __slots__ = ("peer_id", "rejections")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    REJECTIONS_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    rejections: _containers.RepeatedCompositeFieldContainer[SOOperationRejection]
    def __init__(self, peer_id: _Optional[str] = ..., rejections: _Optional[_Iterable[_Union[SOOperationRejection, _Mapping]]] = ...) -> None: ...

class SOClearOperationResult(_message.Message):
    __slots__ = ("inner", "signature")
    INNER_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    inner: bytes
    signature: _peer_pb2.Signature
    def __init__(self, inner: _Optional[bytes] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ...) -> None: ...

class SOClearOperationResultInner(_message.Message):
    __slots__ = ("peer_id", "local_id")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    LOCAL_ID_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    local_id: str
    def __init__(self, peer_id: _Optional[str] = ..., local_id: _Optional[str] = ...) -> None: ...

class SOKeyEpoch(_message.Message):
    __slots__ = ("epoch", "seqno_start", "seqno_end", "grants")
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    SEQNO_START_FIELD_NUMBER: _ClassVar[int]
    SEQNO_END_FIELD_NUMBER: _ClassVar[int]
    GRANTS_FIELD_NUMBER: _ClassVar[int]
    epoch: int
    seqno_start: int
    seqno_end: int
    grants: _containers.RepeatedCompositeFieldContainer[SOGrant]
    def __init__(self, epoch: _Optional[int] = ..., seqno_start: _Optional[int] = ..., seqno_end: _Optional[int] = ..., grants: _Optional[_Iterable[_Union[SOGrant, _Mapping]]] = ...) -> None: ...

class SOConfigChainResponse(_message.Message):
    __slots__ = ("config_changes", "key_epochs")
    CONFIG_CHANGES_FIELD_NUMBER: _ClassVar[int]
    KEY_EPOCHS_FIELD_NUMBER: _ClassVar[int]
    config_changes: _containers.RepeatedCompositeFieldContainer[SOConfigChange]
    key_epochs: _containers.RepeatedCompositeFieldContainer[SOKeyEpoch]
    def __init__(self, config_changes: _Optional[_Iterable[_Union[SOConfigChange, _Mapping]]] = ..., key_epochs: _Optional[_Iterable[_Union[SOKeyEpoch, _Mapping]]] = ...) -> None: ...

class QueuedSOOperation(_message.Message):
    __slots__ = ("local_id", "op_data")
    LOCAL_ID_FIELD_NUMBER: _ClassVar[int]
    OP_DATA_FIELD_NUMBER: _ClassVar[int]
    local_id: str
    op_data: bytes
    def __init__(self, local_id: _Optional[str] = ..., op_data: _Optional[bytes] = ...) -> None: ...

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
