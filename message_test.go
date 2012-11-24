package dbus

import . "launchpad.net/gocheck"

var testMessage = []byte{
	'l', // Byte order
	1,   // Message type
	0,   // Flags
	1,   // Protocol
	8, 0, 0, 0, // Body length
	1, 0, 0, 0, // Serial
	127, 0, 0, 0, // Header fields array length
	1, 1, 'o', 0, // Path, type OBJECT_PATH
	21, 0, 0, 0, '/', 'o', 'r', 'g', '/', 'f', 'r', 'e', 'e', 'd', 'e', 's', 'k', 't', 'o', 'p', '/', 'D', 'B', 'u', 's', 0,
	0, 0,
	2, 1, 's', 0, // Interface, type STRING
	20, 0, 0, 0, 'o', 'r', 'g', '.', 'f', 'r', 'e', 'e', 'd', 'e', 's', 'k', 't', 'o', 'p', '.', 'D', 'B', 'u', 's', 0,
	0, 0, 0,
	3, 1, 's', 0, // Member, type STRING
	12, 0, 0, 0, 'N', 'a', 'm', 'e', 'H', 'a', 's', 'O', 'w', 'n', 'e', 'r', 0,
	0, 0, 0,
	6, 1, 's', 0, // Destination, type STRING
	20, 0, 0, 0, 'o', 'r', 'g', '.', 'f', 'r', 'e', 'e', 'd', 'e', 's', 'k', 't', 'o', 'p', '.', 'D', 'B', 'u', 's', 0,
	0, 0, 0,
	8, 1, 'g', 0, // Signature, type SIGNATURE
	1, 's', 0,
	0,
	// Message body
	3, 0, 0, 0,
	'x', 'y', 'z', 0}

func (s *S) TestUnmarshalMessage(c *C) {

	msg, _, err := _Unmarshal(testMessage)
	if nil != err {
		c.Error(err)
	}
	c.Check(msg.Type, Equals, TypeMethodCall)
	c.Check(msg.Path, Equals, ObjectPath("/org/freedesktop/DBus"))
	c.Check(msg.Dest, Equals, "org.freedesktop.DBus")
	c.Check(msg.Iface, Equals, "org.freedesktop.DBus")
	c.Check(msg.Member, Equals, "NameHasOwner")
	c.Check(msg.sig, Equals, Signature("s"))
	c.Check(msg.GetArgs(), DeepEquals, []interface{}{"xyz"})
}

func (s *S) TestMarshalMessage(c *C) {
	msg := newMessage()
	msg.Type = TypeMethodCall
	msg.Flags = MessageFlag(0)
	msg.serial = 1
	msg.Path = "/org/freedesktop/DBus"
	msg.Dest = "org.freedesktop.DBus"
	msg.Iface = "org.freedesktop.DBus"
	msg.Member = "NameHasOwner"
	if err := msg.Append("xyz"); err != nil {
		c.Error(err)
	}

	buff, err := msg._Marshal()
	if err != nil {
		c.Error(err)
	} else {
		c.Check(buff, DeepEquals, testMessage)
	}
}

func (s* S) TestNewMethodCallMessage(c *C) {
	msg := NewMethodCallMessage("com.destination", "/path", "com.interface", "method")
	c.Check(msg.Type, Equals, TypeMethodCall)
	c.Check(msg.Dest, Equals, "com.destination")
	c.Check(msg.Path, Equals, ObjectPath("/path"))
	c.Check(msg.Iface, Equals, "com.interface")
	c.Check(msg.Member, Equals, "method")

	// No signature or data
	c.Check(msg.sig, Equals, Signature(""))
	c.Check(msg.body, DeepEquals, []byte{})
}

func (s *S) TestNewMethodReturnMessage(c *C) {
	call := NewMethodCallMessage("com.destination", "/path", "com.interface", "method")
	call.serial = 42
	call.Sender = ":1.2"

	reply := NewMethodReturnMessage(call)
	c.Check(reply.Type, Equals, TypeMethodReturn)
	c.Check(reply.Dest, Equals, ":1.2")
	c.Check(reply.replySerial, Equals, uint32(42))

	// No signature or data
	c.Check(reply.sig, Equals, Signature(""))
	c.Check(reply.body, DeepEquals, []byte{})
}

func (s *S) TestNewSignalMessage(c *C) {
	msg := NewSignalMessage("/path", "com.interface", "signal")
	c.Check(msg.Type, Equals, TypeSignal)
	c.Check(msg.Dest, Equals, "")
	c.Check(msg.Path, Equals, ObjectPath("/path"))
	c.Check(msg.Iface, Equals, "com.interface")
	c.Check(msg.Member, Equals, "signal")

	// No signature or data
	c.Check(msg.sig, Equals, Signature(""))
	c.Check(msg.body, DeepEquals, []byte{})
}

func (s *S) TestNewErrorMessage(c *C) {
	call := NewMethodCallMessage("com.destination", "/path", "com.interface", "method")
	call.serial = 42
	call.Sender = ":1.2"

	reply := NewErrorMessage(call, "com.interface.Error", "message")
	c.Check(reply.Type, Equals, TypeError)
	c.Check(reply.Dest, Equals, ":1.2")
	c.Check(reply.replySerial, Equals, uint32(42))
	c.Check(reply.ErrorName, Equals, "com.interface.Error")

	// No signature or data
	c.Check(reply.sig, Equals, Signature("s"))
	c.Check(reply.GetArgs(), DeepEquals, []interface{}{"message"})
}
