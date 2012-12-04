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
	owner   *nameOwner
	handler func(busName, oldOwner, newOwner string)
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
		owner.handleOwnerChange(oldOwner, newOwner)
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
	go func() {
		currentOwner, err := bus.busProxy.GetNameOwner(busName)
		if err != nil {
			if dbusErr, ok := err.(*Error); !ok || dbusErr.Name != "org.freedesktop.DBus.Error.NameHasNoOwner" {
				log.Println("Unexpected error from GetNameOwner:", err)
			}
		}
		if owner.currentOwner == "" {
			// Simulate an ownership change message.
			owner.handleOwnerChange("", currentOwner)
		}
	}()

	return owner, nil
}

func (self *nameOwner) handleOwnerChange(oldOwner, newOwner string) {
	for _, watch := range self.watches {
		watch.handler(self.busName, oldOwner, newOwner)
	}
	self.currentOwner = newOwner
}

func (p *Connection) WatchNameOwner(busName string, handler func(busName, oldOwner, newOwner string)) (watch *NameOwnerWatch, err error) {
	p.nameOwnerMutex.Lock()
	defer p.nameOwnerMutex.Unlock()
	owner, ok := p.nameOwners[busName]
	if !ok {
		if owner, err = newNameOwner(p, busName); err != nil {
			return
		}
		p.nameOwners[busName] = owner
	}
	watch = &NameOwnerWatch{owner, handler}
	owner.watches = append(owner.watches, watch)
	return
}

func (watch *NameOwnerWatch) Cancel() error {
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
