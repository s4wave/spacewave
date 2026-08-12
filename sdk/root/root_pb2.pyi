from core.changelog import changelog_pb2 as _changelog_pb2
from core.provider import provider_pb2 as _provider_pb2
from core.session import session_pb2 as _session_pb2
from core.space import space_pb2 as _space_pb2
from net.hash import hash_pb2 as _hash_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SpaceRootKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SpaceRootKind_UNSPECIFIED: _ClassVar[SpaceRootKind]
    SpaceRootKind_NATIVE_DIRECTORY: _ClassVar[SpaceRootKind]
    SpaceRootKind_S4WAVE_FILE: _ClassVar[SpaceRootKind]

class SpaceRootOpenMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SpaceRootOpenMode_UNSPECIFIED: _ClassVar[SpaceRootOpenMode]
    SpaceRootOpenMode_OPEN_EXISTING: _ClassVar[SpaceRootOpenMode]
    SpaceRootOpenMode_CREATE: _ClassVar[SpaceRootOpenMode]

class SpaceRootStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SpaceRootStatus_UNKNOWN: _ClassVar[SpaceRootStatus]
    SpaceRootStatus_READY: _ClassVar[SpaceRootStatus]
    SpaceRootStatus_MISSING: _ClassVar[SpaceRootStatus]
    SpaceRootStatus_UNSUPPORTED: _ClassVar[SpaceRootStatus]
    SpaceRootStatus_INVALID: _ClassVar[SpaceRootStatus]

class SpaceRootRuntimeStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SpaceRootRuntimeStatus_IDLE: _ClassVar[SpaceRootRuntimeStatus]
    SpaceRootRuntimeStatus_CONNECTING: _ClassVar[SpaceRootRuntimeStatus]
    SpaceRootRuntimeStatus_STARTING: _ClassVar[SpaceRootRuntimeStatus]
    SpaceRootRuntimeStatus_READY: _ClassVar[SpaceRootRuntimeStatus]
    SpaceRootRuntimeStatus_ERROR: _ClassVar[SpaceRootRuntimeStatus]
SpaceRootKind_UNSPECIFIED: SpaceRootKind
SpaceRootKind_NATIVE_DIRECTORY: SpaceRootKind
SpaceRootKind_S4WAVE_FILE: SpaceRootKind
SpaceRootOpenMode_UNSPECIFIED: SpaceRootOpenMode
SpaceRootOpenMode_OPEN_EXISTING: SpaceRootOpenMode
SpaceRootOpenMode_CREATE: SpaceRootOpenMode
SpaceRootStatus_UNKNOWN: SpaceRootStatus
SpaceRootStatus_READY: SpaceRootStatus
SpaceRootStatus_MISSING: SpaceRootStatus
SpaceRootStatus_UNSUPPORTED: SpaceRootStatus
SpaceRootStatus_INVALID: SpaceRootStatus
SpaceRootRuntimeStatus_IDLE: SpaceRootRuntimeStatus
SpaceRootRuntimeStatus_CONNECTING: SpaceRootRuntimeStatus
SpaceRootRuntimeStatus_STARTING: SpaceRootRuntimeStatus
SpaceRootRuntimeStatus_READY: SpaceRootRuntimeStatus
SpaceRootRuntimeStatus_ERROR: SpaceRootRuntimeStatus

class LookupProviderRequest(_message.Message):
    __slots__ = ("provider_id",)
    PROVIDER_ID_FIELD_NUMBER: _ClassVar[int]
    provider_id: str
    def __init__(self, provider_id: _Optional[str] = ...) -> None: ...

class LookupProviderResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class MountSessionRequest(_message.Message):
    __slots__ = ("session_ref",)
    SESSION_REF_FIELD_NUMBER: _ClassVar[int]
    session_ref: _session_pb2.SessionRef
    def __init__(self, session_ref: _Optional[_Union[_session_pb2.SessionRef, _Mapping]] = ...) -> None: ...

class MountSessionResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class MountSessionByIdxRequest(_message.Message):
    __slots__ = ("session_idx",)
    SESSION_IDX_FIELD_NUMBER: _ClassVar[int]
    session_idx: int
    def __init__(self, session_idx: _Optional[int] = ...) -> None: ...

