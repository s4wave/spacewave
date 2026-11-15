import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class PluginApprovalState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    PluginApprovalState_UNKNOWN: _ClassVar[PluginApprovalState]
    PluginApprovalState_ALLOWED: _ClassVar[PluginApprovalState]
    PluginApprovalState_DECLINED: _ClassVar[PluginApprovalState]
    PluginApprovalState_TURNED_OFF: _ClassVar[PluginApprovalState]
PluginApprovalState_UNKNOWN: PluginApprovalState
PluginApprovalState_ALLOWED: PluginApprovalState
PluginApprovalState_DECLINED: PluginApprovalState
PluginApprovalState_TURNED_OFF: PluginApprovalState

class PluginApproval(_message.Message):
    __slots__ = ("plugin_id", "manifest_root", "state", "decided_at", "name", "version")
    PLUGIN_ID_FIELD_NUMBER: _ClassVar[int]
    MANIFEST_ROOT_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    DECIDED_AT_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    plugin_id: str
    manifest_root: str
    state: PluginApprovalState
    decided_at: _timestamp_pb2.Timestamp
    name: str
    version: str
    def __init__(self, plugin_id: _Optional[str] = ..., manifest_root: _Optional[str] = ..., state: _Optional[_Union[PluginApprovalState, str]] = ..., decided_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., name: _Optional[str] = ..., version: _Optional[str] = ...) -> None: ...
