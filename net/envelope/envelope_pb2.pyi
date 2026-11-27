from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Envelope(_message.Message):
    __slots__ = ("envelope_id", "context_hash", "threshold", "ciphertext", "grants", "keypairs", "contents")
    ENVELOPE_ID_FIELD_NUMBER: _ClassVar[int]
    CONTEXT_HASH_FIELD_NUMBER: _ClassVar[int]
    THRESHOLD_FIELD_NUMBER: _ClassVar[int]
    CIPHERTEXT_FIELD_NUMBER: _ClassVar[int]
    GRANTS_FIELD_NUMBER: _ClassVar[int]
    KEYPAIRS_FIELD_NUMBER: _ClassVar[int]
    CONTENTS_FIELD_NUMBER: _ClassVar[int]
    envelope_id: str
    context_hash: bytes
    threshold: int
    ciphertext: bytes
    grants: _containers.RepeatedCompositeFieldContainer[EnvelopeGrant]
    keypairs: _containers.RepeatedCompositeFieldContainer[EnvelopeKeypair]
    contents: bytes
    def __init__(self, envelope_id: _Optional[str] = ..., context_hash: _Optional[bytes] = ..., threshold: _Optional[int] = ..., ciphertext: _Optional[bytes] = ..., grants: _Optional[_Iterable[_Union[EnvelopeGrant, _Mapping]]] = ..., keypairs: _Optional[_Iterable[_Union[EnvelopeKeypair, _Mapping]]] = ..., contents: _Optional[bytes] = ...) -> None: ...

class EnvelopeGrant(_message.Message):
    __slots__ = ("keypair_indexes", "ciphertexts")
    KEYPAIR_INDEXES_FIELD_NUMBER: _ClassVar[int]
    CIPHERTEXTS_FIELD_NUMBER: _ClassVar[int]
    keypair_indexes: _containers.RepeatedScalarFieldContainer[int]
    ciphertexts: _containers.RepeatedScalarFieldContainer[bytes]
    def __init__(self, keypair_indexes: _Optional[_Iterable[int]] = ..., ciphertexts: _Optional[_Iterable[bytes]] = ...) -> None: ...

class EnvelopeGrantInner(_message.Message):
    __slots__ = ("shares",)
    SHARES_FIELD_NUMBER: _ClassVar[int]
    shares: _containers.RepeatedCompositeFieldContainer[EnvelopeShare]
    def __init__(self, shares: _Optional[_Iterable[_Union[EnvelopeShare, _Mapping]]] = ...) -> None: ...

class EnvelopeShare(_message.Message):
    __slots__ = ("id", "value")
    ID_FIELD_NUMBER: _ClassVar[int]
    VALUE_FIELD_NUMBER: _ClassVar[int]
    id: bytes
    value: bytes
    def __init__(self, id: _Optional[bytes] = ..., value: _Optional[bytes] = ...) -> None: ...

class EnvelopeKeypair(_message.Message):
    __slots__ = ("pub_key", "auth_method_id", "auth_method_params")
    PUB_KEY_FIELD_NUMBER: _ClassVar[int]
    AUTH_METHOD_ID_FIELD_NUMBER: _ClassVar[int]
    AUTH_METHOD_PARAMS_FIELD_NUMBER: _ClassVar[int]
    pub_key: bytes
    auth_method_id: str
    auth_method_params: bytes
    def __init__(self, pub_key: _Optional[bytes] = ..., auth_method_id: _Optional[str] = ..., auth_method_params: _Optional[bytes] = ...) -> None: ...

class EnvelopeConfig(_message.Message):
    __slots__ = ("envelope_id", "threshold", "total_shares", "grant_configs")
    ENVELOPE_ID_FIELD_NUMBER: _ClassVar[int]
    THRESHOLD_FIELD_NUMBER: _ClassVar[int]
    TOTAL_SHARES_FIELD_NUMBER: _ClassVar[int]
    GRANT_CONFIGS_FIELD_NUMBER: _ClassVar[int]
    envelope_id: str
    threshold: int
    total_shares: int
    grant_configs: _containers.RepeatedCompositeFieldContainer[EnvelopeGrantConfig]
    def __init__(self, envelope_id: _Optional[str] = ..., threshold: _Optional[int] = ..., total_shares: _Optional[int] = ..., grant_configs: _Optional[_Iterable[_Union[EnvelopeGrantConfig, _Mapping]]] = ...) -> None: ...

class EnvelopeGrantConfig(_message.Message):
    __slots__ = ("share_count", "keypair_indexes")
    SHARE_COUNT_FIELD_NUMBER: _ClassVar[int]
    KEYPAIR_INDEXES_FIELD_NUMBER: _ClassVar[int]
    share_count: int
    keypair_indexes: _containers.RepeatedScalarFieldContainer[int]
    def __init__(self, share_count: _Optional[int] = ..., keypair_indexes: _Optional[_Iterable[int]] = ...) -> None: ...

class EnvelopeUnlockResult(_message.Message):
    __slots__ = ("success", "shares_available", "shares_needed", "unlocked_grant_indexes")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    SHARES_AVAILABLE_FIELD_NUMBER: _ClassVar[int]
    SHARES_NEEDED_FIELD_NUMBER: _ClassVar[int]
    UNLOCKED_GRANT_INDEXES_FIELD_NUMBER: _ClassVar[int]
    success: bool
    shares_available: int
    shares_needed: int
    unlocked_grant_indexes: _containers.RepeatedScalarFieldContainer[int]
    def __init__(self, success: _Optional[bool] = ..., shares_available: _Optional[int] = ..., shares_needed: _Optional[int] = ..., unlocked_grant_indexes: _Optional[_Iterable[int]] = ...) -> None: ...
