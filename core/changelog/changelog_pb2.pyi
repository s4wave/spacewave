from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Changelog(_message.Message):
    __slots__ = ("releases",)
    RELEASES_FIELD_NUMBER: _ClassVar[int]
    releases: _containers.RepeatedCompositeFieldContainer[Release]
    def __init__(self, releases: _Optional[_Iterable[_Union[Release, _Mapping]]] = ...) -> None: ...

class Release(_message.Message):
    __slots__ = ("version", "date", "summary", "summary_markdown", "features", "fixes", "improvements", "security", "release_url")
    VERSION_FIELD_NUMBER: _ClassVar[int]
    DATE_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_MARKDOWN_FIELD_NUMBER: _ClassVar[int]
    FEATURES_FIELD_NUMBER: _ClassVar[int]
    FIXES_FIELD_NUMBER: _ClassVar[int]
    IMPROVEMENTS_FIELD_NUMBER: _ClassVar[int]
    SECURITY_FIELD_NUMBER: _ClassVar[int]
    RELEASE_URL_FIELD_NUMBER: _ClassVar[int]
    version: str
    date: str
    summary: str
    summary_markdown: str
    features: _containers.RepeatedCompositeFieldContainer[ChangeEntry]
    fixes: _containers.RepeatedCompositeFieldContainer[ChangeEntry]
    improvements: _containers.RepeatedCompositeFieldContainer[ChangeEntry]
    security: _containers.RepeatedCompositeFieldContainer[ChangeEntry]
    release_url: str
    def __init__(self, version: _Optional[str] = ..., date: _Optional[str] = ..., summary: _Optional[str] = ..., summary_markdown: _Optional[str] = ..., features: _Optional[_Iterable[_Union[ChangeEntry, _Mapping]]] = ..., fixes: _Optional[_Iterable[_Union[ChangeEntry, _Mapping]]] = ..., improvements: _Optional[_Iterable[_Union[ChangeEntry, _Mapping]]] = ..., security: _Optional[_Iterable[_Union[ChangeEntry, _Mapping]]] = ..., release_url: _Optional[str] = ...) -> None: ...

class ChangeEntry(_message.Message):
    __slots__ = ("description", "description_markdown")
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_MARKDOWN_FIELD_NUMBER: _ClassVar[int]
    description: str
    description_markdown: str
    def __init__(self, description: _Optional[str] = ..., description_markdown: _Optional[str] = ...) -> None: ...
