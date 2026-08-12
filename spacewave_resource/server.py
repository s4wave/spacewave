"""Server-side ownership for immutable Resource generations."""

from __future__ import annotations

import asyncio
import contextlib
import inspect
from collections.abc import AsyncGenerator, AsyncIterator, Awaitable, Callable
from dataclasses import dataclass, field
from typing import TypeAlias, cast

from rpcstream import rpcstream_pb2
from starpc.rpcstream import ComponentRegistry, handle_rpc_stream
from starpc.server import Handler, Server, ServiceRegistry

from bldr.resource import resource_pb2
from bldr.resource.resource_srpc import register_resource_service

from .errors import (
    ResourceProtocolError,
    ResourceReleasedError,
    ResourceUnsupportedError,
)

ReleaseCallback: TypeAlias = Callable[[], Awaitable[None] | None]
ResourceFactory: TypeAlias = Callable[[ServiceRegistry, "ResourceCall"], None]

_END = object()


class ResourceCall:
    """Authority and exact invocation provenance for one ResourceRpc route."""

    def __init__(
        self,
        server: ResourceServer,
        generation: _Generation,
        parent: int,
        route: asyncio.Task[object],
    ) -> None:
        self._server = server
        self._generation = generation
        self._parent = parent
        self._route = route
        self._service = ""
        self._method = ""

    @property
    def generation(self) -> int:
        return self._generation.id

    @property
    def parent(self) -> int:
        return self._parent

    @property
    def service(self) -> str:
        return self._service

    @property
    def method(self) -> str:
        return self._method

    async def construct_child_resource(
        self,
        factory: ResourceFactory,
        on_release: ReleaseCallback | None = None,
    ) -> _ConstructedResource:
        """Create a pending child owned by this invocation's selected resource."""
        if not self._service or not self._method:
            raise ResourceProtocolError("child construction requires an active handler")
        resource_id = await self._server._construct_child(
            self._generation,
            self._parent,
            self._service,
            self._method,
            factory,
            on_release,
        )
        return _ConstructedResource(self._server, self._generation, resource_id)

    def _bind(self, service: str, method: str) -> None:
        if self._service or self._method:
            raise ResourceProtocolError(
                "ResourceCall handler provenance is already bound"
            )
        self._service = service
        self._method = method


class _ConstructedResource:
    def __init__(
        self, server: ResourceServer, generation: _Generation, resource_id: int
    ) -> None:
        self._server = server
        self._generation = generation
        self.id = resource_id

    async def release_resource(self) -> None:
        """Release this Resource and notify its generation.

        Other active routes settle before the callback and notification. If
        the current handler belongs to this Resource, this raises
        ResourceReleasedError after that release barrier so it cannot continue.
        """
        await self._server._release_resource(
            self._generation, self.id, notify_target=True, keep_root=True
        )


class _RouteRegistry(ServiceRegistry):
    def __init__(self, resource_call: ResourceCall) -> None:
        super().__init__()
        self._resource_call = resource_call

    def register(self, service: str, method: str, handler: Handler) -> None:
        async def bound(call: object) -> None:
            self._resource_call._bind(service, method)
            task = asyncio.current_task()
            assert task is not None
            self._resource_call._server._bind_handler_route(
                task, self._resource_call._route
            )
            try:
                await handler(call)  # type: ignore[arg-type]
            finally:
                self._resource_call._server._unbind_handler_route(task)

        super().register(service, method, bound)


@dataclass(eq=False)
class _Resource:
    id: int
    factory: ResourceFactory
    parent: int = 0
    service: str = ""
    method: str = ""
    pending: bool = False
    on_release: ReleaseCallback | None = None
    routes: set[asyncio.Task[object]] = field(default_factory=set)


@dataclass(eq=False)
class _Generation:
    id: int
    root: int
    responses: asyncio.Queue[resource_pb2.ResourceClientResponse | Exception | object]
    resources: dict[int, _Resource] = field(default_factory=dict)
    children: dict[int, set[int]] = field(default_factory=dict)
    tombstones: set[int] = field(default_factory=set)
    last_control_id: int = 0
    closing: bool = False
    lifecycle: asyncio.Lock = field(default_factory=asyncio.Lock)


