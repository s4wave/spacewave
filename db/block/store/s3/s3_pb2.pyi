import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
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
    __slots__ = ("block_store_id", "client", "bucket_name", "object_prefix", "bucket_ids", "skip_not_found", "verbose")
    BLOCK_STORE_ID_FIELD_NUMBER: _ClassVar[int]
    CLIENT_FIELD_NUMBER: _ClassVar[int]
    BUCKET_NAME_FIELD_NUMBER: _ClassVar[int]
    OBJECT_PREFIX_FIELD_NUMBER: _ClassVar[int]
    BUCKET_IDS_FIELD_NUMBER: _ClassVar[int]
    SKIP_NOT_FOUND_FIELD_NUMBER: _ClassVar[int]
    VERBOSE_FIELD_NUMBER: _ClassVar[int]
    block_store_id: str
    client: ClientConfig
    bucket_name: str
    object_prefix: str
    bucket_ids: _containers.RepeatedScalarFieldContainer[str]
    skip_not_found: bool
    verbose: bool
    def __init__(self, block_store_id: _Optional[str] = ..., client: _Optional[_Union[ClientConfig, _Mapping]] = ..., bucket_name: _Optional[str] = ..., object_prefix: _Optional[str] = ..., bucket_ids: _Optional[_Iterable[str]] = ..., skip_not_found: _Optional[bool] = ..., verbose: _Optional[bool] = ...) -> None: ...

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
    __slots__ = ("outcome", "detail", "usage")
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    USAGE_FIELD_NUMBER: _ClassVar[int]
    outcome: CheckOutcome
    detail: str
    usage: ObjectUsage
    def __init__(self, outcome: _Optional[_Union[CheckOutcome, str]] = ..., detail: _Optional[str] = ..., usage: _Optional[_Union[ObjectUsage, _Mapping]] = ...) -> None: ...

class ObjectUsage(_message.Message):
    __slots__ = ("objects", "bytes")
    OBJECTS_FIELD_NUMBER: _ClassVar[int]
    BYTES_FIELD_NUMBER: _ClassVar[int]
    objects: int
    bytes: int
    def __init__(self, objects: _Optional[int] = ..., bytes: _Optional[int] = ...) -> None: ...

class ReclaimState(_message.Message):
    __slots__ = ("passed_at", "dead_bytes", "dead_bytes_per_day", "packs", "blocks")
    PASSED_AT_FIELD_NUMBER: _ClassVar[int]
    DEAD_BYTES_FIELD_NUMBER: _ClassVar[int]
    DEAD_BYTES_PER_DAY_FIELD_NUMBER: _ClassVar[int]
    PACKS_FIELD_NUMBER: _ClassVar[int]
    BLOCKS_FIELD_NUMBER: _ClassVar[int]
    passed_at: _timestamp_pb2.Timestamp
    dead_bytes: int
    dead_bytes_per_day: int
    packs: int
    blocks: int
    def __init__(self, passed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., dead_bytes: _Optional[int] = ..., dead_bytes_per_day: _Optional[int] = ..., packs: _Optional[int] = ..., blocks: _Optional[int] = ...) -> None: ...

class Pricing(_message.Message):
    __slots__ = ("storage_gb_month", "egress_gb", "class_a_per_million", "class_b_per_million", "min_storage_days")
    STORAGE_GB_MONTH_FIELD_NUMBER: _ClassVar[int]
    EGRESS_GB_FIELD_NUMBER: _ClassVar[int]
    CLASS_A_PER_MILLION_FIELD_NUMBER: _ClassVar[int]
    CLASS_B_PER_MILLION_FIELD_NUMBER: _ClassVar[int]
    MIN_STORAGE_DAYS_FIELD_NUMBER: _ClassVar[int]
    storage_gb_month: float
    egress_gb: float
    class_a_per_million: float
    class_b_per_million: float
    min_storage_days: int
    def __init__(self, storage_gb_month: _Optional[float] = ..., egress_gb: _Optional[float] = ..., class_a_per_million: _Optional[float] = ..., class_b_per_million: _Optional[float] = ..., min_storage_days: _Optional[int] = ...) -> None: ...
