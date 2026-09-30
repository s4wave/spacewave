package resource_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// typedObjectWatch owns one stream's demand, factory work and revocable child.
// Its owner routine reconciles events independently of stream backpressure.
type typedObjectWatch struct {
	// resource retains the exact granting World and factory dependencies.
	resource *TypedObjectResource
	// objectKey identifies the watched object within that World.
	objectKey string
	// engineID is the trusted registry scope of this watch.
	engineID string
	// client owns and can revoke the stream's adopted children.
	client resource_server.ResourceClientContext

	// bcast guards World observations, directive values and factory results.
	bcast broadcast.Broadcast
	// worldReady indicates that the first consistent World observation arrived.
	worldReady bool
	// typeID is the last observed type, empty for an absent or untyped object.
	typeID string
	// typeEpoch distinguishes intervening deletion or type changes.
	typeEpoch uint64
	// worldErr ends the watch when its granted World fails.
	worldErr error
	// lookup retains callback state for the current type's directive.
	lookup *typedObjectWatchLookup
	// generation owns the selected attached value's factory result.
	generation *typedObjectWatchGeneration
	// response is the latest immutable snapshot published by the owner.
	response *s4wave_world.WatchTypedObjectResponse
	// done indicates that demand and current children have been released.
	done bool
	// err is the terminal owner error published with done.
	err error
}

// typedObjectWatchLookup publishes only values from one directive instance.
// Callback fields are guarded by the watch broadcast; references belong to its owner.
type typedObjectWatchLookup struct {
	// typeID identifies the demanded handler type.
	typeID string
	// epoch retains the object's type lifecycle at admission.
	epoch uint64
	// values tracks attached value identity without reconstructing registry state.
	values map[uint32]objecttype.ObjectType
	// idle allows absence to be reported after initial resolution.
	idle bool
	// err records disposal or resolver failure.
	err error
	// ref retains standing demand until replacement or cancellation.
	ref directive.Reference
	// releaseIdle removes the idle listener before demand is released.
	releaseIdle func()
}

// typedObjectWatchGeneration owns one selected value's cancellable factory result.
type typedObjectWatchGeneration struct {
	// valueID identifies the actual attached handler generation.
	valueID uint32
	// objectType supplies the admitted factory.
	objectType objecttype.ObjectType
	// ctx is the factory and child lifetime, ended by retirement.
	ctx context.Context
	// cancel withdraws factory work when its generation is superseded.
	cancel context.CancelFunc
	// ready records completion under the watch broadcast.
	ready bool
	// invoker is transferred to the child after successful factory completion.
	invoker srpc.Invoker
	// cleanup is transferred to the child or run on superseded completion.
	cleanup func()
	// err records factory failure under the watch broadcast.
	err error
	// childID belongs exclusively to the owner routine.
	childID uint32
}

// newTypedObjectWatch constructs a watch within the granting mount's authority.
func newTypedObjectWatch(r *TypedObjectResource, key, engineID string, client resource_server.ResourceClientContext) *typedObjectWatch {
	return &typedObjectWatch{resource: r, objectKey: key, engineID: engineID, client: client}
}

// execute publishes owner snapshots and waits for teardown before returning.
func (w *typedObjectWatch) execute(ctx context.Context, cancel context.CancelFunc, stream s4wave_world.SRPCTypedObjectResourceService_WatchTypedObjectStream) error {
	// Start one event reconciler independently of the potentially blocked sender.
	owner := routine.NewRoutineContainer(routine.WithExitCb(func(err error) {
		w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if !w.done {
				w.done, w.err = true, err
				broadcast()
			}
		})
	}))
	owner.SetRoutine(w.run)
	owner.SetContext(ctx, false)
	defer func() {
		cancel()
		w.waitDone()
		owner.ClearContext()
	}()

	// Send immutable availability snapshots outside the lifecycle lock.
	var sent *s4wave_world.WatchTypedObjectResponse
	for {
		// Capture the current snapshot and its next notification together.
		var response *s4wave_world.WatchTypedObjectResponse
		var done bool
		var err error
		var wait <-chan struct{}
		w.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			response, done, err = w.response, w.done, w.err
			wait = getWait()
		})

		// Stop after the owner has revoked children and withdrawn demand.
		if done {
			return err
		}
		if response != nil && response != sent {
			if err := stream.Send(response); err != nil {
				return err
			}
			sent = response
		}

		// Wait for state publication or cancellation without polling.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wait:
		}
	}
}

// waitDone joins the owner teardown without waiting for an uncooperative factory.
func (w *typedObjectWatch) waitDone() {
	for {
		// Capture teardown completion with its wakeup position.
		var done bool
		var wait <-chan struct{}
		w.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			done, wait = w.done, getWait()
		})

		// Return only after current children and demand have been released.
		if done {
			return
		}
		<-wait
	}
}

