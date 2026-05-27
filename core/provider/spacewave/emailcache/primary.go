package emailcache

import api "github.com/s4wave/spacewave/core/provider/spacewave/api"

// SetPrimaryEmail clones rows and marks exactly the matching email as primary.
func SetPrimaryEmail(rows []*api.AccountEmailInfo, email string) ([]*api.AccountEmailInfo, bool) {
	if email == "" || len(rows) == 0 {
		return rows, false
	}

	next := make([]*api.AccountEmailInfo, len(rows))
	var changed bool
	var found bool
	for i, row := range rows {
		if row == nil {
			continue
		}
		clone := row.CloneVT()
		primary := clone.GetEmail() == email
		if primary {
			found = true
		}
		if clone.GetPrimary() != primary {
			clone.Primary = primary
			changed = true
		}
		next[i] = clone
	}
	if !found || !changed {
		return rows, false
	}
	return next, true
}
