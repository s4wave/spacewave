from core.session import session_pb2 as _session_pb2
from core.provider import provider_pb2 as _provider_pb2
from core.sobject import sobject_pb2 as _sobject_pb2
from sdk.command import command_pb2 as _command_pb2
from sdk.secret import secret_pb2 as _secret_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class AccountSettings(_message.Message):
    __slots__ = ("display_name", "paired_devices", "entity_keypairs", "session_presentations", "keybinding_overrides", "sessions", "catalog", "transition", "accepted_migrations", "storage_backends", "default_storage_backend_id", "block_store_placements", "storage_releases")
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    PAIRED_DEVICES_FIELD_NUMBER: _ClassVar[int]
    ENTITY_KEYPAIRS_FIELD_NUMBER: _ClassVar[int]
    SESSION_PRESENTATIONS_FIELD_NUMBER: _ClassVar[int]
    KEYBINDING_OVERRIDES_FIELD_NUMBER: _ClassVar[int]
    SESSIONS_FIELD_NUMBER: _ClassVar[int]
    CATALOG_FIELD_NUMBER: _ClassVar[int]
    TRANSITION_FIELD_NUMBER: _ClassVar[int]
    ACCEPTED_MIGRATIONS_FIELD_NUMBER: _ClassVar[int]
    STORAGE_BACKENDS_FIELD_NUMBER: _ClassVar[int]
    DEFAULT_STORAGE_BACKEND_ID_FIELD_NUMBER: _ClassVar[int]
    BLOCK_STORE_PLACEMENTS_FIELD_NUMBER: _ClassVar[int]
    STORAGE_RELEASES_FIELD_NUMBER: _ClassVar[int]
    display_name: str
    paired_devices: _containers.RepeatedCompositeFieldContainer[PairedDevice]
    entity_keypairs: _containers.RepeatedCompositeFieldContainer[_session_pb2.EntityKeypair]
    session_presentations: _containers.RepeatedCompositeFieldContainer[SessionPresentation]
    keybinding_overrides: _command_pb2.KeybindingOverrideSet
    sessions: _containers.RepeatedCompositeFieldContainer[AccountSession]
    catalog: _containers.RepeatedCompositeFieldContainer[AccountCatalogEntry]
    transition: _provider_pb2.AccountTransition
    accepted_migrations: _containers.RepeatedCompositeFieldContainer[_provider_pb2.AccountTransition]
    storage_backends: _containers.RepeatedCompositeFieldContainer[StorageBackend]
    default_storage_backend_id: str
    block_store_placements: _containers.RepeatedCompositeFieldContainer[BlockStorePlacement]
    storage_releases: _containers.RepeatedCompositeFieldContainer[BlockStorePlacement]
    def __init__(self, display_name: _Optional[str] = ..., paired_devices: _Optional[_Iterable[_Union[PairedDevice, _Mapping]]] = ..., entity_keypairs: _Optional[_Iterable[_Union[_session_pb2.EntityKeypair, _Mapping]]] = ..., session_presentations: _Optional[_Iterable[_Union[SessionPresentation, _Mapping]]] = ..., keybinding_overrides: _Optional[_Union[_command_pb2.KeybindingOverrideSet, _Mapping]] = ..., sessions: _Optional[_Iterable[_Union[AccountSession, _Mapping]]] = ..., catalog: _Optional[_Iterable[_Union[AccountCatalogEntry, _Mapping]]] = ..., transition: _Optional[_Union[_provider_pb2.AccountTransition, _Mapping]] = ..., accepted_migrations: _Optional[_Iterable[_Union[_provider_pb2.AccountTransition, _Mapping]]] = ..., storage_backends: _Optional[_Iterable[_Union[StorageBackend, _Mapping]]] = ..., default_storage_backend_id: _Optional[str] = ..., block_store_placements: _Optional[_Iterable[_Union[BlockStorePlacement, _Mapping]]] = ..., storage_releases: _Optional[_Iterable[_Union[BlockStorePlacement, _Mapping]]] = ...) -> None: ...

class StorageBackend(_message.Message):
    __slots__ = ("id", "display_name", "s3", "credential")
    ID_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    S3_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_FIELD_NUMBER: _ClassVar[int]
    id: str
    display_name: str
    s3: S3Location
    credential: _secret_pb2.Secret
    def __init__(self, id: _Optional[str] = ..., display_name: _Optional[str] = ..., s3: _Optional[_Union[S3Location, _Mapping]] = ..., credential: _Optional[_Union[_secret_pb2.Secret, _Mapping]] = ...) -> None: ...

class S3Location(_message.Message):
    __slots__ = ("endpoint", "region", "bucket", "object_prefix", "disable_ssl")
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    REGION_FIELD_NUMBER: _ClassVar[int]
    BUCKET_FIELD_NUMBER: _ClassVar[int]
    OBJECT_PREFIX_FIELD_NUMBER: _ClassVar[int]
    DISABLE_SSL_FIELD_NUMBER: _ClassVar[int]
    endpoint: str
    region: str
    bucket: str
    object_prefix: str
    disable_ssl: bool
    def __init__(self, endpoint: _Optional[str] = ..., region: _Optional[str] = ..., bucket: _Optional[str] = ..., object_prefix: _Optional[str] = ..., disable_ssl: _Optional[bool] = ...) -> None: ...

