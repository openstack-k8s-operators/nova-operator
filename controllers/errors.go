package controllers

import (
	"errors"
)

var (
	ErrResourceIsNotReady = errors.New("Resource is not ready")
	ErrInvalidStatus      = errors.New("invalid status")
	ErrCannotUpdateObject = errors.New("cannot update object")
	ErrFieldNotFound      = errors.New("field not found in Secret")
)
