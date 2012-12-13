package dbus

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

type StandardBus int

const (
	SessionBus StandardBus = iota
	SystemBus
)

const (
	BUS_DAEMON_NAME  = "org.freedesktop.DBus"
	BUS_DAEMON_PATH  = ObjectPath("/org/freedesktop/DBus")
	BUS_DAEMON_IFACE = "org.freedesktop.DBus"
)

type SignalWatch struct {
	bus     *Connection
	rule    MatchRule
	handler func(*Message)

	// If the rule tries to match against a bus name as the
	// sender, we need to track the current owner of that name.
	nameWatch *NameWatch

	cancelled bool
}

// A structure to store the set of signal watches, keyed by object
// path, interface and member.
type signalWatchSet map[ObjectPath] map[string] map[string] []*SignalWatch

func (self signalWatchSet) Add(watch *SignalWatch) {
	byInterface, ok := self[watch.rule.Path]
	if !ok {
		byInterface = make(map[string] map[string] []*SignalWatch)
		self[watch.rule.Path] = byInterface
	}
	byMember, ok := byInterface[watch.rule.Interface]
	if !ok {
		byMember = make(map[string] []*SignalWatch)
		byInterface[watch.rule.Interface] = byMember
	}
	watches, ok := byMember[watch.rule.Member]
	if !ok {
		watches = make([]*SignalWatch, 0, 1)
	}
	byMember[watch.rule.Member] = append(watches, watch)
}

func (self signalWatchSet) Remove(watch *SignalWatch) bool {
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

func (self signalWatchSet) FindMatches(msg *Message) (matches []*SignalWatch) {
	pathKeys := []ObjectPath{""}
	if msg.Path != ObjectPath("") {
		pathKeys = append(pathKeys, msg.Path)
	}
	ifaceKeys := []string{""}
	if msg.Iface != "" {
		ifaceKeys = append(ifaceKeys, msg.Iface)
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
					if watch.rule._Match(msg) {
						matches = append(matches, watch)
					}
				}
			}
		}
	}
	return
}

type MessageFilter struct {
	filter func(*Message) *Message
}

type Connection struct {
	addressMap         map[string]string
	UniqueName         string
	conn               net.Conn
	busProxy           BusDaemon
	lastSerial         uint32

	handlerMutex       sync.Mutex // covers the next three
	messageFilters     []*MessageFilter
	methodCallReplies  map[uint32] chan<- *Message
	objectPathHandlers map[ObjectPath] chan<- *Message
	signalMatchRules   signalWatchSet

	nameInfoMutex     sync.Mutex
	nameInfo          map[string] *nameInfo
}

type ObjectProxy struct {
	bus *Connection
	destination string
	path ObjectPath
}

func (o *ObjectProxy) GetObjectPath() ObjectPath {
	return o.path
}

func (o *ObjectProxy) Call(iface, method string, args ...interface{}) (*Message, error) {
	msg := NewMethodCallMessage(o.destination, o.path, iface, method)
	if err := msg.AppendArgs(args...); err != nil {
		return nil, err
	}
	reply, err := o.bus.SendWithReply(msg)
	if err != nil {
		return nil, err
	}
	if reply.Type == TypeError {
		return nil, reply.AsError()
	}
	return reply, nil
}

func (o *ObjectProxy) WatchSignal(iface, member string, handler func(*Message)) (*SignalWatch, error) {
	return o.bus.WatchSignal(&MatchRule{
		Type: TypeSignal,
		Sender: o.destination,
		Path: o.path,
		Interface: iface,
		Member: member}, handler)
}

