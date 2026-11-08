package world

type GraphQuad interface{ Pred() string }

type quad struct{ s, p, o string }

func (q *quad) Pred() string { return q.p }

func NewGraphQuad(subj, pred, obj, value string) GraphQuad { return &quad{subj, pred, obj} }

func NewGraphQuadWithKeys(subj, pred, obj, value string) GraphQuad { return &quad{subj, pred, obj} }
