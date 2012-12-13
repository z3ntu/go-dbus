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


type BusName struct {
	bus *Connection
	Name string
	Flags NameFlags

	cancelled bool
	needsRelease bool

	acquiredCallback func (*BusName)
	lostCallback     func(*BusName)

	acquiredWatch *SignalWatch
	lostWatch     *SignalWatch
}

type NameFlags uint32

const (
	NameFlagAllowReplacement = NameFlags(0x1)
	NameFlagReplaceExisting = NameFlags(0x2)
	NameFlagDoNotQueue = NameFlags(0x4)
)

func (p *Connection) RequestName(busName string, flags NameFlags, nameAcquired func(*BusName), nameLost func(*BusName)) *BusName {
	name := &BusName{
		bus: p,
		Name: busName,
		Flags: flags,
		acquiredCallback: nameAcquired,
		lostCallback: nameLost}
	go name.request()
	return name
}

func (name *BusName) request() {
	if name.cancelled {
		return
	}
	result, err := name.bus.busProxy.RequestName(name.Name, uint32(name.Flags))
	if err != nil {
		log.Println("Error requesting bus name", name.Name, "err =", err)
		return
	}
	subscribe := false
	switch result {
	case 1:
		// DBUS_REQUEST_NAME_REPLY_PRIMARY_OWNER
		if name.acquiredCallback != nil {
			name.acquiredCallback(name)
		}
		subscribe = true
		name.needsRelease = true
	case 2:
		// DBUS_REQUEST_NAME_REPLY_IN_QUEUE
		if name.lostCallback != nil {
			name.lostCallback(name)
		}
		subscribe = true
		name.needsRelease = true
	case 3:
		// DBUS_REQUEST_NAME_REPLY_EXISTS
		fallthrough
	case 4:
		// DBUS_REQUEST_NAME_REPLY_ALREADY_OWNER
		fallthrough
	default:
		// assume that other responses mean we couldn't own
		// the name
		if name.lostCallback != nil {
			name.lostCallback(name)
		}
	}

	if subscribe && !name.cancelled {
		watch, err := name.bus.WatchSignal(&MatchRule{
			Type: TypeSignal,
			Sender: BUS_DAEMON_NAME,
			Path: BUS_DAEMON_PATH,
			Interface: BUS_DAEMON_IFACE,
			Member: "NameLost",
			Arg0: name.Name},
			func(msg *Message) {
				if !name.cancelled && name.lostCallback != nil {
					name.lostCallback(name)
				}
			})
		if err != nil {
			log.Println("Could not set up NameLost signal watch")
			name.Release()
			return
		}
		name.lostWatch = watch

		watch, err = name.bus.WatchSignal(&MatchRule{
			Type: TypeSignal,
			Sender: BUS_DAEMON_NAME,
			Path: BUS_DAEMON_PATH,
			Interface: BUS_DAEMON_IFACE,
			Member: "NameAcquired",
			Arg0: name.Name},
			func(msg *Message) {
				if !name.cancelled && name.acquiredCallback != nil {
					name.acquiredCallback(name)
				}
			})
		if err != nil {
			log.Println("Could not set up NameLost signal watch")
			name.Release()
			return
		}
		name.acquiredWatch = watch

		// XXX: if we disconnect from the bus, we should
		// report the name being lost.
	}
}

func (name *BusName) Release() error {
	if name.cancelled {
		return nil
	}
	name.cancelled = true
	if name.acquiredWatch != nil {
		if err := name.acquiredWatch.Cancel(); err != nil {
			return err
		}
		name.acquiredWatch = nil
	}
	if name.lostWatch != nil {
		if err := name.lostWatch.Cancel(); err != nil {
			return err
		}
		name.lostWatch = nil
	}

	if name.needsRelease {
		result, err := name.bus.busProxy.ReleaseName(name.Name)
		if err != nil {
			return err
		}
		if result != 1 { // DBUS_RELEASE_NAME_REPLY_RELEASED
			log.Println("Unexpected result when releasing name", name.Name, "result =", result)
		}
		name.needsRelease = false
	}
	return nil
}
