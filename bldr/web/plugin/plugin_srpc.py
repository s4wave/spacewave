from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Protocol

from bldr.web.plugin import (
    plugin_pb2 as _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2,
)
from starpc.call import Call, CallProtocolError
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import MethodDescriptor, ServiceDescriptor

WEBPLUGIN_SERVICE = ServiceDescriptor(
    "bldr.web.plugin.WebPlugin",
    (
        MethodDescriptor(
            "OpenOrFocusDesktop",
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopRequest,
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchDesktopPresence",
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest,
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "WaitDesktopExit",
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest,
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "HandleWebViewViaPlugin",
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginRequest,
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "HandleWebPkgViaPlugin",
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginRequest,
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "HandleRpcViaPlugin",
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginRequest,
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "HandleWebViewViaHandlers",
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersRequest,
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "HandleWebPkgsViaPluginAssets",
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsRequest,
            _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsResponse,
            False,
            True,
        ),
    ),
)


class WebPluginClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "bldr.web.plugin.WebPlugin"

    async def open_or_focus_desktop(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopRequest,
    ) -> _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopResponse:
        call = await self._client.open_call(
            self._service,
            "OpenOrFocusDesktop",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_desktop_presence(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchDesktopPresence",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def wait_desktop_exit(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest,
    ) -> _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse:
        call = await self._client.open_call(
            self._service,
            "WaitDesktopExit",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def handle_web_view_via_plugin(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "HandleWebViewViaPlugin",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def handle_web_pkg_via_plugin(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "HandleWebPkgViaPlugin",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def handle_rpc_via_plugin(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "HandleRpcViaPlugin",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def handle_web_view_via_handlers(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "HandleWebViewViaHandlers",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def handle_web_pkgs_via_plugin_assets(
        self,
        request: _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "HandleWebPkgsViaPluginAssets",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()


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
) -> None:
    async def open_or_focus_desktop_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.OpenOrFocusDesktopRequest()
        request.ParseFromString(first)
        response = await implementation.open_or_focus_desktop(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "OpenOrFocusDesktop", open_or_focus_desktop_handler)

    async def watch_desktop_presence_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_desktop_presence(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchDesktopPresence", watch_desktop_presence_handler)

    async def wait_desktop_exit_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.WatchDesktopPresenceRequest()
        request.ParseFromString(first)
        response = await implementation.wait_desktop_exit(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WaitDesktopExit", wait_desktop_exit_handler)

    async def handle_web_view_via_plugin_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaPluginRequest()
        request.ParseFromString(first)
        async for response in implementation.handle_web_view_via_plugin(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "HandleWebViewViaPlugin", handle_web_view_via_plugin_handler
    )

    async def handle_web_pkg_via_plugin_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgViaPluginRequest()
        request.ParseFromString(first)
        async for response in implementation.handle_web_pkg_via_plugin(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "HandleWebPkgViaPlugin", handle_web_pkg_via_plugin_handler
    )

    async def handle_rpc_via_plugin_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleRpcViaPluginRequest()
        request.ParseFromString(first)
        async for response in implementation.handle_rpc_via_plugin(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "HandleRpcViaPlugin", handle_rpc_via_plugin_handler)

    async def handle_web_view_via_handlers_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebViewViaHandlersRequest()
        request.ParseFromString(first)
        async for response in implementation.handle_web_view_via_handlers(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "HandleWebViewViaHandlers", handle_web_view_via_handlers_handler
    )

    async def handle_web_pkgs_via_plugin_assets_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_bldr_web_plugin_plugin_pb2.HandleWebPkgsViaPluginAssetsRequest()
        request.ParseFromString(first)
        async for response in implementation.handle_web_pkgs_via_plugin_assets(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service,
        "HandleWebPkgsViaPluginAssets",
        handle_web_pkgs_via_plugin_assets_handler,
    )
