from __future__ import annotations

from collections.abc import AsyncIterable, AsyncIterator
from typing import Protocol

from rpcstream import (
    rpcstream_pb2 as _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2,
)
from bldr.resource import (
    resource_pb2 as _github_com_s4wave_spacewave_bldr_resource_resource_pb2,
)
from starpc.call import Call
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import MethodDescriptor, ServiceDescriptor, bidirectional_bytes

RESOURCESERVICE_SERVICE = ServiceDescriptor(
    "resource.ResourceService",
    (
        MethodDescriptor(
            "ResourceClient",
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientRequest,
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientResponse,
            True,
            True,
        ),
        MethodDescriptor(
            "ResourceRpc",
            _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket,
            _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket,
            True,
            True,
        ),
        MethodDescriptor(
            "ResourceAttach",
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachRequest,
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachResponse,
            True,
            True,
        ),
    ),
)


class ResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "resource.ResourceService"

    async def resource_client(
        self,
        requests: AsyncIterable[
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientRequest
        ],
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientResponse
    ]:
        call = await self._client.open_call(self._service, "ResourceClient")

        async def encoded() -> AsyncIterator[bytes]:
            async for request in requests:
                yield request.SerializeToString(deterministic=True)

        async for data in bidirectional_bytes(call, encoded()):
            response = _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientResponse()
            response.ParseFromString(data)
            yield response

    async def resource_rpc(
        self,
        requests: AsyncIterable[
            _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket
        ],
    ) -> AsyncIterator[
        _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket
    ]:
        call = await self._client.open_call(self._service, "ResourceRpc")

        async def encoded() -> AsyncIterator[bytes]:
            async for request in requests:
                yield request.SerializeToString(deterministic=True)

        async for data in bidirectional_bytes(call, encoded()):
            response = _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket()
            response.ParseFromString(data)
            yield response

    async def resource_attach(
        self,
        requests: AsyncIterable[
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachRequest
        ],
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachResponse
    ]:
        call = await self._client.open_call(self._service, "ResourceAttach")

        async def encoded() -> AsyncIterator[bytes]:
            async for request in requests:
                yield request.SerializeToString(deterministic=True)

        async for data in bidirectional_bytes(call, encoded()):
            response = _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachResponse()
            response.ParseFromString(data)
            yield response


class ResourceServiceServer(Protocol):
    def resource_client(
        self,
        requests: AsyncIterator[
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientRequest
        ],
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientResponse
    ]: ...
    def resource_rpc(
        self,
        requests: AsyncIterator[
            _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket
        ],
    ) -> AsyncIterator[
        _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket
    ]: ...
    def resource_attach(
        self,
        requests: AsyncIterator[
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachRequest
        ],
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachResponse
    ]: ...


def register_resource_service(
    registry: ServiceRegistry,
    implementation: ResourceServiceServer,
    service: str = "resource.ResourceService",
) -> None:
    async def resource_client_handler(call: Call) -> None:
        async def requests() -> AsyncIterator[
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientRequest
        ]:
            while True:
                data = await call.receive()
                if data is None:
                    return
                request = _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientRequest()
                request.ParseFromString(data)
                yield request

        async for response in implementation.resource_client(requests()):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ResourceClient", resource_client_handler)

    async def resource_rpc_handler(call: Call) -> None:
        async def requests() -> AsyncIterator[
            _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket
        ]:
            while True:
                data = await call.receive()
                if data is None:
                    return
                request = _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket()
                request.ParseFromString(data)
                yield request

        async for response in implementation.resource_rpc(requests()):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ResourceRpc", resource_rpc_handler)

    async def resource_attach_handler(call: Call) -> None:
        async def requests() -> AsyncIterator[
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachRequest
        ]:
            while True:
                data = await call.receive()
                if data is None:
                    return
                request = _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachRequest()
                request.ParseFromString(data)
                yield request

        async for response in implementation.resource_attach(requests()):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ResourceAttach", resource_attach_handler)
