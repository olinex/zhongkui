package auth

import (
	"context"
	"time"
)

type AccessKey interface {
	UserID() string
	IsEnabled() bool
	Secret() string
	ExpiredAt() *time.Time
}

type AccessKeyPool interface {
	GetKey(ctx context.Context, id string) (key AccessKey, err error)
	IsDuplicated(ctx context.Context, id string, timestamp time.Time, nonce string) (bool, error)
	UpdateLastAccess(ctx context.Context, id string) error
}
