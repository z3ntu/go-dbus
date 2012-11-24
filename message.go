package dbus

import (
	"encoding/binary"
	"errors"
	"reflect"
)

// See the D-Bus tutorial for information about message types.
//		http://dbus.freedesktop.org/doc/dbus-tutorial.html#messages
type MessageType uint8

const (
	TypeInvalid MessageType = iota
	TypeMethodCall
	TypeMethodReturn
	TypeError
	TypeSignal
)

var messageTypeString = map[MessageType]string{
	TypeInvalid:      "invalid",
	TypeMethodCall:   "method_call",
	TypeMethodReturn: "method_return",
	TypeSignal:       "signal",
	TypeError:        "error",
}

func (t MessageType) String() string { return messageTypeString[t] }

type MessageFlag uint8

const (
	FlagNoReplyExpected MessageFlag = 1 << iota
	FlagNoAutoStart
)

type Message struct {
	Type        MessageType
	Flags       MessageFlag
	Protocol    int
	bodyLength  int
	Path        ObjectPath
	Dest        string
	Iface       string
	Member      string
	sig         Signature
	params      []interface{}
	serial      uint32
	replySerial uint32
	ErrorName   string
	Sender      string
}

// Create a new message with Flags == 0 and Protocol == 1.
func newMessage() *Message {
	msg := new(Message)

	msg.serial = 0
	msg.replySerial = 0
	msg.Flags = 0
	msg.Protocol = 1

	msg.params = make([]interface{}, 0)

	return msg
}

func NewMethodCallMessage(destination string, path ObjectPath, iface string, member string) *Message {
	msg := newMessage()
	msg.Type = TypeMethodCall
	msg.Dest = destination
	msg.Path = path
	msg.Iface = iface
	msg.Member = member
	return msg
}

func NewMethodReturnMessage(methodCall *Message) *Message {
	if methodCall.serial == 0 {
		panic("methodCall.serial == 0")
	}
	if methodCall.Type != TypeMethodCall {
		panic("replies should be sent in response to method calls")
	}
	msg := newMessage()
	msg.Type = TypeMethodReturn
	msg.replySerial = methodCall.serial
	msg.Dest = methodCall.Sender
	return msg
}

func NewSignalMessage(path ObjectPath, iface string, member string) *Message {
	msg := newMessage()
	msg.Type = TypeSignal
	msg.Path = path
	msg.Iface = iface
	msg.Member = member
	return msg
}

func NewErrorMessage(methodCall *Message, errorName string, message string) *Message {
	if methodCall.serial == 0 {
		panic("methodCall.serial == 0")
	}
	if methodCall.Type != TypeMethodCall {
		panic("errors should be sent in response to method calls")
	}
	msg := newMessage()
	msg.Type = TypeError
	msg.replySerial = methodCall.serial
	msg.Dest = methodCall.Sender
	msg.ErrorName = errorName
	if message != "" {
		msg.sig = "s"
		msg.params = []interface{}{message}
	}
	return msg
}

func (p *Message) setSerial(serial uint32) {
	if p.serial != 0 {
		panic("Message already has a serial number")
	}
	p.serial = serial
}

func (p *Message) Append(args ...interface{}) error {
	p.params = append(p.params, args...)
	for _, arg := range(args) {
		argSig, err := getSignature(reflect.ValueOf(arg).Type())
		if err != nil {
			return err
		}
		p.sig += argSig
	}
	return nil
}

func (p *Message) GetArgs() []interface{} {
	return p.params
}

type headerField struct {
	Code byte
	Value Variant
}

func (p *Message) _BufferToMessage(buff []byte) (int, error) {
	if len(buff) < 16 {
		return 0, errors.New("Message buffer too short")
	}
	var order binary.ByteOrder
	switch buff[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return 0, errors.New("Unknown message endianness: " + string(buff[0]))
	}
	dec := newDecoder("yyyyuua(yv)", buff, order)
	var msgOrder, msgType, msgFlags, msgProtocol byte
	var msgBodyLength, msgSerial uint32
	fields := make([]headerField, 0, 10)
	if err := dec.Decode(&msgOrder, &msgType, &msgFlags, &msgProtocol, &msgBodyLength, &msgSerial, &fields); err != nil {
		return 0, err
	}
	p.Type = MessageType(msgType)
	p.Flags = MessageFlag(msgFlags)
	p.Protocol = int(msgProtocol)
	p.bodyLength = int(msgBodyLength)
	p.serial = msgSerial

	for _, field := range fields {
		switch field.Code {
		case 1:
			p.Path = field.Value.Value.(ObjectPath)
		case 2:
			p.Iface = field.Value.Value.(string)
		case 3:
			p.Member = field.Value.Value.(string)
		case 4:
			p.ErrorName = field.Value.Value.(string)
		case 5:
			p.replySerial = field.Value.Value.(uint32)
		case 6:
			p.Dest = field.Value.Value.(string)
		case 7:
			p.Sender = field.Value.Value.(string)
		case 8:
			p.sig = field.Value.Value.(Signature)
		}
	}

	dec.align(8)
	if 0 < p.bodyLength {
		dec.signature = Signature(p.sig)
		dec.sigOffset = 0
		for dec.HasMore() {
			var param interface{}
			if err := dec.Decode(&param); err != nil {
				return 0, err
			}
			p.params = append(p.params, param)
		}
	}
	idx := dec.dataOffset
	return idx, nil
}

func _Unmarshal(buff []byte) (*Message, int, error) {
	msg := newMessage()
	idx, e := msg._BufferToMessage(buff)
	if e != nil {
		return nil, 0, e
	}
	return msg, idx, nil
}

func (p *Message) _Marshal() ([]byte, error) {
	var body encoder
	if err := body.Append(p.params...); err != nil {
		return nil, err
	}

	// encode optional fields
	fields := make([]headerField, 0, 10)
	if p.Path != "" {
		fields = append(fields, headerField{1, Variant{p.Path}})
	}
	if p.Iface != "" {
		fields = append(fields, headerField{2, Variant{p.Iface}})
	}
	if p.Member != "" {
		fields = append(fields, headerField{3, Variant{p.Member}})
	}
	if p.replySerial != 0 {
		fields = append(fields, headerField{5, Variant{p.replySerial}})
	}
	if p.Dest != "" {
		fields = append(fields, headerField{6, Variant{p.Dest}})
	}
	if p.Sender != "" {
		fields = append(fields, headerField{7, Variant{p.Sender}})
	}
	if p.sig != "" {
		fields = append(fields, headerField{8, Variant{p.sig}})
	}

	var message encoder
	if err := message.Append(byte('l'), byte(p.Type), byte(p.Flags), byte(p.Protocol), uint32(body.data.Len()), p.serial, fields); err != nil {
		return nil, err
	}

	// append the body
	message.align(8)
	message.data.Write(body.data.Bytes())

	return message.data.Bytes(), nil
}
