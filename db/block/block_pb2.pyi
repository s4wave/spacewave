from net.hash import hash_pb2 as _hash_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class StoreFeature(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    STORE_FEATURE_UNKNOWN: _ClassVar[StoreFeature]
    STORE_FEATURE_NATIVE_BATCH_PUT: _ClassVar[StoreFeature]
    STORE_FEATURE_NATIVE_BATCH_EXISTS: _ClassVar[StoreFeature]
    STORE_FEATURE_SELF_BUFFERED: _ClassVar[StoreFeature]

class OverlayMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    UPPER_ONLY: _ClassVar[OverlayMode]
    LOWER_ONLY: _ClassVar[OverlayMode]
    UPPER_CACHE: _ClassVar[OverlayMode]
    LOWER_CACHE: _ClassVar[OverlayMode]
    UPPER_READ_CACHE: _ClassVar[OverlayMode]
    LOWER_READ_CACHE: _ClassVar[OverlayMode]
    UPPER_WRITE_CACHE: _ClassVar[OverlayMode]
    LOWER_WRITE_CACHE: _ClassVar[OverlayMode]
    UPPER_READBACK_CACHE: _ClassVar[OverlayMode]
STORE_FEATURE_UNKNOWN: StoreFeature
STORE_FEATURE_NATIVE_BATCH_PUT: StoreFeature
STORE_FEATURE_NATIVE_BATCH_EXISTS: StoreFeature
STORE_FEATURE_SELF_BUFFERED: StoreFeature
UPPER_ONLY: OverlayMode
LOWER_ONLY: OverlayMode
UPPER_CACHE: OverlayMode
LOWER_CACHE: OverlayMode
UPPER_READ_CACHE: OverlayMode
LOWER_READ_CACHE: OverlayMode
UPPER_WRITE_CACHE: OverlayMode
LOWER_WRITE_CACHE: OverlayMode
UPPER_READBACK_CACHE: OverlayMode

class BlockRef(_message.Message):
    __slots__ = ("hash",)
    HASH_FIELD_NUMBER: _ClassVar[int]
    hash: _hash_pb2.Hash
    def __init__(self, hash: _Optional[_Union[_hash_pb2.Hash, _Mapping]] = ...) -> None: ...

class BlockObject(_message.Message):
    __slots__ = ("data", "refs", "refs_unknown")
    DATA_FIELD_NUMBER: _ClassVar[int]
    REFS_FIELD_NUMBER: _ClassVar[int]
    REFS_UNKNOWN_FIELD_NUMBER: _ClassVar[int]
    data: bytes
    refs: _containers.RepeatedCompositeFieldContainer[BlockRef]
    refs_unknown: bool
    def __init__(self, data: _Optional[bytes] = ..., refs: _Optional[_Iterable[_Union[BlockRef, _Mapping]]] = ..., refs_unknown: _Optional[bool] = ...) -> None: ...

class AccessLog(_message.Message):
    __slots__ = ("accesses",)
    ACCESSES_FIELD_NUMBER: _ClassVar[int]
    accesses: _containers.RepeatedCompositeFieldContainer[Access]
    def __init__(self, accesses: _Optional[_Iterable[_Union[Access, _Mapping]]] = ...) -> None: ...

class Access(_message.Message):
    __slots__ = ("ref", "size", "elapsed_us")
    REF_FIELD_NUMBER: _ClassVar[int]
    SIZE_FIELD_NUMBER: _ClassVar[int]
    ELAPSED_US_FIELD_NUMBER: _ClassVar[int]
    ref: BlockRef
    size: int
    elapsed_us: int
    def __init__(self, ref: _Optional[_Union[BlockRef, _Mapping]] = ..., size: _Optional[int] = ..., elapsed_us: _Optional[int] = ...) -> None: ...

class PutOpts(_message.Message):
    __slots__ = ("hash_type", "force_block_ref", "refs", "sync", "cache_fill", "refs_known", "background")
    HASH_TYPE_FIELD_NUMBER: _ClassVar[int]
    FORCE_BLOCK_REF_FIELD_NUMBER: _ClassVar[int]
    REFS_FIELD_NUMBER: _ClassVar[int]
    SYNC_FIELD_NUMBER: _ClassVar[int]
    CACHE_FILL_FIELD_NUMBER: _ClassVar[int]
    REFS_KNOWN_FIELD_NUMBER: _ClassVar[int]
    BACKGROUND_FIELD_NUMBER: _ClassVar[int]
    hash_type: _hash_pb2.HashType
    force_block_ref: BlockRef
    refs: _containers.RepeatedCompositeFieldContainer[BlockRef]
    sync: bool
    cache_fill: bool
    refs_known: bool
    background: bool
    def __init__(self, hash_type: _Optional[_Union[_hash_pb2.HashType, str]] = ..., force_block_ref: _Optional[_Union[BlockRef, _Mapping]] = ..., refs: _Optional[_Iterable[_Union[BlockRef, _Mapping]]] = ..., sync: _Optional[bool] = ..., cache_fill: _Optional[bool] = ..., refs_known: _Optional[bool] = ..., background: _Optional[bool] = ...) -> None: ...
