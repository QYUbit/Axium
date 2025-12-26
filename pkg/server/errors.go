package server

type ErrAlreadyRunning struct{}

func (ErrAlreadyRunning) Error() string { return "server is already running" }

type ErrNotRunning struct{}

func (ErrNotRunning) Error() string { return "server is not running" }
