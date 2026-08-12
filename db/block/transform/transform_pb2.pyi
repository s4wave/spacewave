from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Config(_message.Message):
    __slots__ = ("steps",)
    STEPS_FIELD_NUMBER: _ClassVar[int]
    steps: _containers.RepeatedCompositeFieldContainer[StepConfig]
    def __init__(self, steps: _Optional[_Iterable[_Union[StepConfig, _Mapping]]] = ...) -> None: ...

class StepConfig(_message.Message):
    __slots__ = ("id", "config")
    ID_FIELD_NUMBER: _ClassVar[int]
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    id: str
    config: bytes
    def __init__(self, id: _Optional[str] = ..., config: _Optional[bytes] = ...) -> None: ...
