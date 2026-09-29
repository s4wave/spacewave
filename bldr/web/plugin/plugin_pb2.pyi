from backoff import backoff_pb2 as _backoff_pb2
from bldr.web.view.handler import handler_pb2 as _handler_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class DesktopPresenceState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    DESKTOP_PRESENCE_STATE_UNKNOWN: _ClassVar[DesktopPresenceState]
    DESKTOP_PRESENCE_STATE_ACTIVE: _ClassVar[DesktopPresenceState]
    DESKTOP_PRESENCE_STATE_ENDED: _ClassVar[DesktopPresenceState]
DESKTOP_PRESENCE_STATE_UNKNOWN: DesktopPresenceState
DESKTOP_PRESENCE_STATE_ACTIVE: DesktopPresenceState
DESKTOP_PRESENCE_STATE_ENDED: DesktopPresenceState

class OpenOrFocusDesktopRequest(_message.Message):
    __slots__ = ("route", "installed_app")
    ROUTE_FIELD_NUMBER: _ClassVar[int]
    INSTALLED_APP_FIELD_NUMBER: _ClassVar[int]
    route: str
    installed_app: str
    def __init__(self, route: _Optional[str] = ..., installed_app: _Optional[str] = ...) -> None: ...

class OpenOrFocusDesktopResponse(_message.Message):
    __slots__ = ("generation",)
    GENERATION_FIELD_NUMBER: _ClassVar[int]
    generation: int
    def __init__(self, generation: _Optional[int] = ...) -> None: ...

class WatchDesktopPresenceRequest(_message.Message):
    __slots__ = ("generation",)
    GENERATION_FIELD_NUMBER: _ClassVar[int]
    generation: int
    def __init__(self, generation: _Optional[int] = ...) -> None: ...

class WatchDesktopPresenceResponse(_message.Message):
    __slots__ = ("state", "error")
    STATE_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    state: DesktopPresenceState
    error: str
    def __init__(self, state: _Optional[_Union[DesktopPresenceState, str]] = ..., error: _Optional[str] = ...) -> None: ...

class HandleWebViewViaPluginRequest(_message.Message):
    __slots__ = ("handle_plugin_id", "web_view_id_re")
    HANDLE_PLUGIN_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_VIEW_ID_RE_FIELD_NUMBER: _ClassVar[int]
    handle_plugin_id: str
    web_view_id_re: str
    def __init__(self, handle_plugin_id: _Optional[str] = ..., web_view_id_re: _Optional[str] = ...) -> None: ...

class HandleWebViewViaPluginResponse(_message.Message):
    __slots__ = ("ready",)
    READY_FIELD_NUMBER: _ClassVar[int]
    ready: bool
    def __init__(self, ready: _Optional[bool] = ...) -> None: ...

class HandleWebPkgViaPluginRequest(_message.Message):
    __slots__ = ("handle_plugin_id", "web_pkg_id_re", "web_pkg_id_prefixes", "web_pkg_id_list")
    HANDLE_PLUGIN_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_PKG_ID_RE_FIELD_NUMBER: _ClassVar[int]
    WEB_PKG_ID_PREFIXES_FIELD_NUMBER: _ClassVar[int]
    WEB_PKG_ID_LIST_FIELD_NUMBER: _ClassVar[int]
    handle_plugin_id: str
    web_pkg_id_re: str
    web_pkg_id_prefixes: _containers.RepeatedScalarFieldContainer[str]
    web_pkg_id_list: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, handle_plugin_id: _Optional[str] = ..., web_pkg_id_re: _Optional[str] = ..., web_pkg_id_prefixes: _Optional[_Iterable[str]] = ..., web_pkg_id_list: _Optional[_Iterable[str]] = ...) -> None: ...

class HandleWebPkgViaPluginResponse(_message.Message):
    __slots__ = ("ready",)
    READY_FIELD_NUMBER: _ClassVar[int]
    ready: bool
    def __init__(self, ready: _Optional[bool] = ...) -> None: ...

class HandleRpcViaPluginRequest(_message.Message):
    __slots__ = ("handle_plugin_id", "service_id_re", "server_id_re", "backoff")
    HANDLE_PLUGIN_ID_FIELD_NUMBER: _ClassVar[int]
    SERVICE_ID_RE_FIELD_NUMBER: _ClassVar[int]
    SERVER_ID_RE_FIELD_NUMBER: _ClassVar[int]
    BACKOFF_FIELD_NUMBER: _ClassVar[int]
    handle_plugin_id: str
    service_id_re: str
    server_id_re: str
    backoff: _backoff_pb2.Backoff
    def __init__(self, handle_plugin_id: _Optional[str] = ..., service_id_re: _Optional[str] = ..., server_id_re: _Optional[str] = ..., backoff: _Optional[_Union[_backoff_pb2.Backoff, _Mapping]] = ...) -> None: ...

class HandleRpcViaPluginResponse(_message.Message):
    __slots__ = ("ready",)
    READY_FIELD_NUMBER: _ClassVar[int]
    ready: bool
    def __init__(self, ready: _Optional[bool] = ...) -> None: ...

class HandleWebViewViaHandlersRequest(_message.Message):
    __slots__ = ("config",)
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    config: _handler_pb2.WebViewHandlersConfig
    def __init__(self, config: _Optional[_Union[_handler_pb2.WebViewHandlersConfig, _Mapping]] = ...) -> None: ...

class HandleWebViewViaHandlersResponse(_message.Message):
    __slots__ = ("ready",)
    READY_FIELD_NUMBER: _ClassVar[int]
    ready: bool
    def __init__(self, ready: _Optional[bool] = ...) -> None: ...

class HandleWebPkgsViaPluginAssetsRequest(_message.Message):
    __slots__ = ("handle_plugin_id", "web_pkgs_path", "web_pkg_id_list")
    HANDLE_PLUGIN_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_PKGS_PATH_FIELD_NUMBER: _ClassVar[int]
    WEB_PKG_ID_LIST_FIELD_NUMBER: _ClassVar[int]
    handle_plugin_id: str
    web_pkgs_path: str
    web_pkg_id_list: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, handle_plugin_id: _Optional[str] = ..., web_pkgs_path: _Optional[str] = ..., web_pkg_id_list: _Optional[_Iterable[str]] = ...) -> None: ...

class HandleWebPkgsViaPluginAssetsResponse(_message.Message):
    __slots__ = ("ready",)
    READY_FIELD_NUMBER: _ClassVar[int]
    ready: bool
    def __init__(self, ready: _Optional[bool] = ...) -> None: ...
