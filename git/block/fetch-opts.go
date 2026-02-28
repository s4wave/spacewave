package git_block

import (
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
)

// IsEmpty checks if the fetch opts are empty.
func (f *FetchOpts) IsEmpty() bool {
	return f == nil || (f.GetRemoteName() == "" && f.GetRemoteUrl() == "" && len(f.GetRefSpecs()) == 0)
}

// Validate performs cursory checks of the config.
func (f *FetchOpts) Validate() error {
	return nil
}

// BuildFetchOpts constructs the go-git fetch configuration.
// Note: the auth and progress options are /not/ applied yet.
func (f *FetchOpts) BuildFetchOpts() *git.FetchOptions {
	tagMode := f.GetTagMode().ToGitTagMode()

	var refSpecs []config.RefSpec
	for _, rs := range f.GetRefSpecs() {
		refSpecs = append(refSpecs, config.RefSpec(rs))
	}

	return &git.FetchOptions{
		RemoteName: f.GetRemoteName(),
		RemoteURL:  f.GetRemoteUrl(),
		RefSpecs:   refSpecs,
		Depth:      int(f.GetDepth()),
		Tags:       tagMode,
		Force:      f.GetForce(),
		Prune:      f.GetPrune(),
	}
}
