package fsm

import "context"

func (handler Handler[T]) apply(ctx context.Context, fsm *FSM[T], params ...any) error {
	for _, stage := range []Actor[T]{handler.BeforeHandler, handler.Handler, handler.AfterHandler} {
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
