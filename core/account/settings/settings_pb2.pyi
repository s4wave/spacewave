from core.session import session_pb2 as _session_pb2
from sdk.command import command_pb2 as _command_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class AccountSettings(_message.Message):
    __slots__ = ("display_name", "paired_devices", "entity_keypairs", "session_presentations", "keybinding_overrides")
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    PAIRED_DEVICES_FIELD_NUMBER: _ClassVar[int]
    ENTITY_KEYPAIRS_FIELD_NUMBER: _ClassVar[int]
    SESSION_PRESENTATIONS_FIELD_NUMBER: _ClassVar[int]
    KEYBINDING_OVERRIDES_FIELD_NUMBER: _ClassVar[int]
    display_name: str
    paired_devices: _containers.RepeatedCompositeFieldContainer[PairedDevice]
    entity_keypairs: _containers.RepeatedCompositeFieldContainer[_session_pb2.EntityKeypair]
    session_presentations: _containers.RepeatedCompositeFieldContainer[SessionPresentation]
    keybinding_overrides: _command_pb2.KeybindingOverrideSet
    def __init__(self, display_name: _Optional[str] = ..., paired_devices: _Optional[_Iterable[_Union[PairedDevice, _Mapping]]] = ..., entity_keypairs: _Optional[_Iterable[_Union[_session_pb2.EntityKeypair, _Mapping]]] = ..., session_presentations: _Optional[_Iterable[_Union[SessionPresentation, _Mapping]]] = ..., keybinding_overrides: _Optional[_Union[_command_pb2.KeybindingOverrideSet, _Mapping]] = ...) -> None: ...

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
    __slots__ = ("update_display_name", "add_paired_device", "remove_paired_device", "add_entity_keypair", "remove_entity_keypair", "upsert_session_presentation", "remove_session_presentation", "replace_keybinding_override_set")
    UPDATE_DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    ADD_PAIRED_DEVICE_FIELD_NUMBER: _ClassVar[int]
    REMOVE_PAIRED_DEVICE_FIELD_NUMBER: _ClassVar[int]
    ADD_ENTITY_KEYPAIR_FIELD_NUMBER: _ClassVar[int]
    REMOVE_ENTITY_KEYPAIR_FIELD_NUMBER: _ClassVar[int]
    UPSERT_SESSION_PRESENTATION_FIELD_NUMBER: _ClassVar[int]
    REMOVE_SESSION_PRESENTATION_FIELD_NUMBER: _ClassVar[int]
    REPLACE_KEYBINDING_OVERRIDE_SET_FIELD_NUMBER: _ClassVar[int]
    update_display_name: UpdateDisplayNameOp
    add_paired_device: PairedDevice
    remove_paired_device: RemovePairedDeviceOp
    add_entity_keypair: _session_pb2.EntityKeypair
    remove_entity_keypair: RemoveEntityKeypairOp
    upsert_session_presentation: SessionPresentation
    remove_session_presentation: RemoveSessionPresentationOp
    replace_keybinding_override_set: ReplaceKeybindingOverrideSetOp
    def __init__(self, update_display_name: _Optional[_Union[UpdateDisplayNameOp, _Mapping]] = ..., add_paired_device: _Optional[_Union[PairedDevice, _Mapping]] = ..., remove_paired_device: _Optional[_Union[RemovePairedDeviceOp, _Mapping]] = ..., add_entity_keypair: _Optional[_Union[_session_pb2.EntityKeypair, _Mapping]] = ..., remove_entity_keypair: _Optional[_Union[RemoveEntityKeypairOp, _Mapping]] = ..., upsert_session_presentation: _Optional[_Union[SessionPresentation, _Mapping]] = ..., remove_session_presentation: _Optional[_Union[RemoveSessionPresentationOp, _Mapping]] = ..., replace_keybinding_override_set: _Optional[_Union[ReplaceKeybindingOverrideSetOp, _Mapping]] = ...) -> None: ...

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