// run reconciles World and directive events while factories run outside callbacks.
func (w *typedObjectWatch) run(ctx context.Context) (retErr error) {
	// Observe the supplied World's events and serialize factory generations.
	worldObserver := routine.NewRoutineContainer()
	worldObserver.SetRoutine(func(ctx context.Context) error {
		err := w.observeWorld(ctx)
		w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			w.worldErr = err
			broadcast()
		})
		return nil
	})
	worldObserver.SetContext(ctx, false)
	factories := routine.NewStateRoutineContainer[*typedObjectWatchGeneration](func(a, b *typedObjectWatchGeneration) bool { return a == b })
	factories.SetStateRoutine(w.build)
	factories.SetContext(ctx, false)
	defer func() {
		// Withdraw observation, factory work and demand before publishing teardown.
		worldObserver.ClearContext()
		w.retire(factories)
		w.releaseLookup()
		factories.ClearContext()
		w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			w.done, w.err = true, retErr
			broadcast()
		})
	}()

	// Reconcile only after an owning component publishes a change.
	for {
		// Capture World state, selected attached identity and factory completion.
		var typeID string
		var epoch uint64
		var ready, idle bool
		var valueID uint32
		var objectType objecttype.ObjectType
		var generation *typedObjectWatchGeneration
		var err error
		var wait <-chan struct{}
		w.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			typeID, epoch, ready, err = w.typeID, w.typeEpoch, w.worldReady, w.worldErr
			generation = w.generation
			if lookup := w.lookup; lookup != nil {
				idle = lookup.idle
				if err == nil {
					err = lookup.err
				}
				for id, value := range lookup.values {
					if valueID == 0 || id < valueID {
						valueID, objectType = id, value
					}
				}
			}
			wait = getWait()
		})

		// Return terminal World or registry errors without replaying a method.
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		// Replace demand when the object's type or existence changed.
		if ready && (w.lookup == nil && typeID != "" || w.lookup != nil && (w.lookup.typeID != typeID || w.lookup.epoch != epoch)) {
			w.retire(factories)
			w.releaseLookup()
			if typeID != "" {
				if err := w.addLookup(typeID, epoch); err != nil {
					return err
				}
			}
			w.publish(0, typeID)
			continue
		}

		// Revoke the previous adopted child before admitting a new factory.
		if generation != nil && generation.valueID != valueID {
			w.retire(factories)
			w.publish(0, typeID)
			generation = nil
		}
		if generation == nil && valueID != 0 {
			factoryCtx, cancel := context.WithCancel(ctx)
			generation = &typedObjectWatchGeneration{valueID: valueID, objectType: objectType, ctx: factoryCtx, cancel: cancel}
			w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { w.generation = generation })
			factories.SetState(generation)
		}

		// Transfer a completed current factory into a revocable invocation child.
		if generation != nil && generation.childID == 0 {
			if err := w.admit(generation, typeID); err != nil {
				return err
			}
		} else if ready && (typeID == "" || valueID == 0 && idle) {
			w.publish(0, typeID)
		}

		// Wait on the exact revision captured with the reconciler state.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wait:
		}
	}
}

// observeWorld follows revisions within the granted mutable state or pinned snapshot.
func (w *typedObjectWatch) observeWorld(ctx context.Context) error {
	// Capture the World sequence before reading existence and graph metadata.
	seqno, err := w.resource.ws.GetSeqno(ctx)
	if err != nil {
		return err
	}

	// Publish observations only within one unchanged World sequence.
	for {
		// Read the object and type without retaining an ObjectState handle.
		found, err := w.resource.ws.HasObject(ctx, w.objectKey)
		if err != nil {
			return err
		}
		var typeID string
		if found {
			typeID, err = world_types.GetObjectType(ctx, w.resource.ws, w.objectKey)
			if err != nil {
				return err
			}
		}

		// Consume a concurrent World revision before publishing mixed metadata.
		observed, err := w.resource.ws.GetSeqno(ctx)
		if err != nil {
			return err
		}
		if observed == seqno {
			w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if !w.worldReady || w.typeID != typeID {
					w.typeEpoch++
					w.typeID, w.worldReady = typeID, true
					broadcast()
				}
			})
		}

		// Wait for the next owning World event, including pending writer changes.
		seqno, err = w.resource.ws.WaitSeqno(ctx, seqno+1)
		if err != nil {
			return err
		}
	}
}

