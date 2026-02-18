package server

// The Context struct represents the context of an incomming message.
// It contains the sender session, the message's path, the message's
// room and a context specific key-value-store.
// All handlers involved in a message's handler chain share the same
// message context.
// Context structs are being pooled; use the copy method for usage in
// different go routines.
type Context struct {
	// Session which sent the message.
	Session *Session

	// Target room. Can be nil.
	Room Room

	// RequestID, used to corelate request and response.
	// RequestIDs equal to 0 represents a notify message.
	ReqID uint64

	// The message's path.
	Path string

	// Context key-value-store.
	Values map[string]any

	halt       bool
	serializer Serializer
}

// used for GC Hygiene
func (c *Context) reset() {
	c.ReqID = 0
	c.Path = ""
	c.Session = nil
	c.Room = nil
	c.Values = nil
	c.serializer = nil
	c.halt = false
}

// Copy returns a copy of a context.
func (c *Context) Copy() *Context {
	return &Context{
		Session: c.Session,
		ReqID:   c.ReqID,
		Path:    c.Path,
	}
}

// IsRequest reports whether an incomming message is a request.
// When IsRequest returns false the message is a notify message.
func (c *Context) IsRequest() bool {
	return c.ReqID != 0
}

// HasRoom reports whether an incomming message is directed to a
// specific room.
func (c *Context) HasRoom() bool {
	return c.Room != nil
}

// SetSerializer sets a context specific serializer.
func (c *Context) SetSerializer(s Serializer) {
	c.serializer = s
}

// Halt will interrupt the handler chain.
func (c *Context) Halt() {
	c.halt = true
}

// Respond sends a response message. Responses can only be send for
// request messages.
// v can be a byte slice, a string
// or any value that can be serialized by the serializer.
func (c *Context) Respond(v any) error {
	if !c.IsRequest() {
		return ErrNotifyResponse
	}

	data, err := marshalPayload(c.serializer, v)
	if err != nil {
		return err
	}

	msg := Message{
		ReqID: c.ReqID,
		Data:  data,
	}

	return c.Session.send(msg)
}
