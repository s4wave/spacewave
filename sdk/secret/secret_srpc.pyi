from collections.abc import AsyncIterator
from typing import Protocol

from sdk.secret import (
    secret_pb2 as _github_com_s4wave_spacewave_sdk_secret_secret_pb2,
)
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import ServiceDescriptor

SECRETRESOURCESERVICE_SERVICE: ServiceDescriptor

class SecretResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
) -> None: ...
