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
	c.Check(bus.Authenticate(), Equals, nil)

	for i, test := range callTests {
		err = test.Call(bus)
		if err != nil {
			c.Errorf("callTest %d: %v", i, err)
		}
	}

	err = bus.Close()
	c.Check(err, Equals, nil)
}

func (s *S) TestSendSignal(c *C) {
	bus1, err := Connect(SessionBus)
	c.Check(err, Equals, nil)
	defer bus1.Close()
	c.Check(bus1.Authenticate(), Equals, nil)

	// Set up a second bus connection to receive a signal.
	watchReady := make(chan int)
	complete := make(chan *Message)
	go func(sender string, watchReady chan<- int, complete chan<- *Message) {
		bus2, err := Connect(SessionBus)
		if err != nil {
			c.Error(err)
			watchReady <- 0
			complete <- nil
			return
		}
		defer bus2.Close()
		if err := bus2.Authenticate(); err != nil {
			c.Error(err)
			watchReady <- 0
			complete <- nil
			return
		}
		msgChan := make(chan *Message)
		watch, err := bus2.WatchSignal(&MatchRule{
			Type: TypeSignal,
			Sender: sender,
			Path: "/go/dbus/test",
			Interface: "com.example.GoDbus",
			Member: "TestSignal"},
			func(msg *Message) { msgChan <- msg })
		watchReady <- 0
		if err != nil {
			c.Error(err)
			bus2.Close()
			complete <- nil
			return
		}
		msg := <-msgChan
		if err := watch.Cancel(); err != nil {
			c.Error(err)
		}
		complete <- msg
	}(bus1.uniqName, watchReady, complete)

	// Wait for the goroutine to configure the signal watch
	<-watchReady

	// Send the signal and wait for it to be received at the other end.
	signal := NewSignalMessage("/go/dbus/test", "com.example.GoDbus", "TestSignal")
	if err := bus1.Send(signal); err != nil {
		c.Fatal(err)
	}

	signal2 := <- complete
	c.Check(signal2, Not(Equals), nil)
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
	watch1 := SignalWatch{nil, MatchRule{
		Type: TypeSignal,
		Sender: ":1.42",
		Path: "/foo",
		Interface: "com.example.Foo",
		Member: "Bar"}, nil}
	set.Add(&watch1)
	watch2 := SignalWatch{nil, MatchRule{
		Type: TypeSignal,
		Sender: ":1.43",
		Path: "/foo",
		Interface: "com.example.Foo",
		Member: "Bar"}, nil}
	set.Add(&watch2)

	c.Check(set.Remove(&watch1), Equals, true)
	c.Check(set["/foo"]["com.example.Foo"]["Bar"], DeepEquals, []*SignalWatch{&watch2})

	// A second attempt at removal fails
	c.Check(set.Remove(&watch1), Equals, false)
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
