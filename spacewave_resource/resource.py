"""The base wrapper that owns one Resource reference."""

from __future__ import annotations

from typing import Self

from .client import ResourceRef


class Resource:
    """Hold one Resource reference for a domain wrapper.

    The reference is the wrapper's whole lifecycle: release() and async-with
    exit delegate to it, and the wrapper keeps no resource map, client
    generation, transport, callback, or reconnect state of its own.
    """

    def __init__(self, resource_ref: ResourceRef) -> None:
        self._resource_ref = resource_ref

    @property
    def resource_ref(self) -> ResourceRef:
        """Return the reference this wrapper was constructed around."""
        return self._resource_ref

    async def release(self) -> None:
        """Release the held reference, awaiting its Release acknowledgement."""
        await self._resource_ref.release()

    async def __aenter__(self) -> Self:
        return self

    async def __aexit__(self, exc_type: object, exc: object, tb: object) -> None:
        await self.release()


__all__ = ["Resource"]
