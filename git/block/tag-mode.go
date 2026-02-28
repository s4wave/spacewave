package git_block

import "github.com/go-git/go-git/v6/plumbing"

// ToGitTagMode converts the tag mode to a git tag mode.
func (t TagMode) ToGitTagMode() plumbing.TagMode {
	switch t {
	case TagMode_TagMode_NONE:
		return plumbing.NoTags
	case TagMode_TagMode_FOLLOWING:
		return plumbing.TagFollowing
	default:
		return plumbing.AllTags
	}
}
