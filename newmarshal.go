package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
)


type encoder struct {
	signature string
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

func _getSignature(t reflect.Type) (string, error) {
	switch t.Kind() {
	case reflect.Uint8:
		return "y", nil
	case reflect.Bool:
		return "b", nil
	case reflect.Int16:
		return "n", nil
	case reflect.Uint16:
		return "q", nil
	case reflect.Int32:
		return "i", nil
	case reflect.Uint32:
		return "u", nil
	case reflect.Int64:
		return "x", nil
	case reflect.Uint64:
		return "t", nil
	case reflect.Float64:
		return "d", nil
	case reflect.String:
		// XXX: have some way to detect ObjectPath (o) and Signature (g)
		return "s", nil
	case reflect.Array, reflect.Slice:
		valueSig, err := _getSignature(t.Elem())
		if err != nil {
			return "", err
		}
		return "a" + valueSig, nil
	case reflect.Map:
		keySig, err := _getSignature(t.Key())
		if err != nil {
			return "", err
		}
		valueSig, err := _getSignature(t.Elem())
		if err != nil {
			return "", err
		}
		return "a{" + keySig + valueSig + "}", nil
	case reflect.Struct:
		sig := "("
		for i := 0; i != t.NumField(); i++ {
			fieldSig, err := _getSignature(t.Field(i).Type)
			if err != nil {
				return "", err
			}
			sig += fieldSig
		}
		sig += ")"
		return sig, nil
	case reflect.Ptr:
		// dereference pointers
		sig, err := _getSignature(t.Elem())
		return sig, err
	}
	return "", errors.New("Can not determine signature for " + t.String())
}

func (self *encoder) appendValue(v reflect.Value) error {
	signature, err := _getSignature(v.Type())
	if err != nil {
		return err
	}
	self.signature += signature
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
		self.align(4)
		s := v.String()
		binary.Write(&self.data, binary.LittleEndian, uint32(len(s)))
		self.data.Write([]byte(s))
		self.data.WriteByte(0)
		// XXX: Should handle signatures here, which have
		// slightly different encoding.
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
		self.align(4)
		// XXX: save and restore the signature, since we wrote
		// out the entire struct signature previously.
		saveSig := self.signature
		for i := 0; i != v.NumField(); i++ {
			if err := self.appendValue(v.Field(i)); err != nil {
				return err
			}
		}
		self.signature = saveSig
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

func newDecoder(signature string, data []byte, order binary.ByteOrder) *decoder {
	return &decoder{signature: signature, data: data, order: order}
}

func (self *decoder) align(alignment int) {
	for self.dataOffset & alignment != 0 {
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

func (self *decoder) decodeValue(v reflect.Value) error {
	return nil
}
