package transport

import "context"

type ctxKey struct{}

func WithReplier(ctx context.Context, r Replier) context.Context {
	return context.WithValue(ctx, ctxKey{}, r)
}

func ReplierFromContext(ctx context.Context) (Replier, bool) {
	r, ok := ctx.Value(ctxKey{}).(Replier)
	return r, ok
}
