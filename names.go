package dbus

import (
	"errors"
	"log"
	"sync"
)

type nameInfo struct {
	bus          *Connection
	busName      string
	signalWatch  *SignalWatch
	lock         sync.Mutex
	currentOwner string
	watches      []*NameWatch
}

type NameWatch struct {
	info       *nameInfo
	C          chan string
	cancelLock sync.Mutex
	cancelled  bool
}

func newNameInfo(bus *Connection, busName string) (*nameInfo, error) {
	info := &nameInfo{
		bus:     bus,
		busName: busName,
		watches: []*NameWatch{}}
	watch, err := bus.WatchSignal(&MatchRule{
		Type:      TypeSignal,
		Sender:    BUS_DAEMON_NAME,
		Path:      BUS_DAEMON_PATH,
		Interface: BUS_DAEMON_IFACE,
		Member:    "NameOwnerChanged",
		Arg0:      busName})
	if err != nil {
		return nil, err
	}
	info.signalWatch = watch
	go func() {
		for msg := range watch.C {
			var busName, oldOwner, newOwner string
			if err := msg.Args(&busName, &oldOwner, &newOwner); err != nil {
				log.Println("Could not decode NameOwnerChanged message:", err)
				continue
			}
			info.handleOwnerChange(newOwner, false)
		}
	}()

	// spawn a goroutine to find the current name owner
	go info.checkCurrentOwner()

	return info, nil
}

func (self *nameInfo) checkCurrentOwner() {
	currentOwner, err := self.bus.busProxy.GetNameOwner(self.busName)
	if err != nil {
		if dbusErr, ok := err.(*Error); !ok || dbusErr.Name != "org.freedesktop.DBus.Error.NameHasNoOwner" {
			log.Println("Unexpected error from GetNameOwner:", err)
		}
	}
	self.lock.Lock()
	defer self.lock.Unlock()
	if self.currentOwner == "" {
		// Simulate an ownership change message.
		self.handleOwnerChange(currentOwner, true)
	}
}

func (self *nameInfo) handleOwnerChange(newOwner string, lockAcquired bool) {
	if !lockAcquired {
		self.lock.Lock()
		defer self.lock.Unlock()
	}
	for _, watch := range self.watches {
		watch.C <- newOwner
	}
	self.currentOwner = newOwner
}

func (p *Connection) WatchName(busName string) (watch *NameWatch, err error) {
	p.nameInfoMutex.Lock()
	defer p.nameInfoMutex.Unlock()
	info, ok := p.nameInfo[busName]
	if !ok {
		if info, err = newNameInfo(p, busName); err != nil {
			return
		}
		p.nameInfo[busName] = info
	}
	watch = &NameWatch{info: info, C: make(chan string, 1)}
	info.lock.Lock()
	defer info.lock.Unlock()
	info.watches = append(info.watches, watch)

	// If we're hooking up to an existing nameOwner and it already
	// knows the current name owner, tell our callback.
	if ok && info.currentOwner != "" {
		watch.C <- info.currentOwner
	}
	return
}

func (watch *NameWatch) Cancel() error {
	watch.cancelLock.Lock()
	defer watch.cancelLock.Unlock()
	if watch.cancelled {
		return nil
	}
	watch.cancelled = true

	info := watch.info
	bus := info.bus
	bus.nameInfoMutex.Lock()
	defer bus.nameInfoMutex.Unlock()
	info.lock.Lock()
	defer info.lock.Unlock()

	found := false
	for i, other := range info.watches {
		if other == watch {
			info.watches[i] = info.watches[len(info.watches)-1]
			info.watches = info.watches[:len(info.watches)-1]
			found = true
			break
		}
	}
	if !found {
		return errors.New("NameOwnerWatch already cancelled")
	}
	close(watch.C)
	if len(info.watches) != 0 {
		// There are other watches interested in this name, so
		// leave the nameOwner in place.
		return nil
	}
	delete(bus.nameInfo, info.busName)
	return info.signalWatch.Cancel()
}

// BusName acts as a handle for a well known bus name owned by this client.
type BusName struct {
	bus   *Connection
	Name  string
	Flags NameFlags
	C     chan error

	lock         sync.Mutex
	cancelled    bool
	needsRelease bool

	acquiredWatch *SignalWatch
	lostWatch     *SignalWatch
}

