from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Protocol

from sdk.root import (
    root_pb2 as _github_com_s4wave_spacewave_sdk_root_root_pb2,
)
from starpc.call import Call, CallProtocolError
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import MethodDescriptor, ServiceDescriptor

ROOTRESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.root.RootResourceService",
    (
        MethodDescriptor(
            "ListProviders",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ListProvidersRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ListProvidersResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "LookupProvider",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.LookupProviderRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.LookupProviderResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "MountSession",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "MountSessionByIdx",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionByIdxRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionByIdxResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ListSessions",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSessionsRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSessionsResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchSessions",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionsRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionsResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "WatchAllAccountStatuses",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchAllAccountStatusesRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchAllAccountStatusesResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "GetSessionMetadata",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.GetSessionMetadataRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.GetSessionMetadataResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchSessionMetadata",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionMetadataRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionMetadataResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "UnlockSession",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.UnlockSessionByIdxRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.UnlockSessionByIdxResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "DeleteSession",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.DeleteSessionRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.DeleteSessionResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ResetSession",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ResetSessionByIdxRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ResetSessionByIdxResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "AccessStateAtom",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessStateAtomRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessStateAtomResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchStateAtoms",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchStateAtomsRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchStateAtomsResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "ListSpaceRootAliases",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSpaceRootAliasesRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSpaceRootAliasesResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchSpaceRootAliases",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootAliasesRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootAliasesResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "UpsertSpaceRootAlias",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.UpsertSpaceRootAliasRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.UpsertSpaceRootAliasResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "RemoveSpaceRootAlias",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.RemoveSpaceRootAliasRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.RemoveSpaceRootAliasResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchSpaceRootRuntime",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootRuntimeRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootRuntimeResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "MarshalHash",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.MarshalHashRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.MarshalHashResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ParseHash",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ParseHashRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ParseHashResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "HashSum",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.HashSumRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.HashSumResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "HashValidate",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.HashValidateRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.HashValidateResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetChangelog",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.GetChangelogRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.GetChangelogResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetDebugDb",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.GetDebugDbRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.GetDebugDbResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetCdn",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.GetCdnRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.GetCdnResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "AccessWebListener",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessWebListenerRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessWebListenerResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchWebListeners",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchWebListenersRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchWebListenersResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "StopWebListener",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.StopWebListenerRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.StopWebListenerResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchListenerYieldPrompts",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerYieldPromptsRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerYieldPromptsResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "RespondToListenerYieldPrompt",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.RespondToListenerYieldPromptRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.RespondToListenerYieldPromptResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchRuntimeHandoff",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchRuntimeHandoffRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchRuntimeHandoffResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "ReclaimRuntime",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ReclaimRuntimeRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ReclaimRuntimeResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchListenerStatus",
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerStatusRequest,
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerStatusResponse,
            False,
            True,
        ),
    ),
)


class RootResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.root.RootResourceService"

    async def list_providers(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ListProvidersRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ListProvidersResponse:
        call = await self._client.open_call(
            self._service,
            "ListProviders",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.ListProvidersResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def lookup_provider(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.LookupProviderRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.LookupProviderResponse:
        call = await self._client.open_call(
            self._service,
            "LookupProvider",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.LookupProviderResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def mount_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionResponse:
        call = await self._client.open_call(
            self._service, "MountSession", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def mount_session_by_idx(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionByIdxRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionByIdxResponse:
        call = await self._client.open_call(
            self._service,
            "MountSessionByIdx",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionByIdxResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def list_sessions(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSessionsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSessionsResponse:
        call = await self._client.open_call(
            self._service, "ListSessions", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSessionsResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_sessions(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionsResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchSessions",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionsResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def watch_all_account_statuses(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchAllAccountStatusesRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchAllAccountStatusesResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchAllAccountStatuses",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchAllAccountStatusesResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def get_session_metadata(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.GetSessionMetadataRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.GetSessionMetadataResponse:
        call = await self._client.open_call(
            self._service,
            "GetSessionMetadata",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.GetSessionMetadataResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_session_metadata(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionMetadataRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionMetadataResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchSessionMetadata",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionMetadataResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def unlock_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.UnlockSessionByIdxRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.UnlockSessionByIdxResponse:
        call = await self._client.open_call(
            self._service,
            "UnlockSession",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.UnlockSessionByIdxResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def delete_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.DeleteSessionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.DeleteSessionResponse:
        call = await self._client.open_call(
            self._service,
            "DeleteSession",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.DeleteSessionResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def reset_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ResetSessionByIdxRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ResetSessionByIdxResponse:
        call = await self._client.open_call(
            self._service, "ResetSession", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.ResetSessionByIdxResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def access_state_atom(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessStateAtomRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessStateAtomResponse:
        call = await self._client.open_call(
            self._service,
            "AccessStateAtom",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessStateAtomResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_state_atoms(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchStateAtomsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchStateAtomsResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchStateAtoms",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchStateAtomsResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def list_space_root_aliases(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSpaceRootAliasesRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSpaceRootAliasesResponse:
        call = await self._client.open_call(
            self._service,
            "ListSpaceRootAliases",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSpaceRootAliasesResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_space_root_aliases(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootAliasesRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootAliasesResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchSpaceRootAliases",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootAliasesResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def upsert_space_root_alias(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.UpsertSpaceRootAliasRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.UpsertSpaceRootAliasResponse:
        call = await self._client.open_call(
            self._service,
            "UpsertSpaceRootAlias",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.UpsertSpaceRootAliasResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def remove_space_root_alias(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.RemoveSpaceRootAliasRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.RemoveSpaceRootAliasResponse:
        call = await self._client.open_call(
            self._service,
            "RemoveSpaceRootAlias",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.RemoveSpaceRootAliasResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_space_root_runtime(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootRuntimeRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootRuntimeResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchSpaceRootRuntime",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootRuntimeResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def marshal_hash(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.MarshalHashRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.MarshalHashResponse:
        call = await self._client.open_call(
            self._service, "MarshalHash", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.MarshalHashResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def parse_hash(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ParseHashRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ParseHashResponse:
        call = await self._client.open_call(
            self._service, "ParseHash", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.ParseHashResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def hash_sum(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.HashSumRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.HashSumResponse:
        call = await self._client.open_call(
            self._service, "HashSum", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.HashSumResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def hash_validate(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.HashValidateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.HashValidateResponse:
        call = await self._client.open_call(
            self._service, "HashValidate", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.HashValidateResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_changelog(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.GetChangelogRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.GetChangelogResponse:
        call = await self._client.open_call(
            self._service, "GetChangelog", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.GetChangelogResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_debug_db(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.GetDebugDbRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.GetDebugDbResponse:
        call = await self._client.open_call(
            self._service, "GetDebugDb", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.GetDebugDbResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_cdn(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.GetCdnRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.GetCdnResponse:
        call = await self._client.open_call(
            self._service, "GetCdn", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.GetCdnResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def access_web_listener(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessWebListenerRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessWebListenerResponse:
        call = await self._client.open_call(
            self._service,
            "AccessWebListener",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessWebListenerResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_web_listeners(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchWebListenersRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchWebListenersResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchWebListeners",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchWebListenersResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def stop_web_listener(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.StopWebListenerRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.StopWebListenerResponse:
        call = await self._client.open_call(
            self._service,
            "StopWebListener",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.StopWebListenerResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_listener_yield_prompts(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerYieldPromptsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerYieldPromptsResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchListenerYieldPrompts",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerYieldPromptsResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def respond_to_listener_yield_prompt(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.RespondToListenerYieldPromptRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.RespondToListenerYieldPromptResponse:
        call = await self._client.open_call(
            self._service,
            "RespondToListenerYieldPrompt",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_root_root_pb2.RespondToListenerYieldPromptResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_runtime_handoff(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchRuntimeHandoffRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchRuntimeHandoffResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchRuntimeHandoff",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchRuntimeHandoffResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def reclaim_runtime(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ReclaimRuntimeRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ReclaimRuntimeResponse:
        call = await self._client.open_call(
            self._service,
            "ReclaimRuntime",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_root_root_pb2.ReclaimRuntimeResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_listener_status(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerStatusRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerStatusResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchListenerStatus",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerStatusResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()


class RootResourceServiceServer(Protocol):
    async def list_providers(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ListProvidersRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ListProvidersResponse: ...
    async def lookup_provider(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.LookupProviderRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.LookupProviderResponse: ...
    async def mount_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionResponse: ...
    async def mount_session_by_idx(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionByIdxRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionByIdxResponse: ...
    async def list_sessions(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSessionsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSessionsResponse: ...
    def watch_sessions(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionsResponse
    ]: ...
    def watch_all_account_statuses(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchAllAccountStatusesRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchAllAccountStatusesResponse
    ]: ...
    async def get_session_metadata(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.GetSessionMetadataRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.GetSessionMetadataResponse: ...
    def watch_session_metadata(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionMetadataRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionMetadataResponse
    ]: ...
    async def unlock_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.UnlockSessionByIdxRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.UnlockSessionByIdxResponse: ...
    async def delete_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.DeleteSessionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.DeleteSessionResponse: ...
    async def reset_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ResetSessionByIdxRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ResetSessionByIdxResponse: ...
    async def access_state_atom(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessStateAtomRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessStateAtomResponse: ...
    def watch_state_atoms(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchStateAtomsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchStateAtomsResponse
    ]: ...
    async def list_space_root_aliases(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSpaceRootAliasesRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSpaceRootAliasesResponse
    ): ...
    def watch_space_root_aliases(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootAliasesRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootAliasesResponse
    ]: ...
    async def upsert_space_root_alias(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.UpsertSpaceRootAliasRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_root_root_pb2.UpsertSpaceRootAliasResponse
    ): ...
    async def remove_space_root_alias(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.RemoveSpaceRootAliasRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_root_root_pb2.RemoveSpaceRootAliasResponse
    ): ...
    def watch_space_root_runtime(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootRuntimeRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootRuntimeResponse
    ]: ...
    async def marshal_hash(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.MarshalHashRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.MarshalHashResponse: ...
    async def parse_hash(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ParseHashRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ParseHashResponse: ...
    async def hash_sum(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.HashSumRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.HashSumResponse: ...
    async def hash_validate(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.HashValidateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.HashValidateResponse: ...
    async def get_changelog(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.GetChangelogRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.GetChangelogResponse: ...
    async def get_debug_db(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.GetDebugDbRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.GetDebugDbResponse: ...
    async def get_cdn(
        self, request: _github_com_s4wave_spacewave_sdk_root_root_pb2.GetCdnRequest
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.GetCdnResponse: ...
    async def access_web_listener(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessWebListenerRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessWebListenerResponse: ...
    def watch_web_listeners(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchWebListenersRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchWebListenersResponse
    ]: ...
    async def stop_web_listener(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.StopWebListenerRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.StopWebListenerResponse: ...
    def watch_listener_yield_prompts(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerYieldPromptsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerYieldPromptsResponse
    ]: ...
    async def respond_to_listener_yield_prompt(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.RespondToListenerYieldPromptRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.RespondToListenerYieldPromptResponse: ...
    def watch_runtime_handoff(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchRuntimeHandoffRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchRuntimeHandoffResponse
    ]: ...
    async def reclaim_runtime(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.ReclaimRuntimeRequest,
    ) -> _github_com_s4wave_spacewave_sdk_root_root_pb2.ReclaimRuntimeResponse: ...
    def watch_listener_status(
        self,
        request: _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerStatusRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerStatusResponse
    ]: ...


def register_root_resource_service(
    registry: ServiceRegistry,
    implementation: RootResourceServiceServer,
    service: str = "s4wave.root.RootResourceService",
) -> None:
    async def list_providers_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.ListProvidersRequest()
        request.ParseFromString(first)
        response = await implementation.list_providers(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ListProviders", list_providers_handler)

    async def lookup_provider_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.LookupProviderRequest()
        request.ParseFromString(first)
        response = await implementation.lookup_provider(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "LookupProvider", lookup_provider_handler)

    async def mount_session_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionRequest()
        request.ParseFromString(first)
        response = await implementation.mount_session(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "MountSession", mount_session_handler)

    async def mount_session_by_idx_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.MountSessionByIdxRequest()
        )
        request.ParseFromString(first)
        response = await implementation.mount_session_by_idx(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "MountSessionByIdx", mount_session_by_idx_handler)

    async def list_sessions_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSessionsRequest()
        request.ParseFromString(first)
        response = await implementation.list_sessions(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ListSessions", list_sessions_handler)

    async def watch_sessions_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionsRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_sessions(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchSessions", watch_sessions_handler)

    async def watch_all_account_statuses_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchAllAccountStatusesRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_all_account_statuses(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "WatchAllAccountStatuses", watch_all_account_statuses_handler
    )

    async def get_session_metadata_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.GetSessionMetadataRequest()
        )
        request.ParseFromString(first)
        response = await implementation.get_session_metadata(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetSessionMetadata", get_session_metadata_handler)

    async def watch_session_metadata_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSessionMetadataRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_session_metadata(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchSessionMetadata", watch_session_metadata_handler)

    async def unlock_session_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.UnlockSessionByIdxRequest()
        )
        request.ParseFromString(first)
        response = await implementation.unlock_session(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "UnlockSession", unlock_session_handler)

    async def delete_session_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.DeleteSessionRequest()
        request.ParseFromString(first)
        response = await implementation.delete_session(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "DeleteSession", delete_session_handler)

    async def reset_session_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ResetSessionByIdxRequest()
        )
        request.ParseFromString(first)
        response = await implementation.reset_session(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ResetSession", reset_session_handler)

    async def access_state_atom_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessStateAtomRequest()
        )
        request.ParseFromString(first)
        response = await implementation.access_state_atom(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "AccessStateAtom", access_state_atom_handler)

    async def watch_state_atoms_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchStateAtomsRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_state_atoms(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchStateAtoms", watch_state_atoms_handler)

    async def list_space_root_aliases_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.ListSpaceRootAliasesRequest()
        )
        request.ParseFromString(first)
        response = await implementation.list_space_root_aliases(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ListSpaceRootAliases", list_space_root_aliases_handler)

    async def watch_space_root_aliases_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootAliasesRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_space_root_aliases(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "WatchSpaceRootAliases", watch_space_root_aliases_handler
    )

    async def upsert_space_root_alias_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.UpsertSpaceRootAliasRequest()
        )
        request.ParseFromString(first)
        response = await implementation.upsert_space_root_alias(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "UpsertSpaceRootAlias", upsert_space_root_alias_handler)

    async def remove_space_root_alias_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.RemoveSpaceRootAliasRequest()
        )
        request.ParseFromString(first)
        response = await implementation.remove_space_root_alias(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "RemoveSpaceRootAlias", remove_space_root_alias_handler)

    async def watch_space_root_runtime_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchSpaceRootRuntimeRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_space_root_runtime(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "WatchSpaceRootRuntime", watch_space_root_runtime_handler
    )

    async def marshal_hash_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.MarshalHashRequest()
        request.ParseFromString(first)
        response = await implementation.marshal_hash(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "MarshalHash", marshal_hash_handler)

    async def parse_hash_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.ParseHashRequest()
        request.ParseFromString(first)
        response = await implementation.parse_hash(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ParseHash", parse_hash_handler)

    async def hash_sum_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.HashSumRequest()
        request.ParseFromString(first)
        response = await implementation.hash_sum(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "HashSum", hash_sum_handler)

    async def hash_validate_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.HashValidateRequest()
        request.ParseFromString(first)
        response = await implementation.hash_validate(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "HashValidate", hash_validate_handler)

    async def get_changelog_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.GetChangelogRequest()
        request.ParseFromString(first)
        response = await implementation.get_changelog(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetChangelog", get_changelog_handler)

    async def get_debug_db_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.GetDebugDbRequest()
        request.ParseFromString(first)
        response = await implementation.get_debug_db(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetDebugDb", get_debug_db_handler)

    async def get_cdn_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.GetCdnRequest()
        request.ParseFromString(first)
        response = await implementation.get_cdn(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetCdn", get_cdn_handler)

    async def access_web_listener_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.AccessWebListenerRequest()
        )
        request.ParseFromString(first)
        response = await implementation.access_web_listener(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "AccessWebListener", access_web_listener_handler)

    async def watch_web_listeners_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchWebListenersRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_web_listeners(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchWebListeners", watch_web_listeners_handler)

    async def stop_web_listener_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.StopWebListenerRequest()
        )
        request.ParseFromString(first)
        response = await implementation.stop_web_listener(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "StopWebListener", stop_web_listener_handler)

    async def watch_listener_yield_prompts_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerYieldPromptsRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_listener_yield_prompts(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "WatchListenerYieldPrompts", watch_listener_yield_prompts_handler
    )

    async def respond_to_listener_yield_prompt_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.RespondToListenerYieldPromptRequest()
        request.ParseFromString(first)
        response = await implementation.respond_to_listener_yield_prompt(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service,
        "RespondToListenerYieldPrompt",
        respond_to_listener_yield_prompt_handler,
    )

    async def watch_runtime_handoff_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchRuntimeHandoffRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_runtime_handoff(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchRuntimeHandoff", watch_runtime_handoff_handler)

    async def reclaim_runtime_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_root_root_pb2.ReclaimRuntimeRequest()
        request.ParseFromString(first)
        response = await implementation.reclaim_runtime(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ReclaimRuntime", reclaim_runtime_handler)

    async def watch_listener_status_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_root_root_pb2.WatchListenerStatusRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_listener_status(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchListenerStatus", watch_listener_status_handler)
