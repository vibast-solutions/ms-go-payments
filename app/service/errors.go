package service

import "errors"

var (
	ErrInvalidRequest       = errors.New("invalid request")
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrPaymentAlreadyExists = errors.New("payment already exists")
	ErrInvalidStatus        = errors.New("invalid status")
	ErrProviderUnsupported  = errors.New("provider is not supported")
	ErrInvalidProvider      = errors.New("invalid provider")
	ErrCallbackRejected     = errors.New("callback rejected")
)