class ResourceServer:
    """Serve one root Resource factory across immutable client generations."""

    def __init__(self, root_factory: ResourceFactory) -> None:
        self._root_factory = root_factory
        self._lock = asyncio.Lock()
        self._next_id = 0
        self._generations: dict[int, _Generation] = {}
        self._handler_routes: dict[asyncio.Task[object], asyncio.Task[object]] = {}
        self._closed = False

    def register(self, registry: ServiceRegistry) -> None:
        """Register this generated Resource service in a StarPC registry."""
        register_resource_service(registry, self)

    async def resource_client(
        self,
        requests: AsyncIterator[resource_pb2.ResourceClientRequest],
    ) -> AsyncGenerator[resource_pb2.ResourceClientResponse, None]:
        try:
            first = await anext(requests)
        except StopAsyncIteration as exc:
            raise ResourceProtocolError("expected ResourceClient init packet") from exc
        if first.WhichOneof("body") != "init" or first.control_id != 0:
            raise ResourceProtocolError("expected ResourceClient init packet")

        async with self._lock:
            if self._closed:
                raise ResourceReleasedError("Resource server is closed")
            generation_id = self._allocate_id_locked()
            root_id = self._allocate_id_locked()
            responses: asyncio.Queue[
                resource_pb2.ResourceClientResponse | Exception | object
            ] = asyncio.Queue()
            generation = _Generation(generation_id, root_id, responses)
            generation.resources[root_id] = _Resource(root_id, self._root_factory)
            self._generations[generation_id] = generation

        yield resource_pb2.ResourceClientResponse(
            init=resource_pb2.ResourceClientInit(
                client_handle_id=generation_id, root_resource_id=root_id
            )
        )
        controls = asyncio.create_task(self._consume_controls(generation, requests))
        try:
            while True:
                response = await responses.get()
                if response is _END:
                    return
                if isinstance(response, Exception):
                    raise response
                assert isinstance(response, resource_pb2.ResourceClientResponse)
                yield response
        finally:
            if not controls.done():
                controls.cancel()
            with contextlib.suppress(asyncio.CancelledError, Exception):
                await controls
            await self._release_generation(generation)

    async def _consume_controls(
        self,
        generation: _Generation,
        requests: AsyncIterator[resource_pb2.ResourceClientRequest],
    ) -> None:
        try:
            async for request in requests:
                await self._apply_control(generation, request)
        except asyncio.CancelledError:
            raise
        except Exception as exc:  # noqa: BLE001
            await generation.responses.put(exc)
        else:
            await generation.responses.put(_END)

    async def _apply_control(
        self, generation: _Generation, request: resource_pb2.ResourceClientRequest
    ) -> None:
        control_id = request.control_id
        if control_id == 0 or control_id != generation.last_control_id + 1:
            raise ResourceProtocolError(
                f"unexpected ResourceClient control ID {control_id} "
                f"after {generation.last_control_id}"
            )
        body = request.WhichOneof("body")
        if body == "adopt":
            await self._adopt(generation, request.adopt.resource_id)
        elif body == "release":
            await self._release_resource(
                generation,
                request.release.resource_id,
                notify_target=False,
                keep_root=True,
            )
        else:
            raise ResourceProtocolError("unexpected ResourceClient init/control packet")
        generation.last_control_id = control_id
        await generation.responses.put(
            resource_pb2.ResourceClientResponse(
                control_ack=resource_pb2.ResourceClientControlAck(control_id=control_id)
            )
        )

    async def _adopt(self, generation: _Generation, resource_id: int) -> None:
        async with generation.lifecycle:
            async with self._lock:
                if generation.closing:
                    raise ResourceReleasedError("Resource generation is released")
                resource = generation.resources.get(resource_id)
                if resource is not None:
                    resource.pending = False
                    return
                if resource_id not in generation.tombstones:
                    raise ResourceReleasedError("Resource was not found")
            await generation.responses.put(
                resource_pb2.ResourceClientResponse(
                    resource_released=resource_pb2.ResourceReleasedResponse(
                        resource_id=resource_id
                    )
                )
            )

    async def resource_rpc(
        self,
        requests: AsyncIterator[rpcstream_pb2.RpcStreamPacket],
    ) -> AsyncGenerator[rpcstream_pb2.RpcStreamPacket, None]:
        try:
            first = await anext(requests)
        except StopAsyncIteration as exc:
            raise ResourceProtocolError(
                "expected nested stream initialization"
            ) from exc
        if first.WhichOneof("body") != "init":
            raise ResourceProtocolError("expected nested stream initialization")
        component = first.init.component_id
        if not component.isascii() or not component.isdecimal():
            raise ResourceProtocolError("invalid Resource component ID")
        resource_id = int(component)
        if resource_id == 0 or resource_id > 0xFFFFFFFF:
            raise ResourceProtocolError("invalid Resource component ID")

        task = asyncio.current_task()
        assert task is not None
        async with self._lock:
            generation, resource = self._find_resource_locked(resource_id)
            if generation is None or resource is None:
                # Let the Resource-owned ComponentRegistry produce the standard
                # nested unknown-component acknowledgement.
                generation = None
            else:
                resource.routes.add(task)

        async def restored() -> AsyncIterator[rpcstream_pb2.RpcStreamPacket]:
            yield first
            async for request in requests:
                yield request

        try:
            components = ComponentRegistry()
            if generation is not None and resource is not None:
                resource_call = ResourceCall(self, generation, resource_id, task)
                registry = _RouteRegistry(resource_call)
                resource.factory(registry, resource_call)
                await components.register(component, Server(registry))
            async for response in handle_rpc_stream(restored(), components):
                yield response
        finally:
            if generation is not None and resource is not None:
                async with self._lock:
                    resource.routes.discard(task)

    async def resource_attach(
        self,
        requests: AsyncIterator[resource_pb2.ResourceAttachRequest],
    ) -> AsyncGenerator[resource_pb2.ResourceAttachResponse, None]:
        del requests
        raise ResourceUnsupportedError("ResourceAttach is unsupported")
        if False:
            yield resource_pb2.ResourceAttachResponse()

    async def _construct_child(
        self,
        generation: _Generation,
        parent: int,
        service: str,
        method: str,
        factory: ResourceFactory,
        on_release: ReleaseCallback | None,
    ) -> int:
        async with self._lock:
            if generation.closing or parent not in generation.resources:
                raise ResourceReleasedError("Resource or generation is released")
            resource_id = self._allocate_id_locked()
            generation.resources[resource_id] = _Resource(
                resource_id,
                factory,
                parent,
                service,
                method,
                True,
                on_release,
            )
            generation.children.setdefault(parent, set()).add(resource_id)
            return resource_id

    async def _release_resource(
        self,
        generation: _Generation,
        resource_id: int,
        *,
        notify_target: bool,
        keep_root: bool,
    ) -> bool:
        task = asyncio.current_task()
        assert task is not None
        current = cast(asyncio.Task[object], task)
        caller = self._handler_routes.get(current, current)
        operation = asyncio.create_task(
            self._release_resource_owned(
                generation,
                resource_id,
                notify_target,
                keep_root,
                caller,
            )
        )
        try:
            released, caller_removed = await asyncio.shield(operation)
        except asyncio.CancelledError:
            await operation
            raise
        if caller_removed:
            raise ResourceReleasedError("Resource was released")
        return released

    async def _release_resource_owned(
        self,
        generation: _Generation,
        resource_id: int,
        notify_target: bool,
        keep_root: bool,
        caller: asyncio.Task[object] | None,
    ) -> tuple[bool, bool]:
        async with generation.lifecycle:
            async with self._lock:
                if generation.closing:
                    return False, False
                target = generation.resources.get(resource_id)
                if target is None:
                    if resource_id in generation.tombstones:
                        return False, False
                    raise ResourceReleasedError("Resource was not found")
                removed = self._remove_locked(
                    generation,
                    resource_id,
                    keep_root and resource_id == generation.root,
                )
                caller_removed = caller is not None and any(
                    caller in resource.routes for resource in removed
                )

            await self._settle_removed(removed, caller)
            for resource in removed:
                if notify_target or resource.id != resource_id:
                    await generation.responses.put(
                        resource_pb2.ResourceClientResponse(
                            resource_released=resource_pb2.ResourceReleasedResponse(
                                resource_id=resource.id
                            )
                        )
                    )
            return True, caller_removed

    def _remove_locked(
        self, generation: _Generation, resource_id: int, retain_target: bool
    ) -> list[_Resource]:
        removed: list[_Resource] = []

        def remove_pending(parent: int) -> None:
            for child_id in sorted(generation.children.get(parent, ())):
                child = generation.resources.get(child_id)
                if child is not None and child.pending:
                    remove_one(child_id)

        def remove_one(selected: int) -> None:
            resource = generation.resources.get(selected)
            if resource is None:
                return
            remove_pending(selected)
            generation.resources.pop(selected, None)
            siblings = generation.children.get(resource.parent)
            if siblings is not None:
                siblings.discard(selected)
                if not siblings:
                    generation.children.pop(resource.parent, None)
            generation.tombstones.add(selected)
            removed.append(resource)

        remove_pending(resource_id)
        if not retain_target:
            remove_one(resource_id)
        return removed

    async def _settle_removed(
        self,
        resources: list[_Resource],
        current: asyncio.Task[object] | None,
    ) -> None:
        for resource in resources:
            routes = tuple(route for route in resource.routes if route is not current)
            for route in routes:
                if not route.done():
                    route.cancel()
            for route in routes:
                with contextlib.suppress(asyncio.CancelledError, Exception):
                    await route
            resource.routes.clear()
            if resource.on_release is not None:
                try:
                    result = resource.on_release()
                    if inspect.isawaitable(result):
                        await result
                except Exception as exc:  # noqa: BLE001
                    asyncio.get_running_loop().call_exception_handler(
                        {
                            "message": "Resource release callback failed",
                            "exception": exc,
                            "resource_id": resource.id,
                        }
                    )

    async def _release_generation(self, generation: _Generation) -> None:
        async with generation.lifecycle:
            await self._release_generation_owned(generation)

    async def _release_generation_owned(self, generation: _Generation) -> None:
        async with self._lock:
            if generation.closing:
                return
            generation.closing = True
            self._generations.pop(generation.id, None)
            removed: list[_Resource] = []
            visited: set[int] = set()

            def remove_tree(resource_id: int) -> None:
                if resource_id in visited:
                    return
                visited.add(resource_id)
                for child_id in sorted(generation.children.get(resource_id, ())):
                    remove_tree(child_id)
                resource = generation.resources.pop(resource_id, None)
                if resource is not None:
                    generation.tombstones.add(resource_id)
                    removed.append(resource)

            remove_tree(generation.root)
            for resource_id in sorted(generation.resources):
                remove_tree(resource_id)
            generation.children.clear()

        await self._settle_removed(removed, asyncio.current_task())
        await generation.responses.put(_END)

    async def aclose(self) -> None:
        """Invalidate every generation and join all active ResourceRpc routes."""
        async with self._lock:
            self._closed = True
            generations = tuple(self._generations.values())
        for generation in generations:
            await self._release_generation(generation)

    def _bind_handler_route(
        self, handler: asyncio.Task[object], route: asyncio.Task[object]
    ) -> None:
        if handler in self._handler_routes:
            raise ResourceProtocolError("Resource handler route is already bound")
        self._handler_routes[handler] = route

    def _unbind_handler_route(self, handler: asyncio.Task[object]) -> None:
        if self._handler_routes.pop(handler, None) is None:
            raise ResourceProtocolError("Resource handler route is not bound")

    def _allocate_id_locked(self) -> int:
        self._next_id += 1
        if self._next_id > 0xFFFFFFFF:
            raise ResourceProtocolError("Resource ID space is exhausted")
        return self._next_id

    def _find_resource_locked(
        self, resource_id: int
    ) -> tuple[_Generation | None, _Resource | None]:
        for generation in self._generations.values():
            if generation.closing:
                continue
            resource = generation.resources.get(resource_id)
            if resource is not None:
                return generation, resource
        return None, None


__all__ = ["ResourceCall", "ResourceFactory", "ResourceServer"]
