package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"reflect"
)

type encoder struct {
	signature Signature
	data bytes.Buffer
	offset int
}

func (self *encoder) align(alignment int) {
	for (self.data.Len() + self.offset) % alignment != 0 {
		self.data.WriteByte(0)
	}
}

func (self *encoder) Append(args ...interface{}) error {
	for _, arg := range args {
		if err := self.appendValue(reflect.ValueOf(arg)); err != nil {
			return err
		}
	}
	return nil
}

func (self *encoder) appendValue(v reflect.Value) error {
	signature, err := getSignature(v.Type())
	if err != nil {
		return err
	}
	self.signature += signature

	// Convert HasObjectPath values to ObjectPath strings
	if v.Type().AssignableTo(typeHasObjectPath) {
		path := v.Interface().(HasObjectPath).GetObjectPath()
		v = reflect.ValueOf(path)
	}

	// We want pointer values here, rather than the pointers themselves.
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Uint8:
		self.align(1)
		self.data.WriteByte(byte(v.Uint()))
		return nil
	case reflect.Bool:
		self.align(4)
		var uintval uint32
		if v.Bool() {
			uintval = 1
		} else {
			uintval = 0
		}
		binary.Write(&self.data, binary.LittleEndian, uintval)
		return nil
	case reflect.Int16:
		self.align(2)
		binary.Write(&self.data, binary.LittleEndian, int16(v.Int()))
		return nil
	case reflect.Uint16:
		self.align(2)
		binary.Write(&self.data, binary.LittleEndian, uint16(v.Uint()))
		return nil
	case reflect.Int32:
		self.align(4)
		binary.Write(&self.data, binary.LittleEndian, int32(v.Int()))
		return nil
	case reflect.Uint32:
		self.align(4)
		binary.Write(&self.data, binary.LittleEndian, uint32(v.Uint()))
		return nil
	case reflect.Int64:
		self.align(8)
		binary.Write(&self.data, binary.LittleEndian, int64(v.Int()))
		return nil
	case reflect.Uint64:
		self.align(8)
		binary.Write(&self.data, binary.LittleEndian, uint64(v.Uint()))
		return nil
	case reflect.Float64:
		self.align(8)
		binary.Write(&self.data, binary.LittleEndian, float64(v.Float()))
		return nil
	case reflect.String:
		s := v.String()
		// Signatures only use a single byte for the length.
		if v.Type() == typeSignature {
			self.align(1)
			self.data.WriteByte(byte(len(s)))
		} else {
			self.align(4)
			binary.Write(&self.data, binary.LittleEndian, uint32(len(s)))
		}
		self.data.Write([]byte(s))
		self.data.WriteByte(0)
		return nil
	case reflect.Array, reflect.Slice:
		// Marshal array contents to a separate buffer so we
		// can find its length.
		var content encoder
		// Offset alignment by current data and length field
		content.offset = self.data.Len() + 4
		for i := 0; i < v.Len(); i++ {
			if err := content.appendValue(v.Index(i)); err != nil {
				return err
			}
		}
		self.align(4)
		binary.Write(&self.data, binary.LittleEndian, uint32(content.data.Len()))
		self.data.Write(content.data.Bytes())
		return nil
	case reflect.Map:
		// Marshal array contents to a separate buffer so we
		// can find its length.
		var content encoder
		// Offset alignment by current data and length field
		content.offset = self.data.Len() + 4
		for _, key := range v.MapKeys() {
			content.align(8)
			if err := content.appendValue(key); err != nil {
				return err
			}
			if err := content.appendValue(v.MapIndex(key)); err != nil {
				return err
			}
		}
		self.align(4)
		binary.Write(&self.data, binary.LittleEndian, uint32(content.data.Len()))
		self.data.Write(content.data.Bytes())
		return nil
	case reflect.Struct:
		if v.Type() == typeVariant {
			variant := v.Interface().(Variant)
			variantSig, err := variant.GetVariantSignature()
			if err != nil {
				return err
			}
			// Save the signature, so we don't add the
			// typecodes for the variant value to the
			// signature.
			savedSig := self.signature
			if err := self.appendValue(reflect.ValueOf(variantSig)); err != nil {
				return err
			}
			if err := self.appendValue(reflect.ValueOf(variant.Value)); err != nil {
				return err
			}
			self.signature = savedSig
			return nil
		}
		self.align(4)
		// XXX: save and restore the signature, since we wrote
		// out the entire struct signature previously.
		savedSig := self.signature
		for i := 0; i != v.NumField(); i++ {
			if err := self.appendValue(v.Field(i)); err != nil {
				return err
			}
		}
		self.signature = savedSig
		return nil
	}
	return errors.New("Could not marshal " + v.Type().String())
}


