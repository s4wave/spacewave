package world

// Decision is an Authority's answer to a Candidate.
type Decision int

const (
	// Undecided means the Authority has not answered yet, or the wait for its
	// answer ended first, so the outcome is unknown.
	Undecided Decision = iota
	// Accepted means the candidate is the next head.
	Accepted
	// Superseded means another candidate became the head after Base, so this
	// one never will. The writer rebases onto the head Admission.Head reports
	// and runs its change again.
	Superseded
	// Rejected means the Authority refused the candidate itself, such as an
	// operation it could not validate. Running it again does not help.
	Rejected
)

// String returns the decision's name.
func (d Decision) String() string {
	switch d {
	case Accepted:
		return "accepted"
	case Superseded:
		return "superseded"
	case Rejected:
		return "rejected"
	default:
		return "undecided"
	}
}
