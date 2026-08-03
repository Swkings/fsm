package fsm

import "context"

// Actor is one handler stage. Params are the values passed to ProcessEvent.
type Actor[T IPayload] func(*FSM[T], ...any) error

// Handler defines the fixed before, action, and after stages of a transition.
type Handler[T IPayload] struct {
	BeforeHandler Actor[T]
	Handler       Actor[T]
	AfterHandler  Actor[T]
}

func (handler Handler[T]) apply(
	ctx context.Context,
	fsm *FSM[T],
	params ...any,
) error {
	for _, stage := range []Actor[T]{
		handler.BeforeHandler,
		handler.Handler,
		handler.AfterHandler,
	} {
		if err := ctx.Err(); err != nil {
			return err
		}
		if stage == nil {
			continue
		}
		if err := stage(fsm, params...); err != nil {
			return err
		}
	}

	return ctx.Err()
}
