import datetime

from core.sobject import sobject_pb2 as _sobject_pb2
from net.peer import peer_pb2 as _peer_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Secret(_message.Message):
    __slots__ = ("display_name", "kind", "nested_shared_object_id", "ref", "created_at", "updated_at")
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    KIND_FIELD_NUMBER: _ClassVar[int]
    NESTED_SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    REF_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    display_name: str
    kind: str
    nested_shared_object_id: str
    ref: _sobject_pb2.SharedObjectRef
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    def __init__(self, display_name: _Optional[str] = ..., kind: _Optional[str] = ..., nested_shared_object_id: _Optional[str] = ..., ref: _Optional[_Union[_sobject_pb2.SharedObjectRef, _Mapping]] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class SecretPayload(_message.Message):
    __slots__ = ("value", "content_type", "version", "updated_at")
    VALUE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    value: bytes
    content_type: str
    version: int
    updated_at: _timestamp_pb2.Timestamp
    def __init__(self, value: _Optional[bytes] = ..., content_type: _Optional[str] = ..., version: _Optional[int] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class SecretGrantStatus(_message.Message):
    __slots__ = ("peer_id", "participant", "readable", "role", "grant_count")
    PEER_ID_FIELD_NUMBER: _ClassVar[int]
    PARTICIPANT_FIELD_NUMBER: _ClassVar[int]
    READABLE_FIELD_NUMBER: _ClassVar[int]
    ROLE_FIELD_NUMBER: _ClassVar[int]
    GRANT_COUNT_FIELD_NUMBER: _ClassVar[int]
    peer_id: str
    participant: bool
    readable: bool
    role: _sobject_pb2.SOParticipantRole
    grant_count: int
    def __init__(self, peer_id: _Optional[str] = ..., participant: _Optional[bool] = ..., readable: _Optional[bool] = ..., role: _Optional[_Union[_sobject_pb2.SOParticipantRole, str]] = ..., grant_count: _Optional[int] = ...) -> None: ...

class SecretState(_message.Message):
    __slots__ = ("secret", "grant_status", "health")
    SECRET_FIELD_NUMBER: _ClassVar[int]
    GRANT_STATUS_FIELD_NUMBER: _ClassVar[int]
    HEALTH_FIELD_NUMBER: _ClassVar[int]
    secret: Secret
    grant_status: SecretGrantStatus
    health: _sobject_pb2.SharedObjectHealth
    def __init__(self, secret: _Optional[_Union[Secret, _Mapping]] = ..., grant_status: _Optional[_Union[SecretGrantStatus, _Mapping]] = ..., health: _Optional[_Union[_sobject_pb2.SharedObjectHealth, _Mapping]] = ...) -> None: ...

class WatchStateRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class WatchStateResponse(_message.Message):
    __slots__ = ("state",)
    STATE_FIELD_NUMBER: _ClassVar[int]
    state: SecretState
    def __init__(self, state: _Optional[_Union[SecretState, _Mapping]] = ...) -> None: ...

class BeginReadPayloadRequest(_message.Message):
    __slots__ = ("reader_peer_id", "expected_kind")
    READER_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_KIND_FIELD_NUMBER: _ClassVar[int]
    reader_peer_id: str
    expected_kind: str
    def __init__(self, reader_peer_id: _Optional[str] = ..., expected_kind: _Optional[str] = ...) -> None: ...

class BeginReadPayloadResponse(_message.Message):
    __slots__ = ("challenge_id", "challenge", "expires_at", "secret")
    CHALLENGE_ID_FIELD_NUMBER: _ClassVar[int]
    CHALLENGE_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    SECRET_FIELD_NUMBER: _ClassVar[int]
    challenge_id: str
    challenge: bytes
    expires_at: _timestamp_pb2.Timestamp
    secret: Secret
    def __init__(self, challenge_id: _Optional[str] = ..., challenge: _Optional[bytes] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., secret: _Optional[_Union[Secret, _Mapping]] = ...) -> None: ...

class ReadPayloadChallenge(_message.Message):
    __slots__ = ("challenge_id", "reader_peer_id", "object_key", "secret_kind", "expected_kind", "nested_shared_object_id", "nonce", "expires_at")
    CHALLENGE_ID_FIELD_NUMBER: _ClassVar[int]
    READER_PEER_ID_FIELD_NUMBER: _ClassVar[int]
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    SECRET_KIND_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_KIND_FIELD_NUMBER: _ClassVar[int]
    NESTED_SHARED_OBJECT_ID_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    challenge_id: str
    reader_peer_id: str
    object_key: str
    secret_kind: str
    expected_kind: str
    nested_shared_object_id: str
    nonce: bytes
    expires_at: _timestamp_pb2.Timestamp
    def __init__(self, challenge_id: _Optional[str] = ..., reader_peer_id: _Optional[str] = ..., object_key: _Optional[str] = ..., secret_kind: _Optional[str] = ..., expected_kind: _Optional[str] = ..., nested_shared_object_id: _Optional[str] = ..., nonce: _Optional[bytes] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ReadPayloadRequest(_message.Message):
    __slots__ = ("challenge_id", "signature")
    CHALLENGE_ID_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    challenge_id: str
    signature: _peer_pb2.Signature
    def __init__(self, challenge_id: _Optional[str] = ..., signature: _Optional[_Union[_peer_pb2.Signature, _Mapping]] = ...) -> None: ...

class ReadPayloadResponse(_message.Message):
    __slots__ = ("payload",)
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    payload: SecretPayload
    def __init__(self, payload: _Optional[_Union[SecretPayload, _Mapping]] = ...) -> None: ...
