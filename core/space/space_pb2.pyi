from core.sobject import sobject_pb2 as _sobject_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SpaceSoMeta(_message.Message):
    __slots__ = ("name",)
    NAME_FIELD_NUMBER: _ClassVar[int]
    name: str
    def __init__(self, name: _Optional[str] = ...) -> None: ...

class SpaceSoListEntry(_message.Message):
    __slots__ = ("entry", "space_meta", "index_object_type")
    ENTRY_FIELD_NUMBER: _ClassVar[int]
    SPACE_META_FIELD_NUMBER: _ClassVar[int]
    INDEX_OBJECT_TYPE_FIELD_NUMBER: _ClassVar[int]
    entry: _sobject_pb2.SharedObjectListEntry
    space_meta: SpaceSoMeta
    index_object_type: str
    def __init__(self, entry: _Optional[_Union[_sobject_pb2.SharedObjectListEntry, _Mapping]] = ..., space_meta: _Optional[_Union[SpaceSoMeta, _Mapping]] = ..., index_object_type: _Optional[str] = ...) -> None: ...
