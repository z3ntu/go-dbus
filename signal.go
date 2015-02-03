package dbus

import (
	"errors"
	"sync"
)

// A structure to store the set of signal watches, keyed by object
// path, interface and member.
type signalWatchSet map[ObjectPath]map[string]map[string][]*signalWatch

func (self signalWatchSet) Add(watch *signalWatch) {
	byInterface, ok := self[watch.rule.Path]
	if !ok {
		byInterface = make(map[string]map[string][]*signalWatch)
		self[watch.rule.Path] = byInterface
	}
	byMember, ok := byInterface[watch.rule.Interface]
	if !ok {
		byMember = make(map[string][]*signalWatch)
		byInterface[watch.rule.Interface] = byMember
	}
	watches, ok := byMember[watch.rule.Member]
	if !ok {
		watches = make([]*signalWatch, 0, 1)
	}
	byMember[watch.rule.Member] = append(watches, watch)
}

func (self signalWatchSet) Remove(watch *signalWatch) bool {
	byInterface, ok := self[watch.rule.Path]
	if !ok {
		return false
	}
	byMember, ok := byInterface[watch.rule.Interface]
	if !ok {
		return false
	}
	watches, ok := byMember[watch.rule.Member]
	if !ok {
		return false
	}
	for i, other := range watches {
		if other == watch {
			// Truncate the watch slice, moving the item
			// at the end to the new location.
			watches[i] = watches[len(watches)-1]
			byMember[watch.rule.Member] = watches[:len(watches)-1]
			return true
		}
	}
	return false
}

func (self signalWatchSet) FindMatches(msg *Message) (matches []*signalWatch) {
	pathKeys := []ObjectPath{""}
	if msg.Path != ObjectPath("") {
		pathKeys = append(pathKeys, msg.Path)
	}
	ifaceKeys := []string{""}
	if msg.Interface != "" {
		ifaceKeys = append(ifaceKeys, msg.Interface)
	}
	memberKeys := []string{""}
	if msg.Member != "" {
		memberKeys = append(memberKeys, msg.Member)
	}
	for _, path := range pathKeys {
		byInterface, ok := self[path]
		if !ok {
			continue
		}
		for _, iface := range ifaceKeys {
			byMember, ok := byInterface[iface]
			if !ok {
				continue
			}
			for _, member := range memberKeys {
				watches, ok := byMember[member]
				if !ok {
					continue
				}
				for _, watch := range watches {
					if watch.rule.Match(msg) {
						matches = append(matches, watch)
					}
				}
			}
		}
	}
	return
}

// handle receiving signals with callback
type signalWatch struct {
	bus  *Connection
	rule *MatchRule
	cb   func(*Message)

	cancelLock sync.Mutex
	cancelled  bool
}

func (p *Connection) watchSignal(rule *MatchRule, cb func(*Message)) (*signalWatch, error) {
	if rule.Type != TypeSignal {
		return nil, errors.New("Match rule is not for signals")
	}
	watch := &signalWatch{
		bus:  p,
		rule: rule,
		cb:   cb}

	p.handlerMutex.Lock()
	p.signalMatchRules.Add(watch)
	p.handlerMutex.Unlock()

	if err := p.busProxy.AddMatch(rule.String()); err != nil {
		p.handlerMutex.Lock()
		p.signalMatchRules.Remove(watch)
		p.handlerMutex.Unlock()
		return nil, err
	}

	return watch, nil
}

func (watch *signalWatch) cancel() error {
	watch.cancelLock.Lock()
	defer watch.cancelLock.Unlock()
	if watch.cancelled {
		return nil
	}
	watch.bus.handlerMutex.Lock()
	foundMatch := watch.bus.signalMatchRules.Remove(watch)
	watch.bus.handlerMutex.Unlock()

	if foundMatch {
		if err := watch.bus.busProxy.RemoveMatch(watch.rule.String()); err != nil {
			return err
		}
	}
	watch.cancelled = true
	return nil
}

type SignalWatch struct {
	lock sync.Mutex

	*signalWatch

	// If the rule tries to match against a bus name as the
	// sender, we need to track the current owner of that name.
	nameWatch *nameWatch
	C         chan *Message
}

// Handle received signals.
func (p *Connection) WatchSignal(rule *MatchRule) (*SignalWatch, error) {
	if rule.Type != TypeSignal {
		return nil, errors.New("Match rule is not for signals")
	}
	watch := &SignalWatch{
		C: make(chan *Message)}
	// lock because we expose watch early to a possible watch.Cancel()
	// through the name watch
	watch.lock.Lock()
	defer watch.lock.Unlock()
	// Does the rule match a bus name other than the daemon?
	if rule.Sender != "" && rule.Sender != BUS_DAEMON_NAME {
		nameWatch, err := p.ensureNameWatch(rule.Sender, func(newOwner string) {
			if rule.Sender[0] == ':' {
				// For unique names, cancel the signal watch
				// when the name is lost.
				if newOwner == "" {
					watch.Cancel()
				}
			} else {
				// Otherwise, update the sender owner.
				rule.senderNameOwner = newOwner
			}
		})
		if err != nil {
			return nil, err
		}
		watch.nameWatch = nameWatch
	}

	internal, err := p.watchSignal(rule, func(msg *Message) {
		watch.C <- msg
	})
	if err != nil {
		if watch.nameWatch != nil {
			removeNameWatch(watch.nameWatch)
		}
		return nil, err
	}
	watch.signalWatch = internal
	return watch, nil
}

func (watch *SignalWatch) Cancel() error {
	watch.lock.Lock()
	defer watch.lock.Unlock()
	if watch.signalWatch == nil {
		return nil
	}
	internal := watch.signalWatch
	if watch.nameWatch != nil {
		if err := removeNameWatch(watch.nameWatch); err != nil {
			return err
		}
	}
	err := internal.cancel()
	if err != nil {
		return err
	}
	watch.signalWatch = nil
	close(watch.C)
	return nil
}
