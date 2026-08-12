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

class SORevocationReason(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_REVOCATION_REASON_UNKNOWN: _ClassVar[SORevocationReason]
    SO_REVOCATION_REASON_SESSION_REVOKED: _ClassVar[SORevocationReason]
    SO_REVOCATION_REASON_ORG_REMOVED: _ClassVar[SORevocationReason]
    SO_REVOCATION_REASON_OWNER_REMOVED: _ClassVar[SORevocationReason]
    SO_REVOCATION_REASON_INVITE_REVOKED: _ClassVar[SORevocationReason]

class SOJournalRecordKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_JOURNAL_RECORD_KIND_UNSPECIFIED: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_INTENT: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_SENT: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_RECEIPT: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_BODY_PROJECTION: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_STALE_TRANSFORM_EPOCH: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_RECOVERY_BLOCKED: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_LINEAGE_RECOVERY_BLOCKED: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP: _ClassVar[SOJournalRecordKind]
    SO_JOURNAL_RECORD_KIND_RESEND_AUTHORIZED: _ClassVar[SOJournalRecordKind]

class SOJournalAttemptState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_JOURNAL_ATTEMPT_STATE_UNSPECIFIED: _ClassVar[SOJournalAttemptState]
    SO_JOURNAL_ATTEMPT_STATE_INTENT_DURABLE: _ClassVar[SOJournalAttemptState]
    SO_JOURNAL_ATTEMPT_STATE_ENVELOPE_DURABLE: _ClassVar[SOJournalAttemptState]
    SO_JOURNAL_ATTEMPT_STATE_SENT: _ClassVar[SOJournalAttemptState]
    SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE: _ClassVar[SOJournalAttemptState]
    SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH: _ClassVar[SOJournalAttemptState]
    SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED: _ClassVar[SOJournalAttemptState]

class SOJournalOutcome(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_JOURNAL_OUTCOME_UNSPECIFIED: _ClassVar[SOJournalOutcome]
    SO_JOURNAL_OUTCOME_ACCEPTED: _ClassVar[SOJournalOutcome]
    SO_JOURNAL_OUTCOME_REJECTED: _ClassVar[SOJournalOutcome]

class SOJournalReadiness(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_JOURNAL_READINESS_UNSPECIFIED: _ClassVar[SOJournalReadiness]
    SO_JOURNAL_READINESS_READY: _ClassVar[SOJournalReadiness]
    SO_JOURNAL_READINESS_MISSING: _ClassVar[SOJournalReadiness]
    SO_JOURNAL_READINESS_CORRUPT: _ClassVar[SOJournalReadiness]
    SO_JOURNAL_READINESS_OBSOLETE: _ClassVar[SOJournalReadiness]

class SOJournalRecoveryReason(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_JOURNAL_RECOVERY_REASON_UNSPECIFIED: _ClassVar[SOJournalRecoveryReason]
    SO_JOURNAL_RECOVERY_REASON_STALE_TRANSFORM_EPOCH: _ClassVar[SOJournalRecoveryReason]
    SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE: _ClassVar[SOJournalRecoveryReason]
    SO_JOURNAL_RECOVERY_REASON_AUTHORITY_FAILURE: _ClassVar[SOJournalRecoveryReason]
    SO_JOURNAL_RECOVERY_REASON_BODY_MISSING: _ClassVar[SOJournalRecoveryReason]
    SO_JOURNAL_RECOVERY_REASON_BODY_CORRUPT: _ClassVar[SOJournalRecoveryReason]
    SO_JOURNAL_RECOVERY_REASON_BODY_OBSOLETE: _ClassVar[SOJournalRecoveryReason]

class SOReceiptState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SO_RECEIPT_STATE_UNSPECIFIED: _ClassVar[SOReceiptState]
    SO_RECEIPT_STATE_NO_RECORD: _ClassVar[SOReceiptState]
    SO_RECEIPT_STATE_PENDING: _ClassVar[SOReceiptState]
    SO_RECEIPT_STATE_ACCEPTED: _ClassVar[SOReceiptState]
    SO_RECEIPT_STATE_REJECTED: _ClassVar[SOReceiptState]
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
SO_REVOCATION_REASON_UNKNOWN: SORevocationReason
SO_REVOCATION_REASON_SESSION_REVOKED: SORevocationReason
SO_REVOCATION_REASON_ORG_REMOVED: SORevocationReason
SO_REVOCATION_REASON_OWNER_REMOVED: SORevocationReason
SO_REVOCATION_REASON_INVITE_REVOKED: SORevocationReason
SO_JOURNAL_RECORD_KIND_UNSPECIFIED: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_INTENT: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_SENT: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_RECEIPT: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_ACKNOWLEDGEMENT: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_BODY_PROJECTION: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_STALE_TRANSFORM_EPOCH: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_RECOVERY_BLOCKED: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_LINEAGE_RECOVERY_BLOCKED: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_RECEIPT_LOOKUP: SOJournalRecordKind
SO_JOURNAL_RECORD_KIND_RESEND_AUTHORIZED: SOJournalRecordKind
SO_JOURNAL_ATTEMPT_STATE_UNSPECIFIED: SOJournalAttemptState
SO_JOURNAL_ATTEMPT_STATE_INTENT_DURABLE: SOJournalAttemptState
SO_JOURNAL_ATTEMPT_STATE_ENVELOPE_DURABLE: SOJournalAttemptState
SO_JOURNAL_ATTEMPT_STATE_SENT: SOJournalAttemptState
SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE: SOJournalAttemptState
SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH: SOJournalAttemptState
SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED: SOJournalAttemptState
SO_JOURNAL_OUTCOME_UNSPECIFIED: SOJournalOutcome
SO_JOURNAL_OUTCOME_ACCEPTED: SOJournalOutcome
SO_JOURNAL_OUTCOME_REJECTED: SOJournalOutcome
SO_JOURNAL_READINESS_UNSPECIFIED: SOJournalReadiness
SO_JOURNAL_READINESS_READY: SOJournalReadiness
SO_JOURNAL_READINESS_MISSING: SOJournalReadiness
SO_JOURNAL_READINESS_CORRUPT: SOJournalReadiness
SO_JOURNAL_READINESS_OBSOLETE: SOJournalReadiness
SO_JOURNAL_RECOVERY_REASON_UNSPECIFIED: SOJournalRecoveryReason
SO_JOURNAL_RECOVERY_REASON_STALE_TRANSFORM_EPOCH: SOJournalRecoveryReason
SO_JOURNAL_RECOVERY_REASON_KEY_UNAVAILABLE: SOJournalRecoveryReason
SO_JOURNAL_RECOVERY_REASON_AUTHORITY_FAILURE: SOJournalRecoveryReason
SO_JOURNAL_RECOVERY_REASON_BODY_MISSING: SOJournalRecoveryReason
SO_JOURNAL_RECOVERY_REASON_BODY_CORRUPT: SOJournalRecoveryReason
SO_JOURNAL_RECOVERY_REASON_BODY_OBSOLETE: SOJournalRecoveryReason
SO_RECEIPT_STATE_UNSPECIFIED: SOReceiptState
SO_RECEIPT_STATE_NO_RECORD: SOReceiptState
SO_RECEIPT_STATE_PENDING: SOReceiptState
SO_RECEIPT_STATE_ACCEPTED: SOReceiptState
SO_RECEIPT_STATE_REJECTED: SOReceiptState

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
    __slots__ = ("ref", "meta", "source")
    REF_FIELD_NUMBER: _ClassVar[int]
    META_FIELD_NUMBER: _ClassVar[int]
    SOURCE_FIELD_NUMBER: _ClassVar[int]
    ref: SharedObjectRef
    meta: SharedObjectMeta
    source: str
    def __init__(self, ref: _Optional[_Union[SharedObjectRef, _Mapping]] = ..., meta: _Optional[_Union[SharedObjectMeta, _Mapping]] = ..., source: _Optional[str] = ...) -> None: ...

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
    __slots__ = ("status", "layer", "common_reason", "remediation_hint", "error", "metadata")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    LAYER_FIELD_NUMBER: _ClassVar[int]
    COMMON_REASON_FIELD_NUMBER: _ClassVar[int]
    REMEDIATION_HINT_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    METADATA_FIELD_NUMBER: _ClassVar[int]
    status: SharedObjectHealthStatus
    layer: SharedObjectHealthLayer
    common_reason: SharedObjectHealthCommonReason
    remediation_hint: SharedObjectHealthRemediationHint
    error: str
    metadata: bytes
    def __init__(self, status: _Optional[_Union[SharedObjectHealthStatus, str]] = ..., layer: _Optional[_Union[SharedObjectHealthLayer, str]] = ..., common_reason: _Optional[_Union[SharedObjectHealthCommonReason, str]] = ..., remediation_hint: _Optional[_Union[SharedObjectHealthRemediationHint, str]] = ..., error: _Optional[str] = ..., metadata: _Optional[bytes] = ...) -> None: ...

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

class SORevocationInfo(_message.Message):
    __slots__ = ("reason", "timestamp", "nonce")
    REASON_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    reason: SORevocationReason
    timestamp: _timestamp_pb2.Timestamp
    nonce: int
    def __init__(self, reason: _Optional[_Union[SORevocationReason, str]] = ..., timestamp: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., nonce: _Optional[int] = ...) -> None: ...

class SOConfigChange(_message.Message):
    __slots__ = ("config_seqno", "config", "signed_by", "signature", "previous_hash", "change_type", "revocation_info")
    CONFIG_SEQNO_FIELD_NUMBER: _ClassVar[int]
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    SIGNED_BY_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    PREVIOUS_HASH_FIELD_NUMBER: _ClassVar[int]
    CHANGE_TYPE_FIELD_NUMBER: _ClassVar[int]
    REVOCATION_INFO_FIELD_NUMBER: _ClassVar[int]
    config_seqno: int
    config: SharedObjectConfig
    signed_by: bytes
    signature: _peer_pb2.Signature
    previous_hash: bytes
    change_type: SOConfigChangeType
    revocation_info: SORevocationInfo
    def __init__(self, config_seqno: _Optional[int] = ..., config: _Optional[_Union[SharedObjectConfig, _Mapping]] = ..., signed_by: _Optional[bytes] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ..., previous_hash: _Optional[bytes] = ..., change_type: _Optional[_Union[SOConfigChangeType, str]] = ..., revocation_info: _Optional[_Union[SORevocationInfo, _Mapping]] = ...) -> None: ...

class SOParticipantConfig(_message.Message):
    __slots__ = ("peer_id", "role", "entity_id")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    ROLE_FIELD_NUMBER: _ClassVar[int]
    ENTITY_ID_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    role: SOParticipantRole
    entity_id: str
    def __init__(self, peer_id: _Optional[str] = ..., role: _Optional[_Union[SOParticipantRole, str]] = ..., entity_id: _Optional[str] = ...) -> None: ...

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
    __slots__ = ("error_msg",)
    ERROR_MSG_FIELD_NUMBER: _ClassVar[int]
    error_msg: str
    def __init__(self, error_msg: _Optional[str] = ...) -> None: ...

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
    __slots__ = ("invite_id", "shared_object_id", "owner_peer_id", "provider_id", "token", "role", "target_peer_id", "expires_at", "max_uses", "signature")
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
    def __init__(self, invite_id: _Optional[str] = ..., shared_object_id: _Optional[str] = ..., owner_peer_id: _Optional[str] = ..., provider_id: _Optional[str] = ..., token: _Optional[bytes] = ..., role: _Optional[_Union[SOParticipantRole, str]] = ..., target_peer_id: _Optional[str] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., max_uses: _Optional[int] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ...) -> None: ...

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

class SOMutationKey(_message.Message):
    __slots__ = ("origin_scope_id", "shared_object_id", "participant_peer_id", "local_id")
    ORIGIN_SCOPE_ID_FIELD_NUMBER: _ClassVar[int]
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    PARTICIPANT_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    LOCAL_ID_FIELD_NUMBER: _ClassVar[int]
    origin_scope_id: bytes
    shared_object_id: str
    participant_peer_id: str
    local_id: str
    def __init__(self, origin_scope_id: _Optional[bytes] = ..., shared_object_id: _Optional[str] = ..., participant_peer_id: _Optional[str] = ..., local_id: _Optional[str] = ...) -> None: ...

class SOJournalLineage(_message.Message):
    __slots__ = ("root_key", "supersedes")
    ROOT_KEY_FIELD_NUMBER: _ClassVar[int]
    SUPERSEDES_FIELD_NUMBER: _ClassVar[int]
    root_key: SOMutationKey
    supersedes: SOMutationKey
    def __init__(self, root_key: _Optional[_Union[SOMutationKey, _Mapping]] = ..., supersedes: _Optional[_Union[SOMutationKey, _Mapping]] = ...) -> None: ...

class SOJournalVersionTuple(_message.Message):
    __slots__ = ("local_version", "remote_version", "transform_epoch", "config_chain_digest")
    LOCAL_VERSION_FIELD_NUMBER: _ClassVar[int]
    REMOTE_VERSION_FIELD_NUMBER: _ClassVar[int]
    TRANSFORM_EPOCH_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_DIGEST_FIELD_NUMBER: _ClassVar[int]
    local_version: int
    remote_version: int
    transform_epoch: int
    config_chain_digest: bytes
    def __init__(self, local_version: _Optional[int] = ..., remote_version: _Optional[int] = ..., transform_epoch: _Optional[int] = ..., config_chain_digest: _Optional[bytes] = ...) -> None: ...

class SOJournalIntent(_message.Message):
    __slots__ = ("key", "lineage", "version", "canonical_operation")
    KEY_FIELD_NUMBER: _ClassVar[int]
    LINEAGE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    CANONICAL_OPERATION_FIELD_NUMBER: _ClassVar[int]
    key: SOMutationKey
    lineage: SOJournalLineage
    version: SOJournalVersionTuple
    canonical_operation: bytes
    def __init__(self, key: _Optional[_Union[SOMutationKey, _Mapping]] = ..., lineage: _Optional[_Union[SOJournalLineage, _Mapping]] = ..., version: _Optional[_Union[SOJournalVersionTuple, _Mapping]] = ..., canonical_operation: _Optional[bytes] = ...) -> None: ...

class SOJournalEncryptedPayload(_message.Message):
    __slots__ = ("nonce", "ciphertext")
    NONCE_FIELD_NUMBER: _ClassVar[int]
    CIPHERTEXT_FIELD_NUMBER: _ClassVar[int]
    nonce: bytes
    ciphertext: bytes
    def __init__(self, nonce: _Optional[bytes] = ..., ciphertext: _Optional[bytes] = ...) -> None: ...

class SOJournalLookup(_message.Message):
    __slots__ = ("key", "state", "receipt", "response", "response_digest", "config_chain_digest")
    KEY_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    RECEIPT_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_DIGEST_FIELD_NUMBER: _ClassVar[int]
    key: SOMutationKey
    state: SOReceiptState
    receipt: SOJournalReceipt
    response: bytes
    response_digest: bytes
    config_chain_digest: bytes
    def __init__(self, key: _Optional[_Union[SOMutationKey, _Mapping]] = ..., state: _Optional[_Union[SOReceiptState, str]] = ..., receipt: _Optional[_Union[SOJournalReceipt, _Mapping]] = ..., response: _Optional[bytes] = ..., response_digest: _Optional[bytes] = ..., config_chain_digest: _Optional[bytes] = ...) -> None: ...

class SOJournalReceipt(_message.Message):
    __slots__ = ("key", "envelope_digest", "outcome", "terminal_receipt", "terminal_receipt_digest", "authoritative_root_seqno", "authoritative_root_digest", "config_chain_digest", "terminal_unix_millis", "supersedes")
    KEY_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_RECEIPT_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_RECEIPT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    AUTHORITATIVE_ROOT_SEQNO_FIELD_NUMBER: _ClassVar[int]
    AUTHORITATIVE_ROOT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_DIGEST_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_UNIX_MILLIS_FIELD_NUMBER: _ClassVar[int]
    SUPERSEDES_FIELD_NUMBER: _ClassVar[int]
    key: SOMutationKey
    envelope_digest: bytes
    outcome: SOJournalOutcome
    terminal_receipt: bytes
    terminal_receipt_digest: bytes
    authoritative_root_seqno: int
    authoritative_root_digest: bytes
    config_chain_digest: bytes
    terminal_unix_millis: int
    supersedes: SOMutationKey
    def __init__(self, key: _Optional[_Union[SOMutationKey, _Mapping]] = ..., envelope_digest: _Optional[bytes] = ..., outcome: _Optional[_Union[SOJournalOutcome, str]] = ..., terminal_receipt: _Optional[bytes] = ..., terminal_receipt_digest: _Optional[bytes] = ..., authoritative_root_seqno: _Optional[int] = ..., authoritative_root_digest: _Optional[bytes] = ..., config_chain_digest: _Optional[bytes] = ..., terminal_unix_millis: _Optional[int] = ..., supersedes: _Optional[_Union[SOMutationKey, _Mapping]] = ...) -> None: ...

class SOJournalAcknowledgement(_message.Message):
    __slots__ = ("key", "receipt_digest", "acknowledged_unix_millis")
    KEY_FIELD_NUMBER: _ClassVar[int]
    RECEIPT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    ACKNOWLEDGED_UNIX_MILLIS_FIELD_NUMBER: _ClassVar[int]
    key: SOMutationKey
    receipt_digest: bytes
    acknowledged_unix_millis: int
    def __init__(self, key: _Optional[_Union[SOMutationKey, _Mapping]] = ..., receipt_digest: _Optional[bytes] = ..., acknowledged_unix_millis: _Optional[int] = ...) -> None: ...

class SOJournalProjection(_message.Message):
    __slots__ = ("key", "receipt_digest", "authoritative_root_seqno", "authoritative_root_digest")
    KEY_FIELD_NUMBER: _ClassVar[int]
    RECEIPT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    AUTHORITATIVE_ROOT_SEQNO_FIELD_NUMBER: _ClassVar[int]
    AUTHORITATIVE_ROOT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    key: SOMutationKey
    receipt_digest: bytes
    authoritative_root_seqno: int
    authoritative_root_digest: bytes
    def __init__(self, key: _Optional[_Union[SOMutationKey, _Mapping]] = ..., receipt_digest: _Optional[bytes] = ..., authoritative_root_seqno: _Optional[int] = ..., authoritative_root_digest: _Optional[bytes] = ...) -> None: ...

class SOJournalRecord(_message.Message):
    __slots__ = ("format_version", "sequence", "kind", "key", "lineage", "version", "intent", "envelope", "receipt", "acknowledgement", "projection", "readiness", "recovery_reason", "attempt_state", "envelope_digest", "lookup")
    FORMAT_VERSION_FIELD_NUMBER: _ClassVar[int]
    SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    KIND_FIELD_NUMBER: _ClassVar[int]
    KEY_FIELD_NUMBER: _ClassVar[int]
    LINEAGE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    INTENT_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_FIELD_NUMBER: _ClassVar[int]
    RECEIPT_FIELD_NUMBER: _ClassVar[int]
    ACKNOWLEDGEMENT_FIELD_NUMBER: _ClassVar[int]
    PROJECTION_FIELD_NUMBER: _ClassVar[int]
    READINESS_FIELD_NUMBER: _ClassVar[int]
    RECOVERY_REASON_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_STATE_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    LOOKUP_FIELD_NUMBER: _ClassVar[int]
    format_version: int
    sequence: int
    kind: SOJournalRecordKind
    key: SOMutationKey
    lineage: SOJournalLineage
    version: SOJournalVersionTuple
    intent: SOJournalEncryptedPayload
    envelope: SOJournalEncryptedPayload
    receipt: SOJournalReceipt
    acknowledgement: SOJournalAcknowledgement
    projection: SOJournalProjection
    readiness: SOJournalReadiness
    recovery_reason: SOJournalRecoveryReason
    attempt_state: SOJournalAttemptState
    envelope_digest: bytes
    lookup: SOJournalLookup
    def __init__(self, format_version: _Optional[int] = ..., sequence: _Optional[int] = ..., kind: _Optional[_Union[SOJournalRecordKind, str]] = ..., key: _Optional[_Union[SOMutationKey, _Mapping]] = ..., lineage: _Optional[_Union[SOJournalLineage, _Mapping]] = ..., version: _Optional[_Union[SOJournalVersionTuple, _Mapping]] = ..., intent: _Optional[_Union[SOJournalEncryptedPayload, _Mapping]] = ..., envelope: _Optional[_Union[SOJournalEncryptedPayload, _Mapping]] = ..., receipt: _Optional[_Union[SOJournalReceipt, _Mapping]] = ..., acknowledgement: _Optional[_Union[SOJournalAcknowledgement, _Mapping]] = ..., projection: _Optional[_Union[SOJournalProjection, _Mapping]] = ..., readiness: _Optional[_Union[SOJournalReadiness, str]] = ..., recovery_reason: _Optional[_Union[SOJournalRecoveryReason, str]] = ..., attempt_state: _Optional[_Union[SOJournalAttemptState, str]] = ..., envelope_digest: _Optional[bytes] = ..., lookup: _Optional[_Union[SOJournalLookup, _Mapping]] = ...) -> None: ...

class SOJournalCheckpointAttempt(_message.Message):
    __slots__ = ("key", "lineage", "version", "state", "readiness", "intent", "envelope", "envelope_digest", "receipt", "acknowledgement", "projection", "lookup", "send_attempted", "resend_authorized", "lineage_recovery_blocked", "intent_sequence", "envelope_sequence", "checkpoint_eligible")
    KEY_FIELD_NUMBER: _ClassVar[int]
    LINEAGE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    READINESS_FIELD_NUMBER: _ClassVar[int]
    INTENT_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    RECEIPT_FIELD_NUMBER: _ClassVar[int]
    ACKNOWLEDGEMENT_FIELD_NUMBER: _ClassVar[int]
    PROJECTION_FIELD_NUMBER: _ClassVar[int]
    LOOKUP_FIELD_NUMBER: _ClassVar[int]
    SEND_ATTEMPTED_FIELD_NUMBER: _ClassVar[int]
    RESEND_AUTHORIZED_FIELD_NUMBER: _ClassVar[int]
    LINEAGE_RECOVERY_BLOCKED_FIELD_NUMBER: _ClassVar[int]
    INTENT_SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    CHECKPOINT_ELIGIBLE_FIELD_NUMBER: _ClassVar[int]
    key: SOMutationKey
    lineage: SOJournalLineage
    version: SOJournalVersionTuple
    state: SOJournalAttemptState
    readiness: SOJournalReadiness
    intent: SOJournalEncryptedPayload
    envelope: SOJournalEncryptedPayload
    envelope_digest: bytes
    receipt: SOJournalReceipt
    acknowledgement: SOJournalAcknowledgement
    projection: SOJournalProjection
    lookup: SOJournalLookup
    send_attempted: bool
    resend_authorized: bool
    lineage_recovery_blocked: bool
    intent_sequence: int
    envelope_sequence: int
    checkpoint_eligible: bool
    def __init__(self, key: _Optional[_Union[SOMutationKey, _Mapping]] = ..., lineage: _Optional[_Union[SOJournalLineage, _Mapping]] = ..., version: _Optional[_Union[SOJournalVersionTuple, _Mapping]] = ..., state: _Optional[_Union[SOJournalAttemptState, str]] = ..., readiness: _Optional[_Union[SOJournalReadiness, str]] = ..., intent: _Optional[_Union[SOJournalEncryptedPayload, _Mapping]] = ..., envelope: _Optional[_Union[SOJournalEncryptedPayload, _Mapping]] = ..., envelope_digest: _Optional[bytes] = ..., receipt: _Optional[_Union[SOJournalReceipt, _Mapping]] = ..., acknowledgement: _Optional[_Union[SOJournalAcknowledgement, _Mapping]] = ..., projection: _Optional[_Union[SOJournalProjection, _Mapping]] = ..., lookup: _Optional[_Union[SOJournalLookup, _Mapping]] = ..., send_attempted: _Optional[bool] = ..., resend_authorized: _Optional[bool] = ..., lineage_recovery_blocked: _Optional[bool] = ..., intent_sequence: _Optional[int] = ..., envelope_sequence: _Optional[int] = ..., checkpoint_eligible: _Optional[bool] = ...) -> None: ...

class SOJournalCheckpoint(_message.Message):
    __slots__ = ("journal_identity", "generation", "next_sequence", "attempts")
    JOURNAL_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    GENERATION_FIELD_NUMBER: _ClassVar[int]
    NEXT_SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    ATTEMPTS_FIELD_NUMBER: _ClassVar[int]
    journal_identity: bytes
    generation: int
    next_sequence: int
    attempts: _containers.RepeatedCompositeFieldContainer[SOJournalCheckpointAttempt]
    def __init__(self, journal_identity: _Optional[bytes] = ..., generation: _Optional[int] = ..., next_sequence: _Optional[int] = ..., attempts: _Optional[_Iterable[_Union[SOJournalCheckpointAttempt, _Mapping]]] = ...) -> None: ...

class SOTerminalReceiptAccepted(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SOTerminalReceiptInner(_message.Message):
    __slots__ = ("key", "envelope_digest", "accepted", "signed_rejection", "authoritative_root_seqno", "authoritative_root_digest", "config_chain_digest", "consensus_mode", "validator_set_digest", "terminal_unix_millis", "supersedes")
    KEY_FIELD_NUMBER: _ClassVar[int]
    ENVELOPE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    SIGNED_REJECTION_FIELD_NUMBER: _ClassVar[int]
    AUTHORITATIVE_ROOT_SEQNO_FIELD_NUMBER: _ClassVar[int]
    AUTHORITATIVE_ROOT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    CONFIG_CHAIN_DIGEST_FIELD_NUMBER: _ClassVar[int]
    CONSENSUS_MODE_FIELD_NUMBER: _ClassVar[int]
    VALIDATOR_SET_DIGEST_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_UNIX_MILLIS_FIELD_NUMBER: _ClassVar[int]
    SUPERSEDES_FIELD_NUMBER: _ClassVar[int]
    key: SOMutationKey
    envelope_digest: bytes
    accepted: SOTerminalReceiptAccepted
    signed_rejection: SOOperationRejection
    authoritative_root_seqno: int
    authoritative_root_digest: bytes
    config_chain_digest: bytes
    consensus_mode: SOConsensusMode
    validator_set_digest: bytes
    terminal_unix_millis: int
    supersedes: SOMutationKey
    def __init__(self, key: _Optional[_Union[SOMutationKey, _Mapping]] = ..., envelope_digest: _Optional[bytes] = ..., accepted: _Optional[_Union[SOTerminalReceiptAccepted, _Mapping]] = ..., signed_rejection: _Optional[_Union[SOOperationRejection, _Mapping]] = ..., authoritative_root_seqno: _Optional[int] = ..., authoritative_root_digest: _Optional[bytes] = ..., config_chain_digest: _Optional[bytes] = ..., consensus_mode: _Optional[_Union[SOConsensusMode, str]] = ..., validator_set_digest: _Optional[bytes] = ..., terminal_unix_millis: _Optional[int] = ..., supersedes: _Optional[_Union[SOMutationKey, _Mapping]] = ...) -> None: ...

class SOTerminalReceipt(_message.Message):
    __slots__ = ("inner", "validator_signatures")
    INNER_FIELD_NUMBER: _ClassVar[int]
    VALIDATOR_SIGNATURES_FIELD_NUMBER: _ClassVar[int]
    inner: bytes
    validator_signatures: _containers.RepeatedCompositeFieldContainer[_peer_pb2.Signature]
    def __init__(self, inner: _Optional[bytes] = ..., validator_signatures: _Optional[_Iterable[_Union[_peer_pb2.Signature, _Mapping]]] = ...) -> None: ...