class MountSessionByIdxResponse(_message.Message):
    __slots__ = ("resource_id", "session_ref", "not_found")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    SESSION_REF_FIELD_NUMBER: _ClassVar[int]
    NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    session_ref: _session_pb2.SessionRef
    not_found: bool
    def __init__(self, resource_id: _Optional[int] = ..., session_ref: _Optional[_Union[_session_pb2.SessionRef, _Mapping]] = ..., not_found: _Optional[bool] = ...) -> None: ...

class ListProvidersRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListProvidersResponse(_message.Message):
    __slots__ = ("providers",)
    PROVIDERS_FIELD_NUMBER: _ClassVar[int]
    providers: _containers.RepeatedCompositeFieldContainer[_provider_pb2.ProviderInfo]
    def __init__(self, providers: _Optional[_Iterable[_Union[_provider_pb2.ProviderInfo, _Mapping]]] = ...) -> None: ...

class ListSessionsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListSessionsResponse(_message.Message):
    __slots__ = ("sessions",)
    SESSIONS_FIELD_NUMBER: _ClassVar[int]
    sessions: _containers.RepeatedCompositeFieldContainer[_session_pb2.SessionListEntry]
    def __init__(self, sessions: _Optional[_Iterable[_Union[_session_pb2.SessionListEntry, _Mapping]]] = ...) -> None: ...

class WatchSessionsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchSessionsResponse(_message.Message):
    __slots__ = ("sessions",)
    SESSIONS_FIELD_NUMBER: _ClassVar[int]
    sessions: _containers.RepeatedCompositeFieldContainer[_session_pb2.SessionListEntry]
    def __init__(self, sessions: _Optional[_Iterable[_Union[_session_pb2.SessionListEntry, _Mapping]]] = ...) -> None: ...

class WatchAllAccountStatusesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SessionAccountStatus(_message.Message):
    __slots__ = ("session_idx", "account_status")
    SESSION_IDX_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_STATUS_FIELD_NUMBER: _ClassVar[int]
    session_idx: int
    account_status: _provider_pb2.ProviderAccountStatus
    def __init__(self, session_idx: _Optional[int] = ..., account_status: _Optional[_Union[_provider_pb2.ProviderAccountStatus, str]] = ...) -> None: ...

class WatchAllAccountStatusesResponse(_message.Message):
    __slots__ = ("statuses",)
    STATUSES_FIELD_NUMBER: _ClassVar[int]
    statuses: _containers.RepeatedCompositeFieldContainer[SessionAccountStatus]
    def __init__(self, statuses: _Optional[_Iterable[_Union[SessionAccountStatus, _Mapping]]] = ...) -> None: ...

class GetSessionMetadataRequest(_message.Message):
    __slots__ = ("session_idx",)
    SESSION_IDX_FIELD_NUMBER: _ClassVar[int]
    session_idx: int
    def __init__(self, session_idx: _Optional[int] = ...) -> None: ...

class GetSessionMetadataResponse(_message.Message):
    __slots__ = ("metadata", "not_found")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    metadata: _session_pb2.SessionMetadata
    not_found: bool
    def __init__(self, metadata: _Optional[_Union[_session_pb2.SessionMetadata, _Mapping]] = ..., not_found: _Optional[bool] = ...) -> None: ...

class WatchSessionMetadataRequest(_message.Message):
    __slots__ = ("session_idx",)
    SESSION_IDX_FIELD_NUMBER: _ClassVar[int]
    session_idx: int
    def __init__(self, session_idx: _Optional[int] = ...) -> None: ...

class WatchSessionMetadataResponse(_message.Message):
    __slots__ = ("metadata", "not_found")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    metadata: _session_pb2.SessionMetadata
    not_found: bool
    def __init__(self, metadata: _Optional[_Union[_session_pb2.SessionMetadata, _Mapping]] = ..., not_found: _Optional[bool] = ...) -> None: ...

class UnlockSessionByIdxRequest(_message.Message):
    __slots__ = ("session_idx", "pin")
    SESSION_IDX_FIELD_NUMBER: _ClassVar[int]
    PIN_FIELD_NUMBER: _ClassVar[int]
    session_idx: int
    pin: bytes
    def __init__(self, session_idx: _Optional[int] = ..., pin: _Optional[bytes] = ...) -> None: ...

class UnlockSessionByIdxResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class DeleteSessionRequest(_message.Message):
    __slots__ = ("session_idx",)
    SESSION_IDX_FIELD_NUMBER: _ClassVar[int]
    session_idx: int
    def __init__(self, session_idx: _Optional[int] = ...) -> None: ...

class DeleteSessionResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ResetSessionByIdxRequest(_message.Message):
    __slots__ = ("session_idx", "credential")
    SESSION_IDX_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_FIELD_NUMBER: _ClassVar[int]
    session_idx: int
    credential: _session_pb2.EntityCredential
    def __init__(self, session_idx: _Optional[int] = ..., credential: _Optional[_Union[_session_pb2.EntityCredential, _Mapping]] = ...) -> None: ...

class ResetSessionByIdxResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class MarshalHashRequest(_message.Message):
    __slots__ = ("hash",)
    HASH_FIELD_NUMBER: _ClassVar[int]
    hash: _hash_pb2.Hash
    def __init__(self, hash: _Optional[_Union[_hash_pb2.Hash, _Mapping]] = ...) -> None: ...

class MarshalHashResponse(_message.Message):
    __slots__ = ("hash_str",)
    HASH_STR_FIELD_NUMBER: _ClassVar[int]
    hash_str: str
    def __init__(self, hash_str: _Optional[str] = ...) -> None: ...

class ParseHashRequest(_message.Message):
    __slots__ = ("hash_str",)
    HASH_STR_FIELD_NUMBER: _ClassVar[int]
    hash_str: str
    def __init__(self, hash_str: _Optional[str] = ...) -> None: ...

class ParseHashResponse(_message.Message):
    __slots__ = ("hash",)
    HASH_FIELD_NUMBER: _ClassVar[int]
    hash: _hash_pb2.Hash
    def __init__(self, hash: _Optional[_Union[_hash_pb2.Hash, _Mapping]] = ...) -> None: ...

class HashSumRequest(_message.Message):
    __slots__ = ("hash_type", "data")
    HASH_TYPE_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    hash_type: _hash_pb2.HashType
    data: bytes
    def __init__(self, hash_type: _Optional[_Union[_hash_pb2.HashType, str]] = ..., data: _Optional[bytes] = ...) -> None: ...

class HashSumResponse(_message.Message):
    __slots__ = ("hash",)
    HASH_FIELD_NUMBER: _ClassVar[int]
    hash: _hash_pb2.Hash
    def __init__(self, hash: _Optional[_Union[_hash_pb2.Hash, _Mapping]] = ...) -> None: ...

class HashValidateRequest(_message.Message):
    __slots__ = ("hash",)
    HASH_FIELD_NUMBER: _ClassVar[int]
    hash: _hash_pb2.Hash
    def __init__(self, hash: _Optional[_Union[_hash_pb2.Hash, _Mapping]] = ...) -> None: ...

class HashValidateResponse(_message.Message):
    __slots__ = ("valid", "error")
    VALID_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    valid: bool
    error: str
    def __init__(self, valid: _Optional[bool] = ..., error: _Optional[str] = ...) -> None: ...

class AccessStateAtomRequest(_message.Message):
    __slots__ = ("store_id",)
    STORE_ID_FIELD_NUMBER: _ClassVar[int]
    store_id: str
    def __init__(self, store_id: _Optional[str] = ...) -> None: ...

class AccessStateAtomResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class WatchStateAtomsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchStateAtomsResponse(_message.Message):
    __slots__ = ("store_ids", "store_count")
    STORE_IDS_FIELD_NUMBER: _ClassVar[int]
    STORE_COUNT_FIELD_NUMBER: _ClassVar[int]
    store_ids: _containers.RepeatedScalarFieldContainer[str]
    store_count: int
    def __init__(self, store_ids: _Optional[_Iterable[str]] = ..., store_count: _Optional[int] = ...) -> None: ...

class NativeSpaceRootMetadata(_message.Message):
    __slots__ = ("path",)
    PATH_FIELD_NUMBER: _ClassVar[int]
    path: str
    def __init__(self, path: _Optional[str] = ...) -> None: ...

