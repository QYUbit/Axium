package server

type Context struct {
	Session *Session
	ReqID   uint64
	Path    string
	Values  map[string]any

	halt       bool
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
	c.Values = nil
	c.serializer = nil
	c.halt = false
}

func (c *Context) SetSerializer(s Serializer) {
	c.serializer = s
}

func (c *Context) Halt() {
	c.halt = true
}

func (c *Context) Respond(v any) error {
	if !c.IsRequest() {
		return ErrNotifyResponse
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
