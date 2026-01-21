package frame

type Context struct {
	Session *Session
	ReqID   uint64
	Path    string

	serializer Serializer
}

func (c *Context) IsRequest() bool {
	return c.ReqID != 0
}

func (c *Context) Copy() *Context {
	return &Context{
		Session: c.Session,
		ReqID:   c.ReqID,
		Path:    c.Path,
	}
}

func (c *Context) reset() {
	c.Session = nil
	c.ReqID = 0
	c.Path = ""
	c.serializer = nil
}

func (c *Context) Respond(v any) error {
	if !c.IsRequest() {
		return nil // TODO Error
	}

	var err error
	var data []byte

	switch val := v.(type) {
	case []byte:
		data = val
	case string:
		data = []byte(val)
	default:
		data, err = c.serializer.Marshal(v)
		if err != nil {
			return err
		}
	}

	msg := Message{
		ReqID: c.ReqID,
		Data:  data,
	}

	return c.Session.send(msg)
}
