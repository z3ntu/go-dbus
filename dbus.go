package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
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


// ---- Remaining functions from old marshaller ----
func _Align(length int, index int) int {
	switch length {
	case 1:
		return index
	case 2, 4, 8:
		bit := length - 1
		return ^bit & (index + bit)
	}
	// default
	return -1
}

func _GetInt32(buff []byte, index int) (int32, error) {
	if len(buff) <= index+4-1 {
		return 0, errors.New("index error")
	}
	var l int32
	e := binary.Read(bytes.NewBuffer(buff[index:len(buff)]), binary.LittleEndian, &l)
	if e != nil {
		return 0, e
	}
	return l, nil
}

// ----------------

type signalHandler struct {
	mr   MatchRule
	proc func(*Message)
}

type Connection struct {
	addressMap         map[string]string
	uniqName           string
	conn               net.Conn
	buffer             *bytes.Buffer

	handlerMutex       sync.Mutex // covers the next three
	methodCallReplies  map[uint32]chan<- *Message
	objectPathHandlers map[ObjectPath]chan<- *Message
	signalMatchRules   []signalHandler

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
	bus.signalMatchRules = make([]signalHandler, 0)
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
		e := p._FillBuffer()
		if e != nil {
			continue
		}
		msg, e := p._PopMessage()
		if e == nil {
			msgChan <- msg
			continue // might be another msg in p.buffer
		}
	}
}

func (p *Connection) _RunLoop() {
	msgChan := make(chan *Message)
	go p._MessageReceiver(msgChan)
	for {
		select {
		case msg := <-msgChan:
			p._MessageDispatch(msg)
		}
	}
}

func (p *Connection) _MessageDispatch(msg *Message) {
	if msg == nil {
		return
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
		// XXX: grab handlerMutex when looking up signal handler.
		for _, handler := range p.signalMatchRules {
			if handler.mr._Match(msg) {
				handler.proc(msg)
			}
		}
	}
}

func (p *Connection) _PopMessage() (*Message, error) {
	msg, err := _Unmarshal(p.buffer.Bytes())
	if err != nil {
		return nil, err
	}
	//	p.buffer.Read(make([]byte, n)) // remove first n bytes
	p.buffer.Reset()
	return msg, nil
}

func (p *Connection) _FillBuffer() error {
	// Read header signature
	headSig := make([]byte, 16)
	n, e := p.conn.Read(headSig)
	if n != 16 {
		return e
	}
	// Calculate whole message length
	bodyLength, _ := _GetInt32(headSig, 4)
	arrayLength, _ := _GetInt32(headSig, 12)
	headerLen := 16 + int(arrayLength)
	pad := _Align(8, headerLen) - headerLen
	restOfMsg := make([]byte, pad+int(arrayLength)+int(bodyLength))
	n, e = p.conn.Read(restOfMsg)

	if n != len(restOfMsg) {
		return e
	}

	p.buffer.Write(headSig)
	p.buffer.Write(restOfMsg)
	return e
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
func (p *Connection) Handle(rule *MatchRule, handler func(*Message)) {
	method := NewMethodCallMessage("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "AddMatch")
	method.AppendArgs(rule.String())

	reply, err := p.SendWithReply(method)
	if err != nil {
		panic(err)
	}
	if reply.Type == TypeError {
		panic(reply.AsError())
	}
}
