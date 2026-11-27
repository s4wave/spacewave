from __future__ import annotations

from collections.abc import AsyncIterable, AsyncIterator
from typing import Protocol

from sdk.world import (
    world_pb2 as _github_com_s4wave_spacewave_sdk_world_world_pb2,
)
from starpc.call import Call, CallProtocolError
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import MethodDescriptor, ServiceDescriptor, bidirectional_bytes

ENGINERESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.world.EngineResourceService",
    (
        MethodDescriptor(
            "GetEngineInfo",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetEngineInfoRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetEngineInfoResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetWorldRootSnapshot",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetWorldRootSnapshotRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WorldRootSnapshot,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchWorldRootSnapshots",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldRootSnapshotsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WorldRootSnapshot,
            False,
            True,
        ),
        MethodDescriptor(
            "SetRetainedRoot",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRetainedRootRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRetainedRootResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "NewTransaction",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.NewTransactionRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.NewTransactionResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ExecuteWorldOp",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ExecuteWorldBatch",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ExecuteWorldBatchRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ExecuteWorldBatchResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "Sync",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "SyncThrough",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncThroughRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncThroughResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetSeqno",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WaitSeqno",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchTypeCounts",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "SearchObjectsWithType",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "WatchChanges",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchChangesRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchChangesResponse,
            True,
            True,
        ),
        MethodDescriptor(
            "WaitObjectRev",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitObjectRevRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitObjectRevResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "BuildStorageCursor",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "AccessWorldState",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "StageWorldState",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateResponse,
            False,
            False,
        ),
    ),
)

WORLDSTAGERESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.world.WorldStageResourceService",
    (
        MethodDescriptor(
            "BuildStorageCursor",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "AccessWorldState",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ReleaseRoots",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ReleaseRootsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ReleaseRootsResponse,
            False,
            False,
        ),
    ),
)

WORLDSTATERESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.world.WorldStateResourceService",
    (
        MethodDescriptor(
            "CompareObjectRecords",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CompareObjectRecordsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CompareObjectRecordsResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetReadOnly",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetReadOnlyRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetReadOnlyResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "Sync",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetSeqno",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WaitSeqno",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "BuildStorageCursor",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "AccessWorldState",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "StageWorldState",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "OpenNestedWorld",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenNestedWorldRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenNestedWorldResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "OpenOuterWorld",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenOuterWorldRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenOuterWorldResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "CreateObject",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CreateObjectRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CreateObjectResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetObject",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "IterateObjects",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.IterateObjectsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.IterateObjectsResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ListObjects",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "RenameObject",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.RenameObjectRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.RenameObjectResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "DeleteObject",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteObjectRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteObjectResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "SetGraphQuad",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SetGraphQuadRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SetGraphQuadResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "DeleteGraphQuad",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphQuadRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphQuadResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "LookupGraphQuads",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "LookupGraphQuadsBatch",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsBatchRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsBatchResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ListGraphEdgeBuckets",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ListGraphEdgeBucketsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ListGraphEdgeBucketsResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ListObjectsWithType",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsWithTypeRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsWithTypeResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ListTypeCounts",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ListTypeCountsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ListTypeCountsResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchTypeCounts",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "SearchObjectsWithType",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "GetObjectRootRefsBatch",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRootRefsBatchRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRootRefsBatchResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetObjectMetadataBatch",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectMetadataBatchRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectMetadataBatchResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetObjectBodiesBatch",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectBodiesBatchRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectBodiesBatchResponse,
            False,
            True,
        ),
        MethodDescriptor(
            "QueryGraphPath",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.QueryGraphPathRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.QueryGraphPathResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "DeleteGraphObject",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphObjectRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphObjectResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ApplyWorldOp",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpResponse,
            False,
            False,
        ),
    ),
)

WATCHWORLDSTATERESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.world.WatchWorldStateResourceService",
    (
        MethodDescriptor(
            "WatchWorldState",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateResponse,
            False,
            True,
        ),
    ),
)

TXRESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.world.TxResourceService",
    (
        MethodDescriptor(
            "CommitTx",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "Discard",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardResponse,
            False,
            False,
        ),
    ),
)

OBJECTITERATORRESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.world.ObjectIteratorResourceService",
    (
        MethodDescriptor(
            "Err",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ErrRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ErrResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "Valid",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ValidRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ValidResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "Key",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.KeyRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.KeyResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "Next",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.NextRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.NextResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "Seek",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SeekRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SeekResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "Close",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseResponse,
            False,
            False,
        ),
    ),
)

GRAPHPATHQUERYRESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.world.GraphPathQueryResourceService",
    (
        MethodDescriptor(
            "Next",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.NextGraphPathQueryRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.NextGraphPathQueryResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "Close",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseGraphPathQueryRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseGraphPathQueryResponse,
            False,
            False,
        ),
    ),
)

OBJECTSTATERESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.world.ObjectStateResourceService",
    (
        MethodDescriptor(
            "GetKey",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetKeyRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetKeyResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetRootRef",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetRootRefRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetRootRefResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "GetModSeqno",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetModSeqnoRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetModSeqnoResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "SetRootRef",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRootRefRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRootRefResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "AccessWorldState",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "ApplyObjectOp",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyObjectOpRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyObjectOpResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "IncrementRev",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.IncrementRevRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.IncrementRevResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WaitRev",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitRevRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitRevResponse,
            False,
            False,
        ),
    ),
)

TYPEDOBJECTRESOURCESERVICE_SERVICE = ServiceDescriptor(
    "s4wave.world.TypedObjectResourceService",
    (
        MethodDescriptor(
            "AccessTypedObject",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessTypedObjectRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessTypedObjectResponse,
            False,
            False,
        ),
        MethodDescriptor(
            "WatchTypedObject",
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypedObjectRequest,
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypedObjectResponse,
            False,
            True,
        ),
    ),
)


class EngineResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.world.EngineResourceService"

    async def get_engine_info(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetEngineInfoRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetEngineInfoResponse:
        call = await self._client.open_call(
            self._service,
            "GetEngineInfo",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.GetEngineInfoResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_world_root_snapshot(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetWorldRootSnapshotRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WorldRootSnapshot:
        call = await self._client.open_call(
            self._service,
            "GetWorldRootSnapshot",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.WorldRootSnapshot()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_world_root_snapshots(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldRootSnapshotsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WorldRootSnapshot
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchWorldRootSnapshots",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = (
                    _github_com_s4wave_spacewave_sdk_world_world_pb2.WorldRootSnapshot()
                )
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def set_retained_root(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRetainedRootRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRetainedRootResponse:
        call = await self._client.open_call(
            self._service,
            "SetRetainedRoot",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRetainedRootResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def new_transaction(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.NewTransactionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.NewTransactionResponse:
        call = await self._client.open_call(
            self._service,
            "NewTransaction",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.NewTransactionResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def execute_world_op(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpResponse:
        call = await self._client.open_call(
            self._service,
            "ExecuteWorldOp",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def execute_world_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ExecuteWorldBatchRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ExecuteWorldBatchResponse:
        call = await self._client.open_call(
            self._service,
            "ExecuteWorldBatch",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.ExecuteWorldBatchResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def sync(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncResponse:
        call = await self._client.open_call(
            self._service, "Sync", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def sync_through(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncThroughRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncThroughResponse:
        call = await self._client.open_call(
            self._service, "SyncThrough", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncThroughResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_seqno(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoResponse:
        call = await self._client.open_call(
            self._service, "GetSeqno", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def wait_seqno(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoResponse:
        call = await self._client.open_call(
            self._service, "WaitSeqno", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_type_counts(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchTypeCounts",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def search_objects_with_type(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "SearchObjectsWithType",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def watch_changes(
        self,
        requests: AsyncIterable[
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchChangesRequest
        ],
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchChangesResponse
    ]:
        call = await self._client.open_call(self._service, "WatchChanges")

        async def encoded() -> AsyncIterator[bytes]:
            async for request in requests:
                yield request.SerializeToString(deterministic=True)

        async for data in bidirectional_bytes(call, encoded()):
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchChangesResponse()
            )
            response.ParseFromString(data)
            yield response

    async def wait_object_rev(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitObjectRevRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitObjectRevResponse:
        call = await self._client.open_call(
            self._service,
            "WaitObjectRev",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitObjectRevResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def build_storage_cursor(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse:
        call = await self._client.open_call(
            self._service,
            "BuildStorageCursor",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def access_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse:
        call = await self._client.open_call(
            self._service,
            "AccessWorldState",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def stage_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateResponse:
        call = await self._client.open_call(
            self._service,
            "StageWorldState",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()


class WorldStageResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.world.WorldStageResourceService"

    async def build_storage_cursor(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse:
        call = await self._client.open_call(
            self._service,
            "BuildStorageCursor",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def access_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse:
        call = await self._client.open_call(
            self._service,
            "AccessWorldState",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def release_roots(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ReleaseRootsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ReleaseRootsResponse:
        call = await self._client.open_call(
            self._service, "ReleaseRoots", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.ReleaseRootsResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()


class WorldStateResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.world.WorldStateResourceService"

    async def compare_object_records(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CompareObjectRecordsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.CompareObjectRecordsResponse:
        call = await self._client.open_call(
            self._service,
            "CompareObjectRecords",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.CompareObjectRecordsResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_read_only(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetReadOnlyRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetReadOnlyResponse:
        call = await self._client.open_call(
            self._service, "GetReadOnly", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.GetReadOnlyResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def sync(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncResponse:
        call = await self._client.open_call(
            self._service, "Sync", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_seqno(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoResponse:
        call = await self._client.open_call(
            self._service, "GetSeqno", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def wait_seqno(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoResponse:
        call = await self._client.open_call(
            self._service, "WaitSeqno", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def build_storage_cursor(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse:
        call = await self._client.open_call(
            self._service,
            "BuildStorageCursor",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def access_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse:
        call = await self._client.open_call(
            self._service,
            "AccessWorldState",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def stage_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateResponse:
        call = await self._client.open_call(
            self._service,
            "StageWorldState",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def open_nested_world(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenNestedWorldRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenNestedWorldResponse:
        call = await self._client.open_call(
            self._service,
            "OpenNestedWorld",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenNestedWorldResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def open_outer_world(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenOuterWorldRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenOuterWorldResponse:
        call = await self._client.open_call(
            self._service,
            "OpenOuterWorld",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenOuterWorldResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def create_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CreateObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.CreateObjectResponse:
        call = await self._client.open_call(
            self._service, "CreateObject", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.CreateObjectResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_object(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectResponse:
        call = await self._client.open_call(
            self._service, "GetObject", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def iterate_objects(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.IterateObjectsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.IterateObjectsResponse:
        call = await self._client.open_call(
            self._service,
            "IterateObjects",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.IterateObjectsResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def list_objects(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsResponse:
        call = await self._client.open_call(
            self._service, "ListObjects", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def rename_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.RenameObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.RenameObjectResponse:
        call = await self._client.open_call(
            self._service, "RenameObject", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.RenameObjectResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def delete_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteObjectResponse:
        call = await self._client.open_call(
            self._service, "DeleteObject", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteObjectResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def set_graph_quad(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SetGraphQuadRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SetGraphQuadResponse:
        call = await self._client.open_call(
            self._service, "SetGraphQuad", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.SetGraphQuadResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def delete_graph_quad(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphQuadRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphQuadResponse:
        call = await self._client.open_call(
            self._service,
            "DeleteGraphQuad",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphQuadResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def lookup_graph_quads(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsResponse:
        call = await self._client.open_call(
            self._service,
            "LookupGraphQuads",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def lookup_graph_quads_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsBatchRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsBatchResponse:
        call = await self._client.open_call(
            self._service,
            "LookupGraphQuadsBatch",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsBatchResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def list_graph_edge_buckets(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ListGraphEdgeBucketsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ListGraphEdgeBucketsResponse:
        call = await self._client.open_call(
            self._service,
            "ListGraphEdgeBuckets",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.ListGraphEdgeBucketsResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def list_objects_with_type(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsWithTypeRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsWithTypeResponse:
        call = await self._client.open_call(
            self._service,
            "ListObjectsWithType",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsWithTypeResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def list_type_counts(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ListTypeCountsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ListTypeCountsResponse:
        call = await self._client.open_call(
            self._service,
            "ListTypeCounts",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.ListTypeCountsResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_type_counts(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchTypeCounts",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def search_objects_with_type(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "SearchObjectsWithType",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def get_object_root_refs_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRootRefsBatchRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRootRefsBatchResponse
    ):
        call = await self._client.open_call(
            self._service,
            "GetObjectRootRefsBatch",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRootRefsBatchResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_object_metadata_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectMetadataBatchRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectMetadataBatchResponse
    ):
        call = await self._client.open_call(
            self._service,
            "GetObjectMetadataBatch",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectMetadataBatchResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_object_bodies_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectBodiesBatchRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectBodiesBatchResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "GetObjectBodiesBatch",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectBodiesBatchResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()

    async def query_graph_path(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.QueryGraphPathRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.QueryGraphPathResponse:
        call = await self._client.open_call(
            self._service,
            "QueryGraphPath",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.QueryGraphPathResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def delete_graph_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphObjectResponse:
        call = await self._client.open_call(
            self._service,
            "DeleteGraphObject",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphObjectResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def apply_world_op(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpResponse:
        call = await self._client.open_call(
            self._service, "ApplyWorldOp", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()


class WatchWorldStateResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.world.WatchWorldStateResourceService"

    async def watch_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchWorldState",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()


class TxResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.world.TxResourceService"

    async def commit_tx(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitResponse:
        call = await self._client.open_call(
            self._service, "CommitTx", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def discard(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardResponse:
        call = await self._client.open_call(
            self._service, "Discard", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()


class ObjectIteratorResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.world.ObjectIteratorResourceService"

    async def err(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ErrRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ErrResponse:
        call = await self._client.open_call(
            self._service, "Err", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.ErrResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def valid(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ValidRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ValidResponse:
        call = await self._client.open_call(
            self._service, "Valid", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.ValidResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def key(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.KeyRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.KeyResponse:
        call = await self._client.open_call(
            self._service, "Key", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.KeyResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def next(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.NextRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.NextResponse:
        call = await self._client.open_call(
            self._service, "Next", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.NextResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def seek(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SeekRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SeekResponse:
        call = await self._client.open_call(
            self._service, "Seek", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.SeekResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def close(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseResponse:
        call = await self._client.open_call(
            self._service, "Close", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()


class GraphPathQueryResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.world.GraphPathQueryResourceService"

    async def next(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.NextGraphPathQueryRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.NextGraphPathQueryResponse:
        call = await self._client.open_call(
            self._service, "Next", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.NextGraphPathQueryResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def close(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseGraphPathQueryRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseGraphPathQueryResponse:
        call = await self._client.open_call(
            self._service, "Close", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseGraphPathQueryResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()


class ObjectStateResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.world.ObjectStateResourceService"

    async def get_key(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetKeyRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetKeyResponse:
        call = await self._client.open_call(
            self._service, "GetKey", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetKeyResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_root_ref(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetRootRefRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetRootRefResponse:
        call = await self._client.open_call(
            self._service, "GetRootRef", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.GetRootRefResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def get_mod_seqno(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetModSeqnoRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetModSeqnoResponse:
        call = await self._client.open_call(
            self._service, "GetModSeqno", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.GetModSeqnoResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def set_root_ref(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRootRefRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRootRefResponse:
        call = await self._client.open_call(
            self._service, "SetRootRef", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRootRefResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def access_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse:
        call = await self._client.open_call(
            self._service,
            "AccessWorldState",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def apply_object_op(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyObjectOpRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyObjectOpResponse:
        call = await self._client.open_call(
            self._service,
            "ApplyObjectOp",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyObjectOpResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def increment_rev(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.IncrementRevRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.IncrementRevResponse:
        call = await self._client.open_call(
            self._service, "IncrementRev", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.IncrementRevResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def wait_rev(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitRevRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitRevResponse:
        call = await self._client.open_call(
            self._service, "WaitRev", request.SerializeToString(deterministic=True)
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = (
                _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitRevResponse()
            )
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()


class TypedObjectResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None:
        self._client = client
        self._service = service or "s4wave.world.TypedObjectResourceService"

    async def access_typed_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessTypedObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessTypedObjectResponse:
        call = await self._client.open_call(
            self._service,
            "AccessTypedObject",
            request.SerializeToString(deterministic=True),
        )
        try:
            data = await call.receive()
            if data is None:
                raise CallProtocolError("missing unary response")
            response = _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessTypedObjectResponse()
            response.ParseFromString(data)
            if await call.receive() is not None:
                raise CallProtocolError("extra unary response")
            return response
        finally:
            await call.aclose()

    async def watch_typed_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypedObjectRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypedObjectResponse
    ]:
        call = await self._client.open_call(
            self._service,
            "WatchTypedObject",
            request.SerializeToString(deterministic=True),
        )
        try:
            while True:
                data = await call.receive()
                if data is None:
                    return
                response = _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypedObjectResponse()
                response.ParseFromString(data)
                yield response
        finally:
            await call.aclose()


class EngineResourceServiceServer(Protocol):
    async def get_engine_info(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetEngineInfoRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetEngineInfoResponse: ...
    async def get_world_root_snapshot(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetWorldRootSnapshotRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WorldRootSnapshot: ...
    def watch_world_root_snapshots(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldRootSnapshotsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WorldRootSnapshot
    ]: ...
    async def set_retained_root(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRetainedRootRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRetainedRootResponse: ...
    async def new_transaction(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.NewTransactionRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.NewTransactionResponse: ...
    async def execute_world_op(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpResponse: ...
    async def execute_world_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ExecuteWorldBatchRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ExecuteWorldBatchResponse: ...
    async def sync(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncResponse: ...
    async def sync_through(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncThroughRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncThroughResponse: ...
    async def get_seqno(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoResponse: ...
    async def wait_seqno(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoResponse: ...
    def watch_type_counts(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsResponse
    ]: ...
    def search_objects_with_type(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeResponse
    ]: ...
    def watch_changes(
        self,
        requests: AsyncIterator[
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchChangesRequest
        ],
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchChangesResponse
    ]: ...
    async def wait_object_rev(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitObjectRevRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitObjectRevResponse: ...
    async def build_storage_cursor(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse
    ): ...
    async def access_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse: ...
    async def stage_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateResponse: ...


def register_engine_resource_service(
    registry: ServiceRegistry,
    implementation: EngineResourceServiceServer,
    service: str = "s4wave.world.EngineResourceService",
) -> None:
    async def get_engine_info_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.GetEngineInfoRequest()
        )
        request.ParseFromString(first)
        response = await implementation.get_engine_info(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetEngineInfo", get_engine_info_handler)

    async def get_world_root_snapshot_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetWorldRootSnapshotRequest()
        request.ParseFromString(first)
        response = await implementation.get_world_root_snapshot(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetWorldRootSnapshot", get_world_root_snapshot_handler)

    async def watch_world_root_snapshots_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldRootSnapshotsRequest()
        request.ParseFromString(first)
        async for response in implementation.watch_world_root_snapshots(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "WatchWorldRootSnapshots", watch_world_root_snapshots_handler
    )

    async def set_retained_root_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRetainedRootRequest()
        )
        request.ParseFromString(first)
        response = await implementation.set_retained_root(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "SetRetainedRoot", set_retained_root_handler)

    async def new_transaction_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.NewTransactionRequest()
        )
        request.ParseFromString(first)
        response = await implementation.new_transaction(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "NewTransaction", new_transaction_handler)

    async def execute_world_op_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpRequest()
        request.ParseFromString(first)
        response = await implementation.execute_world_op(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ExecuteWorldOp", execute_world_op_handler)

    async def execute_world_batch_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ExecuteWorldBatchRequest()
        )
        request.ParseFromString(first)
        response = await implementation.execute_world_batch(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ExecuteWorldBatch", execute_world_batch_handler)

    async def sync_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncRequest()
        request.ParseFromString(first)
        response = await implementation.sync(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Sync", sync_handler)

    async def sync_through_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncThroughRequest()
        request.ParseFromString(first)
        response = await implementation.sync_through(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "SyncThrough", sync_through_handler)

    async def get_seqno_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoRequest()
        request.ParseFromString(first)
        response = await implementation.get_seqno(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetSeqno", get_seqno_handler)

    async def wait_seqno_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoRequest()
        request.ParseFromString(first)
        response = await implementation.wait_seqno(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WaitSeqno", wait_seqno_handler)

    async def watch_type_counts_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_type_counts(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchTypeCounts", watch_type_counts_handler)

    async def search_objects_with_type_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeRequest()
        request.ParseFromString(first)
        async for response in implementation.search_objects_with_type(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "SearchObjectsWithType", search_objects_with_type_handler
    )

    async def watch_changes_handler(call: Call) -> None:
        async def requests() -> AsyncIterator[
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchChangesRequest
        ]:
            while True:
                data = await call.receive()
                if data is None:
                    return
                request = _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchChangesRequest()
                request.ParseFromString(data)
                yield request

        async for response in implementation.watch_changes(requests()):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchChanges", watch_changes_handler)

    async def wait_object_rev_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitObjectRevRequest()
        )
        request.ParseFromString(first)
        response = await implementation.wait_object_rev(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WaitObjectRev", wait_object_rev_handler)

    async def build_storage_cursor_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest()
        )
        request.ParseFromString(first)
        response = await implementation.build_storage_cursor(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "BuildStorageCursor", build_storage_cursor_handler)

    async def access_world_state_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest()
        )
        request.ParseFromString(first)
        response = await implementation.access_world_state(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "AccessWorldState", access_world_state_handler)

    async def stage_world_state_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateRequest()
        )
        request.ParseFromString(first)
        response = await implementation.stage_world_state(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "StageWorldState", stage_world_state_handler)


class WorldStageResourceServiceServer(Protocol):
    async def build_storage_cursor(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse
    ): ...
    async def access_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse: ...
    async def release_roots(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ReleaseRootsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ReleaseRootsResponse: ...


def register_world_stage_resource_service(
    registry: ServiceRegistry,
    implementation: WorldStageResourceServiceServer,
    service: str = "s4wave.world.WorldStageResourceService",
) -> None:
    async def build_storage_cursor_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest()
        )
        request.ParseFromString(first)
        response = await implementation.build_storage_cursor(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "BuildStorageCursor", build_storage_cursor_handler)

    async def access_world_state_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest()
        )
        request.ParseFromString(first)
        response = await implementation.access_world_state(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "AccessWorldState", access_world_state_handler)

    async def release_roots_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.ReleaseRootsRequest()
        request.ParseFromString(first)
        response = await implementation.release_roots(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ReleaseRoots", release_roots_handler)


class WorldStateResourceServiceServer(Protocol):
    async def compare_object_records(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CompareObjectRecordsRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.CompareObjectRecordsResponse
    ): ...
    async def get_read_only(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetReadOnlyRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetReadOnlyResponse: ...
    async def sync(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncResponse: ...
    async def get_seqno(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoResponse: ...
    async def wait_seqno(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoResponse: ...
    async def build_storage_cursor(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorResponse
    ): ...
    async def access_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse: ...
    async def stage_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateResponse: ...
    async def open_nested_world(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenNestedWorldRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenNestedWorldResponse: ...
    async def open_outer_world(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenOuterWorldRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenOuterWorldResponse: ...
    async def create_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CreateObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.CreateObjectResponse: ...
    async def get_object(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectResponse: ...
    async def iterate_objects(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.IterateObjectsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.IterateObjectsResponse: ...
    async def list_objects(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsResponse: ...
    async def rename_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.RenameObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.RenameObjectResponse: ...
    async def delete_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteObjectResponse: ...
    async def set_graph_quad(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SetGraphQuadRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SetGraphQuadResponse: ...
    async def delete_graph_quad(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphQuadRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphQuadResponse: ...
    async def lookup_graph_quads(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsResponse: ...
    async def lookup_graph_quads_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsBatchRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsBatchResponse
    ): ...
    async def list_graph_edge_buckets(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ListGraphEdgeBucketsRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.ListGraphEdgeBucketsResponse
    ): ...
    async def list_objects_with_type(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsWithTypeRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsWithTypeResponse
    ): ...
    async def list_type_counts(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ListTypeCountsRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ListTypeCountsResponse: ...
    def watch_type_counts(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsResponse
    ]: ...
    def search_objects_with_type(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeResponse
    ]: ...
    async def get_object_root_refs_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRootRefsBatchRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRootRefsBatchResponse
    ): ...
    async def get_object_metadata_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectMetadataBatchRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectMetadataBatchResponse
    ): ...
    def get_object_bodies_batch(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectBodiesBatchRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectBodiesBatchResponse
    ]: ...
    async def query_graph_path(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.QueryGraphPathRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.QueryGraphPathResponse: ...
    async def delete_graph_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphObjectResponse: ...
    async def apply_world_op(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpResponse: ...


def register_world_state_resource_service(
    registry: ServiceRegistry,
    implementation: WorldStateResourceServiceServer,
    service: str = "s4wave.world.WorldStateResourceService",
) -> None:
    async def compare_object_records_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.CompareObjectRecordsRequest()
        request.ParseFromString(first)
        response = await implementation.compare_object_records(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "CompareObjectRecords", compare_object_records_handler)

    async def get_read_only_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetReadOnlyRequest()
        request.ParseFromString(first)
        response = await implementation.get_read_only(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetReadOnly", get_read_only_handler)

    async def sync_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.SyncRequest()
        request.ParseFromString(first)
        response = await implementation.sync(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Sync", sync_handler)

    async def get_seqno_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetSeqnoRequest()
        request.ParseFromString(first)
        response = await implementation.get_seqno(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetSeqno", get_seqno_handler)

    async def wait_seqno_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitSeqnoRequest()
        request.ParseFromString(first)
        response = await implementation.wait_seqno(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WaitSeqno", wait_seqno_handler)

    async def build_storage_cursor_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.BuildStorageCursorRequest()
        )
        request.ParseFromString(first)
        response = await implementation.build_storage_cursor(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "BuildStorageCursor", build_storage_cursor_handler)

    async def access_world_state_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest()
        )
        request.ParseFromString(first)
        response = await implementation.access_world_state(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "AccessWorldState", access_world_state_handler)

    async def stage_world_state_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.StageWorldStateRequest()
        )
        request.ParseFromString(first)
        response = await implementation.stage_world_state(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "StageWorldState", stage_world_state_handler)

    async def open_nested_world_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenNestedWorldRequest()
        )
        request.ParseFromString(first)
        response = await implementation.open_nested_world(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "OpenNestedWorld", open_nested_world_handler)

    async def open_outer_world_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.OpenOuterWorldRequest()
        )
        request.ParseFromString(first)
        response = await implementation.open_outer_world(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "OpenOuterWorld", open_outer_world_handler)

    async def create_object_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.CreateObjectRequest()
        request.ParseFromString(first)
        response = await implementation.create_object(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "CreateObject", create_object_handler)

    async def get_object_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRequest()
        request.ParseFromString(first)
        response = await implementation.get_object(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetObject", get_object_handler)

    async def iterate_objects_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.IterateObjectsRequest()
        )
        request.ParseFromString(first)
        response = await implementation.iterate_objects(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "IterateObjects", iterate_objects_handler)

    async def list_objects_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsRequest()
        request.ParseFromString(first)
        response = await implementation.list_objects(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ListObjects", list_objects_handler)

    async def rename_object_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.RenameObjectRequest()
        request.ParseFromString(first)
        response = await implementation.rename_object(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "RenameObject", rename_object_handler)

    async def delete_object_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteObjectRequest()
        request.ParseFromString(first)
        response = await implementation.delete_object(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "DeleteObject", delete_object_handler)

    async def set_graph_quad_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.SetGraphQuadRequest()
        request.ParseFromString(first)
        response = await implementation.set_graph_quad(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "SetGraphQuad", set_graph_quad_handler)

    async def delete_graph_quad_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphQuadRequest()
        )
        request.ParseFromString(first)
        response = await implementation.delete_graph_quad(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "DeleteGraphQuad", delete_graph_quad_handler)

    async def lookup_graph_quads_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsRequest()
        )
        request.ParseFromString(first)
        response = await implementation.lookup_graph_quads(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "LookupGraphQuads", lookup_graph_quads_handler)

    async def lookup_graph_quads_batch_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.LookupGraphQuadsBatchRequest()
        request.ParseFromString(first)
        response = await implementation.lookup_graph_quads_batch(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "LookupGraphQuadsBatch", lookup_graph_quads_batch_handler
    )

    async def list_graph_edge_buckets_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.ListGraphEdgeBucketsRequest()
        request.ParseFromString(first)
        response = await implementation.list_graph_edge_buckets(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ListGraphEdgeBuckets", list_graph_edge_buckets_handler)

    async def list_objects_with_type_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.ListObjectsWithTypeRequest()
        request.ParseFromString(first)
        response = await implementation.list_objects_with_type(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ListObjectsWithType", list_objects_with_type_handler)

    async def list_type_counts_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ListTypeCountsRequest()
        )
        request.ParseFromString(first)
        response = await implementation.list_type_counts(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ListTypeCounts", list_type_counts_handler)

    async def watch_type_counts_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypeCountsRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_type_counts(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchTypeCounts", watch_type_counts_handler)

    async def search_objects_with_type_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.SearchObjectsWithTypeRequest()
        request.ParseFromString(first)
        async for response in implementation.search_objects_with_type(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "SearchObjectsWithType", search_objects_with_type_handler
    )

    async def get_object_root_refs_batch_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectRootRefsBatchRequest()
        request.ParseFromString(first)
        response = await implementation.get_object_root_refs_batch(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "GetObjectRootRefsBatch", get_object_root_refs_batch_handler
    )

    async def get_object_metadata_batch_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectMetadataBatchRequest()
        request.ParseFromString(first)
        response = await implementation.get_object_metadata_batch(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(
        service, "GetObjectMetadataBatch", get_object_metadata_batch_handler
    )

    async def get_object_bodies_batch_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetObjectBodiesBatchRequest()
        request.ParseFromString(first)
        async for response in implementation.get_object_bodies_batch(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetObjectBodiesBatch", get_object_bodies_batch_handler)

    async def query_graph_path_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.QueryGraphPathRequest()
        )
        request.ParseFromString(first)
        response = await implementation.query_graph_path(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "QueryGraphPath", query_graph_path_handler)

    async def delete_graph_object_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.DeleteGraphObjectRequest()
        )
        request.ParseFromString(first)
        response = await implementation.delete_graph_object(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "DeleteGraphObject", delete_graph_object_handler)

    async def apply_world_op_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyWorldOpRequest()
        request.ParseFromString(first)
        response = await implementation.apply_world_op(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ApplyWorldOp", apply_world_op_handler)


class WatchWorldStateResourceServiceServer(Protocol):
    def watch_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateResponse
    ]: ...


def register_watch_world_state_resource_service(
    registry: ServiceRegistry,
    implementation: WatchWorldStateResourceServiceServer,
    service: str = "s4wave.world.WatchWorldStateResourceService",
) -> None:
    async def watch_world_state_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_world_state(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchWorldState", watch_world_state_handler)


class TxResourceServiceServer(Protocol):
    async def commit_tx(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitResponse: ...
    async def discard(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardResponse: ...


def register_tx_resource_service(
    registry: ServiceRegistry,
    implementation: TxResourceServiceServer,
    service: str = "s4wave.world.TxResourceService",
) -> None:
    async def commit_tx_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitRequest()
        request.ParseFromString(first)
        response = await implementation.commit_tx(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "CommitTx", commit_tx_handler)

    async def discard_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardRequest()
        request.ParseFromString(first)
        response = await implementation.discard(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Discard", discard_handler)


class ObjectIteratorResourceServiceServer(Protocol):
    async def err(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ErrRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ErrResponse: ...
    async def valid(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ValidRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ValidResponse: ...
    async def key(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.KeyRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.KeyResponse: ...
    async def next(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.NextRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.NextResponse: ...
    async def seek(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SeekRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SeekResponse: ...
    async def close(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseResponse: ...


def register_object_iterator_resource_service(
    registry: ServiceRegistry,
    implementation: ObjectIteratorResourceServiceServer,
    service: str = "s4wave.world.ObjectIteratorResourceService",
) -> None:
    async def err_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.ErrRequest()
        request.ParseFromString(first)
        response = await implementation.err(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Err", err_handler)

    async def valid_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.ValidRequest()
        request.ParseFromString(first)
        response = await implementation.valid(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Valid", valid_handler)

    async def key_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.KeyRequest()
        request.ParseFromString(first)
        response = await implementation.key(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Key", key_handler)

    async def next_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.NextRequest()
        request.ParseFromString(first)
        response = await implementation.next(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Next", next_handler)

    async def seek_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.SeekRequest()
        request.ParseFromString(first)
        response = await implementation.seek(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Seek", seek_handler)

    async def close_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseRequest()
        request.ParseFromString(first)
        response = await implementation.close(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Close", close_handler)


class GraphPathQueryResourceServiceServer(Protocol):
    async def next(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.NextGraphPathQueryRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.NextGraphPathQueryResponse
    ): ...
    async def close(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseGraphPathQueryRequest,
    ) -> (
        _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseGraphPathQueryResponse
    ): ...


def register_graph_path_query_resource_service(
    registry: ServiceRegistry,
    implementation: GraphPathQueryResourceServiceServer,
    service: str = "s4wave.world.GraphPathQueryResourceService",
) -> None:
    async def next_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.NextGraphPathQueryRequest()
        )
        request.ParseFromString(first)
        response = await implementation.next(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Next", next_handler)

    async def close_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.CloseGraphPathQueryRequest()
        request.ParseFromString(first)
        response = await implementation.close(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "Close", close_handler)


class ObjectStateResourceServiceServer(Protocol):
    async def get_key(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetKeyRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetKeyResponse: ...
    async def get_root_ref(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetRootRefRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetRootRefResponse: ...
    async def get_mod_seqno(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.GetModSeqnoRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.GetModSeqnoResponse: ...
    async def set_root_ref(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRootRefRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRootRefResponse: ...
    async def access_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateResponse: ...
    async def apply_object_op(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyObjectOpRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyObjectOpResponse: ...
    async def increment_rev(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.IncrementRevRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.IncrementRevResponse: ...
    async def wait_rev(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitRevRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitRevResponse: ...


def register_object_state_resource_service(
    registry: ServiceRegistry,
    implementation: ObjectStateResourceServiceServer,
    service: str = "s4wave.world.ObjectStateResourceService",
) -> None:
    async def get_key_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetKeyRequest()
        request.ParseFromString(first)
        response = await implementation.get_key(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetKey", get_key_handler)

    async def get_root_ref_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetRootRefRequest()
        request.ParseFromString(first)
        response = await implementation.get_root_ref(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetRootRef", get_root_ref_handler)

    async def get_mod_seqno_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.GetModSeqnoRequest()
        request.ParseFromString(first)
        response = await implementation.get_mod_seqno(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "GetModSeqno", get_mod_seqno_handler)

    async def set_root_ref_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.SetRootRefRequest()
        request.ParseFromString(first)
        response = await implementation.set_root_ref(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "SetRootRef", set_root_ref_handler)

    async def access_world_state_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessWorldStateRequest()
        )
        request.ParseFromString(first)
        response = await implementation.access_world_state(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "AccessWorldState", access_world_state_handler)

    async def apply_object_op_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.ApplyObjectOpRequest()
        )
        request.ParseFromString(first)
        response = await implementation.apply_object_op(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "ApplyObjectOp", apply_object_op_handler)

    async def increment_rev_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.IncrementRevRequest()
        request.ParseFromString(first)
        response = await implementation.increment_rev(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "IncrementRev", increment_rev_handler)

    async def wait_rev_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = _github_com_s4wave_spacewave_sdk_world_world_pb2.WaitRevRequest()
        request.ParseFromString(first)
        response = await implementation.wait_rev(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WaitRev", wait_rev_handler)


class TypedObjectResourceServiceServer(Protocol):
    async def access_typed_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessTypedObjectRequest,
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessTypedObjectResponse: ...
    def watch_typed_object(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypedObjectRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypedObjectResponse
    ]: ...


def register_typed_object_resource_service(
    registry: ServiceRegistry,
    implementation: TypedObjectResourceServiceServer,
    service: str = "s4wave.world.TypedObjectResourceService",
) -> None:
    async def access_typed_object_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.AccessTypedObjectRequest()
        )
        request.ParseFromString(first)
        response = await implementation.access_typed_object(request)
        await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "AccessTypedObject", access_typed_object_handler)

    async def watch_typed_object_handler(call: Call) -> None:
        first = await call.receive()
        if first is None:
            raise CallProtocolError("missing initial request")
        request = (
            _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchTypedObjectRequest()
        )
        request.ParseFromString(first)
        async for response in implementation.watch_typed_object(request):
            await call.send(response.SerializeToString(deterministic=True))

    registry.register(service, "WatchTypedObject", watch_typed_object_handler)
