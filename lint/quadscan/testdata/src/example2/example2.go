package example2

import "example"

func lookups() {
	_ = example.NewMemberQuad("", "") // want "fixes only the predicate"
	_ = example.NewMemberQuad("", "m")
}
