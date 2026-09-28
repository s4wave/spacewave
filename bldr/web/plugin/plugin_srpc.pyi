from collections.abc import AsyncIterator
from typing import Protocol

from bldr.web.plugin import (
    plugin_pb2 as _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2,
)
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import ServiceDescriptor

WEBPLUGIN_SERVICE: ServiceDescriptor

class WebPluginClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
    async def open_or_focus_desktop(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopRequest,
    ) -> _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopResponse: ...
    def watch_desktop_presence(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse
    ]: ...
    async def wait_desktop_exit(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest,
    ) -> _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse: ...
    def handle_web_view_via_plugin(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginResponse
    ]: ...
    def handle_web_pkg_via_plugin(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginResponse
    ]: ...
    def handle_rpc_via_plugin(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginResponse
    ]: ...
    def handle_web_view_via_handlers(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersResponse
    ]: ...
    def handle_web_pkgs_via_plugin_assets(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsResponse
    ]: ...

class WebPluginServer(Protocol):
    async def open_or_focus_desktop(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopRequest,
    ) -> _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopResponse: ...
    def watch_desktop_presence(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse
    ]: ...
    async def wait_desktop_exit(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest,
    ) -> _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse: ...
    def handle_web_view_via_plugin(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginResponse
    ]: ...
    def handle_web_pkg_via_plugin(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginResponse
    ]: ...
    def handle_rpc_via_plugin(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginResponse
    ]: ...
    def handle_web_view_via_handlers(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersResponse
    ]: ...
    def handle_web_pkgs_via_plugin_assets(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsResponse
    ]: ...

def register_web_plugin(
    registry: ServiceRegistry,
    implementation: WebPluginServer,
    service: str = "bldr.web.plugin.WebPlugin",
) -> None: ...
