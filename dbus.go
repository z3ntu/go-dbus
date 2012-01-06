package dbus

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
)

const dbusXMLIntro = `
<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN"
"http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
  <interface name="org.freedesktop.DBus.Introspectable">
    <method name="Introspect">
      <arg name="data" direction="out" type="s"/>
    </method>
  </interface>
  <interface name="org.freedesktop.DBus">
    <method name="RequestName">
      <arg direction="in" type="s"/>
      <arg direction="in" type="u"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="ReleaseName">
      <arg direction="in" type="s"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="StartServiceByName">
      <arg direction="in" type="s"/>
      <arg direction="in" type="u"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="Hello">
      <arg direction="out" type="s"/>
    </method>
    <method name="NameHasOwner">
      <arg direction="in" type="s"/>
      <arg direction="out" type="b"/>
    </method>
    <method name="ListNames">
      <arg direction="out" type="as"/>
    </method>
    <method name="ListActivatableNames">
      <arg direction="out" type="as"/>
    </method>
    <method name="AddMatch">
      <arg direction="in" type="s"/>
    </method>
    <method name="RemoveMatch">
      <arg direction="in" type="s"/>
    </method>
    <method name="GetNameOwner">
      <arg direction="in" type="s"/>
      <arg direction="out" type="s"/>
    </method>
    <method name="ListQueuedOwners">
      <arg direction="in" type="s"/>
      <arg direction="out" type="as"/>
    </method>
    <method name="GetConnectionUnixUser">
      <arg direction="in" type="s"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="GetConnectionUnixProcessID">
      <arg direction="in" type="s"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="GetConnectionSELinuxSecurityContext">
      <arg direction="in" type="s"/>
      <arg direction="out" type="ay"/>
    </method>
    <method name="ReloadConfig">
    </method>
    <signal name="NameOwnerChanged">
      <arg type="s"/>
      <arg type="s"/>
      <arg type="s"/>
    </signal>
    <signal name="NameLost">
      <arg type="s"/>
    </signal>
    <signal name="NameAcquired">
      <arg type="s"/>
    </signal>
  </interface>
</node>`

type signalHandler struct {
	mr   MatchRule
	proc func(*Message)
}

type Connection struct {
	path              string
	uniqName          string
	guid              string
	methodCallReplies map[uint32](func(msg *Message))
	signalMatchRules  []signalHandler
	conn              net.Conn
	buffer            *bytes.Buffer
	proxy             *Interface
}

type Object struct {
	dest  string
	path  string
	intro Introspect
}

type Interface struct {
	obj   *Object
	name  string
	intro InterfaceData
}

func NewSessionBus() (*Connection, error) {
	bus := new(Connection)
	bus.path = os.Getenv("DBUS_SESSION_BUS_ADDRESS")

	re, _ := regexp.Compile("^unix:abstract=(.*),guid=(.*)")

	m := re.FindAllStringSubmatch(bus.path, -1)
	if nil != m {
		abPath := m[0][1] // get regexp 1st group
		addr, err := net.ResolveUnixAddr("unix", "@"+abPath)
		if err != nil {
			return nil, err
		}
		conn, err := net.DialUnix("unix", nil, addr)
		if err != nil {
			return nil, err
		}
		bus.conn = conn
		return bus, nil
	}

	return nil, errors.New("NewSessionBus Failed")
}

func NewSystemBus() (*Connection, error) {
	bus := new(Connection)
	bus.path = "unix:path=/var/run/dbus/system_bus_socket"

	addr, _ := net.ResolveUnixAddr("unix", "/var/run/dbus/system_bus_socket")
	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return nil, err
	}
	bus.conn = conn
	return bus, nil
}

func (p *Connection) Initialize() error {
	p.methodCallReplies = make(map[uint32]func(*Message))
	p.signalMatchRules = make([]signalHandler, 0)
	p.proxy = p._GetProxy()
	p.buffer = bytes.NewBuffer([]byte{})
	err := p._Auth()
	if err != nil {
		return err
	}
	go p._RunLoop()
	p._SendHello()
	return nil
}

func (p *Connection) _Auth() error {
	auth := new(authState)
	auth.AddAuthenticator(new(AuthExternal))

	return auth.Authenticate(p.conn)
}

