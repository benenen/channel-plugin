package domain

import "errors"

var (
	ErrNotFound   = errors.New("not found")
	ErrInvalidArg = errors.New("invalid argument")
)
