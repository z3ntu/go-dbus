package dbus

import (
	"errors"
	"log"
)

type nameOwner struct {
	bus          *Connection
	busName      string
	currentOwner string
	signalWatch  *SignalWatch
	watches      []*NameOwnerWatch
}

type NameOwnerWatch struct {
	owner     *nameOwner
	handler   func(newOwner string)
	cancelled bool
}

func newNameOwner(bus *Connection, busName string) (*nameOwner, error) {
	owner := &nameOwner{
		bus: bus,
		busName: busName,
		watches: []*NameOwnerWatch{}}
	handler := func(msg *Message) {
		var busName, oldOwner, newOwner string
		if err := msg.GetArgs(&busName, &oldOwner, &newOwner); err != nil {
			log.Println("Could not decode NameOwnerChanged message:", err)
			return
		}
		owner.handleOwnerChange(newOwner)
	}
	watch, err := bus.WatchSignal(&MatchRule{
		Type: TypeSignal,
		Sender: BUS_DAEMON_NAME,
		Path: BUS_DAEMON_PATH,
		Interface: BUS_DAEMON_IFACE,
		Member: "NameOwnerChanged",
		Arg0: busName}, handler)
	if err != nil {
		return nil, err
	}
	owner.signalWatch = watch

	// spawn a goroutine to find the current name owner
	go owner.checkCurrentOwner()

	return owner, nil
}

func (self *nameOwner) checkCurrentOwner() {
	currentOwner, err := self.bus.busProxy.GetNameOwner(self.busName)
	if err != nil {
		if dbusErr, ok := err.(*Error); !ok || dbusErr.Name != "org.freedesktop.DBus.Error.NameHasNoOwner" {
			log.Println("Unexpected error from GetNameOwner:", err)
		}
	}
	if self.currentOwner == "" {
		// Simulate an ownership change message.
		self.handleOwnerChange(currentOwner)
	}
}

func (self *nameOwner) handleOwnerChange(newOwner string) {
	for _, watch := range self.watches {
		if watch.handler != nil {
			watch.handler(newOwner)
		}
	}
	self.currentOwner = newOwner
}

func (p *Connection) WatchNameOwner(busName string, handler func(newOwner string)) (watch *NameOwnerWatch, err error) {
	p.nameOwnerMutex.Lock()
	owner, ok := p.nameOwners[busName]
	if !ok {
		if owner, err = newNameOwner(p, busName); err != nil {
			p.nameOwnerMutex.Unlock()
			return
		}
		p.nameOwners[busName] = owner
	}
	watch = &NameOwnerWatch{owner: owner, handler: handler}
	owner.watches = append(owner.watches, watch)
	p.nameOwnerMutex.Unlock()

	// If we're hooking up to an existing nameOwner and it already
	// knows the current name owner, tell our callback.
	if !ok && owner.currentOwner != "" {
		handler(owner.currentOwner)
	}
	return
}

func (watch *NameOwnerWatch) Cancel() error {
	if watch.cancelled {
		return nil
	}
	watch.cancelled = true

	owner := watch.owner
	bus := owner.bus
	bus.nameOwnerMutex.Lock()
	defer bus.nameOwnerMutex.Unlock()

	found := false
	for i, other := range(owner.watches) {
		if other == watch {
			owner.watches[i] = owner.watches[len(owner.watches)-1]
			owner.watches = owner.watches[:len(owner.watches)-1]
			found = true
			break
		}
	}
	if !found {
		return errors.New("NameOwnerWatch already cancelled")
	}
	if len(owner.watches) != 0 {
		// There are other watches interested in this name, so
		// leave the nameOwner in place.
		return nil
	}
	delete(bus.nameOwners, owner.busName)
	return owner.signalWatch.Cancel()
}