type decoder struct {
	signature string
	data []byte
	order binary.ByteOrder

	dataOffset, sigOffset int
}

var (
	bufferOverrunError = errors.New("Buffer too small")
	signatureOverrunError = errors.New("Signature too small"))

func newDecoder(signature string, data []byte, order binary.ByteOrder) *decoder {
	return &decoder{signature: signature, data: data, order: order}
}

func (self *decoder) align(alignment int) {
	for self.dataOffset % alignment != 0 {
		self.dataOffset += 1
	}
}

func (self *decoder) Decode(args ...interface{}) error {
	for _, arg := range args {
		v := reflect.ValueOf(arg)
		// We expect to be given pointers here, so the caller
		// can see the decoded values.
		if v.Kind() != reflect.Ptr {
			return errors.New("arguments to Decode should be pointers")
		}
		if err := self.decodeValue(v.Elem()); err != nil {
			return err
		}
	}
	return nil
}

func (self *decoder) readByte() (byte, error) {
	if len(self.data) < self.dataOffset + 1 {
		return 0, bufferOverrunError
	}
	value := self.data[self.dataOffset]
	self.dataOffset += 1
	return value, nil
}

func (self *decoder) readInt16() (int16, error) {
	self.align(2)
	if len(self.data) < self.dataOffset + 2 {
		return 0, bufferOverrunError
	}
	value := int16(self.order.Uint16(self.data[self.dataOffset:]))
	self.dataOffset += 2
	return value, nil
}

func (self *decoder) readUint16() (uint16, error) {
	self.align(2)
	if len(self.data) < self.dataOffset + 2 {
		return 0, bufferOverrunError
	}
	value := self.order.Uint16(self.data[self.dataOffset:])
	self.dataOffset += 2
	return value, nil
}

func (self *decoder) readInt32() (int32, error) {
	self.align(4)
	if len(self.data) < self.dataOffset + 4 {
		return 0, bufferOverrunError
	}
	value := int32(self.order.Uint32(self.data[self.dataOffset:]))
	self.dataOffset += 4
	return value, nil
}

func (self *decoder) readUint32() (uint32, error) {
	self.align(4)
	if len(self.data) < self.dataOffset + 4 {
		return 0, bufferOverrunError
	}
	value := self.order.Uint32(self.data[self.dataOffset:])
	self.dataOffset += 4
	return value, nil
}

func (self *decoder) readInt64() (int64, error) {
	self.align(8)
	if len(self.data) < self.dataOffset + 8 {
		return 0, bufferOverrunError
	}
		value := int64(self.order.Uint64(self.data[self.dataOffset:]))
	self.dataOffset += 8
	return value, nil
}

func (self *decoder) readUint64() (uint64, error) {
	self.align(8)
	if len(self.data) < self.dataOffset + 8 {
		return 0, bufferOverrunError
	}
	value := self.order.Uint64(self.data[self.dataOffset:])
	self.dataOffset += 8
	return value, nil
}

func (self *decoder) readFloat64() (float64, error) {
	value, err := self.readUint64()
	return math.Float64frombits(value), err
}

