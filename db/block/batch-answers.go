package block

import (
	"context"
	"sync"
	"time"
)

// batchAnswersContextKey is the context key of a batchAnswersLink.
type batchAnswersContextKey struct{}

// BatchAnswers holds the answers of a batch read until the reads that follow
// take them, as BatchLoad describes. A walk that reads the blocks over several
// calls, such as an iterator reading a page's children one step at a time,
// holds it and attaches it to each call's context with Context; dropping it
// drops the answers no read took.
type BatchAnswers struct {
	// mtx guards answers.
	mtx sync.Mutex
	// answers holds each block's answer by its ref's string until a read
	// takes it.
	answers map[string]*batchAnswer
}

// batchAnswersLink is one BatchAnswers on a context, above those of the
// context it extended.
type batchAnswersLink struct {
	// answers is the attached BatchAnswers.
	answers *BatchAnswers
	// parent is the link of the extended context, or nil.
	parent *batchAnswersLink
}

// Context returns ctx with a attached, so the reads on it take a's answers
// before those of batches attached earlier. A nil a returns ctx.
func (a *BatchAnswers) Context(ctx context.Context) context.Context {
	if a == nil {
		return ctx
	}
	return context.WithValue(ctx, batchAnswersContextKey{}, &batchAnswersLink{answers: a, parent: batchAnswersFromContext(ctx)})
}

// batchAnswer is the answer of a batch read for one block.
type batchAnswer struct {
	// stored is the block, or nil when the store does not hold it.
	stored *StoredBlock
	// err is the block's read error.
	err error
	// mark records where the bytes came from, for the read that takes them.
	mark *blockReadMark
	// elapsed is the block's share of the batch read's time.
	elapsed time.Duration
}

// batchAnswersFromContext returns the newest batch answers link on ctx, or nil.
func batchAnswersFromContext(ctx context.Context) *batchAnswersLink {
	link, _ := ctx.Value(batchAnswersContextKey{}).(*batchAnswersLink)
	return link
}

// takeBatchAnswer returns and removes the answer for ref from the batch
// answers on ctx, or nil when none holds one.
func takeBatchAnswer(ctx context.Context, ref *BlockRef) *batchAnswer {
	// Skip the key's encoding when no batch answers are on ctx.
	link := batchAnswersFromContext(ctx)
	if link == nil {
		return nil
	}

	// Take the answer from the newest batch that holds one.
	key := ref.MarshalString()
	for ; link != nil; link = link.parent {
		answers := link.answers
		answers.mtx.Lock()
		a := answers.answers[key]
		delete(answers.answers, key)
		answers.mtx.Unlock()
		if a != nil {
			return a
		}
	}
	return nil
}
