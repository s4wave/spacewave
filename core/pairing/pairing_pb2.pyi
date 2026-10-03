from core.sobject import sobject_pb2 as _sobject_pb2
from core.session import session_pb2 as _session_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class AccountOutcome(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    AccountOutcome_UNSPECIFIED: _ClassVar[AccountOutcome]
    AccountOutcome_SIGN_IN_OFFERED: _ClassVar[AccountOutcome]
    AccountOutcome_SIGN_IN_RECEIVING: _ClassVar[AccountOutcome]
    AccountOutcome_MERGE_INTO_OFFERED: _ClassVar[AccountOutcome]
    AccountOutcome_MERGE_INTO_RECEIVING: _ClassVar[AccountOutcome]
AccountOutcome_UNSPECIFIED: AccountOutcome
AccountOutcome_SIGN_IN_OFFERED: AccountOutcome
AccountOutcome_SIGN_IN_RECEIVING: AccountOutcome
AccountOutcome_MERGE_INTO_OFFERED: AccountOutcome
AccountOutcome_MERGE_INTO_RECEIVING: AccountOutcome

class Approval(_message.Message):
    __slots__ = ("confirmed", "rejected", "operation_context")
    CONFIRMED_FIELD_NUMBER: _ClassVar[int]
    REJECTED_FIELD_NUMBER: _ClassVar[int]
    OPERATION_CONTEXT_FIELD_NUMBER: _ClassVar[int]
    confirmed: bool
    rejected: bool
    operation_context: str
    def __init__(self, confirmed: _Optional[bool] = ..., rejected: _Optional[bool] = ..., operation_context: _Optional[str] = ...) -> None: ...

class AccountOffer(_message.Message):
    __slots__ = ("account_id", "settings_id", "operation_id", "storage_peer_id", "display_name", "provider_id", "provider_endpoint", "revoked_session_peer_ids", "active_session_peer_ids", "selection_context", "machine_name", "space_count", "session_count")
    ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    SETTINGS_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATION_ID_FIELD_NUMBER: _ClassVar[int]
    STORAGE_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    REVOKED_SESSION_PEER_IDS_FIELD_NUMBER: _ClassVar[int]
    ACTIVE_SESSION_PEER_IDS_FIELD_NUMBER: _ClassVar[int]
    SELECTION_CONTEXT_FIELD_NUMBER: _ClassVar[int]
    MACHINE_NAME_FIELD_NUMBER: _ClassVar[int]
    SPACE_COUNT_FIELD_NUMBER: _ClassVar[int]
    SESSION_COUNT_FIELD_NUMBER: _ClassVar[int]
    account_id: str
    settings_id: str
    operation_id: str
    storage_peer_id: str
    display_name: str
    provider_id: str
    provider_endpoint: str
    revoked_session_peer_ids: _containers.RepeatedScalarFieldContainer[str]
    active_session_peer_ids: _containers.RepeatedScalarFieldContainer[str]
    selection_context: str
    machine_name: str
    space_count: int
    session_count: int
    def __init__(self, account_id: _Optional[str] = ..., settings_id: _Optional[str] = ..., operation_id: _Optional[str] = ..., storage_peer_id: _Optional[str] = ..., display_name: _Optional[str] = ..., provider_id: _Optional[str] = ..., provider_endpoint: _Optional[str] = ..., revoked_session_peer_ids: _Optional[_Iterable[str]] = ..., active_session_peer_ids: _Optional[_Iterable[str]] = ..., selection_context: _Optional[str] = ..., machine_name: _Optional[str] = ..., space_count: _Optional[int] = ..., session_count: _Optional[int] = ...) -> None: ...

class AccountChoice(_message.Message):
    __slots__ = ("offered_account", "receiving_account", "outcome")
    OFFERED_ACCOUNT_FIELD_NUMBER: _ClassVar[int]
    RECEIVING_ACCOUNT_FIELD_NUMBER: _ClassVar[int]
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    offered_account: AccountOffer
    receiving_account: AccountOffer
    outcome: AccountOutcome
    def __init__(self, offered_account: _Optional[_Union[AccountOffer, _Mapping]] = ..., receiving_account: _Optional[_Union[AccountOffer, _Mapping]] = ..., outcome: _Optional[_Union[AccountOutcome, str]] = ...) -> None: ...

class Identity(_message.Message):
    __slots__ = ("session_ref", "session_proof", "storage_proof")
    SESSION_REF_FIELD_NUMBER: _ClassVar[int]
    SESSION_PROOF_FIELD_NUMBER: _ClassVar[int]
    STORAGE_PROOF_FIELD_NUMBER: _ClassVar[int]
    session_ref: _session_pb2.SessionRef
    session_proof: _sobject_pb2.SOJoinResponse
    storage_proof: _sobject_pb2.SOJoinResponse
    def __init__(self, session_ref: _Optional[_Union[_session_pb2.SessionRef, _Mapping]] = ..., session_proof: _Optional[_Union[_sobject_pb2.SOJoinResponse, _Mapping]] = ..., storage_proof: _Optional[_Union[_sobject_pb2.SOJoinResponse, _Mapping]] = ...) -> None: ...

class SharedObject(_message.Message):
    __slots__ = ("entry", "state", "config_lineage")
    ENTRY_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    CONFIG_LINEAGE_FIELD_NUMBER: _ClassVar[int]
    entry: _sobject_pb2.SharedObjectListEntry
    state: _sobject_pb2.SOState
    config_lineage: _containers.RepeatedCompositeFieldContainer[_sobject_pb2.SOConfigChange]
    def __init__(self, entry: _Optional[_Union[_sobject_pb2.SharedObjectListEntry, _Mapping]] = ..., state: _Optional[_Union[_sobject_pb2.SOState, _Mapping]] = ..., config_lineage: _Optional[_Iterable[_Union[_sobject_pb2.SOConfigChange, _Mapping]]] = ...) -> None: ...

class Frame(_message.Message):
    __slots__ = ("account", "identity", "object", "complete", "error", "choice")
    ACCOUNT_FIELD_NUMBER: _ClassVar[int]
    IDENTITY_FIELD_NUMBER: _ClassVar[int]
    OBJECT_FIELD_NUMBER: _ClassVar[int]
    COMPLETE_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    CHOICE_FIELD_NUMBER: _ClassVar[int]
    account: AccountOffer
    identity: Identity
    object: SharedObject
    complete: bool
    error: str
    choice: AccountChoice
    def __init__(self, account: _Optional[_Union[AccountOffer, _Mapping]] = ..., identity: _Optional[_Union[Identity, _Mapping]] = ..., object: _Optional[_Union[SharedObject, _Mapping]] = ..., complete: _Optional[bool] = ..., error: _Optional[str] = ..., choice: _Optional[_Union[AccountChoice, _Mapping]] = ...) -> None: ...
