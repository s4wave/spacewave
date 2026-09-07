from collections.abc import AsyncIterator
from typing import Protocol

from sdk.session import (
    session_pb2 as _github_com_s4wave_spacewave_sdk_session_session_pb2,
)
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import ServiceDescriptor

SESSIONRESOURCESERVICE_SERVICE: ServiceDescriptor

class SessionResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
    async def access_peer_transport(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessPeerTransportRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessPeerTransportResponse
    ): ...
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
    async def access_peer_transport(
        self,
        request: _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessPeerTransportRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_session_session_pb2.AccessPeerTransportResponse
    ): ...
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
) -> None: ...
