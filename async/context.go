package async

import (
	"context"
	"errors"
)

type executorsContextKey struct{}

func With(ctx context.Context, exec Executors) context.Context {
	return context.WithValue(ctx, executorsContextKey{}, exec)
}

func From(ctx context.Context) Executors {
	exec, ok := ctx.Value(executorsContextKey{}).(Executors)
	if ok && exec != nil {
		return exec
	}
	panic("async: there is no executors in context")
	return nil
}

func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

func IsTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

func TryExecute(ctx context.Context, task func()) bool {
	if task == nil {
		return false
	}
	exec := From(ctx)
	return exec.TryExecute(ctx, task)
}

func Execute(ctx context.Context, task func()) (err error) {
	if task == nil {
		return
	}
	exec := From(ctx)
	err = exec.Execute(ctx, task)
	return
}