func (self *decoder) readString() (string, error) {
	length, err := self.readUint32()
	if err != nil {
		return "", err
	}
	// One extra byte for null termination
	if len(self.data) < self.dataOffset + int(length) + 1 {
		return "", bufferOverrunError
	}
	value := string(self.data[self.dataOffset:self.dataOffset+int(length)])
	self.dataOffset += int(length) + 1
	return value, nil
}

func (self *decoder) readSignature() (string, error) {
	length, err := self.readByte()
	if err != nil {
		return "", err
	}
	// One extra byte for null termination
	if len(self.data) < self.dataOffset + int(length) + 1 {
		return "", bufferOverrunError
	}
	value := string(self.data[self.dataOffset:self.dataOffset+int(length)])
	self.dataOffset += int(length) + 1
	return value, nil
}

func (self *decoder) decodeValue(v reflect.Value) error {
	if len(self.signature) < self.sigOffset {
		return signatureOverrunError
	}
	sigCode := self.signature[self.sigOffset]
	self.sigOffset += 1
	switch sigCode {
	case 'y':
		value, err := self.readByte()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.Uint8, reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 'b':
		value, err := self.readUint32()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.Bool:
			v.SetBool(value != 0)
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value != 0))
			return nil
		}
	case 'n':
		value, err := self.readInt16()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.Int16:
			v.SetInt(int64(value))
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 'q':
		value, err := self.readUint16()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.Uint16:
			v.SetUint(uint64(value))
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 'i':
		value, err := self.readInt32()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.Int32:
			v.SetInt(int64(value))
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 'u':
		value, err := self.readUint32()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.Uint32:
			v.SetUint(uint64(value))
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 'x':
		value, err := self.readInt64()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.Int64:
			v.SetInt(int64(value))
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 't':
		value, err := self.readUint64()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.Uint32:
			v.SetUint(uint64(value))
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 'd':
		value, err := self.readFloat64()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.Float64:
			v.SetFloat(value)
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 's', 'o':
		value, err := self.readString()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.String:
			v.SetString(value)
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 'g':
		value, err := self.readSignature()
		if err != nil {
			return err
		}
		switch v.Kind() {
		case reflect.String:
			v.SetString(value)
			return nil
		case reflect.Interface:
			v.Set(reflect.ValueOf(value))
			return nil
		}
	case 'a':
		// XXX: Need to support maps here (i.e. next signature
		// char is '{')
		length, err := self.readUint32()
		if err != nil {
			return err
		}
		elemSigOffset := self.sigOffset
		arrayEnd := self.dataOffset + int(length)
		if len(self.data) < arrayEnd {
			return bufferOverrunError
		}
		switch v.Kind() {
		case reflect.Array:
			for i := 0; self.dataOffset < arrayEnd; i++ {
				// Reset signature offset to the array element.
				self.sigOffset = elemSigOffset
				if err := self.decodeValue(v.Index(i)); err != nil {
					return err
				}
			}
			return nil
		case reflect.Slice:
			if v.IsNil() {
				v.Set(reflect.MakeSlice(v.Type(), 0, 0))
			}
			v.SetLen(0)
			for self.dataOffset < arrayEnd {
				// Reset signature offset to the array element.
				self.sigOffset = elemSigOffset
				elem := reflect.New(v.Type().Elem()).Elem()
				if err := self.decodeValue(elem); err != nil {
					return err
				}
				v.Set(reflect.Append(v, elem))
			}
			return nil
		case reflect.Interface:
			array := make([]interface{}, 0)
			for self.dataOffset < arrayEnd {
				// Reset signature offset to the array element.
				self.sigOffset = elemSigOffset
				var elem interface{}
				if err := self.decodeValue(reflect.ValueOf(&elem).Elem()); err != nil {
					return err
				}
				array = append(array, elem)
			}
			v.Set(reflect.ValueOf(array))
			return nil
		}
	}
	return errors.New("Could not decode " + string(sigCode) + " to " + v.Type().String())
}
