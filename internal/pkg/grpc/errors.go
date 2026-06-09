package grpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrInternal          = status.Errorf(codes.Internal, "internal error")
	ErrMissingMetadata   = status.Errorf(codes.InvalidArgument, "missing metadata")
	ErrBadMetadata       = status.Errorf(codes.Unauthenticated, "bad metadata")
	ErrInvalidAuthType   = status.Errorf(codes.Unauthenticated, "invalid auth type")
	ErrInvalidToken      = status.Errorf(codes.Unauthenticated, "invalid token")
	ErrNonceDuplicated   = status.Errorf(codes.Unauthenticated, "nonce duplicated")
	ErrAccessKeyDisabled = status.Errorf(codes.PermissionDenied, "access key is disabled")
	ErrAccessKeyExpired  = status.Errorf(codes.PermissionDenied, "access key is expired")
)
