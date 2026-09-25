from db.block import block_pb2 as _block_pb2
from net.hash import hash_pb2 as _hash_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class CheckOutcome(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    CHECK_OUTCOME_UNKNOWN: _ClassVar[CheckOutcome]
    CHECK_OUTCOME_OK: _ClassVar[CheckOutcome]
    CHECK_OUTCOME_UNREACHABLE: _ClassVar[CheckOutcome]
    CHECK_OUTCOME_CREDENTIALS_REJECTED: _ClassVar[CheckOutcome]
    CHECK_OUTCOME_ACCESS_DENIED: _ClassVar[CheckOutcome]
    CHECK_OUTCOME_BUCKET_NOT_FOUND: _ClassVar[CheckOutcome]
    CHECK_OUTCOME_WRONG_REGION: _ClassVar[CheckOutcome]
    CHECK_OUTCOME_FAILED: _ClassVar[CheckOutcome]
CHECK_OUTCOME_UNKNOWN: CheckOutcome
CHECK_OUTCOME_OK: CheckOutcome
CHECK_OUTCOME_UNREACHABLE: CheckOutcome
CHECK_OUTCOME_CREDENTIALS_REJECTED: CheckOutcome
CHECK_OUTCOME_ACCESS_DENIED: CheckOutcome
CHECK_OUTCOME_BUCKET_NOT_FOUND: CheckOutcome
CHECK_OUTCOME_WRONG_REGION: CheckOutcome
CHECK_OUTCOME_FAILED: CheckOutcome

class Config(_message.Message):
    __slots__ = ("block_store_id", "client", "bucket_name", "object_prefix", "read_only", "force_hash_type", "bucket_ids", "skip_not_found", "verbose")
    BLOCK_STORE_ID_FIELD_NUMBER: _ClassVar[int]
    CLIENT_FIELD_NUMBER: _ClassVar[int]
    BUCKET_NAME_FIELD_NUMBER: _ClassVar[int]
    OBJECT_PREFIX_FIELD_NUMBER: _ClassVar[int]
    READ_ONLY_FIELD_NUMBER: _ClassVar[int]
    FORCE_HASH_TYPE_FIELD_NUMBER: _ClassVar[int]
    BUCKET_IDS_FIELD_NUMBER: _ClassVar[int]
    SKIP_NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    VERBOSE_FIELD_NUMBER: _ClassVar[int]
    block_store_id: str
    client: ClientConfig
    bucket_name: str
    object_prefix: str
    read_only: bool
    force_hash_type: _hash_pb2.HashType
    bucket_ids: _containers.RepeatedScalarFieldContainer[str]
    skip_not_found: bool
    verbose: bool
    def __init__(self, block_store_id: _Optional[str] = ..., client: _Optional[_Union[ClientConfig, _Mapping]] = ..., bucket_name: _Optional[str] = ..., object_prefix: _Optional[str] = ..., read_only: _Optional[bool] = ..., force_hash_type: _Optional[_Union[_hash_pb2.HashType, str]] = ..., bucket_ids: _Optional[_Iterable[str]] = ..., skip_not_found: _Optional[bool] = ..., verbose: _Optional[bool] = ...) -> None: ...

class BlockObject(_message.Message):
    __slots__ = ("data", "refs")
    DATA_FIELD_NUMBER: _ClassVar[int]
    REFS_FIELD_NUMBER: _ClassVar[int]
    data: bytes
    refs: _containers.RepeatedCompositeFieldContainer[_block_pb2.BlockRef]
    def __init__(self, data: _Optional[bytes] = ..., refs: _Optional[_Iterable[_Union[_block_pb2.BlockRef, _Mapping]]] = ...) -> None: ...

class ClientConfig(_message.Message):
    __slots__ = ("endpoint", "credentials", "disable_ssl", "region")
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    CREDENTIALS_FIELD_NUMBER: _ClassVar[int]
    DISABLE_SSL_FIELD_NUMBER: _ClassVar[int]
    REGION_FIELD_NUMBER: _ClassVar[int]
    endpoint: str
    credentials: Credentials
    disable_ssl: bool
    region: str
    def __init__(self, endpoint: _Optional[str] = ..., credentials: _Optional[_Union[Credentials, _Mapping]] = ..., disable_ssl: _Optional[bool] = ..., region: _Optional[str] = ...) -> None: ...

class Credentials(_message.Message):
    __slots__ = ("access_key_id", "secret_access_key", "token")
    ACCESS_KEY_ID_FIELD_NUMBER: _ClassVar[int]
    SECRET_ACCESS_KEY_FIELD_NUMBER: _ClassVar[int]
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    access_key_id: str
    secret_access_key: str
    token: str
    def __init__(self, access_key_id: _Optional[str] = ..., secret_access_key: _Optional[str] = ..., token: _Optional[str] = ...) -> None: ...

class CheckResult(_message.Message):
    __slots__ = ("outcome", "detail", "usage", "usage_error")
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    USAGE_FIELD_NUMBER: _ClassVar[int]
    USAGE_ERROR_FIELD_NUMBER: _ClassVar[int]
    outcome: CheckOutcome
    detail: str
    usage: ObjectUsage
    usage_error: str
    def __init__(self, outcome: _Optional[_Union[CheckOutcome, str]] = ..., detail: _Optional[str] = ..., usage: _Optional[_Union[ObjectUsage, _Mapping]] = ..., usage_error: _Optional[str] = ...) -> None: ...

class ObjectUsage(_message.Message):
    __slots__ = ("objects", "bytes")
    OBJECTS_FIELD_NUMBER: _ClassVar[int]
    BYTES_FIELD_NUMBER: _ClassVar[int]
    objects: int
    bytes: int
    def __init__(self, objects: _Optional[int] = ..., bytes: _Optional[int] = ...) -> None: ...
