from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ProviderFeature(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ProviderFeature_NONE: _ClassVar[ProviderFeature]
    ProviderFeature_SESSION: _ClassVar[ProviderFeature]
    ProviderFeature_SHARED_OBJECT: _ClassVar[ProviderFeature]
    ProviderFeature_BLOCK_STORE: _ClassVar[ProviderFeature]
    ProviderFeature_SHARED_OBJECT_RECOVERY: _ClassVar[ProviderFeature]

class ProviderAccountStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ProviderAccountStatus_NONE: _ClassVar[ProviderAccountStatus]
    ProviderAccountStatus_PENDING: _ClassVar[ProviderAccountStatus]
    ProviderAccountStatus_READY: _ClassVar[ProviderAccountStatus]
    ProviderAccountStatus_DELETED: _ClassVar[ProviderAccountStatus]
    ProviderAccountStatus_FAILED: _ClassVar[ProviderAccountStatus]
    ProviderAccountStatus_UNAUTHENTICATED: _ClassVar[ProviderAccountStatus]
    ProviderAccountStatus_DORMANT: _ClassVar[ProviderAccountStatus]
ProviderFeature_NONE: ProviderFeature
ProviderFeature_SESSION: ProviderFeature
ProviderFeature_SHARED_OBJECT: ProviderFeature
ProviderFeature_BLOCK_STORE: ProviderFeature
ProviderFeature_SHARED_OBJECT_RECOVERY: ProviderFeature
ProviderAccountStatus_NONE: ProviderAccountStatus
ProviderAccountStatus_PENDING: ProviderAccountStatus
ProviderAccountStatus_READY: ProviderAccountStatus
ProviderAccountStatus_DELETED: ProviderAccountStatus
ProviderAccountStatus_FAILED: ProviderAccountStatus
ProviderAccountStatus_UNAUTHENTICATED: ProviderAccountStatus
ProviderAccountStatus_DORMANT: ProviderAccountStatus

class ProviderInfo(_message.Message):
    __slots__ = ("provider_id", "provider_features")
    PROVIDER_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_FEATURES_FIELD_NUMBER: _ClassVar[int]
    provider_id: str
    provider_features: _containers.RepeatedScalarFieldContainer[ProviderFeature]
    def __init__(self, provider_id: _Optional[str] = ..., provider_features: _Optional[_Iterable[_Union[ProviderFeature, str]]] = ...) -> None: ...

class ProviderFeatureMap(_message.Message):
    __slots__ = ("map_items",)
    MAP_ITEMS_FIELD_NUMBER: _ClassVar[int]
    map_items: _containers.RepeatedCompositeFieldContainer[ProviderFeatureMapItem]
    def __init__(self, map_items: _Optional[_Iterable[_Union[ProviderFeatureMapItem, _Mapping]]] = ...) -> None: ...

class ProviderFeatureMapItem(_message.Message):
    __slots__ = ("provider_features", "provider_id", "provider_account_id")
    PROVIDER_FEATURES_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    provider_features: _containers.RepeatedScalarFieldContainer[ProviderFeature]
    provider_id: str
    provider_account_id: str
    def __init__(self, provider_features: _Optional[_Iterable[_Union[ProviderFeature, str]]] = ..., provider_id: _Optional[str] = ..., provider_account_id: _Optional[str] = ...) -> None: ...

class ProviderAccountInfo(_message.Message):
    __slots__ = ("provider_id", "provider_account_id", "provider_features", "provider_account_status", "provider_account_state")
    PROVIDER_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_FEATURES_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ACCOUNT_STATUS_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ACCOUNT_STATE_FIELD_NUMBER: _ClassVar[int]
    provider_id: str
    provider_account_id: str
    provider_features: _containers.RepeatedScalarFieldContainer[ProviderFeature]
    provider_account_status: ProviderAccountStatus
    provider_account_state: bytes
    def __init__(self, provider_id: _Optional[str] = ..., provider_account_id: _Optional[str] = ..., provider_features: _Optional[_Iterable[_Union[ProviderFeature, str]]] = ..., provider_account_status: _Optional[_Union[ProviderAccountStatus, str]] = ..., provider_account_state: _Optional[bytes] = ...) -> None: ...

class ProviderResourceRef(_message.Message):
    __slots__ = ("id", "provider_id", "provider_account_id")
    ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    id: str
    provider_id: str
    provider_account_id: str
    def __init__(self, id: _Optional[str] = ..., provider_id: _Optional[str] = ..., provider_account_id: _Optional[str] = ...) -> None: ...

class ProviderFeatureResourceRef(_message.Message):
    __slots__ = ("provider_resource_ref", "provider_feature", "provider_feature_meta")
    PROVIDER_RESOURCE_REF_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_FEATURE_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_FEATURE_META_FIELD_NUMBER: _ClassVar[int]
    provider_resource_ref: ProviderResourceRef
    provider_feature: ProviderFeature
    provider_feature_meta: bytes
    def __init__(self, provider_resource_ref: _Optional[_Union[ProviderResourceRef, _Mapping]] = ..., provider_feature: _Optional[_Union[ProviderFeature, str]] = ..., provider_feature_meta: _Optional[bytes] = ...) -> None: ...
