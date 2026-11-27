import datetime

from controller.configset.proto import configset_pb2 as _configset_pb2
from db.block import block_pb2 as _block_pb2
from db.block.transform import transform_pb2 as _transform_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Config(_message.Message):
    __slots__ = ("id", "rev", "put_opts", "lookup")
    ID_FIELD_NUMBER: _ClassVar[int]
    REV_FIELD_NUMBER: _ClassVar[int]
    PUT_OPTS_FIELD_NUMBER: _ClassVar[int]
    LOOKUP_FIELD_NUMBER: _ClassVar[int]
    id: str
    rev: int
    put_opts: _block_pb2.PutOpts
    lookup: LookupConfig
    def __init__(self, id: _Optional[str] = ..., rev: _Optional[int] = ..., put_opts: _Optional[_Union[_block_pb2.PutOpts, _Mapping]] = ..., lookup: _Optional[_Union[LookupConfig, _Mapping]] = ...) -> None: ...

class BucketInfo(_message.Message):
    __slots__ = ("config",)
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    config: Config
    def __init__(self, config: _Optional[_Union[Config, _Mapping]] = ...) -> None: ...

class LookupConfig(_message.Message):
    __slots__ = ("disable", "controller")
    DISABLE_FIELD_NUMBER: _ClassVar[int]
    CONTROLLER_FIELD_NUMBER: _ClassVar[int]
    disable: bool
    controller: _configset_pb2.ControllerConfig
    def __init__(self, disable: _Optional[bool] = ..., controller: _Optional[_Union[_configset_pb2.ControllerConfig, _Mapping]] = ...) -> None: ...

class ApplyBucketConfigResult(_message.Message):
    __slots__ = ("volume_id", "bucket_id", "bucket_conf", "old_bucket_conf", "timestamp", "updated", "error")
    VOLUME_ID_FIELD_NUMBER: _ClassVar[int]
    BUCKET_ID_FIELD_NUMBER: _ClassVar[int]
    BUCKET_CONF_FIELD_NUMBER: _ClassVar[int]
    OLD_BUCKET_CONF_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    UPDATED_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    volume_id: str
    bucket_id: str
    bucket_conf: Config
    old_bucket_conf: Config
    timestamp: _timestamp_pb2.Timestamp
    updated: bool
    error: str
    def __init__(self, volume_id: _Optional[str] = ..., bucket_id: _Optional[str] = ..., bucket_conf: _Optional[_Union[Config, _Mapping]] = ..., old_bucket_conf: _Optional[_Union[Config, _Mapping]] = ..., timestamp: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updated: _Optional[bool] = ..., error: _Optional[str] = ...) -> None: ...

class ObjectRef(_message.Message):
    __slots__ = ("root_ref", "bucket_id", "transform_conf_ref", "transform_conf")
    ROOT_REF_FIELD_NUMBER: _ClassVar[int]
    BUCKET_ID_FIELD_NUMBER: _ClassVar[int]
    TRANSFORM_CONF_REF_FIELD_NUMBER: _ClassVar[int]
    TRANSFORM_CONF_FIELD_NUMBER: _ClassVar[int]
    root_ref: _block_pb2.BlockRef
    bucket_id: str
    transform_conf_ref: _block_pb2.BlockRef
    transform_conf: _transform_pb2.Config
    def __init__(self, root_ref: _Optional[_Union[_block_pb2.BlockRef, _Mapping]] = ..., bucket_id: _Optional[str] = ..., transform_conf_ref: _Optional[_Union[_block_pb2.BlockRef, _Mapping]] = ..., transform_conf: _Optional[_Union[_transform_pb2.Config, _Mapping]] = ...) -> None: ...

class BucketOpArgs(_message.Message):
    __slots__ = ("bucket_id", "volume_id")
    BUCKET_ID_FIELD_NUMBER: _ClassVar[int]
    VOLUME_ID_FIELD_NUMBER: _ClassVar[int]
    bucket_id: str
    volume_id: str
    def __init__(self, bucket_id: _Optional[str] = ..., volume_id: _Optional[str] = ...) -> None: ...
