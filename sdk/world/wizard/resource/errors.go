package wizard_resource

import "github.com/pkg/errors"

var errGitCloneProgressComplete = errors.New("git clone progress complete")

// ErrWizardGone reports a wizard that was finished or deleted elsewhere, so
// the work it would publish has nothing left to replace.
var ErrWizardGone = errors.New("this wizard no longer exists")

// ErrNoWorldEngine refuses a write on a wizard opened without a World engine,
// such as one an operation handler reaches through the operation's World
// state. Such a wizard still serves its state and clone progress.
var ErrNoWorldEngine = errors.New("this wizard was opened without a World engine and cannot write")

// GitRepoKeyTakenError refuses a clone publish whose repository key another
// object already holds.
type GitRepoKeyTakenError struct {
	// Key is the repository object key that is taken.
	Key string
}

// Error reports the taken key.
func (e *GitRepoKeyTakenError) Error() string {
	return "a repository already exists at " + e.Key
}