class BrowserSpaceRootMetadata(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SpaceRootAliasRecord(_message.Message):
    __slots__ = ("alias_id", "display_name", "kind", "open_mode", "native", "status", "status_message", "browser", "created_at_unix_ms", "updated_at_unix_ms")
    ALIAS_ID_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    KIND_FIELD_NUMBER: _ClassVar[int]
    OPEN_MODE_FIELD_NUMBER: _ClassVar[int]
    NATIVE_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    STATUS_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    BROWSER_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    alias_id: str
    display_name: str
    kind: SpaceRootKind
    open_mode: SpaceRootOpenMode
    native: NativeSpaceRootMetadata
    status: SpaceRootStatus
    status_message: str
    browser: BrowserSpaceRootMetadata
    created_at_unix_ms: int
    updated_at_unix_ms: int
    def __init__(self, alias_id: _Optional[str] = ..., display_name: _Optional[str] = ..., kind: _Optional[_Union[SpaceRootKind, str]] = ..., open_mode: _Optional[_Union[SpaceRootOpenMode, str]] = ..., native: _Optional[_Union[NativeSpaceRootMetadata, _Mapping]] = ..., status: _Optional[_Union[SpaceRootStatus, str]] = ..., status_message: _Optional[str] = ..., browser: _Optional[_Union[BrowserSpaceRootMetadata, _Mapping]] = ..., created_at_unix_ms: _Optional[int] = ..., updated_at_unix_ms: _Optional[int] = ...) -> None: ...

class ListSpaceRootAliasesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListSpaceRootAliasesResponse(_message.Message):
    __slots__ = ("records",)
    RECORDS_FIELD_NUMBER: _ClassVar[int]
    records: _containers.RepeatedCompositeFieldContainer[SpaceRootAliasRecord]
    def __init__(self, records: _Optional[_Iterable[_Union[SpaceRootAliasRecord, _Mapping]]] = ...) -> None: ...

class WatchSpaceRootAliasesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchSpaceRootAliasesResponse(_message.Message):
    __slots__ = ("records",)
    RECORDS_FIELD_NUMBER: _ClassVar[int]
    records: _containers.RepeatedCompositeFieldContainer[SpaceRootAliasRecord]
    def __init__(self, records: _Optional[_Iterable[_Union[SpaceRootAliasRecord, _Mapping]]] = ...) -> None: ...

class UpsertSpaceRootAliasRequest(_message.Message):
    __slots__ = ("record",)
    RECORD_FIELD_NUMBER: _ClassVar[int]
    record: SpaceRootAliasRecord
    def __init__(self, record: _Optional[_Union[SpaceRootAliasRecord, _Mapping]] = ...) -> None: ...

class UpsertSpaceRootAliasResponse(_message.Message):
    __slots__ = ("record",)
    RECORD_FIELD_NUMBER: _ClassVar[int]
    record: SpaceRootAliasRecord
    def __init__(self, record: _Optional[_Union[SpaceRootAliasRecord, _Mapping]] = ...) -> None: ...

class RemoveSpaceRootAliasRequest(_message.Message):
    __slots__ = ("alias_id",)
    ALIAS_ID_FIELD_NUMBER: _ClassVar[int]
    alias_id: str
    def __init__(self, alias_id: _Optional[str] = ...) -> None: ...

class RemoveSpaceRootAliasResponse(_message.Message):
    __slots__ = ("not_found",)
    NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    not_found: bool
    def __init__(self, not_found: _Optional[bool] = ...) -> None: ...

class WatchSpaceRootRuntimeRequest(_message.Message):
    __slots__ = ("alias_id", "autostart")
    ALIAS_ID_FIELD_NUMBER: _ClassVar[int]
    AUTOSTART_FIELD_NUMBER: _ClassVar[int]
    alias_id: str
    autostart: bool
    def __init__(self, alias_id: _Optional[str] = ..., autostart: _Optional[bool] = ...) -> None: ...

class WatchSpaceRootRuntimeResponse(_message.Message):
    __slots__ = ("status", "alias_id", "state_path", "socket_path", "sessions", "error", "runtime_sessions")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    ALIAS_ID_FIELD_NUMBER: _ClassVar[int]
    STATE_PATH_FIELD_NUMBER: _ClassVar[int]
    SOCKET_PATH_FIELD_NUMBER: _ClassVar[int]
    SESSIONS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_SESSIONS_FIELD_NUMBER: _ClassVar[int]
    status: SpaceRootRuntimeStatus
    alias_id: str
    state_path: str
    socket_path: str
    sessions: _containers.RepeatedCompositeFieldContainer[_session_pb2.SessionListEntry]
    error: str
    runtime_sessions: _containers.RepeatedCompositeFieldContainer[SpaceRootRuntimeSession]
    def __init__(self, status: _Optional[_Union[SpaceRootRuntimeStatus, str]] = ..., alias_id: _Optional[str] = ..., state_path: _Optional[str] = ..., socket_path: _Optional[str] = ..., sessions: _Optional[_Iterable[_Union[_session_pb2.SessionListEntry, _Mapping]]] = ..., error: _Optional[str] = ..., runtime_sessions: _Optional[_Iterable[_Union[SpaceRootRuntimeSession, _Mapping]]] = ...) -> None: ...

class SpaceRootRuntimeSession(_message.Message):
    __slots__ = ("session", "metadata", "spaces", "error")
    SESSION_FIELD_NUMBER: _ClassVar[int]
    METADATA_FIELD_NUMBER: _ClassVar[int]
    SPACES_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    session: _session_pb2.SessionListEntry
    metadata: _session_pb2.SessionMetadata
    spaces: _containers.RepeatedCompositeFieldContainer[_space_pb2.SpaceSoListEntry]
    error: str
    def __init__(self, session: _Optional[_Union[_session_pb2.SessionListEntry, _Mapping]] = ..., metadata: _Optional[_Union[_session_pb2.SessionMetadata, _Mapping]] = ..., spaces: _Optional[_Iterable[_Union[_space_pb2.SpaceSoListEntry, _Mapping]]] = ..., error: _Optional[str] = ...) -> None: ...

class GetChangelogRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetChangelogResponse(_message.Message):
    __slots__ = ("changelog",)
    CHANGELOG_FIELD_NUMBER: _ClassVar[int]
    changelog: _changelog_pb2.Changelog
    def __init__(self, changelog: _Optional[_Union[_changelog_pb2.Changelog, _Mapping]] = ...) -> None: ...

class GetDebugDbRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetDebugDbResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class GetCdnRequest(_message.Message):
    __slots__ = ("cdn_id",)
    CDN_ID_FIELD_NUMBER: _ClassVar[int]
    cdn_id: str
    def __init__(self, cdn_id: _Optional[str] = ...) -> None: ...

class GetCdnResponse(_message.Message):
    __slots__ = ("resource_id", "cdn_space_id")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    CDN_SPACE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    cdn_space_id: str
    def __init__(self, resource_id: _Optional[int] = ..., cdn_space_id: _Optional[str] = ...) -> None: ...

class AccessWebListenerRequest(_message.Message):
    __slots__ = ("listen_multiaddr", "background")
    LISTEN_MULTIADDR_FIELD_NUMBER: _ClassVar[int]
    BACKGROUND_FIELD_NUMBER: _ClassVar[int]
    listen_multiaddr: str
    background: bool
    def __init__(self, listen_multiaddr: _Optional[str] = ..., background: _Optional[bool] = ...) -> None: ...

class AccessWebListenerResponse(_message.Message):
    __slots__ = ("resource_id", "listener_id", "listen_multiaddr", "url", "bootstrap_secret", "reused")
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    LISTENER_ID_FIELD_NUMBER: _ClassVar[int]
    LISTEN_MULTIADDR_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    BOOTSTRAP_SECRET_FIELD_NUMBER: _ClassVar[int]
    REUSED_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    listener_id: str
    listen_multiaddr: str
    url: str
    bootstrap_secret: str
    reused: bool
    def __init__(self, resource_id: _Optional[int] = ..., listener_id: _Optional[str] = ..., listen_multiaddr: _Optional[str] = ..., url: _Optional[str] = ..., bootstrap_secret: _Optional[str] = ..., reused: _Optional[bool] = ...) -> None: ...

class WatchWebListenersRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WebListenerInfo(_message.Message):
    __slots__ = ("listener_id", "listen_multiaddr", "url", "background")
    LISTENER_ID_FIELD_NUMBER: _ClassVar[int]
    LISTEN_MULTIADDR_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    BACKGROUND_FIELD_NUMBER: _ClassVar[int]
    listener_id: str
    listen_multiaddr: str
    url: str
    background: bool
    def __init__(self, listener_id: _Optional[str] = ..., listen_multiaddr: _Optional[str] = ..., url: _Optional[str] = ..., background: _Optional[bool] = ...) -> None: ...

class WatchWebListenersResponse(_message.Message):
    __slots__ = ("listeners",)
    LISTENERS_FIELD_NUMBER: _ClassVar[int]
    listeners: _containers.RepeatedCompositeFieldContainer[WebListenerInfo]
    def __init__(self, listeners: _Optional[_Iterable[_Union[WebListenerInfo, _Mapping]]] = ...) -> None: ...

class StopWebListenerRequest(_message.Message):
    __slots__ = ("listener_id",)
    LISTENER_ID_FIELD_NUMBER: _ClassVar[int]
    listener_id: str
    def __init__(self, listener_id: _Optional[str] = ...) -> None: ...

class StopWebListenerResponse(_message.Message):
    __slots__ = ("not_found",)
    NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    not_found: bool
    def __init__(self, not_found: _Optional[bool] = ...) -> None: ...

class ListenerYieldPrompt(_message.Message):
    __slots__ = ("prompt_id", "requester_name", "socket_path", "deadline_unix_ms")
    PROMPT_ID_FIELD_NUMBER: _ClassVar[int]
    REQUESTER_NAME_FIELD_NUMBER: _ClassVar[int]
    SOCKET_PATH_FIELD_NUMBER: _ClassVar[int]
    DEADLINE_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    prompt_id: str
    requester_name: str
    socket_path: str
    deadline_unix_ms: int
    def __init__(self, prompt_id: _Optional[str] = ..., requester_name: _Optional[str] = ..., socket_path: _Optional[str] = ..., deadline_unix_ms: _Optional[int] = ...) -> None: ...

class WatchListenerYieldPromptsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchListenerYieldPromptsResponse(_message.Message):
    __slots__ = ("prompts",)
    PROMPTS_FIELD_NUMBER: _ClassVar[int]
    prompts: _containers.RepeatedCompositeFieldContainer[ListenerYieldPrompt]
    def __init__(self, prompts: _Optional[_Iterable[_Union[ListenerYieldPrompt, _Mapping]]] = ...) -> None: ...

class RespondToListenerYieldPromptRequest(_message.Message):
    __slots__ = ("prompt_id", "allow")
    PROMPT_ID_FIELD_NUMBER: _ClassVar[int]
    ALLOW_FIELD_NUMBER: _ClassVar[int]
    prompt_id: str
    allow: bool
    def __init__(self, prompt_id: _Optional[str] = ..., allow: _Optional[bool] = ...) -> None: ...

class RespondToListenerYieldPromptResponse(_message.Message):
    __slots__ = ("not_found",)
    NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    not_found: bool
    def __init__(self, not_found: _Optional[bool] = ...) -> None: ...

class RuntimeHandoffState(_message.Message):
    __slots__ = ("active", "requester_name", "socket_path", "since_unix_ms")
    ACTIVE_FIELD_NUMBER: _ClassVar[int]
    REQUESTER_NAME_FIELD_NUMBER: _ClassVar[int]
    SOCKET_PATH_FIELD_NUMBER: _ClassVar[int]
    SINCE_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    active: bool
    requester_name: str
    socket_path: str
    since_unix_ms: int
    def __init__(self, active: _Optional[bool] = ..., requester_name: _Optional[str] = ..., socket_path: _Optional[str] = ..., since_unix_ms: _Optional[int] = ...) -> None: ...

class WatchRuntimeHandoffRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchRuntimeHandoffResponse(_message.Message):
    __slots__ = ("state",)
    STATE_FIELD_NUMBER: _ClassVar[int]
    state: RuntimeHandoffState
    def __init__(self, state: _Optional[_Union[RuntimeHandoffState, _Mapping]] = ...) -> None: ...

class ReclaimRuntimeRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ReclaimRuntimeResponse(_message.Message):
    __slots__ = ("reclaimed",)
    RECLAIMED_FIELD_NUMBER: _ClassVar[int]
    reclaimed: bool
    def __init__(self, reclaimed: _Optional[bool] = ...) -> None: ...

class WatchListenerStatusRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchListenerStatusResponse(_message.Message):
    __slots__ = ("socket_path", "listening", "connected_clients")
    SOCKET_PATH_FIELD_NUMBER: _ClassVar[int]
    LISTENING_FIELD_NUMBER: _ClassVar[int]
    CONNECTED_CLIENTS_FIELD_NUMBER: _ClassVar[int]
    socket_path: str
    listening: bool
    connected_clients: int
    def __init__(self, socket_path: _Optional[str] = ..., listening: _Optional[bool] = ..., connected_clients: _Optional[int] = ...) -> None: ...
