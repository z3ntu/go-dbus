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
	c.Check(msg.Path, Equals, "/org/freedesktop/DBus")
	c.Check(msg.Dest, Equals, "org.freedesktop.DBus")
	c.Check(msg.Iface, Equals, "org.freedesktop.DBus")
	c.Check(msg.Member, Equals, "NameHasOwner")
	c.Check(msg.Sig, Equals, "s")
	c.Check(msg.Params, DeepEquals, []interface{}{"xyz"})
}

func (s *S) TestMarshalMessage(c *C) {
	msg := NewMessage()
	msg.Type = TypeMethodCall
	msg.Flags = MessageFlag(0)
	msg.serial = 1
	msg.Path = "/org/freedesktop/DBus"
	msg.Dest = "org.freedesktop.DBus"
	msg.Iface = "org.freedesktop.DBus"
	msg.Member = "NameHasOwner"
	msg.Sig = "s"
	msg.Params = []interface{}{"xyz"}

	buff, err := msg._Marshal()
	if err != nil {
		c.Error(err)
	} else {
		c.Check(buff, DeepEquals, testMessage)
	}
}
