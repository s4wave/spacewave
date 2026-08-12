from collections.abc import AsyncIterator
from typing import Protocol

from sdk.root import (
    root_pb2 as _github_com_s4wave_spacewave_sdk_root_root_pb2,
)
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import ServiceDescriptor

ROOTRESOURCESERVICE_SERVICE: ServiceDescriptor

class RootResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
) -> None: ...
