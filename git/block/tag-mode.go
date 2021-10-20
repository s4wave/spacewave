package git_block

import "github.com/go-git/go-git/v5"

// ToGitTagMode converts the tag mode to a git tag mode.
func (t TagMode) ToGitTagMode() git.TagMode {
	switch t {
	case TagMode_TagMode_NONE:
		return git.NoTags
	case TagMode_TagMode_FOLLOWING:
		return git.TagFollowing
	default:
		return git.AllTags
	}
}
