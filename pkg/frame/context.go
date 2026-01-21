package frame

type baseContext struct {
	Session *Session
	ReqID   uint64
	Path    string

	serializer Serializer
}

func (c *baseContext) copy() *baseContext {
	return &baseContext{
		Session: c.Session,
		ReqID:   c.ReqID,
		Path:    c.Path,
	}
}

func (c *baseContext) reset() {
	c.Session = nil
	c.ReqID = 0
	c.Path = ""
	c.serializer = nil
}

type Context[T any] struct {
	*baseContext
	Msg *T
}

func (c *Context[T]) IsRequest() bool {
	return c.ReqID != 0
}

func (c *Context[T]) Copy() *Context[T] {
	return &Context[T]{
		baseContext: c.baseContext.copy(),
		Msg:         c.Msg,
	}
}

func (c *Context[T]) Respond(v any) error {
	if !c.IsRequest() {
		return nil // TODO Error
	}

	data, err := c.serializer.Marshal(v)
	if err != nil {
		return err
	}

	msg := Message{
		ReqID: c.ReqID,
		Data:  data,
	}

	return c.Session.send(msg)
}