class BlockStorePlacement(_message.Message):
    __slots__ = ("block_store_id", "storage_backend_id")
    BLOCK_STORE_ID_FIELD_NUMBER: _ClassVar[int]
    STORAGE_BACKEND_ID_FIELD_NUMBER: _ClassVar[int]
    block_store_id: str
    storage_backend_id: str
    def __init__(self, block_store_id: _Optional[str] = ..., storage_backend_id: _Optional[str] = ...) -> None: ...

class AccountSession(_message.Message):
    __slots__ = ("peer_id", "storage_peer_id", "revoked", "revoked_by_storage_peer_id")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    STORAGE_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    REVOKED_FIELD_NUMBER: _ClassVar[int]
    REVOKED_BY_STORAGE_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    storage_peer_id: str
    revoked: bool
    revoked_by_storage_peer_id: str
    def __init__(self, peer_id: _Optional[str] = ..., storage_peer_id: _Optional[str] = ..., revoked: _Optional[bool] = ..., revoked_by_storage_peer_id: _Optional[str] = ...) -> None: ...

class AccountCatalogEntry(_message.Message):
    __slots__ = ("entry", "deleted")
    ENTRY_FIELD_NUMBER: _ClassVar[int]
    DELETED_FIELD_NUMBER: _ClassVar[int]
    entry: _sobject_pb2.SharedObjectListEntry
    deleted: bool
    def __init__(self, entry: _Optional[_Union[_sobject_pb2.SharedObjectListEntry, _Mapping]] = ..., deleted: _Optional[bool] = ...) -> None: ...

class PairedDevice(_message.Message):
    __slots__ = ("peer_id", "display_name", "paired_at")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    PAIRED_AT_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    display_name: str
    paired_at: int
    def __init__(self, peer_id: _Optional[str] = ..., display_name: _Optional[str] = ..., paired_at: _Optional[int] = ...) -> None: ...

class SessionPresentation(_message.Message):
    __slots__ = ("peer_id", "label", "device_type", "client_name", "os", "location")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    LABEL_FIELD_NUMBER: _ClassVar[int]
    DEVICE_TYPE_FIELD_NUMBER: _ClassVar[int]
    CLIENT_NAME_FIELD_NUMBER: _ClassVar[int]
    OS_FIELD_NUMBER: _ClassVar[int]
    LOCATION_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    label: str
    device_type: str
    client_name: str
    os: str
    location: str
    def __init__(self, peer_id: _Optional[str] = ..., label: _Optional[str] = ..., device_type: _Optional[str] = ..., client_name: _Optional[str] = ..., os: _Optional[str] = ..., location: _Optional[str] = ...) -> None: ...

