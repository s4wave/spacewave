from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Protocol

from sdk.secret import (
    secret_pb2 as _github_com_s4wave_spacewave_sdk_secret_secret_pb2,
)
from starpc.call import Call, CallProtocolError
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import MethodDescriptor, ServiceDescriptor

SECRETRESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.secret.SecretResourceService",
    (
        MethodDescriptor(
            "WatchState",
            _github_com_s4wave_spacewave_sdk_secret_secret_pb2.WatchStateRequest,
            _github_com_s4wave_spacewave_sdk_secret_secret_pb2.WatchStateResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "BeginReadPayload",
            _github_com_s4wave_spacewave_sdk_secret_secret_pb2.BeginReadPayloadRequest,
            _github_com_s4wave_spacewave_sdk_secret_secret_pb2.BeginReadPayloadResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ReadPayload",
            _github_com_s4wave_spacewave_sdk_secret_secret_pb2.ReadPayloadRequest,
            _github_com_s4wave_spacewave_sdk_secret_secret_pb2.ReadPayloadResponse,
            False,
            False,
        ),
    ),
)


class SecretResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.secret.SecretResourceService"

    async def watch_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_secret_secret_pb2.WatchStateRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_secret_secret_pb2.WatchStateResponse
    ]:
        call = await self._client.open_call(
            self._service, "WatchState", request.SerializeToString(deterministic=True)
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_secret_secret_pb2.WatchStateResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def begin_read_payload(
        self,
        request: _github_com_s4wave_spacewave_sdk_secret_secret_pb2.BeginReadPayloadRequest,
    ) -> _github_com_s4wave_spacewave_sdk_secret_secret_pb2.BeginReadPayloadResponse:
        call = await self._client.open_call(
            self._service,
            "BeginReadPayload",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_secret_secret_pb2.BeginReadPayloadResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def read_payload(
        self,
        request: _github_com_s4wave_spacewave_sdk_secret_secret_pb2.ReadPayloadRequest,
    ) -> _github_com_s4wave_spacewave_sdk_secret_secret_pb2.ReadPayloadResponse:
        call = await self._client.open_call(
            self._service, "ReadPayload", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_secret_secret_pb2.ReadPayloadResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()


class SecretResourceServiceServer(Protocol):
    def watch_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_secret_secret_pb2.WatchStateRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_secret_secret_pb2.WatchStateResponse
    ]: ...
    async def begin_read_payload(
        self,
        request: _github_com_s4wave_spacewave_sdk_secret_secret_pb2.BeginReadPayloadRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_secret_secret_pb2.BeginReadPayloadResponse
    ): ...
    async def read_payload(
        self,
        request: _github_com_s4wave_spacewave_sdk_secret_secret_pb2.ReadPayloadRequest,
    ) -> _github_com_s4wave_spacewave_sdk_secret_secret_pb2.ReadPayloadResponse: ...


def register_secret_resource_service(
    registry: ServiceRegistry,
    implementation: SecretResourceServiceServer,
    service: str = "s4wave.secret.SecretResourceService",
) -> None:
    async def watch_state_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_secret_secret_pb2.WatchStateRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_state(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchState", watch_state_handler)

    async def begin_read_payload_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_secret_secret_pb2.BeginReadPayloadRequest()
        )
        request.ParseFromString(first)
        response = await implementation.begin_read_payload(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "BeginReadPayload", begin_read_payload_handler)

    async def read_payload_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_secret_secret_pb2.ReadPayloadRequest()
        )
        request.ParseFromString(first)
        response = await implementation.read_payload(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ReadPayload", read_payload_handler)
