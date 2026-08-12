from core.sobject import sobject_pb2 as _sobject_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class TransferMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    TransferMode_UNKNOWN: _ClassVar[TransferMode]
    TransferMode_MERGE: _ClassVar[TransferMode]
    TransferMode_MIGRATE: _ClassVar[TransferMode]
    TransferMode_MIRROR: _ClassVar[TransferMode]

class TransferPhase(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    TransferPhase_IDLE: _ClassVar[TransferPhase]
    TransferPhase_SCANNING: _ClassVar[TransferPhase]
    TransferPhase_COPYING_BLOCKS: _ClassVar[TransferPhase]
    TransferPhase_COPYING_SO: _ClassVar[TransferPhase]
    TransferPhase_CLEANUP: _ClassVar[TransferPhase]
    TransferPhase_COMPLETE: _ClassVar[TransferPhase]
    TransferPhase_FAILED: _ClassVar[TransferPhase]
TransferMode_UNKNOWN: TransferMode
TransferMode_MERGE: TransferMode
TransferMode_MIGRATE: TransferMode
TransferMode_MIRROR: TransferMode
TransferPhase_IDLE: TransferPhase
TransferPhase_SCANNING: TransferPhase
TransferPhase_COPYING_BLOCKS: TransferPhase
TransferPhase_COPYING_SO: TransferPhase
TransferPhase_CLEANUP: TransferPhase
TransferPhase_COMPLETE: TransferPhase
TransferPhase_FAILED: TransferPhase

class SpaceTransferState(_message.Message):
    __slots__ = ("shared_object_id", "phase", "blocks_copied", "blocks_total", "error_message", "meta")
    SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    PHASE_FIELD_NUMBER: _ClassVar[int]
    BLOCKS_COPIED_FIELD_NUMBER: _ClassVar[int]
    BLOCKS_TOTAL_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    META_FIELD_NUMBER: _ClassVar[int]
    shared_object_id: str
    phase: TransferPhase
    blocks_copied: int
    blocks_total: int
    error_message: str
    meta: _sobject_pb2.SharedObjectMeta
    def __init__(self, shared_object_id: _Optional[str] = ..., phase: _Optional[_Union[TransferPhase, str]] = ..., blocks_copied: _Optional[int] = ..., blocks_total: _Optional[int] = ..., error_message: _Optional[str] = ..., meta: _Optional[_Union[_sobject_pb2.SharedObjectMeta, _Mapping]] = ...) -> None: ...

class TransferState(_message.Message):
    __slots__ = ("mode", "phase", "source_session_index", "target_session_index", "spaces", "error_message")
    MODE_FIELD_NUMBER: _ClassVar[int]
    PHASE_FIELD_NUMBER: _ClassVar[int]
    SOURCE_SESSION_INDEX_FIELD_NUMBER: _ClassVar[int]
    TARGET_SESSION_INDEX_FIELD_NUMBER: _ClassVar[int]
    SPACES_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    mode: TransferMode
    phase: TransferPhase
    source_session_index: int
    target_session_index: int
    spaces: _containers.RepeatedCompositeFieldContainer[SpaceTransferState]
    error_message: str
    def __init__(self, mode: _Optional[_Union[TransferMode, str]] = ..., phase: _Optional[_Union[TransferPhase, str]] = ..., source_session_index: _Optional[int] = ..., target_session_index: _Optional[int] = ..., spaces: _Optional[_Iterable[_Union[SpaceTransferState, _Mapping]]] = ..., error_message: _Optional[str] = ...) -> None: ...

class TransferCheckpoint(_message.Message):
    __slots__ = ("state", "space_ids", "current_space_index")
    STATE_FIELD_NUMBER: _ClassVar[int]
    SPACE_IDS_FIELD_NUMBER: _ClassVar[int]
    CURRENT_SPACE_INDEX_FIELD_NUMBER: _ClassVar[int]
    state: TransferState
    space_ids: _containers.RepeatedScalarFieldContainer[str]
    current_space_index: int
    def __init__(self, state: _Optional[_Union[TransferState, _Mapping]] = ..., space_ids: _Optional[_Iterable[str]] = ..., current_space_index: _Optional[int] = ...) -> None: ...
