package server

import "errors"

var (
	ErrNotifyResponse = errors.New("cannot respond to notify messages")
)
