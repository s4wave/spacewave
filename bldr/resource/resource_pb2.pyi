from rpcstream import rpcstream_pb2 as _rpcstream_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ResourceClientRequest(_message.Message):
    __slots__ = ("control_id", "init", "adopt", "release")
    CONTROL_ID_FIELD_NUMBER: _ClassVar[int]
    INIT_FIELD_NUMBER: _ClassVar[int]
    ADOPT_FIELD_NUMBER: _ClassVar[int]
    RELEASE_FIELD_NUMBER: _ClassVar[int]
    control_id: int
    init: ResourceClientInitRequest
    adopt: ResourceClientAdopt
    release: ResourceClientRelease
    def __init__(self, control_id: _Optional[int] = ..., init: _Optional[_Union[ResourceClientInitRequest, _Mapping]] = ..., adopt: _Optional[_Union[ResourceClientAdopt, _Mapping]] = ..., release: _Optional[_Union[ResourceClientRelease, _Mapping]] = ...) -> None: ...

class ResourceClientInitRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ResourceClientAdopt(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class ResourceClientRelease(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class ResourceClientResponse(_message.Message):
    __slots__ = ("init", "resource_released", "control_ack")
    INIT_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_RELEASED_FIELD_NUMBER: _ClassVar[int]
    CONTROL_ACK_FIELD_NUMBER: _ClassVar[int]
    init: ResourceClientInit
    resource_released: ResourceReleasedResponse
    control_ack: ResourceClientControlAck
    def __init__(self, init: _Optional[_Union[ResourceClientInit, _Mapping]] = ..., resource_released: _Optional[_Union[ResourceReleasedResponse, _Mapping]] = ..., control_ack: _Optional[_Union[ResourceClientControlAck, _Mapping]] = ...) -> None: ...

class ResourceClientControlAck(_message.Message):
    __slots__ = ("control_id",)
    CONTROL_ID_FIELD_NUMBER: _ClassVar[int]
    control_id: int
    def __init__(self, control_id: _Optional[int] = ...) -> None: ...

class ResourceReleasedResponse(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class ResourceClientInit(_message.Message):
    __slots__ = ("client_handle_id", "root_resource_id")
    CLIENT_HANDLE_ID_FIELD_NUMBER: _ClassVar[int]
    ROOT_RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    client_handle_id: int
    root_resource_id: int
    def __init__(self, client_handle_id: _Optional[int] = ..., root_resource_id: _Optional[int] = ...) -> None: ...

class ResourceAttachRequest(_message.Message):
    __slots__ = ("init", "add", "detach", "mux_data")
    INIT_FIELD_NUMBER: _ClassVar[int]
    ADD_FIELD_NUMBER: _ClassVar[int]
    DETACH_FIELD_NUMBER: _ClassVar[int]
    MUX_DATA_FIELD_NUMBER: _ClassVar[int]
    init: ResourceAttachInit
    add: ResourceAttachAdd
    detach: ResourceAttachDetach
    mux_data: bytes
    def __init__(self, init: _Optional[_Union[ResourceAttachInit, _Mapping]] = ..., add: _Optional[_Union[ResourceAttachAdd, _Mapping]] = ..., detach: _Optional[_Union[ResourceAttachDetach, _Mapping]] = ..., mux_data: _Optional[bytes] = ...) -> None: ...

class ResourceAttachResponse(_message.Message):
    __slots__ = ("ack", "add_ack", "detach_ack", "mux_data")
    ACK_FIELD_NUMBER: _ClassVar[int]
    ADD_ACK_FIELD_NUMBER: _ClassVar[int]
    DETACH_ACK_FIELD_NUMBER: _ClassVar[int]
    MUX_DATA_FIELD_NUMBER: _ClassVar[int]
    ack: ResourceAttachAck
    add_ack: ResourceAttachAddAck
    detach_ack: ResourceAttachDetachAck
    mux_data: bytes
    def __init__(self, ack: _Optional[_Union[ResourceAttachAck, _Mapping]] = ..., add_ack: _Optional[_Union[ResourceAttachAddAck, _Mapping]] = ..., detach_ack: _Optional[_Union[ResourceAttachDetachAck, _Mapping]] = ..., mux_data: _Optional[bytes] = ...) -> None: ...

class ResourceAttachInit(_message.Message):
    __slots__ = ("client_handle_id",)
    CLIENT_HANDLE_ID_FIELD_NUMBER: _ClassVar[int]
    client_handle_id: int
    def __init__(self, client_handle_id: _Optional[int] = ...) -> None: ...

class ResourceAttachAck(_message.Message):
    __slots__ = ("error",)
    ERROR_FIELD_NUMBER: _ClassVar[int]
    error: str
    def __init__(self, error: _Optional[str] = ...) -> None: ...

class ResourceAttachAdd(_message.Message):
    __slots__ = ("attach_id", "label")
    ATTACH_ID_FIELD_NUMBER: _ClassVar[int]
    LABEL_FIELD_NUMBER: _ClassVar[int]
    attach_id: int
    label: str
    def __init__(self, attach_id: _Optional[int] = ..., label: _Optional[str] = ...) -> None: ...

class ResourceAttachAddAck(_message.Message):
    __slots__ = ("attach_id", "error", "resource_id")
    ATTACH_ID_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    attach_id: int
    error: str
    resource_id: int
    def __init__(self, attach_id: _Optional[int] = ..., error: _Optional[str] = ..., resource_id: _Optional[int] = ...) -> None: ...

class ResourceAttachDetach(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...

class ResourceAttachDetachAck(_message.Message):
    __slots__ = ("resource_id",)
    RESOURCE_ID_FIELD_NUMBER: _ClassVar[int]
    resource_id: int
    def __init__(self, resource_id: _Optional[int] = ...) -> None: ...
