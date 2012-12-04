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
	watch, err := bus.WatchSignal(&MatchRule{
		Type: TypeSignal,
		Sender: "org.freedesktop.DBus",
		Path: "/org/freedesktop/DBus",
		Interface: "org.freedesktop.DBus",
		Member: "NameOwnerChanged",
		Arg0: busName},
		func (msg *Message) { owner.handleOwnerChange(msg) })
	if err != nil {
		return nil, err
	}
	owner.signalWatch = watch

	msg := NewMethodCallMessage("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "GetNameOwner")
	if err := msg.AppendArgs(busName); err != nil {
		// Ignore error
		_ = watch.Cancel()
		return nil, err
	}
	return owner, nil
}

func (self *nameOwner) handleOwnerChange(msg *Message) {
	var busName, oldOwner, newOwner string
	if err := msg.GetArgs(&busName, &oldOwner, &newOwner); err != nil {
		log.Println("Could not decode NameOwnerChanged message:", err)
		return
	}
	for _, watch := range self.watches {
		watch.handler(busName, oldOwner, newOwner)
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
