"""Resource lifecycle failures shared by Resource clients and servers."""

from __future__ import annotations


class ResourceError(Exception):
    """Base class for Resource lifecycle failures."""


class ResourceReleasedError(ResourceError):
    """The selected Resource reference has been released or revoked."""


class ResourceTerminalError(ResourceError):
    """The immutable Resource generation has reached a terminal state."""


class ResourceProtocolError(ResourceTerminalError):
    """The peer sent an invalid Resource lifecycle message."""


class ResourceUnsupportedError(ResourceError):
    """The selected Resource protocol operation is not supported."""


__all__ = [
    "ResourceError",
    "ResourceProtocolError",
    "ResourceReleasedError",
    "ResourceTerminalError",
    "ResourceUnsupportedError",
]