type NameFlags uint32

const (
	NameFlagAllowReplacement NameFlags = 1 << iota
	NameFlagReplaceExisting
	NameFlagDoNotQueue
)

var (
	ErrNameLost         = errors.New("name ownership lost")
	ErrNameInQueue      = errors.New("in queue for name ownership")
	ErrNameExists       = errors.New("name exists")
	ErrNameAlreadyOwned = errors.New("name already owned")
)

// RequestName requests ownership of a well known bus name.
//
// Name ownership is communicated over the the BusName's channel: a
// nil value indicates that the name was successfully acquired, and a
// non-nil value indicates that the name was lost or could not be
// acquired.
func (p *Connection) RequestName(busName string, flags NameFlags) *BusName {
	name := &BusName{
		bus:   p,
		Name:  busName,
		Flags: flags,
		C:     make(chan error, 1)}
	go name.request()
	return name
}

func (name *BusName) request() {
	name.lock.Lock()
	defer name.lock.Unlock()
	if name.cancelled {
		return
	}

	if !name.cancelled {
		watch, err := name.bus.WatchSignal(&MatchRule{
			Type:      TypeSignal,
			Sender:    BUS_DAEMON_NAME,
			Path:      BUS_DAEMON_PATH,
			Interface: BUS_DAEMON_IFACE,
			Member:    "NameLost",
			Arg0:      name.Name})
		if err != nil {
			log.Println("Could not set up NameLost signal watch")
			name.Release()
			return
		}
		name.lostWatch = watch
		go func() {
			for _ = range name.lostWatch.C {
				name.lock.Lock()
				defer name.lock.Unlock()
				name.C <- ErrNameLost
				name.release(false)
				break
			}
		}()

		watch, err = name.bus.WatchSignal(&MatchRule{
			Type:      TypeSignal,
			Sender:    BUS_DAEMON_NAME,
			Path:      BUS_DAEMON_PATH,
			Interface: BUS_DAEMON_IFACE,
			Member:    "NameAcquired",
			Arg0:      name.Name})
		if err != nil {
			log.Println("Could not set up NameLost signal watch")
			name.Release()
			return
		}
		name.acquiredWatch = watch
		go func() {
			for _ = range name.acquiredWatch.C {
				name.C <- nil
			}
		}()

		// XXX: if we disconnect from the bus, we should
		// report the name being lost.
	}

	result, err := name.bus.busProxy.RequestName(name.Name, uint32(name.Flags))
	if err != nil {
		log.Println("Error requesting bus name", name.Name, "err =", err)
		return
	}
	switch result {
	case 1:
		// DBUS_REQUEST_NAME_REPLY_PRIMARY_OWNER
		name.needsRelease = true
	case 2:
		// DBUS_REQUEST_NAME_REPLY_IN_QUEUE
		name.needsRelease = true
		name.C <- ErrNameInQueue
	case 3:
		// DBUS_REQUEST_NAME_REPLY_EXISTS
		name.C <- ErrNameExists
		name.release(false)
	case 4:
		// DBUS_REQUEST_NAME_REPLY_ALREADY_OWNER
		name.C <- ErrNameAlreadyOwned
		name.release(false)
	default:
		// assume that other responses mean we couldn't own
		// the name
		name.C <- errors.New("Unknown error")
		name.release(false)
	}
}

func (name *BusName) checkNeedsRelease() bool {
	name.lock.Lock()
	defer name.lock.Unlock()
	return name.needsRelease
}

// Release releases well known name on the message bus.
func (name *BusName) Release() error {
	name.lock.Lock()
	defer name.lock.Unlock()
	return name.release(name.needsRelease)
}

func (name *BusName) release(needsRelease bool) error {
	if name.cancelled {
		return nil
	}
	name.cancelled = true
	if name.acquiredWatch != nil {
		if err := name.acquiredWatch.Cancel(); err != nil {
			return err
		}
	}
	if name.lostWatch != nil {
		if err := name.lostWatch.Cancel(); err != nil {
			return err
		}
	}
	close(name.C)

	if needsRelease {
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
