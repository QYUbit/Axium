package server

import "errors"

var (
	ErrNotifyResponse   = errors.New("cannot respond to notify messages")
	ErrSessionClosed    = errors.New("session is already closed")
	ErrAlreadyStarted   = errors.New("server has already started")
	ErrServerClosed     = errors.New("server is already closed")
	ErrServerNotRunning = errors.New("server is not running yet")
	ErrRoomClosed       = errors.New("room already closed")
)