func Connect(busType StandardBus) (*Connection, error) {
	var address string

	switch busType {
	case SessionBus:
		address = os.Getenv("DBUS_SESSION_BUS_ADDRESS")

	case SystemBus:
		if address = os.Getenv("DBUS_SYSTEM_BUS_ADDRESS"); len(address) == 0 {
			address = "unix:path=/var/run/dbus/system_bus_socket"
		}

	default:
		return nil, errors.New("Unknown bus")
	}

	if len(address) == 0 {
		return nil, errors.New("Unknown bus address")
	}
	transport := address[:strings.Index(address, ":")]

	bus := new(Connection)
	bus.addressMap = make(map[string]string)
	for _, pair := range strings.Split(address[len(transport)+1:], ",") {
		pair := strings.Split(pair, "=")
		bus.addressMap[pair[0]] = pair[1]
	}

	var ok bool
	if address, ok = bus.addressMap["path"]; ok {
	} else if address, ok = bus.addressMap["abstract"]; ok {
		address = "@" + address
	} else {
		return nil, errors.New("Unknown address key")
	}

	var err error
	if bus.conn, err = net.Dial(transport, address); err != nil {
		return nil, err
	}

	if _, err = bus.conn.Write([]byte{0}); err != nil {
		return nil, err
	}

	bus.busProxy = BusDaemon{bus.Object(BUS_DAEMON_NAME, BUS_DAEMON_PATH)}

	bus.messageFilters = []*MessageFilter{}
	bus.methodCallReplies = make(map[uint32] chan<- *Message)
	bus.objectPathHandlers = make(map[ObjectPath] chan<- *Message)
	bus.signalMatchRules = make(signalWatchSet)

	bus.nameInfo = make(map[string] *nameInfo)

	return bus, nil
}

func (p *Connection) Authenticate() (err error) {
	if err = p._Authenticate(new(AuthDbusCookieSha1)); err != nil {
		return
	}
	go p._RunLoop()
	p.UniqueName, err = p.busProxy.Hello()
	return
}

func (p *Connection) _MessageReceiver(msgChan chan<- *Message) {
	for {
		msg, err := readMessage(p.conn)
		if err != nil {
			if err != io.EOF {
				log.Println("Failed to read message:", err)
			}
			break
		}
		msgChan <- msg
	}
	close(msgChan)
}

func (p *Connection) _RunLoop() {
	msgChan := make(chan *Message)
	go p._MessageReceiver(msgChan)
	for msg := range msgChan {
		p._MessageDispatch(msg)
	}
}

func (p *Connection) _MessageDispatch(msg *Message) {
	// Run the message through the registered filters, stopping
	// processing if a filter returns nil.
	for _, filter := range p.messageFilters {
		msg := filter.filter(msg)
		if msg == nil {
			return
		}
	}

	switch msg.Type {
	case TypeMethodCall:
		switch {
		case msg.Iface == "org.freedesktop.DBus.Peer" && msg.Member == "Ping":
			reply := NewMethodReturnMessage(msg)
			_ = p.Send(reply)
		case msg.Iface == "org.freedesktop.DBus.Peer" && msg.Member == "GetMachineId":
			// Should be returning the UUID found in /var/lib/dbus/machine-id
			fmt.Println("XXX: handle GetMachineId")
			reply := NewMethodReturnMessage(msg)
			_ = reply.AppendArgs("machine-id")
			_ = p.Send(reply)
		default:
			// XXX: need to lock the map
			p.handlerMutex.Lock()
			handler, ok := p.objectPathHandlers[msg.Path]
			p.handlerMutex.Unlock()
			if ok {
				handler <- msg
			} else {
				reply := NewErrorMessage(msg, "org.freedesktop.DBus.Error.UnknownObject", "Unknown object path " + string(msg.Path))
				_ = p.Send(reply)
			}
		}
	case TypeMethodReturn, TypeError:
		p.handlerMutex.Lock()
		rs := msg.replySerial
		replyChan, ok := p.methodCallReplies[rs]
		if ok {
			delete(p.methodCallReplies, rs)
		}
		p.handlerMutex.Unlock()
		if ok {
			replyChan <- msg
		}
	case TypeSignal:
		p.handlerMutex.Lock()
		watches := p.signalMatchRules.FindMatches(msg)
		p.handlerMutex.Unlock()
		for _, watch := range watches {
			watch.handler(msg)
		}
	}
}

func (p *Connection) Close() error {
	return p.conn.Close()
}

func (p *Connection) nextSerial() uint32 {
	return atomic.AddUint32(&p.lastSerial, 1)
}

func (p *Connection) Send(msg *Message) error {
	msg.setSerial(p.nextSerial())
	buff, err := msg._Marshal()
	if err != nil {
		return err
	}
	p.conn.Write(buff)
	return nil
}

