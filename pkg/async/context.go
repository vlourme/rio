package async

import "context"

type executorsContextKey struct{}

func With(ctx context.Context, exec Executors) context.Context {
	return context.WithValue(ctx, executorsContextKey{}, exec)
}

func From(ctx context.Context) Executors {
	executor, ok := ctx.Value(executorsContextKey{}).(Executors)
	if ok && executor != nil {
		return executor
	}
	panic("rio: there is no executors in context")
	return nil
}
