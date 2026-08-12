from net.hash import hash_pb2 as _hash_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Signature(_message.Message):
    __slots__ = ("pub_key", "hash_type", "sig_data")
    PUB_KEY_FIELD_NUMBER: _ClassVar[int]
    HASH_TYPE_FIELD_NUMBER: _ClassVar[int]
    SIG_DATA_FIELD_NUMBER: _ClassVar[int]
    pub_key: bytes
    hash_type: _hash_pb2.HashType
    sig_data: bytes
    def __init__(self, pub_key: _Optional[bytes] = ..., hash_type: _Optional[_Union[_hash_pb2.HashType, str]] = ..., sig_data: _Optional[bytes] = ...) -> None: ...

class SignedMsg(_message.Message):
    __slots__ = ("from_peer_id", "signature", "data")
    FROM_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    from_peer_id: str
    signature: Signature
    data: bytes
    def __init__(self, from_peer_id: _Optional[str] = ..., signature: _Optional[_Union[Signature, _Mapping]] = ..., data: _Optional[bytes] = ...) -> None: ...
