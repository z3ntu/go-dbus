package dbus

import (
	. "launchpad.net/gocheck"
	"fmt"
)

type callTest struct {
	dest string
	path ObjectPath
	iface, method string
	args                      []interface{}
	validate                  func([]interface{}) error
}

var callTests = []callTest{
	{"org.freedesktop.Notifications", "/org/freedesktop/Notifications",
		"org.freedesktop.Notifications", "Notify",
		[]interface{}{
			"go-dbus", uint32(0),
			"info", "testing go-dbus", "test_body",
			[]string{}, map[string]Variant{},
			int32(2000)},
		func([]interface{}) error {
			return nil
		}},
}

func (test callTest) Call(c *Connection) error {
	method, err := c.Object(test.dest, test.path).Interface(test.iface).Method(test.method)
	if err != nil {
		return err
	}
	out, err := c.Call(method, test.args...)
	if err != nil {
		return fmt.Errorf("failed Method.Call: %v", err)
	}
	if err = test.validate(out); err != nil {
		err = fmt.Errorf("failed validation: %v", err)
	}
	return err
}

func (s *S) TestDBus(c *C) {
	bus, err := Connect(SessionBus)
	c.Check(err, Equals, nil)

	err = bus.Authenticate()
	c.Check(err, Equals, nil)

	for i, test := range callTests {
		err = test.Call(bus)
		if err != nil {
			c.Errorf("callTest %d: %v", i, err)
		}
	}
}

func (s *S) TestSignalWatchSetAdd(c *C) {
	set := make(signalWatchSet)
	watch := SignalWatch{nil, MatchRule{
		Type: TypeSignal,
		Sender: ":1.42",
		Path: "/foo",
		Interface: "com.example.Foo",
		Member: "Bar"}, nil}
	set.Add(&watch)

	byInterface, ok := set["/foo"]
	c.Assert(ok, Equals, true)
	byMember, ok := byInterface["com.example.Foo"]
	c.Assert(ok, Equals, true)
	watches, ok := byMember["Bar"]
	c.Assert(ok, Equals, true)
	c.Check(watches, DeepEquals, []*SignalWatch{&watch})
}

func (s *S) TestSignalWatchSetRemove(c *C) {
	set := make(signalWatchSet)
	watch := SignalWatch{nil, MatchRule{
		Type: TypeSignal,
		Sender: ":1.42",
		Path: "/foo",
		Interface: "com.example.Foo",
		Member: "Bar"}, nil}
	set.Add(&watch)

	c.Check(set.Remove(&watch), Equals, true)
	c.Check(set["/foo"]["com.example.Foo"]["Bar"], DeepEquals, []*SignalWatch{})

	// A second attempt at removal fails
	c.Check(set.Remove(&watch), Equals, false)
}

func (s *S) TestSignalWatchSetFindMatches(c *C) {
	msg := NewSignalMessage("/foo", "com.example.Foo", "Bar")
	msg.Sender = ":1.42"

	set := make(signalWatchSet)
	watch := SignalWatch{nil, MatchRule{
		Type: TypeSignal,
		Sender: ":1.42",
		Path: "/foo",
		Interface: "com.example.Foo",
		Member: "Bar"}, nil}

	set.Add(&watch)
	c.Check(set.FindMatches(msg), DeepEquals, []*SignalWatch{&watch})
	set.Remove(&watch)

	// An empty path also matches
	watch.rule.Path = ""
	set.Add(&watch)
	c.Check(set.FindMatches(msg), DeepEquals, []*SignalWatch{&watch})
	set.Remove(&watch)

	// Or an empty interface
	watch.rule.Path = "/foo"
	watch.rule.Interface = ""
	set.Add(&watch)
	c.Check(set.FindMatches(msg), DeepEquals, []*SignalWatch{&watch})
	set.Remove(&watch)

	// Or an empty member
	watch.rule.Interface = "com.example.Foo"
	watch.rule.Member = ""
	set.Add(&watch)
	c.Check(set.FindMatches(msg), DeepEquals, []*SignalWatch{&watch})
	set.Remove(&watch)
}
