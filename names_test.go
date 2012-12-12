package dbus

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestConnectionWatchNameOwner(c *C) {
	bus, err := Connect(SessionBus)
	c.Assert(err, Equals, nil)
	defer bus.Close()
	c.Assert(bus.Authenticate(), Equals, nil)

	// Set up the name watch
	nameChanged := make(chan int, 1)
	owners := []string{}
	watch, err := bus.WatchNameOwner("com.example.GoDbus", func (newOwner string) {
		owners = append(owners, newOwner)
		nameChanged <- 0
	})
	c.Assert(err, Equals, nil)
	defer watch.Cancel()

	// Our handler will be called once with the initial name owner
	<- nameChanged
	c.Check(owners, DeepEquals, []string{""})

	// Acquire the name, and wait for the process to complete.
	nameAcquired := make(chan int, 1)
	name := bus.RequestName("com.example.GoDbus", NameFlagDoNotQueue, func(*BusName) { nameAcquired <- 0 }, nil)
	<- nameAcquired

	<- nameChanged
	c.Check(owners, DeepEquals, []string{"", bus.UniqueName})

	err = name.Release()
	c.Assert(err, Equals, nil)
	<- nameChanged
	c.Check(owners, DeepEquals, []string{"", bus.UniqueName, ""})
}
