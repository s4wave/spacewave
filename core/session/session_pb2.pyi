from core.provider import provider_pb2 as _provider_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SessionType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SESSION_TYPE_UNKNOWN: _ClassVar[SessionType]
    SESSION_TYPE_USER: _ClassVar[SessionType]
    SESSION_TYPE_APP: _ClassVar[SessionType]
    SESSION_TYPE_DEVICE: _ClassVar[SessionType]

class SessionLockMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SESSION_LOCK_MODE_AUTO_UNLOCK: _ClassVar[SessionLockMode]
    SESSION_LOCK_MODE_PIN_ENCRYPTED: _ClassVar[SessionLockMode]

class SessionRecoveryState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SESSION_RECOVERY_STATE_UNKNOWN: _ClassVar[SessionRecoveryState]
    SESSION_RECOVERY_STATE_AVAILABLE: _ClassVar[SessionRecoveryState]
    SESSION_RECOVERY_STATE_UNAVAILABLE: _ClassVar[SessionRecoveryState]
SESSION_TYPE_UNKNOWN: SessionType
SESSION_TYPE_USER: SessionType
SESSION_TYPE_APP: SessionType
SESSION_TYPE_DEVICE: SessionType
SESSION_LOCK_MODE_AUTO_UNLOCK: SessionLockMode
SESSION_LOCK_MODE_PIN_ENCRYPTED: SessionLockMode
SESSION_RECOVERY_STATE_UNKNOWN: SessionRecoveryState
SESSION_RECOVERY_STATE_AVAILABLE: SessionRecoveryState
SESSION_RECOVERY_STATE_UNAVAILABLE: SessionRecoveryState

class SessionRef(_message.Message):
    __slots__ = ("provider_resource_ref",)
    PROVIDER_RESOURCE_REF_FIELD_NUMBER: _ClassVar[int]
    provider_resource_ref: _provider_pb2.ProviderResourceRef
    def __init__(self, provider_resource_ref: _Optional[_Union[_provider_pb2.ProviderResourceRef, _Mapping]] = ...) -> None: ...

class SessionListEntry(_message.Message):
    __slots__ = ("session_index", "session_ref")
    SESSION_INDEX_FIELD_NUMBER: _ClassVar[int]
    SESSION_REF_FIELD_NUMBER: _ClassVar[int]
    session_index: int
    session_ref: SessionRef
    def __init__(self, session_index: _Optional[int] = ..., session_ref: _Optional[_Union[SessionRef, _Mapping]] = ...) -> None: ...

class SessionMetadata(_message.Message):
    __slots__ = ("display_name", "provider_display_name", "provider_account_id", "lock_mode", "created_at", "cloud_account_id", "cloud_entity_id", "provider_id", "recovery_state", "direct_p2p_disabled")
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    LOCK_MODE_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    CLOUD_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    CLOUD_ENTITY_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ID_FIELD_NUMBER: _ClassVar[int]
    RECOVERY_STATE_FIELD_NUMBER: _ClassVar[int]
    DIRECT_P2P_DISABLED_FIELD_NUMBER: _ClassVar[int]
    display_name: str
    provider_display_name: str
    provider_account_id: str
    lock_mode: SessionLockMode
    created_at: int
    cloud_account_id: str
    cloud_entity_id: str
    provider_id: str
    recovery_state: SessionRecoveryState
    direct_p2p_disabled: bool
    def __init__(self, display_name: _Optional[str] = ..., provider_display_name: _Optional[str] = ..., provider_account_id: _Optional[str] = ..., lock_mode: _Optional[_Union[SessionLockMode, str]] = ..., created_at: _Optional[int] = ..., cloud_account_id: _Optional[str] = ..., cloud_entity_id: _Optional[str] = ..., provider_id: _Optional[str] = ..., recovery_state: _Optional[_Union[SessionRecoveryState, str]] = ..., direct_p2p_disabled: _Optional[bool] = ...) -> None: ...

class EntityKeypair(_message.Message):
    __slots__ = ("peer_id", "auth_method", "auth_params")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    AUTH_METHOD_FIELD_NUMBER: _ClassVar[int]
    AUTH_PARAMS_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    auth_method: str
    auth_params: bytes
    def __init__(self, peer_id: _Optional[str] = ..., auth_method: _Optional[str] = ..., auth_params: _Optional[bytes] = ...) -> None: ...

class EntityCredential(_message.Message):
    __slots__ = ("password", "pem_private_key")
    PASSWORD_FIELD_NUMBER: _ClassVar[int]
    PEM_PRIVATE_KEY_FIELD_NUMBER: _ClassVar[int]
    password: str
    pem_private_key: bytes
    def __init__(self, password: _Optional[str] = ..., pem_private_key: _Optional[bytes] = ...) -> None: ...
