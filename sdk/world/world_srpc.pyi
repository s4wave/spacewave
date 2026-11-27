from collections.abc import AsyncIterable, AsyncIterator
from typing import Protocol

from sdk.world import (
    world_pb2 as _github_com_s4wave_spacewave_sdk_world_world_pb2,
)
from starpc.client import Client
from starpc.server import ServiceRegistry
from starpc.service import ServiceDescriptor

ENGINERESOURCESERVICE_SERVICE: ServiceDescriptor

class EngineResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
        requests: AsyncIterable[
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
) -> None: ...

WORLDSTAGERESOURCESERVICE_SERVICE: ServiceDescriptor

class WorldStageResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
) -> None: ...

WORLDSTATERESOURCESERVICE_SERVICE: ServiceDescriptor

class WorldStateResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
) -> None: ...

WATCHWORLDSTATERESOURCESERVICE_SERVICE: ServiceDescriptor

class WatchWorldStateResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
    def watch_world_state(
        self,
        request: _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateRequest,
    ) -> AsyncIterator[
        _github_com_s4wave_spacewave_sdk_world_world_pb2.WatchWorldStateResponse
    ]: ...

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
) -> None: ...

TXRESOURCESERVICE_SERVICE: ServiceDescriptor

class TxResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
    async def commit_tx(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.CommitResponse: ...
    async def discard(
        self, request: _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardRequest
    ) -> _github_com_s4wave_spacewave_sdk_world_world_pb2.DiscardResponse: ...

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
) -> None: ...

OBJECTITERATORRESOURCESERVICE_SERVICE: ServiceDescriptor

class ObjectIteratorResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
) -> None: ...

GRAPHPATHQUERYRESOURCESERVICE_SERVICE: ServiceDescriptor

class GraphPathQueryResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
) -> None: ...

OBJECTSTATERESOURCESERVICE_SERVICE: ServiceDescriptor

class ObjectStateResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
) -> None: ...

TYPEDOBJECTRESOURCESERVICE_SERVICE: ServiceDescriptor

class TypedObjectResourceServiceClient:
    def __init__(self, client: Client, service: str | None = None) -> None: ...
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
) -> None: ...
