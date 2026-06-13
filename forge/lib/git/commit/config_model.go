package forge_lib_git_commit

import (
	"github.com/aperturerobotics/protobuf-go-lite/json"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
)

// Config is the configuration for committing staged Git Worktree files.
type Config struct {
	// WorktreeObjectKey is the git/worktree object to commit from.
	WorktreeObjectKey string `json:"worktreeObjectKey,omitempty"`
	// RepoObjectKey is the linked git/repo object that owns the index.
	RepoObjectKey string `json:"repoObjectKey,omitempty"`
	// CommitRequest describes the staged paths and commit identity.
	CommitRequest *s4wave_git.CommitFilesRequest `json:"commitRequest,omitempty"`
}

// Reset resets the message.
func (c *Config) Reset() {
	*c = Config{}
}

// ProtoMessage marks Config as a protobuf-like message.
func (*Config) ProtoMessage() {}

// GetWorktreeObjectKey returns the Worktree object key.
func (c *Config) GetWorktreeObjectKey() string {
	if c != nil {
		return c.WorktreeObjectKey
	}
	return ""
}

// GetRepoObjectKey returns the Repo object key.
func (c *Config) GetRepoObjectKey() string {
	if c != nil {
		return c.RepoObjectKey
	}
	return ""
}

// GetCommitRequest returns the commit request.
func (c *Config) GetCommitRequest() *s4wave_git.CommitFilesRequest {
	if c != nil {
		return c.CommitRequest
	}
	return nil
}

// EqualVT checks if two configs are equal.
func (c *Config) EqualVT(other *Config) bool {
	if c == nil || other == nil {
		return c == other
	}
	if c.GetWorktreeObjectKey() != other.GetWorktreeObjectKey() {
		return false
	}
	if c.GetRepoObjectKey() != other.GetRepoObjectKey() {
		return false
	}
	req := c.GetCommitRequest()
	oreq := other.GetCommitRequest()
	if req == nil || oreq == nil {
		return req == oreq
	}
	return req.EqualVT(oreq)
}

// MarshalVT marshals the config.
func (c *Config) MarshalVT() ([]byte, error) {
	return c.MarshalJSON()
}

// UnmarshalVT unmarshals the config.
func (c *Config) UnmarshalVT(data []byte) error {
	return c.UnmarshalJSON(data)
}

// SizeVT returns the marshaled size.
func (c *Config) SizeVT() int {
	data, err := c.MarshalVT()
	if err != nil {
		return 0
	}
	return len(data)
}

// MarshalToSizedBufferVT marshals into a sized buffer.
func (c *Config) MarshalToSizedBufferVT(dst []byte) (int, error) {
	data, err := c.MarshalVT()
	if err != nil {
		return 0, err
	}
	return copy(dst, data), nil
}

// MarshalJSON marshals the config to JSON.
func (c *Config) MarshalJSON() ([]byte, error) {
	return json.DefaultMarshalerConfig.Marshal(c)
}

// UnmarshalJSON unmarshals the config from JSON.
func (c *Config) UnmarshalJSON(data []byte) error {
	return json.DefaultUnmarshalerConfig.Unmarshal(data, c)
}

// MarshalProtoJSON marshals the Config message to JSON.
func (c *Config) MarshalProtoJSON(s *json.MarshalState) {
	if c == nil {
		s.WriteNil()
		return
	}
	s.WriteObjectStart()
	var wroteField bool
	if c.WorktreeObjectKey != "" || s.HasField("worktreeObjectKey") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("worktreeObjectKey")
		s.WriteString(c.WorktreeObjectKey)
	}
	if c.RepoObjectKey != "" || s.HasField("repoObjectKey") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("repoObjectKey")
		s.WriteString(c.RepoObjectKey)
	}
	if c.CommitRequest != nil || s.HasField("commitRequest") {
		s.WriteMoreIf(&wroteField)
		s.WriteObjectField("commitRequest")
		c.CommitRequest.MarshalProtoJSON(s.WithField("commitRequest"))
	}
	s.WriteObjectEnd()
}

// UnmarshalProtoJSON unmarshals the Config message from JSON.
func (c *Config) UnmarshalProtoJSON(s *json.UnmarshalState) {
	if s.ReadNil() {
		return
	}
	s.ReadObject(func(key string) {
		switch key {
		default:
			s.Skip()
		case "worktree_object_key", "worktreeObjectKey":
			s.AddField("worktree_object_key")
			c.WorktreeObjectKey = s.ReadString()
		case "repo_object_key", "repoObjectKey":
			s.AddField("repo_object_key")
			c.RepoObjectKey = s.ReadString()
		case "commit_request", "commitRequest":
			s.AddField("commit_request")
			if s.ReadNil() {
				c.CommitRequest = nil
				return
			}
			c.CommitRequest = &s4wave_git.CommitFilesRequest{}
			c.CommitRequest.UnmarshalProtoJSON(s.WithField("commit_request", true))
		}
	})
}

var (
	_ json.Marshaler   = ((*Config)(nil))
	_ json.Unmarshaler = ((*Config)(nil))
)