// addLookup attaches callbacks and retains the registry's standing scoped demand.
func (w *typedObjectWatch) addLookup(typeID string, epoch uint64) error {
	// Publish the callback token before AddDirective can report existing values.
	lookup := &typedObjectWatchLookup{typeID: typeID, epoch: epoch, values: make(map[uint32]objecttype.ObjectType)}
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { w.lookup = lookup })
	handler := directive.NewTypedCallbackHandler[objecttype.ObjectType](
		func(value directive.TypedAttachedValue[objecttype.ObjectType]) {
			w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if w.lookup == lookup {
					lookup.values[value.GetValueID()] = value.GetValue()
					broadcast()
				}
			})
		},
		func(value directive.TypedAttachedValue[objecttype.ObjectType]) {
			w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if w.lookup == lookup {
					delete(lookup.values, value.GetValueID())
					broadcast()
				}
			})
		},
		func() {
			w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if w.lookup == lookup {
					lookup.err = context.Canceled
					broadcast()
				}
			})
		}, nil,
	)

	// Keep the reference after idle so a later registration can satisfy demand.
	instance, ref, err := w.resource.b.AddDirective(objecttype.NewLookupObjectTypeForEngine(typeID, w.engineID), handler)
	if err != nil {
		return err
	}
	lookup.ref = ref
	lookup.releaseIdle = instance.AddIdleCallback(func(idle bool, errs []error) {
		w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if w.lookup == lookup {
				lookup.idle = idle
				if idle && len(errs) != 0 {
					lookup.err = errs[0]
				}
				broadcast()
			}
		})
	})
	return nil
}

// releaseLookup detaches callbacks before releasing this stream's demand.
func (w *typedObjectWatch) releaseLookup() {
	// Remove the token before any queued callback can update the next lookup.
	var lookup *typedObjectWatchLookup
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		lookup, w.lookup = w.lookup, nil
	})

	// Release only this watch's callback and strong directive reference.
	if lookup != nil {
		if lookup.releaseIdle != nil {
			lookup.releaseIdle()
		}
		if lookup.ref != nil {
			lookup.ref.Release()
		}
	}
}

// build constructs a factory outside lifecycle locks and discards late results.
func (w *typedObjectWatch) build(_ context.Context, generation *typedObjectWatchGeneration) error {
	// Invoke only this generation's factory against the exact granted state.
	invoker, cleanup, err := generation.objectType.GetFactory()(generation.ctx, w.resource.le, w.resource.b, w.resource.engine, w.resource.ws, w.objectKey)
	accepted := false
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if w.generation == generation && generation.ctx.Err() == nil {
			generation.invoker, generation.cleanup, generation.err, generation.ready = invoker, cleanup, err, true
			accepted = true
			broadcast()
		}
	})

	// Superseded or canceled completion retains no resource or factory cleanup.
	if !accepted && cleanup != nil {
		cleanup()
	}
	return nil
}

// admit transfers a completed current factory to its child's release callback.
func (w *typedObjectWatch) admit(generation *typedObjectWatchGeneration, typeID string) error {
	// Take the completed result only while its attached value remains selected.
	var invoker srpc.Invoker
	var cleanup func()
	var ready bool
	var err error
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if w.lookup != nil && w.lookup.values[generation.valueID] != nil && w.lookup.typeID == w.typeID && w.lookup.epoch == w.typeEpoch {
			ready, err = generation.ready, generation.err
			if ready && err == nil {
				invoker, cleanup = generation.invoker, generation.cleanup
				generation.cleanup = nil
			}
		}
	})

	// Preserve factory failures and pending state without fabricating availability.
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}

	// Register the child before publishing its immutable availability snapshot.
	id, err := w.client.AddResource(invoker, func() {
		generation.cancel()
		if cleanup != nil {
			cleanup()
		}
	})
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return err
	}
	generation.childID = id
	current := false
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = generation.ctx.Err() == nil && w.lookup != nil && w.lookup.values[generation.valueID] != nil && w.lookup.typeID == w.typeID && w.lookup.epoch == w.typeEpoch
	})
	if current {
		w.publish(id, typeID)
	} else {
		w.client.ReleaseResource(id)
	}
	return nil
}

// retire revokes an adopted child before relinquishing its factory generation.
func (w *typedObjectWatch) retire(factories *routine.StateRoutineContainer[*typedObjectWatchGeneration]) {
	// Detach the generation so a late factory result can only clean itself up.
	var generation *typedObjectWatchGeneration
	var cleanup func()
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		generation, w.generation = w.generation, nil
		if generation != nil {
			cleanup, generation.cleanup = generation.cleanup, nil
		}
	})

	// Cancel factory work and synchronously revoke its current invocation child.
	if generation != nil {
		generation.cancel()
		if generation.childID != 0 {
			w.client.ReleaseResource(generation.childID)
		}
		if cleanup != nil {
			cleanup()
		}
	}
	factories.SetState(nil)
}

// publish replaces the immutable snapshot only when availability changes.
func (w *typedObjectWatch) publish(resourceID uint32, typeID string) {
	w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if w.response == nil || w.response.GetResourceId() != resourceID || w.response.GetTypeId() != typeID {
			w.response = &s4wave_world.WatchTypedObjectResponse{ResourceId: resourceID, TypeId: typeID}
			broadcast()
		}
	})
}
