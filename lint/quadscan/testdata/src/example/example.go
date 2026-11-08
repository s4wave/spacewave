package example

import "github.com/s4wave/spacewave/db/world"

const memberPred = "<member>"

const none = ""

// NewMemberQuad links an index object to a member.
func NewMemberQuad(index, member string) world.GraphQuad { // want NewMemberQuad:"quadCtor"
	return world.NewGraphQuadWithKeys(index, memberPred, member, "")
}

func lookups(key string) {
	// Quads that fix only the predicate, then quads that fix more.
	_ = world.NewGraphQuad("", memberPred, "", "")      // want "fixes only the predicate"
	_ = world.NewGraphQuadWithKeys(none, "<p>", "", "") // want "fixes only the predicate"
	_ = NewMemberQuad("", "")                           // want "fixes only the predicate"
	_ = world.NewGraphQuad(key, memberPred, "", "")
	_ = world.NewGraphQuad("", memberPred, key, "")
	_ = NewMemberQuad("index", "")
	_ = world.NewGraphQuad("", "", "", "")
}