class AccountSettingsOp(_message.Message):
    __slots__ = ("update_display_name", "add_paired_device", "remove_paired_device", "add_entity_keypair", "remove_entity_keypair", "upsert_session_presentation", "remove_session_presentation", "replace_keybinding_override_set", "upsert_account_session", "upsert_catalog_entry", "accept_account_migration", "commit_account_transition", "upsert_storage_backend", "remove_storage_backend", "set_default_storage_backend", "set_block_store_placement", "complete_storage_release")
    UPDATE_DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    ADD_PAIRED_DEVICE_FIELD_NUMBER: _ClassVar[int]
    REMOVE_PAIRED_DEVICE_FIELD_NUMBER: _ClassVar[int]
    ADD_ENTITY_KEYPAIR_FIELD_NUMBER: _ClassVar[int]
    REMOVE_ENTITY_KEYPAIR_FIELD_NUMBER: _ClassVar[int]
    UPSERT_SESSION_PRESENTATION_FIELD_NUMBER: _ClassVar[int]
    REMOVE_SESSION_PRESENTATION_FIELD_NUMBER: _ClassVar[int]
    REPLACE_KEYBINDING_OVERRIDE_SET_FIELD_NUMBER: _ClassVar[int]
    UPSERT_ACCOUNT_SESSION_FIELD_NUMBER: _ClassVar[int]
    UPSERT_CATALOG_ENTRY_FIELD_NUMBER: _ClassVar[int]
    ACCEPT_ACCOUNT_MIGRATION_FIELD_NUMBER: _ClassVar[int]
    COMMIT_ACCOUNT_TRANSITION_FIELD_NUMBER: _ClassVar[int]
    UPSERT_STORAGE_BACKEND_FIELD_NUMBER: _ClassVar[int]
    REMOVE_STORAGE_BACKEND_FIELD_NUMBER: _ClassVar[int]
    SET_DEFAULT_STORAGE_BACKEND_FIELD_NUMBER: _ClassVar[int]
    SET_BLOCK_STORE_PLACEMENT_FIELD_NUMBER: _ClassVar[int]
    COMPLETE_STORAGE_RELEASE_FIELD_NUMBER: _ClassVar[int]
    update_display_name: UpdateDisplayNameOp
    add_paired_device: PairedDevice
    remove_paired_device: RemovePairedDeviceOp
    add_entity_keypair: _session_pb2.EntityKeypair
    remove_entity_keypair: RemoveEntityKeypairOp
    upsert_session_presentation: SessionPresentation
    remove_session_presentation: RemoveSessionPresentationOp
    replace_keybinding_override_set: ReplaceKeybindingOverrideSetOp
    upsert_account_session: AccountSession
    upsert_catalog_entry: AccountCatalogEntry
    accept_account_migration: _provider_pb2.AccountTransition
    commit_account_transition: _provider_pb2.AccountTransition
    upsert_storage_backend: StorageBackend
    remove_storage_backend: RemoveStorageBackendOp
    set_default_storage_backend: SetDefaultStorageBackendOp
    set_block_store_placement: BlockStorePlacement
    complete_storage_release: BlockStorePlacement
    def __init__(self, update_display_name: _Optional[_Union[UpdateDisplayNameOp, _Mapping]] = ..., add_paired_device: _Optional[_Union[PairedDevice, _Mapping]] = ..., remove_paired_device: _Optional[_Union[RemovePairedDeviceOp, _Mapping]] = ..., add_entity_keypair: _Optional[_Union[_session_pb2.EntityKeypair, _Mapping]] = ..., remove_entity_keypair: _Optional[_Union[RemoveEntityKeypairOp, _Mapping]] = ..., upsert_session_presentation: _Optional[_Union[SessionPresentation, _Mapping]] = ..., remove_session_presentation: _Optional[_Union[RemoveSessionPresentationOp, _Mapping]] = ..., replace_keybinding_override_set: _Optional[_Union[ReplaceKeybindingOverrideSetOp, _Mapping]] = ..., upsert_account_session: _Optional[_Union[AccountSession, _Mapping]] = ..., upsert_catalog_entry: _Optional[_Union[AccountCatalogEntry, _Mapping]] = ..., accept_account_migration: _Optional[_Union[_provider_pb2.AccountTransition, _Mapping]] = ..., commit_account_transition: _Optional[_Union[_provider_pb2.AccountTransition, _Mapping]] = ..., upsert_storage_backend: _Optional[_Union[StorageBackend, _Mapping]] = ..., remove_storage_backend: _Optional[_Union[RemoveStorageBackendOp, _Mapping]] = ..., set_default_storage_backend: _Optional[_Union[SetDefaultStorageBackendOp, _Mapping]] = ..., set_block_store_placement: _Optional[_Union[BlockStorePlacement, _Mapping]] = ..., complete_storage_release: _Optional[_Union[BlockStorePlacement, _Mapping]] = ...) -> None: ...

class RemoveStorageBackendOp(_message.Message):
    __slots__ = ("storage_backend_id",)
    STORAGE_BACKEND_ID_FIELD_NUMBER: _ClassVar[int]
    storage_backend_id: str
    def __init__(self, storage_backend_id: _Optional[str] = ...) -> None: ...

class SetDefaultStorageBackendOp(_message.Message):
    __slots__ = ("storage_backend_id",)
    STORAGE_BACKEND_ID_FIELD_NUMBER: _ClassVar[int]
    storage_backend_id: str
    def __init__(self, storage_backend_id: _Optional[str] = ...) -> None: ...

class ReplaceKeybindingOverrideSetOp(_message.Message):
    __slots__ = ("expected_override_set", "override_set")
    EXPECTED_OVERRIDE_SET_FIELD_NUMBER: _ClassVar[int]
    OVERRIDE_SET_FIELD_NUMBER: _ClassVar[int]
    expected_override_set: _command_pb2.KeybindingOverrideSet
    override_set: _command_pb2.KeybindingOverrideSet
    def __init__(self, expected_override_set: _Optional[_Union[_command_pb2.KeybindingOverrideSet, _Mapping]] = ..., override_set: _Optional[_Union[_command_pb2.KeybindingOverrideSet, _Mapping]] = ...) -> None: ...

class UpdateDisplayNameOp(_message.Message):
    __slots__ = ("display_name",)
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    display_name: str
    def __init__(self, display_name: _Optional[str] = ...) -> None: ...

class RemoveEntityKeypairOp(_message.Message):
    __slots__ = ("peer_id",)
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    def __init__(self, peer_id: _Optional[str] = ...) -> None: ...

class RemovePairedDeviceOp(_message.Message):
    __slots__ = ("peer_id",)
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    def __init__(self, peer_id: _Optional[str] = ...) -> None: ...

class RemoveSessionPresentationOp(_message.Message):
    __slots__ = ("peer_id",)
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    def __init__(self, peer_id: _Optional[str] = ...) -> None: ...
