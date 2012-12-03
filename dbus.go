package dbus

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
)

type StandardBus int

const (
	SessionBus StandardBus = iota
	SystemBus
)

type SignalWatch struct {
	rule MatchRule
	proc func(*Message)
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

type Connection struct {
	addressMap         map[string]string
	uniqName           string
	conn               net.Conn
	buffer             *bytes.Buffer

	handlerMutex       sync.Mutex // covers the next three
	methodCallReplies  map[uint32]chan<- *Message
	objectPathHandlers map[ObjectPath]chan<- *Message
	signalMatchRules   signalWatchSet

	lastSerialMutex    sync.Mutex
	lastSerial         uint32
}

type Object struct {
	dest  string
	path  ObjectPath
	intro Introspect
}

type Interface struct {
	obj   *Object
	name  string
	intro InterfaceData
}

type Method struct {
	iface *Interface
	data  MethodData
}

type Signal struct {
	iface *Interface
	data  SignalData
}

// Retrieve a method by name.
func (iface *Interface) Method(name string) (*Method, error) {
	method := iface.intro.GetMethodData(name)
	if nil == method {
		return nil, errors.New("Invalid Method")
	}
	return &Method{iface, method}, nil
}

// Retrieve a signal by name.
func (iface *Interface) Signal(name string) (*Signal, error) {
	signal := iface.intro.GetSignalData(name)
	if nil == signal {
		return nil, errors.New("Invalid Signalx")
	}
	return &Signal{iface, signal}, nil
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

	bus.methodCallReplies = make(map[uint32]chan<- *Message)
	bus.objectPathHandlers = make(map[ObjectPath]chan<- *Message)
	bus.signalMatchRules = make(signalWatchSet)
	bus.buffer = bytes.NewBuffer([]byte{})
	return bus, nil
}

func (p *Connection) Authenticate() error {
	if err := p._Authenticate(new(AuthDbusCookieSha1)); err != nil {
		return err
	}
	go p._RunLoop()
	p._SendHello()
	return nil
}

func (p *Connection) _MessageReceiver(msgChan chan *Message) {
	for {
		msg, err := readMessage(p.conn)
		switch err {
		case nil:
			msgChan <- msg
		case io.EOF:
			break
		default:
			log.Println("Failed to read message:", err)
		}
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
			watch.proc(msg)
		}
	}
}

func (p *Connection) nextSerial() (serial uint32) {
	p.lastSerialMutex.Lock()
	p.lastSerial++
	serial = p.lastSerial
	p.lastSerialMutex.Unlock()
	return
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

	replyChan := make(chan *Message)
	p.handlerMutex.Lock()
	p.methodCallReplies[serial] = replyChan
	p.handlerMutex.Unlock()

	p.conn.Write(buff)

	reply := <-replyChan
	return reply, nil
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

func (p *Connection) _SendHello() error {
	method := NewMethodCallMessage("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "Hello")
	reply, err := p.SendWithReply(method)
	if err != nil {
		return err
	}
	if reply.Type == TypeError {
		return reply.AsError()
	}
	if err := reply.GetArgs(&p.uniqName); err != nil {
		return err
	}
	return nil
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

// Retrieve an interface by name.
func (obj *Object) Interface(name string) *Interface {
	if obj == nil || obj.intro == nil {
		return nil
	}

	iface := new(Interface)
	iface.obj = obj
	iface.name = name

	data := obj.intro.GetInterfaceData(name)
	if nil == data {
		return nil
	}

	iface.intro = data

	return iface
}

// Call a method with the given arguments.
func (p *Connection) Call(method *Method, args ...interface{}) ([]interface{}, error) {
	iface := method.iface
	msg := NewMethodCallMessage(iface.obj.dest, iface.obj.path, iface.name, method.data.GetName())
	if len(args) > 0 {
		if err := msg.AppendArgs(args...); err != nil {
			return nil, err
		}
	}

	reply, err := p.SendWithReply(msg)
	if err != nil {
		return nil, err
	}
	if reply.Type == TypeError {
		return nil, reply.AsError()
	}
	return reply.GetAllArgs(), nil
}

// Emit a signal with the given arguments.
func (p *Connection) Emit(signal *Signal, args ...interface{}) error {
	iface := signal.iface

	msg := NewSignalMessage(iface.obj.path, iface.name, signal.data.GetName())
	msg.Dest = iface.obj.dest
	if err := msg.AppendArgs(args...); err != nil {
		return err
	}

	return p.Send(msg)
}

// Retrieve a specified object.
func (p *Connection) Object(dest string, path ObjectPath) *Object {

	obj := new(Object)
	obj.path = path
	obj.dest = dest
	obj.intro = p._GetIntrospect(dest, path)

	return obj
}

// Handle received signals.
func (p *Connection) WatchSignal(rule *MatchRule, handler func(*Message)) (*SignalWatch, error) {
	if rule.Type != TypeSignal {
		return nil, errors.New("Match rule is not for signals")
	}
	method := NewMethodCallMessage("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "AddMatch")
	method.AppendArgs(rule.String())
	if reply, err := p.SendWithReply(method); err != nil {
		return nil, err
	} else if reply.Type == TypeError {
		return nil, reply.AsError()
	}

	watch := &SignalWatch{*rule, handler}
	p.handlerMutex.Lock()
	p.signalMatchRules.Add(watch)
	p.handlerMutex.Unlock()
	return watch, nil
}

func (p *Connection) UnwatchSignal(watch *SignalWatch) error {
	p.handlerMutex.Lock()
	foundMatch := p.signalMatchRules.Remove(watch)
	p.handlerMutex.Unlock()

	if foundMatch {
		method := NewMethodCallMessage("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "RemoveMatch")
		method.AppendArgs(watch.rule.String())
		if reply, err := p.SendWithReply(method); err != nil {
			return err
		} else if reply.Type == TypeError {
			return reply.AsError()
		}
	}
	return nil
}
