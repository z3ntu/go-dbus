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
	nameAcquired := make(chan int, 1)
	name := bus.RequestName("com.example.GoDbus", NameFlagDoNotQueue, func(*BusName) { nameAcquired <- 0 }, nil)
	<-nameAcquired

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

	nameAcquired := make(chan int, 1)
	name := bus.RequestName("com.example.GoDbus", 0, func(*BusName) { nameAcquired <- 0 }, nil)
	c.Check(name, NotNil)

	<-nameAcquired
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

	ready := make(chan int, 1)
	name1 := bus1.RequestName("com.example.GoDbus", 0, func(*BusName) { ready <- 0 }, nil)
	<-ready
	c.Check(name1.needsRelease, Equals, true)

	callLog := []string{}
	called := make(chan int, 1)
	name2 := bus2.RequestName("com.example.GoDbus", 0,
		func(*BusName) {
			callLog = append(callLog, "acquired")
			called <- 0
		}, func(*BusName) {
			callLog = append(callLog, "lost")
			called <- 0
		})
	<-called
	c.Check(name2.needsRelease, Equals, true)
	c.Check(callLog, DeepEquals, []string{"lost"})

	// Release the name on the first connection
	c.Check(name1.Release(), IsNil)

	<-called
	c.Check(callLog, DeepEquals, []string{"lost", "acquired"})
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

	ready := make(chan int, 1)
	name1 := bus1.RequestName("com.example.GoDbus", 0, func(*BusName) { ready <- 0 }, nil)
	defer name1.Release()
	<-ready
	c.Check(name1.needsRelease, Equals, true)

	callLog := []string{}
	called := make(chan int, 1)
	name2 := bus2.RequestName("com.example.GoDbus", NameFlagDoNotQueue,
		func(*BusName) {
			callLog = append(callLog, "acquired")
			called <- 0
		}, func(*BusName) {
			callLog = append(callLog, "lost")
			called <- 0
		})
	<-called
	c.Check(name2.needsRelease, Equals, false)
	c.Check(callLog, DeepEquals, []string{"lost"})

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

	callLog1 := []string{}
	called1 := make(chan int, 1)
	name1 := bus1.RequestName("com.example.GoDbus", NameFlagAllowReplacement,
		func(*BusName) {
			callLog1 = append(callLog1, "acquired")
			called1 <- 0
		}, func(*BusName) {
			callLog1 = append(callLog1, "lost")
			called1 <- 0
		})
	defer name1.Release()
	<-called1
	c.Check(name1.needsRelease, Equals, true)
	c.Check(callLog1, DeepEquals, []string{"acquired"})

	callLog2 := []string{}
	called2 := make(chan int, 1)
	name2 := bus2.RequestName("com.example.GoDbus", NameFlagReplaceExisting,
		func(*BusName) {
			callLog2 = append(callLog2, "acquired")
			called2 <- 0
		}, func(*BusName) {
			callLog2 = append(callLog2, "lost")
			called2 <- 0
		})
	defer name2.Release()
	<-called2
	c.Check(name2.needsRelease, Equals, true)
	c.Check(callLog2, DeepEquals, []string{"acquired"})

	// The first name owner loses possession.
	<-called1
	c.Check(callLog1, DeepEquals, []string{"acquired", "lost"})
}
