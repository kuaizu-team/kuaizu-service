package requestctx

import "context"

type contextKey string

const openIDKey contextKey = "openID"

// WithOpenID stores the current user's WeChat openid in the request context.
func WithOpenID(ctx context.Context, openID string) context.Context {
	if openID == "" {
		return ctx
	}
	return context.WithValue(ctx, openIDKey, openID)
}

// OpenIDFromContext extracts the current user's WeChat openid from context.
func OpenIDFromContext(ctx context.Context) string {
	openID, _ := ctx.Value(openIDKey).(string)
	return openID
}
