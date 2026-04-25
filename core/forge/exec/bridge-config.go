package space_exec

import (
	"bytes"

	"github.com/aperturerobotics/controllerbus/config"
)

// SpaceExecConfig is a generic config type for space-exec handlers.
// It wraps raw config data bytes and a config ID, satisfying the
// config.Config, protobuf_go_lite.Message, and block.Block interfaces.
//
// The actual config deserialization happens inside each HandlerFactory,
// so this type just carries the raw bytes through the controllerbus
// config resolution pipeline (LoadConfigConstructorByID -> Resolve ->
// LoadFactoryByConfig -> Construct).
type SpaceExecConfig struct {
	// configID is the config ID for this handler.
	configID string
	// data is the raw config bytes (proto or JSON).
	data []byte
}

// NewSpaceExecConfig constructs a SpaceExecConfig with the given config ID.
func NewSpaceExecConfig(configID string) *SpaceExecConfig {
	return &SpaceExecConfig{configID: configID}
}

// GetConfigID returns the config ID.
func (c *SpaceExecConfig) GetConfigID() string {
	return c.configID
}

// Validate returns nil; validation happens in the handler factory.
func (c *SpaceExecConfig) Validate() error {
	return nil
}

// EqualsConfig checks equality by config ID and data.
func (c *SpaceExecConfig) EqualsConfig(other config.Config) bool {
	oc, ok := other.(*SpaceExecConfig)
	if !ok {
		return false
	}
	return c.configID == oc.configID && bytes.Equal(c.data, oc.data)
}

// MarshalJSON returns the raw data (assumed JSON or empty).
func (c *SpaceExecConfig) MarshalJSON() ([]byte, error) {
	if len(c.data) == 0 {
		return []byte("{}"), nil
	}
	return c.data, nil
}

// UnmarshalJSON stores the raw JSON bytes.
func (c *SpaceExecConfig) UnmarshalJSON(data []byte) error {
	c.data = append(c.data[:0], data...)
	return nil
}

// MarshalVT returns the raw data.
func (c *SpaceExecConfig) MarshalVT() ([]byte, error) {
	return c.data, nil
}

// MarshalToSizedBufferVT marshals to a pre-allocated buffer.
func (c *SpaceExecConfig) MarshalToSizedBufferVT(buf []byte) (int, error) {
	n := copy(buf, c.data)
	return len(buf) - n, nil
}

// UnmarshalVT stores the raw protobuf bytes.
func (c *SpaceExecConfig) UnmarshalVT(data []byte) error {
	c.data = append(c.data[:0], data...)
	return nil
}

// SizeVT returns the size of the data.
func (c *SpaceExecConfig) SizeVT() int {
	return len(c.data)
}

// Reset clears the data.
func (c *SpaceExecConfig) Reset() {
	c.data = c.data[:0]
}

// MarshalBlock returns the raw data for block storage.
func (c *SpaceExecConfig) MarshalBlock() ([]byte, error) {
	return c.data, nil
}

// UnmarshalBlock stores the raw block data.
func (c *SpaceExecConfig) UnmarshalBlock(data []byte) error {
	c.data = append(c.data[:0], data...)
	return nil
}

// _ is a type assertion
var _ config.Config = (*SpaceExecConfig)(nil)
