from collections.abc import AsyncIterable, AsyncIterator
from typing import Protocol

from rpcstream import (
    rpcstream_pb2 as _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2,
)
from bldr.resource import (
    resource_pb2 as _github_com_s4wave_spacewave_bldr_resource_resource_pb2,
)
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import ServiceDescriptor

RESOURCESERVICE_SERVICE: ServiceDescriptor

class ResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
    def resource_client(
        self,
        requests: AsyncIterable[
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientRequest
        ],
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceClientResponse
    ]: ...
    def resource_rpc(
        self,
        requests: AsyncIterable[
            _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket
        ],
    ) -> AsyncIterator[
        _github_com_aperturerobotics_starpc_rpcstream_rpcstream_pb2.RpcStreamPacket
    ]: ...
    def resource_attach(
        self,
        requests: AsyncIterable[
            _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachRequest
        ],
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_bldr_resource_resource_pb2.ResourceAttachResponse
    ]: ...

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
) -> None: ...
