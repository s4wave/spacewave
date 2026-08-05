//go:build !js

package goscriptbench

import (
	"os"
	"path/filepath"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

const (
	engineCapabilitySchemaVersion = 1
	engineCapabilityFile          = "capability.json"
	engineCapabilityOPFS          = "opfs"
	engineCapabilityUnsupported   = "unsupported"
)

// EngineCapability records why an engine cannot enter the benchmark workload.
type EngineCapability struct {
	SchemaVersion int
	RunID         string
	Engine        string
	EngineVersion string
	Capability    string
	Status        string
	Reason        string
}

// Validate checks that an unsupported capability record is complete.
func (c EngineCapability) Validate() error {
	if c.SchemaVersion != engineCapabilitySchemaVersion {
		return errors.Errorf("engine capability schema version %d is unsupported", c.SchemaVersion)
	}
	if !validArtifactID(c.RunID) || !validArtifactID(c.Engine) {
		return errors.New("engine capability identity is invalid")
	}
	if c.EngineVersion == "" {
		return errors.New("engine capability browser version is required")
	}
	if c.Capability != engineCapabilityOPFS || c.Status != engineCapabilityUnsupported {
		return errors.New("engine capability must record unsupported OPFS")
	}
	if c.Reason == "" {
		return errors.New("engine capability reason is required")
	}
	return nil
}

// PublishEngineCapability atomically exposes one unsupported engine record.
func PublishEngineCapability(outputRoot string, capability EngineCapability) (string, error) {
	if outputRoot == "" {
		return "", errors.New("capability output root is required")
	}
	if err := capability.Validate(); err != nil {
		return "", err
	}
	root, err := filepath.Abs(outputRoot)
	if err != nil {
		return "", errors.Wrap(err, "resolve capability output root")
	}
	runDir := filepath.Join(root, capability.RunID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", errors.Wrap(err, "create capability run directory")
	}
	finalDir := filepath.Join(runDir, capability.Engine)
	if _, err := os.Lstat(finalDir); err == nil {
		return "", errors.Errorf("capability destination already exists: %s", finalDir)
	} else if !os.IsNotExist(err) {
		return "", errors.Wrap(err, "inspect capability destination")
	}
	tempDir, err := os.MkdirTemp(runDir, "."+capability.Engine+"-")
	if err != nil {
		return "", errors.Wrap(err, "create temporary capability directory")
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	if err := os.WriteFile(filepath.Join(tempDir, engineCapabilityFile), marshalEngineCapability(capability), 0o644); err != nil {
		return "", errors.Wrap(err, "write engine capability")
	}
	if err := os.Rename(tempDir, finalDir); err != nil {
		return "", errors.Wrap(err, "publish engine capability directory")
	}
	return finalDir, nil
}

// ReadEngineCapability validates one published unsupported engine record.
func ReadEngineCapability(dir string) (EngineCapability, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return EngineCapability{}, errors.Wrap(err, "read engine capability directory")
	}
	if len(entries) != 1 || entries[0].IsDir() || entries[0].Name() != engineCapabilityFile {
		return EngineCapability{}, errors.New("engine capability directory has unexpected contents")
	}
	data, err := os.ReadFile(filepath.Join(dir, engineCapabilityFile))
	if err != nil {
		return EngineCapability{}, errors.Wrap(err, "read engine capability")
	}
	capability, err := parseEngineCapability(data)
	if err != nil {
		return EngineCapability{}, err
	}
	if err := capability.Validate(); err != nil {
		return EngineCapability{}, err
	}
	return capability, nil
}

func marshalEngineCapability(capability EngineCapability) []byte {
	var arena fastjson.Arena
	value := arena.NewObject()
	value.Set("schemaVersion", arena.NewNumberInt(capability.SchemaVersion))
	value.Set("runId", arena.NewString(capability.RunID))
	value.Set("engine", arena.NewString(capability.Engine))
	value.Set("engineVersion", arena.NewString(capability.EngineVersion))
	value.Set("capability", arena.NewString(capability.Capability))
	value.Set("status", arena.NewString(capability.Status))
	value.Set("reason", arena.NewString(capability.Reason))
	return append(value.MarshalTo(nil), '\n')
}

func parseEngineCapability(data []byte) (EngineCapability, error) {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return EngineCapability{}, errors.Wrap(err, "parse engine capability JSON")
	}
	if value.Type() != fastjson.TypeObject {
		return EngineCapability{}, errors.New("engine capability JSON root must be an object")
	}
	capability := EngineCapability{}
	if capability.SchemaVersion, err = parseInt(value, "schemaVersion"); err != nil {
		return EngineCapability{}, err
	}
	fields := []struct {
		name   string
		target *string
	}{
		{name: "runId", target: &capability.RunID},
		{name: "engine", target: &capability.Engine},
		{name: "engineVersion", target: &capability.EngineVersion},
		{name: "capability", target: &capability.Capability},
		{name: "status", target: &capability.Status},
		{name: "reason", target: &capability.Reason},
	}
	for _, field := range fields {
		if *field.target, err = parseString(value, field.name); err != nil {
			return EngineCapability{}, err
		}
	}
	return capability, nil
}
