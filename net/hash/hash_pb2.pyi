from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class HashType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    HashType_UNKNOWN: _ClassVar[HashType]
    HashType_SHA256: _ClassVar[HashType]
    HashType_SHA1: _ClassVar[HashType]
    HashType_BLAKE3: _ClassVar[HashType]
HashType_UNKNOWN: HashType
HashType_SHA256: HashType
HashType_SHA1: HashType
HashType_BLAKE3: HashType

class Hash(_message.Message):
    __slots__ = ("hash_type", "hash")
    HASH_TYPE_FIELD_NUMBER: _ClassVar[int]
    HASH_FIELD_NUMBER: _ClassVar[int]
    hash_type: HashType
    hash: bytes
    def __init__(self, hash_type: _Optional[_Union[HashType, str]] = ..., hash: _Optional[bytes] = ...) -> None: ...
