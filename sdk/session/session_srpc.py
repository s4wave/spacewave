from __future__ import annotations

from collections.abc import AsyncIterator
from typing import Protocol

from sdk.session import (
    session_pb2 as _github_com_s4wave_spacewave_sdk_session_session_pb2,
)
from starpc.call import Call, CallProtocolError
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import MethodDescriptor, ServiceDescriptor

SESSIONRESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.session.SessionResourceService",
    (
        MethodDescriptor(
            "GetSessionInfo",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSessionInfoRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSessionInfoResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchResourcesList",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchResourcesListRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchResourcesListResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "CreateSpace",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "MountSharedObject",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.MountSharedObjectRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.MountSharedObjectResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchSharedObjectHealth",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSharedObjectHealthRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSharedObjectHealthResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "WatchSyncStatus",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSyncStatusRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSyncStatusResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "WatchStorageStats",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchStorageStatsRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchStorageStatsResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "DeleteSpace",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteSpaceRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteSpaceResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "RenameSpace",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.RenameSpaceRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.RenameSpaceResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchLockState",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchLockStateRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchLockStateResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "SetLockMode",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.SetLockModeRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.SetLockModeResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "SetDirectP2PEnabled",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.SetDirectP2PEnabledRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.SetDirectP2PEnabledResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "UnlockSession",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlockSessionRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlockSessionResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "LockSession",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.LockSessionRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.LockSessionResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GeneratePairingCode",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GeneratePairingCodeRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GeneratePairingCodeResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "CompletePairing",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CompletePairingRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CompletePairingResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetSASEmoji",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSASEmojiRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSASEmojiResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ConfirmSASMatch",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmSASMatchRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmSASMatchResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ConfirmPairing",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmPairingRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmPairingResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "DeleteAccount",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteAccountRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteAccountResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "AccessStateAtom",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessSessionStateAtomRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessSessionStateAtomResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchStateAtoms",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSessionStateAtomsRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSessionStateAtomsResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "GetTransferInventory",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferInventoryRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferInventoryResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "StartTransfer",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.StartTransferRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.StartTransferResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchTransferProgress",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchTransferProgressRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchTransferProgressResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "CancelTransfer",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CancelTransferRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CancelTransferResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetTransferStatus",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferStatusRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferStatusResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchPairedDevices",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairedDevicesRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairedDevicesResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "WatchPairingStatus",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairingStatusRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairingStatusResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "UnlinkDevice",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlinkDeviceRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlinkDeviceResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "CreateSpaceInvite",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceInviteRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceInviteResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ListSpaceInvites",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceInvitesRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceInvitesResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ListSpaceParticipants",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceParticipantsRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceParticipantsResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "RemoveSpaceParticipant",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.RemoveSpaceParticipantRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.RemoveSpaceParticipantResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "RevokeSpaceInvite",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.RevokeSpaceInviteRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.RevokeSpaceInviteResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "JoinSpaceViaInvite",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.JoinSpaceViaInviteRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.JoinSpaceViaInviteResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "CreateLocalPairingOffer",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateLocalPairingOfferRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateLocalPairingOfferResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "AcceptLocalPairingOffer",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingOfferRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingOfferResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "AcceptLocalPairingAnswer",
            _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingAnswerRequest,
            _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingAnswerResponse,
            False,
            False,
        ),
    ),
)


class SessionResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.session.SessionResourceService"

    async def get_session_info(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSessionInfoRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSessionInfoResponse:
        call = await self._client.open_call(
            self._service,
            "GetSessionInfo",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSessionInfoResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_resources_list(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchResourcesListRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchResourcesListResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchResourcesList",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchResourcesListResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def create_space(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceResponse:
        call = await self._client.open_call(
            self._service, "CreateSpace", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def mount_shared_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.MountSharedObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.MountSharedObjectResponse:
        call = await self._client.open_call(
            self._service,
            "MountSharedObject",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.MountSharedObjectResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_shared_object_health(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSharedObjectHealthRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSharedObjectHealthResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchSharedObjectHealth",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSharedObjectHealthResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def watch_sync_status(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSyncStatusRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSyncStatusResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchSyncStatus",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSyncStatusResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def watch_storage_stats(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchStorageStatsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchStorageStatsResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchStorageStats",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchStorageStatsResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def delete_space(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteSpaceRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteSpaceResponse:
        call = await self._client.open_call(
            self._service, "DeleteSpace", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteSpaceResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def rename_space(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.RenameSpaceRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.RenameSpaceResponse:
        call = await self._client.open_call(
            self._service, "RenameSpace", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.RenameSpaceResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_lock_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchLockStateRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchLockStateResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchLockState",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchLockStateResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def set_lock_mode(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.SetLockModeRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.SetLockModeResponse:
        call = await self._client.open_call(
            self._service, "SetLockMode", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.SetLockModeResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def set_direct_p2_p_enabled(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.SetDirectP2PEnabledRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.SetDirectP2PEnabledResponse
    ):
        call = await self._client.open_call(
            self._service,
            "SetDirectP2PEnabled",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.SetDirectP2PEnabledResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def unlock_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlockSessionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlockSessionResponse:
        call = await self._client.open_call(
            self._service,
            "UnlockSession",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlockSessionResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def lock_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.LockSessionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.LockSessionResponse:
        call = await self._client.open_call(
            self._service, "LockSession", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.LockSessionResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def generate_pairing_code(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GeneratePairingCodeRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.GeneratePairingCodeResponse
    ):
        call = await self._client.open_call(
            self._service,
            "GeneratePairingCode",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.GeneratePairingCodeResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def complete_pairing(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CompletePairingRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.CompletePairingResponse:
        call = await self._client.open_call(
            self._service,
            "CompletePairing",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.CompletePairingResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_sas_emoji(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSASEmojiRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSASEmojiResponse:
        call = await self._client.open_call(
            self._service, "GetSASEmoji", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSASEmojiResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def confirm_sas_match(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmSASMatchRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmSASMatchResponse:
        call = await self._client.open_call(
            self._service,
            "ConfirmSASMatch",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmSASMatchResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def confirm_pairing(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmPairingRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmPairingResponse:
        call = await self._client.open_call(
            self._service,
            "ConfirmPairing",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmPairingResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def delete_account(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteAccountRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteAccountResponse:
        call = await self._client.open_call(
            self._service,
            "DeleteAccount",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteAccountResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def access_state_atom(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessSessionStateAtomRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessSessionStateAtomResponse:
        call = await self._client.open_call(
            self._service,
            "AccessStateAtom",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessSessionStateAtomResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_state_atoms(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSessionStateAtomsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSessionStateAtomsResponse
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
                response = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSessionStateAtomsResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def get_transfer_inventory(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferInventoryRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferInventoryResponse:
        call = await self._client.open_call(
            self._service,
            "GetTransferInventory",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferInventoryResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def start_transfer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.StartTransferRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.StartTransferResponse:
        call = await self._client.open_call(
            self._service,
            "StartTransfer",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.StartTransferResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_transfer_progress(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchTransferProgressRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchTransferProgressResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchTransferProgress",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchTransferProgressResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def cancel_transfer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CancelTransferRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.CancelTransferResponse:
        call = await self._client.open_call(
            self._service,
            "CancelTransfer",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.CancelTransferResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_transfer_status(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferStatusRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferStatusResponse:
        call = await self._client.open_call(
            self._service,
            "GetTransferStatus",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferStatusResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_paired_devices(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairedDevicesRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairedDevicesResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchPairedDevices",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairedDevicesResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def watch_pairing_status(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairingStatusRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairingStatusResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchPairingStatus",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairingStatusResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def unlink_device(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlinkDeviceRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlinkDeviceResponse:
        call = await self._client.open_call(
            self._service, "UnlinkDevice", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlinkDeviceResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def create_space_invite(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceInviteRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceInviteResponse:
        call = await self._client.open_call(
            self._service,
            "CreateSpaceInvite",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceInviteResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def list_space_invites(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceInvitesRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceInvitesResponse:
        call = await self._client.open_call(
            self._service,
            "ListSpaceInvites",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceInvitesResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def list_space_participants(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceParticipantsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceParticipantsResponse:
        call = await self._client.open_call(
            self._service,
            "ListSpaceParticipants",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceParticipantsResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def remove_space_participant(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.RemoveSpaceParticipantRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.RemoveSpaceParticipantResponse:
        call = await self._client.open_call(
            self._service,
            "RemoveSpaceParticipant",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.RemoveSpaceParticipantResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def revoke_space_invite(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.RevokeSpaceInviteRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.RevokeSpaceInviteResponse:
        call = await self._client.open_call(
            self._service,
            "RevokeSpaceInvite",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.RevokeSpaceInviteResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def join_space_via_invite(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.JoinSpaceViaInviteRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.JoinSpaceViaInviteResponse
    ):
        call = await self._client.open_call(
            self._service,
            "JoinSpaceViaInvite",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.JoinSpaceViaInviteResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def create_local_pairing_offer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateLocalPairingOfferRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateLocalPairingOfferResponse:
        call = await self._client.open_call(
            self._service,
            "CreateLocalPairingOffer",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateLocalPairingOfferResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def accept_local_pairing_offer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingOfferRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingOfferResponse:
        call = await self._client.open_call(
            self._service,
            "AcceptLocalPairingOffer",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingOfferResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def accept_local_pairing_answer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingAnswerRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingAnswerResponse:
        call = await self._client.open_call(
            self._service,
            "AcceptLocalPairingAnswer",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingAnswerResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()


class SessionResourceServiceServer(Protocol):
    async def get_session_info(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSessionInfoRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSessionInfoResponse
    ): ...
    def watch_resources_list(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchResourcesListRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchResourcesListResponse
    ]: ...
    async def create_space(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceResponse: ...
    async def mount_shared_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.MountSharedObjectRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.MountSharedObjectResponse
    ): ...
    def watch_shared_object_health(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSharedObjectHealthRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSharedObjectHealthResponse
    ]: ...
    def watch_sync_status(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSyncStatusRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSyncStatusResponse
    ]: ...
    def watch_storage_stats(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchStorageStatsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchStorageStatsResponse
    ]: ...
    async def delete_space(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteSpaceRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteSpaceResponse: ...
    async def rename_space(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.RenameSpaceRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.RenameSpaceResponse: ...
    def watch_lock_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchLockStateRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchLockStateResponse
    ]: ...
    async def set_lock_mode(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.SetLockModeRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.SetLockModeResponse: ...
    async def set_direct_p2_p_enabled(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.SetDirectP2PEnabledRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.SetDirectP2PEnabledResponse
    ): ...
    async def unlock_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlockSessionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlockSessionResponse: ...
    async def lock_session(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.LockSessionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.LockSessionResponse: ...
    async def generate_pairing_code(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GeneratePairingCodeRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.GeneratePairingCodeResponse
    ): ...
    async def complete_pairing(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CompletePairingRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.CompletePairingResponse
    ): ...
    async def get_sas_emoji(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSASEmojiRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSASEmojiResponse: ...
    async def confirm_sas_match(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmSASMatchRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmSASMatchResponse
    ): ...
    async def confirm_pairing(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmPairingRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmPairingResponse
    ): ...
    async def delete_account(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteAccountRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteAccountResponse: ...
    async def access_state_atom(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessSessionStateAtomRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessSessionStateAtomResponse: ...
    def watch_state_atoms(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSessionStateAtomsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSessionStateAtomsResponse
    ]: ...
    async def get_transfer_inventory(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferInventoryRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferInventoryResponse: ...
    async def start_transfer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.StartTransferRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.StartTransferResponse: ...
    def watch_transfer_progress(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchTransferProgressRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchTransferProgressResponse
    ]: ...
    async def cancel_transfer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CancelTransferRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.CancelTransferResponse
    ): ...
    async def get_transfer_status(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferStatusRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferStatusResponse
    ): ...
    def watch_paired_devices(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairedDevicesRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairedDevicesResponse
    ]: ...
    def watch_pairing_status(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairingStatusRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairingStatusResponse
    ]: ...
    async def unlink_device(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlinkDeviceRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlinkDeviceResponse: ...
    async def create_space_invite(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceInviteRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceInviteResponse
    ): ...
    async def list_space_invites(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceInvitesRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceInvitesResponse
    ): ...
    async def list_space_participants(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceParticipantsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceParticipantsResponse: ...
    async def remove_space_participant(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.RemoveSpaceParticipantRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.RemoveSpaceParticipantResponse: ...
    async def revoke_space_invite(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.RevokeSpaceInviteRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.RevokeSpaceInviteResponse
    ): ...
    async def join_space_via_invite(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.JoinSpaceViaInviteRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.JoinSpaceViaInviteResponse
    ): ...
    async def create_local_pairing_offer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateLocalPairingOfferRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateLocalPairingOfferResponse: ...
    async def accept_local_pairing_offer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingOfferRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingOfferResponse: ...
    async def accept_local_pairing_answer(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingAnswerRequest,
    ) -> _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingAnswerResponse: ...


def register_session_resource_service(
    registry: ServiceRegistry,
    implementation: SessionResourceServiceServer,
    service: str = "s4wave.session.SessionResourceService",
) -> None:
    async def get_session_info_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSessionInfoRequest()
        )
        request.ParseFromString(first)
        response = await implementation.get_session_info(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetSessionInfo", get_session_info_handler)

    async def watch_resources_list_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchResourcesListRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_resources_list(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchResourcesList", watch_resources_list_handler)

    async def create_space_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceRequest()
        )
        request.ParseFromString(first)
        response = await implementation.create_space(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "CreateSpace", create_space_handler)

    async def mount_shared_object_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.MountSharedObjectRequest()
        request.ParseFromString(first)
        response = await implementation.mount_shared_object(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "MountSharedObject", mount_shared_object_handler)

    async def watch_shared_object_health_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSharedObjectHealthRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_shared_object_health(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "WatchSharedObjectHealth", watch_shared_object_health_handler
    )

    async def watch_sync_status_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSyncStatusRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_sync_status(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchSyncStatus", watch_sync_status_handler)

    async def watch_storage_stats_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchStorageStatsRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_storage_stats(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchStorageStats", watch_storage_stats_handler)

    async def delete_space_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteSpaceRequest()
        )
        request.ParseFromString(first)
        response = await implementation.delete_space(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "DeleteSpace", delete_space_handler)

    async def rename_space_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.RenameSpaceRequest()
        )
        request.ParseFromString(first)
        response = await implementation.rename_space(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "RenameSpace", rename_space_handler)

    async def watch_lock_state_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchLockStateRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_lock_state(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchLockState", watch_lock_state_handler)

    async def set_lock_mode_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.SetLockModeRequest()
        )
        request.ParseFromString(first)
        response = await implementation.set_lock_mode(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "SetLockMode", set_lock_mode_handler)

    async def set_direct_p2_p_enabled_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.SetDirectP2PEnabledRequest()
        request.ParseFromString(first)
        response = await implementation.set_direct_p2_p_enabled(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "SetDirectP2PEnabled", set_direct_p2_p_enabled_handler)

    async def unlock_session_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlockSessionRequest()
        )
        request.ParseFromString(first)
        response = await implementation.unlock_session(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "UnlockSession", unlock_session_handler)

    async def lock_session_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.LockSessionRequest()
        )
        request.ParseFromString(first)
        response = await implementation.lock_session(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "LockSession", lock_session_handler)

    async def generate_pairing_code_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.GeneratePairingCodeRequest()
        request.ParseFromString(first)
        response = await implementation.generate_pairing_code(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GeneratePairingCode", generate_pairing_code_handler)

    async def complete_pairing_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.CompletePairingRequest()
        request.ParseFromString(first)
        response = await implementation.complete_pairing(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "CompletePairing", complete_pairing_handler)

    async def get_sas_emoji_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.GetSASEmojiRequest()
        )
        request.ParseFromString(first)
        response = await implementation.get_sas_emoji(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetSASEmoji", get_sas_emoji_handler)

    async def confirm_sas_match_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmSASMatchRequest()
        request.ParseFromString(first)
        response = await implementation.confirm_sas_match(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ConfirmSASMatch", confirm_sas_match_handler)

    async def confirm_pairing_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.ConfirmPairingRequest()
        )
        request.ParseFromString(first)
        response = await implementation.confirm_pairing(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ConfirmPairing", confirm_pairing_handler)

    async def delete_account_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.DeleteAccountRequest()
        )
        request.ParseFromString(first)
        response = await implementation.delete_account(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "DeleteAccount", delete_account_handler)

    async def access_state_atom_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessSessionStateAtomRequest()
        request.ParseFromString(first)
        response = await implementation.access_state_atom(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "AccessStateAtom", access_state_atom_handler)

    async def watch_state_atoms_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchSessionStateAtomsRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_state_atoms(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchStateAtoms", watch_state_atoms_handler)

    async def get_transfer_inventory_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferInventoryRequest()
        request.ParseFromString(first)
        response = await implementation.get_transfer_inventory(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetTransferInventory", get_transfer_inventory_handler)

    async def start_transfer_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.StartTransferRequest()
        )
        request.ParseFromString(first)
        response = await implementation.start_transfer(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "StartTransfer", start_transfer_handler)

    async def watch_transfer_progress_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchTransferProgressRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_transfer_progress(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchTransferProgress", watch_transfer_progress_handler)

    async def cancel_transfer_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.CancelTransferRequest()
        )
        request.ParseFromString(first)
        response = await implementation.cancel_transfer(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "CancelTransfer", cancel_transfer_handler)

    async def get_transfer_status_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.GetTransferStatusRequest()
        request.ParseFromString(first)
        response = await implementation.get_transfer_status(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetTransferStatus", get_transfer_status_handler)

    async def watch_paired_devices_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairedDevicesRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_paired_devices(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchPairedDevices", watch_paired_devices_handler)

    async def watch_pairing_status_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.WatchPairingStatusRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_pairing_status(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchPairingStatus", watch_pairing_status_handler)

    async def unlink_device_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_session_session_pb2.UnlinkDeviceRequest()
        )
        request.ParseFromString(first)
        response = await implementation.unlink_device(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "UnlinkDevice", unlink_device_handler)

    async def create_space_invite_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateSpaceInviteRequest()
        request.ParseFromString(first)
        response = await implementation.create_space_invite(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "CreateSpaceInvite", create_space_invite_handler)

    async def list_space_invites_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceInvitesRequest()
        request.ParseFromString(first)
        response = await implementation.list_space_invites(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ListSpaceInvites", list_space_invites_handler)

    async def list_space_participants_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.ListSpaceParticipantsRequest()
        request.ParseFromString(first)
        response = await implementation.list_space_participants(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ListSpaceParticipants", list_space_participants_handler)

    async def remove_space_participant_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.RemoveSpaceParticipantRequest()
        request.ParseFromString(first)
        response = await implementation.remove_space_participant(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "RemoveSpaceParticipant", remove_space_participant_handler
    )

    async def revoke_space_invite_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.RevokeSpaceInviteRequest()
        request.ParseFromString(first)
        response = await implementation.revoke_space_invite(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "RevokeSpaceInvite", revoke_space_invite_handler)

    async def join_space_via_invite_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.JoinSpaceViaInviteRequest()
        request.ParseFromString(first)
        response = await implementation.join_space_via_invite(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "JoinSpaceViaInvite", join_space_via_invite_handler)

    async def create_local_pairing_offer_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.CreateLocalPairingOfferRequest()
        request.ParseFromString(first)
        response = await implementation.create_local_pairing_offer(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "CreateLocalPairingOffer", create_local_pairing_offer_handler
    )

    async def accept_local_pairing_offer_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingOfferRequest()
        request.ParseFromString(first)
        response = await implementation.accept_local_pairing_offer(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "AcceptLocalPairingOffer", accept_local_pairing_offer_handler
    )

    async def accept_local_pairing_answer_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_session_session_pb2.AcceptLocalPairingAnswerRequest()
        request.ParseFromString(first)
        response = await implementation.accept_local_pairing_answer(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "AcceptLocalPairingAnswer", accept_local_pairing_answer_handler
    )
