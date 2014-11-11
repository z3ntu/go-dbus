package dbus

import (
	. "launchpad.net/gocheck"
)

func (s *S) TestConnectionWatchName(c *C) {
	bus, err := Connect(SessionBus)
	c.Assert(err, IsNil)
	defer bus.Close()

	// Set up the name watch
	nameChanged := make(chan int, 1)
	owners := []string{}
	watch, err := bus.WatchName("com.example.GoDbus")
	c.Assert(err, IsNil)
	defer watch.Cancel()
	go func() {
		for newOwner := range watch.C {
			owners = append(owners, newOwner)
			nameChanged <- 0
		}
	}()

	// Our handler will be called once with the initial name owner
	<-nameChanged
	c.Check(owners, DeepEquals, []string{""})

	// Acquire the name, and wait for the process to complete.
	name := bus.RequestName("com.example.GoDbus", NameFlagDoNotQueue)
	c.Check(<-name.C, IsNil)

	<-nameChanged
	c.Check(owners, DeepEquals, []string{"", bus.UniqueName})

	err = name.Release()
	c.Assert(err, IsNil)
	<-nameChanged
	c.Check(owners, DeepEquals, []string{"", bus.UniqueName, ""})
}

func (s *S) TestConnectionRequestName(c *C) {
	bus, err := Connect(SessionBus)
	c.Assert(err, IsNil)
	defer bus.Close()

	name := bus.RequestName("com.example.GoDbus", 0)
	c.Check(name, NotNil)
	c.Check(<-name.C, IsNil)

	owner, err := bus.busProxy.GetNameOwner("com.example.GoDbus")
	c.Check(err, IsNil)
	c.Check(owner, Equals, bus.UniqueName)

	c.Check(name.Release(), IsNil)
}

func (s *S) TestConnectionRequestNameQueued(c *C) {
	// Acquire the name on a second connection
	bus1, err := Connect(SessionBus)
	c.Assert(err, IsNil)
	defer bus1.Close()

	bus2, err := Connect(SessionBus)
	c.Assert(err, IsNil)
	defer bus2.Close()

	name1 := bus1.RequestName("com.example.GoDbus", 0)
	c.Check(<-name1.C, IsNil)
	c.Check(name1.checkNeedsRelease(), Equals, true)

	name2 := bus2.RequestName("com.example.GoDbus", 0)
	c.Check(<-name2.C, Equals, ErrNameInQueue)
	c.Check(name2.checkNeedsRelease(), Equals, true)

	// Release the name on the first connection
	c.Check(name1.Release(), IsNil)

	// And the second connection can now acquire it
	c.Check(<-name2.C, IsNil)
	c.Check(name2.Release(), IsNil)
}

func (s *S) TestConnectionRequestNameDoNotQueue(c *C) {
	// Acquire the name on a second connection
	bus1, err := Connect(SessionBus)
	c.Assert(err, IsNil)
	defer bus1.Close()

	bus2, err := Connect(SessionBus)
	c.Assert(err, IsNil)
	defer bus2.Close()

	name1 := bus1.RequestName("com.example.GoDbus", 0)
	defer name1.Release()
	c.Check(<-name1.C, IsNil)
	c.Check(name1.checkNeedsRelease(), Equals, true)

	name2 := bus2.RequestName("com.example.GoDbus", NameFlagDoNotQueue)
	c.Check(<-name2.C, Equals, ErrNameExists)
	c.Check(name2.checkNeedsRelease(), Equals, false)

	c.Check(name2.Release(), IsNil)
}

func (s *S) TestConnectionRequestNameAllowReplacement(c *C) {
	// Acquire the name on a second connection
	bus1, err := Connect(SessionBus)
	c.Assert(err, IsNil)
	defer bus1.Close()

	bus2, err := Connect(SessionBus)
	c.Assert(err, IsNil)
	defer bus2.Close()

	name1 := bus1.RequestName("com.example.GoDbus", NameFlagAllowReplacement)
	defer name1.Release()
	c.Check(<-name1.C, IsNil)
	c.Check(name1.checkNeedsRelease(), Equals, true)

	name2 := bus2.RequestName("com.example.GoDbus", NameFlagReplaceExisting)
	defer name2.Release()
	c.Check(<-name2.C, IsNil)
	c.Check(name2.checkNeedsRelease(), Equals, true)

	// The first name owner loses possession.
	c.Check(<-name1.C, Equals, ErrNameLost)
}
