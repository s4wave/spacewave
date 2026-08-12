from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class CommandFocusContext(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    COMMAND_FOCUS_CONTEXT_UNSPECIFIED: _ClassVar[CommandFocusContext]
    COMMAND_FOCUS_CONTEXT_GLOBAL: _ClassVar[CommandFocusContext]
    COMMAND_FOCUS_CONTEXT_SHELL_TAB: _ClassVar[CommandFocusContext]
    COMMAND_FOCUS_CONTEXT_EDITOR: _ClassVar[CommandFocusContext]
    COMMAND_FOCUS_CONTEXT_LIST: _ClassVar[CommandFocusContext]
    COMMAND_FOCUS_CONTEXT_CANVAS: _ClassVar[CommandFocusContext]
    COMMAND_FOCUS_CONTEXT_MODAL: _ClassVar[CommandFocusContext]
    COMMAND_FOCUS_CONTEXT_TEXT_INPUT: _ClassVar[CommandFocusContext]

class CommandSurface(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    COMMAND_SURFACE_UNKNOWN: _ClassVar[CommandSurface]
    COMMAND_SURFACE_WEB: _ClassVar[CommandSurface]
    COMMAND_SURFACE_TUI: _ClassVar[CommandSurface]

class KeybindingDisplayMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    KEYBINDING_DISPLAY_MODE_UNSPECIFIED: _ClassVar[KeybindingDisplayMode]
    KEYBINDING_DISPLAY_MODE_SYMBOLS: _ClassVar[KeybindingDisplayMode]
    KEYBINDING_DISPLAY_MODE_TEXT: _ClassVar[KeybindingDisplayMode]
COMMAND_FOCUS_CONTEXT_UNSPECIFIED: CommandFocusContext
COMMAND_FOCUS_CONTEXT_GLOBAL: CommandFocusContext
COMMAND_FOCUS_CONTEXT_SHELL_TAB: CommandFocusContext
COMMAND_FOCUS_CONTEXT_EDITOR: CommandFocusContext
COMMAND_FOCUS_CONTEXT_LIST: CommandFocusContext
COMMAND_FOCUS_CONTEXT_CANVAS: CommandFocusContext
COMMAND_FOCUS_CONTEXT_MODAL: CommandFocusContext
COMMAND_FOCUS_CONTEXT_TEXT_INPUT: CommandFocusContext
COMMAND_SURFACE_UNKNOWN: CommandSurface
COMMAND_SURFACE_WEB: CommandSurface
COMMAND_SURFACE_TUI: CommandSurface
KEYBINDING_DISPLAY_MODE_UNSPECIFIED: KeybindingDisplayMode
KEYBINDING_DISPLAY_MODE_SYMBOLS: KeybindingDisplayMode
KEYBINDING_DISPLAY_MODE_TEXT: KeybindingDisplayMode

class KeyCombo(_message.Message):
    __slots__ = ("combo",)
    COMBO_FIELD_NUMBER: _ClassVar[int]
    combo: str
    def __init__(self, combo: _Optional[str] = ...) -> None: ...

class KeySequence(_message.Message):
    __slots__ = ("steps",)
    STEPS_FIELD_NUMBER: _ClassVar[int]
    steps: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, steps: _Optional[_Iterable[str]] = ...) -> None: ...

class CommandBinding(_message.Message):
    __slots__ = ("id", "combo", "sequence", "when", "source_label", "surface")
    ID_FIELD_NUMBER: _ClassVar[int]
    COMBO_FIELD_NUMBER: _ClassVar[int]
    SEQUENCE_FIELD_NUMBER: _ClassVar[int]
    WHEN_FIELD_NUMBER: _ClassVar[int]
    SOURCE_LABEL_FIELD_NUMBER: _ClassVar[int]
    SURFACE_FIELD_NUMBER: _ClassVar[int]
    id: str
    combo: KeyCombo
    sequence: KeySequence
    when: CommandFocusContext
    source_label: str
    surface: CommandSurface
    def __init__(self, id: _Optional[str] = ..., combo: _Optional[_Union[KeyCombo, _Mapping]] = ..., sequence: _Optional[_Union[KeySequence, _Mapping]] = ..., when: _Optional[_Union[CommandFocusContext, str]] = ..., source_label: _Optional[str] = ..., surface: _Optional[_Union[CommandSurface, str]] = ...) -> None: ...

class KeybindingDisplaySettings(_message.Message):
    __slots__ = ("mode",)
    MODE_FIELD_NUMBER: _ClassVar[int]
    mode: KeybindingDisplayMode
    def __init__(self, mode: _Optional[_Union[KeybindingDisplayMode, str]] = ...) -> None: ...

class KeybindingOverrideSettings(_message.Message):
    __slots__ = ("leader_combo", "which_key_delay_ms", "display")
    LEADER_COMBO_FIELD_NUMBER: _ClassVar[int]
    WHICH_KEY_DELAY_MS_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_FIELD_NUMBER: _ClassVar[int]
    leader_combo: str
    which_key_delay_ms: int
    display: KeybindingDisplaySettings
    def __init__(self, leader_combo: _Optional[str] = ..., which_key_delay_ms: _Optional[int] = ..., display: _Optional[_Union[KeybindingDisplaySettings, _Mapping]] = ...) -> None: ...

class KeybindingCommandOverride(_message.Message):
    __slots__ = ("command_id", "replace_bindings", "disabled", "cleared_binding_ids", "bindings")
    COMMAND_ID_FIELD_NUMBER: _ClassVar[int]
    REPLACE_BINDINGS_FIELD_NUMBER: _ClassVar[int]
    DISABLED_FIELD_NUMBER: _ClassVar[int]
    CLEARED_BINDING_IDS_FIELD_NUMBER: _ClassVar[int]
    BINDINGS_FIELD_NUMBER: _ClassVar[int]
    command_id: str
    replace_bindings: bool
    disabled: bool
    cleared_binding_ids: _containers.RepeatedScalarFieldContainer[str]
    bindings: _containers.RepeatedCompositeFieldContainer[CommandBinding]
    def __init__(self, command_id: _Optional[str] = ..., replace_bindings: _Optional[bool] = ..., disabled: _Optional[bool] = ..., cleared_binding_ids: _Optional[_Iterable[str]] = ..., bindings: _Optional[_Iterable[_Union[CommandBinding, _Mapping]]] = ...) -> None: ...

class KeybindingOverrideSet(_message.Message):
    __slots__ = ("web_overrides", "tui_overrides", "web_settings", "tui_settings")
    WEB_OVERRIDES_FIELD_NUMBER: _ClassVar[int]
    TUI_OVERRIDES_FIELD_NUMBER: _ClassVar[int]
    WEB_SETTINGS_FIELD_NUMBER: _ClassVar[int]
    TUI_SETTINGS_FIELD_NUMBER: _ClassVar[int]
    web_overrides: _containers.RepeatedCompositeFieldContainer[KeybindingCommandOverride]
    tui_overrides: _containers.RepeatedCompositeFieldContainer[KeybindingCommandOverride]
    web_settings: KeybindingOverrideSettings
    tui_settings: KeybindingOverrideSettings
    def __init__(self, web_overrides: _Optional[_Iterable[_Union[KeybindingCommandOverride, _Mapping]]] = ..., tui_overrides: _Optional[_Iterable[_Union[KeybindingCommandOverride, _Mapping]]] = ..., web_settings: _Optional[_Union[KeybindingOverrideSettings, _Mapping]] = ..., tui_settings: _Optional[_Union[KeybindingOverrideSettings, _Mapping]] = ...) -> None: ...

class Command(_message.Message):
    __slots__ = ("command_id", "label", "menu_path", "menu_group", "menu_order", "icon", "description", "has_sub_items", "default_bindings", "search_aliases")
    COMMAND_ID_FIELD_NUMBER: _ClassVar[int]
    LABEL_FIELD_NUMBER: _ClassVar[int]
    MENU_PATH_FIELD_NUMBER: _ClassVar[int]
    MENU_GROUP_FIELD_NUMBER: _ClassVar[int]
    MENU_ORDER_FIELD_NUMBER: _ClassVar[int]
    ICON_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    HAS_SUB_ITEMS_FIELD_NUMBER: _ClassVar[int]
    DEFAULT_BINDINGS_FIELD_NUMBER: _ClassVar[int]
    SEARCH_ALIASES_FIELD_NUMBER: _ClassVar[int]
    command_id: str
    label: str
    menu_path: str
    menu_group: int
    menu_order: int
    icon: str
    description: str
    has_sub_items: bool
    default_bindings: _containers.RepeatedCompositeFieldContainer[CommandBinding]
    search_aliases: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, command_id: _Optional[str] = ..., label: _Optional[str] = ..., menu_path: _Optional[str] = ..., menu_group: _Optional[int] = ..., menu_order: _Optional[int] = ..., icon: _Optional[str] = ..., description: _Optional[str] = ..., has_sub_items: _Optional[bool] = ..., default_bindings: _Optional[_Iterable[_Union[CommandBinding, _Mapping]]] = ..., search_aliases: _Optional[_Iterable[str]] = ...) -> None: ...