func (p *Connection) _MessageReceiver(msgChan chan *Message) {
	for {
		msg, e := p._PopMessage()
		if e == nil {
			msgChan <- msg
			continue // might be another msg in p.buffer
		}
		p._UpdateBuffer()
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
	case METHOD_RETURN:
		rs := msg.replySerial
		if replyFunc, ok := p.methodCallReplies[rs]; ok {
			replyFunc(msg)
			delete(p.methodCallReplies, rs)
		}
	case SIGNAL:
		for _, handler := range p.signalMatchRules {
			if handler.mr._Match(msg) {
				handler.proc(msg)
			}
		}
	case ERROR:
		fmt.Println("ERROR")
	}
}

func (p *Connection) _PopMessage() (*Message, error) {
	msg, n, err := _Unmarshal(p.buffer.Bytes())
	if err != nil {
		return nil, err
	}
	p.buffer.Read(make([]byte, n)) // remove first n bytes
	return msg, nil
}

func (p *Connection) _UpdateBuffer() error {
	//	_, e := p.buffer.ReadFrom(p.conn);
	buff := make([]byte, 4096)
	n, e := p.conn.Read(buff)
	p.buffer.Write(buff[0:n])
	return e
}

func (p *Connection) _SendSync(msg *Message, callback func(*Message)) error {
	seri := uint32(msg.serial)
	recvChan := make(chan int)
	p.methodCallReplies[seri] = func(rmsg *Message) {
		callback(rmsg)
		recvChan <- 0
	}

	buff, _ := msg._Marshal()
	p.conn.Write(buff)
	<-recvChan // synchronize
	return nil
}

func (p *Connection) _SendHello() error {
	p.CallMethod(p.proxy, "Hello")
	return nil
}

func (p *Connection) _GetIntrospect(dest string, path string) Introspect {
	msg := NewMessage()
	msg.Type = METHOD_CALL
	msg.Path = path
	msg.Dest = dest
	msg.Iface = "org.freedesktop.DBus.Introspectable"
	msg.Member = "Introspect"

	var intro Introspect

	p._SendSync(msg, func(reply *Message) {
		if v, ok := reply.Params[0].(string); ok {
			if i, err := NewIntrospect(v); err == nil {
				intro = i
			}
		}
	})

	return intro
}

func (p *Connection) Interface(obj *Object, name string) *Interface {

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

func (p *Connection) _GetProxy() *Interface {
	obj := new(Object)
	obj.path = "/org/freedesktop/DBus"
	obj.dest = "org.freedesktop.DBus"
	obj.intro, _ = NewIntrospect(dbusXMLIntro)

	iface := new(Interface)
	iface.obj = obj
	iface.name = "org.freedesktop.DBus"
	iface.intro = obj.intro.GetInterfaceData("org.freedesktop.DBus")

	return iface
}

func (p *Connection) CallMethod(iface *Interface, name string, args ...interface{}) ([]interface{}, error) {
	method := iface.intro.GetMethodData(name)
	if nil == method {
		return nil, errors.New("Invalid Method")
	}

	msg := NewMessage()

	msg.Type = METHOD_CALL
	msg.Path = iface.obj.path
	msg.Iface = iface.name
	msg.Dest = iface.obj.dest
	msg.Member = name
	msg.Sig = method.GetInSignature()
	if len(args) > 0 {
		msg.Params = args[:]
	}

	var ret []interface{}
	p._SendSync(msg, func(reply *Message) {
		ret = reply.Params
	})

	return ret, nil
}

func (p *Connection) EmitSignal(iface *Interface, name string, args ...interface{}) error {

	signal := iface.intro.GetSignalData(name)
	if nil == signal {
		return errors.New("Invalid Signalx")
	}

	msg := NewMessage()

	msg.Type = SIGNAL
	msg.Path = iface.obj.path
	msg.Iface = iface.name
	msg.Dest = iface.obj.dest
	msg.Member = name
	msg.Sig = signal.GetSignature()
	msg.Params = args[:]

	buff, _ := msg._Marshal()
	_, err := p.conn.Write(buff)

	return err
}

func (p *Connection) GetObject(dest string, path string) *Object {

	obj := new(Object)
	obj.path = path
	obj.dest = dest
	obj.intro = p._GetIntrospect(dest, path)

	return obj
}

func (p *Connection) AddSignalHandler(mr *MatchRule, proc func(*Message)) {
	p.signalMatchRules = append(p.signalMatchRules, signalHandler{*mr, proc})
	p.CallMethod(p.proxy, "AddMatch", mr._ToString())
}