func (p *Connection) SendWithReply(msg *Message) (*Message, error) {
	// XXX: also check for "no reply" flag.
	if msg.Type != TypeMethodCall {
		panic("Only method calls have replies")
	}
	serial := p.nextSerial()
	msg.setSerial(serial)
	buff, err := msg._Marshal()
	if err != nil {
		return nil, err
	}

	replyChan := make(chan *Message, 1)
	p.handlerMutex.Lock()
	p.methodCallReplies[serial] = replyChan
	p.handlerMutex.Unlock()

	p.conn.Write(buff)

	reply := <-replyChan
	return reply, nil
}

func (p *Connection) RegisterMessageFilter(filter func (*Message) *Message) *MessageFilter {
	msgFilter := &MessageFilter{filter}
	p.messageFilters = append(p.messageFilters, msgFilter)
	return msgFilter
}

func (p *Connection) UnregisterMessageFilter(filter *MessageFilter) {
	for i, other := range p.messageFilters {
		if other == filter {
			p.messageFilters = append(p.messageFilters[:i], p.messageFilters[i+1:]...)
			return
		}
	}
	panic("Message filter not registered to this bus")
}

func (p *Connection) RegisterObjectPath(path ObjectPath, handler chan<- *Message) {
	p.handlerMutex.Lock()
	defer p.handlerMutex.Unlock()
	if _, ok := p.objectPathHandlers[path]; ok {
		panic("A handler has already been registered for " + string(path))
	}
	p.objectPathHandlers[path] = handler
}

func (p *Connection) UnregisterObjectPath(path ObjectPath) {
	p.handlerMutex.Lock()
	defer p.handlerMutex.Unlock()
	if _, ok := p.objectPathHandlers[path]; !ok {
		panic("No handler registered for " + string(path))
	}
	delete(p.objectPathHandlers, path)
}

func (p *Connection) _GetIntrospect(dest string, path ObjectPath) Introspect {
	msg := NewMethodCallMessage(dest, path, "org.freedesktop.DBus.Introspectable", "Introspect")

	reply, err := p.SendWithReply(msg)
	if err != nil {
		return nil
	}
	if v, ok := reply.GetAllArgs()[0].(string); ok {
		if intro, err := NewIntrospect(v); err == nil {
			return intro
		}
	}
	return nil
}

// Retrieve a specified object.
func (p *Connection) Object(dest string, path ObjectPath) *ObjectProxy {
	return &ObjectProxy{p, dest, path}
}

// Handle received signals.
func (p *Connection) WatchSignal(rule *MatchRule, handler func(*Message)) (*SignalWatch, error) {
	if rule.Type != TypeSignal {
		return nil, errors.New("Match rule is not for signals")
	}
	watch := &SignalWatch{bus: p, rule: *rule, handler: handler}

	// Does the rule match a bus name other than the daemon?
	if rule.Sender != "" && rule.Sender != BUS_DAEMON_NAME {
		var nameHandler func(string)
		if rule.Sender[0] == ':' {
			// For unique names, cancel the signal watch
			// when the name is lost.
			nameHandler = func (newOwner string) {
				if newOwner == "" {
					watch.Cancel()
				}
			}
		} else {
			// Otherwise, update the sender owner.
			nameHandler = func (newOwner string) {
				watch.rule.senderNameOwner = newOwner
			}
		}
		nameWatch, err := p.WatchName(rule.Sender, nameHandler)
		if err != nil {
			return nil, err
		}
		watch.nameWatch = nameWatch
	}
	if err := p.busProxy.AddMatch(rule.String()); err != nil {
		watch.nameWatch.Cancel()
		return nil, err
	}

	p.handlerMutex.Lock()
	p.signalMatchRules.Add(watch)
	p.handlerMutex.Unlock()
	return watch, nil
}

func (watch *SignalWatch) Cancel() error {
	if watch.cancelled {
		return nil
	}
	watch.cancelled = true
	watch.bus.handlerMutex.Lock()
	foundMatch := watch.bus.signalMatchRules.Remove(watch)
	watch.bus.handlerMutex.Unlock()

	if foundMatch {
		if err := watch.bus.busProxy.RemoveMatch(watch.rule.String()); err != nil {
			return err
		}
		if watch.nameWatch != nil {
			if err := watch.nameWatch.Cancel(); err != nil {
				return err
			}
		}
	}
	return nil
}
