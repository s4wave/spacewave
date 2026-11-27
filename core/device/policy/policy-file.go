package device_policy

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

const (
	// StateDir is the daemon state-root directory containing Device-local files.
	StateDir = "device"
	// StateFile is the generated-JSON policy file name under StateDir.
	StateFile = "policy.json"
)

// PermitsNodeType reports whether the policy allows nodes of typeID on this
// Device. The allow list holds a few hand-written entries, so a scan is cheap.
func (p *DevicePolicy) PermitsNodeType(typeID string) bool {
	return slices.Contains(p.GetNodeTypeId(), typeID)
}

// LaunchApprovalFor returns the approval this Device holds for spaceID, or an
// empty approval, which grants only the isolated launcher, when it holds none.
func (p *DevicePolicy) LaunchApprovalFor(spaceID string) *LaunchApproval {
	for _, approval := range p.GetLaunchApproval() {
		if approval.GetSpaceId() == spaceID {
			return approval.CloneVT()
		}
	}
	return &LaunchApproval{SpaceId: spaceID}
}

// PermitsSecretPlugin reports whether the policy lets pluginID read, create
// and write the Secrets of spaceID when that Space loads it.
func (p *DevicePolicy) PermitsSecretPlugin(spaceID, pluginID string) bool {
	for _, approval := range p.GetLaunchApproval() {
		if approval.GetSpaceId() == spaceID {
			return slices.Contains(approval.GetSecretPluginIds(), pluginID)
		}
	}
	return false
}

// ServesSpace reports whether the policy lists spaceID among the Spaces whose
// Flowgraph nodes this Device runs.
func (p *DevicePolicy) ServesSpace(spaceID string) bool {
	return slices.ContainsFunc(p.GetServedSpace(), func(served *ServedSpace) bool {
		return served.GetSpaceId() == spaceID
	})
}

// FilePath returns the Device policy file path under stateRoot.
func FilePath(stateRoot string) string {
	return filepath.Join(stateRoot, StateDir, StateFile)
}

// ReadFile reads the Device policy file under stateRoot, ignoring unknown JSON fields.
func ReadFile(stateRoot string) (*DevicePolicy, error) {
	// Read the policy file, treating a missing file as an empty policy.
	data, err := os.ReadFile(FilePath(stateRoot))
	if os.IsNotExist(err) {
		return &DevicePolicy{}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "read device policy")
	}

	// Parse and validate the persisted Device policy.
	policy := &DevicePolicy{}
	if err := policy.UnmarshalJSON(data); err != nil {
		return nil, errors.Wrap(err, "parse device policy")
	}
	if err := Validate(policy); err != nil {
		return nil, err
	}
	return policy, nil
}

// WriteFile writes the Device policy file under stateRoot.
func WriteFile(stateRoot string, policy *DevicePolicy) error {
	// Substitute an empty policy for nil and validate the result.
	if policy == nil {
		policy = &DevicePolicy{}
	}
	if err := Validate(policy); err != nil {
		return err
	}

	// Marshal the policy with a trailing newline.
	data, err := policy.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "marshal device policy")
	}
	data = append(data, '\n')

	// Write the policy file into its state directory.
	path := FilePath(stateRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.Wrap(err, "create device policy state directory")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return errors.Wrap(err, "write device policy")
	}
	return nil
}

// Validate checks the persisted Device policy shape.
func Validate(policy *DevicePolicy) error {
	// Reject node type IDs that are missing or duplicated.
	if value, ok := firstInvalid(policy.GetNodeTypeId()); ok {
		if value == "" {
			return errors.New("device policy node type id is required")
		}
		return errors.Errorf("duplicate device policy node type %q", value)
	}

	// Reject launch approvals without a Space or repeated for one Space.
	spaceIDs := make([]string, 0, len(policy.GetLaunchApproval()))
	for _, approval := range policy.GetLaunchApproval() {
		spaceIDs = append(spaceIDs, approval.GetSpaceId())
	}
	if value, ok := firstInvalid(spaceIDs); ok {
		if value == "" {
			return errors.New("device policy launch approval space id is required")
		}
		return errors.Errorf("duplicate device policy launch approval for space %q", value)
	}

	// Reject served Spaces without a Space or session, or repeated for one Space.
	spaceIDs = spaceIDs[:0]
	for _, served := range policy.GetServedSpace() {
		if served.GetSessionIndex() == 0 {
			return errors.Errorf("device policy served space %q has no session index", served.GetSpaceId())
		}
		spaceIDs = append(spaceIDs, served.GetSpaceId())
	}
	if value, ok := firstInvalid(spaceIDs); ok {
		if value == "" {
			return errors.New("device policy served space id is required")
		}
		return errors.Errorf("duplicate device policy served space %q", value)
	}

	// Reject an approval listing an empty or repeated child model or Secret
	// plugin.
	for _, approval := range policy.GetLaunchApproval() {
		if value, ok := firstInvalid(approval.GetAllowedChildModels()); ok {
			if value == "" {
				return errors.Errorf("launch approval for space %q has an empty child model", approval.GetSpaceId())
			}
			return errors.Errorf("launch approval for space %q repeats child model %q", approval.GetSpaceId(), value)
		}
		if value, ok := firstInvalid(approval.GetSecretPluginIds()); ok {
			if value == "" {
				return errors.Errorf("launch approval for space %q has an empty Secret plugin", approval.GetSpaceId())
			}
			return errors.Errorf("launch approval for space %q repeats Secret plugin %q", approval.GetSpaceId(), value)
		}
	}
	return nil
}

// firstInvalid returns the first blank or repeated value in values, with
// blank reported as "", and whether one was found.
func firstInvalid(values []string) (string, bool) {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return "", true
		}
		if _, ok := seen[value]; ok {
			return value, true
		}
		seen[value] = struct{}{}
	}
	return "", false
}
