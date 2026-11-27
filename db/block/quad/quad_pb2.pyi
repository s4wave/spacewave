from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional

DESCRIPTOR: _descriptor.FileDescriptor

class Quad(_message.Message):
    __slots__ = ("subject", "predicate", "obj", "label")
    SUBJECT_FIELD_NUMBER: _ClassVar[int]
    PREDICATE_FIELD_NUMBER: _ClassVar[int]
    OBJ_FIELD_NUMBER: _ClassVar[int]
    LABEL_FIELD_NUMBER: _ClassVar[int]
    subject: str
    predicate: str
    obj: str
    label: str
    def __init__(self, subject: _Optional[str] = ..., predicate: _Optional[str] = ..., obj: _Optional[str] = ..., label: _Optional[str] = ...) -> None: ...
