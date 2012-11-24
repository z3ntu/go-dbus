package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
)

type encoder struct {
	signature Signature
	data bytes.Buffer
	order binary.ByteOrder
	offset int
}

func newEncoder(signature Signature, data []byte, order binary.ByteOrder) *encoder {
	enc := &encoder{signature: signature, order: order}
	if data != nil {
		enc.data.Write(data)
	}
	return enc
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
		binary.Write(&self.data, self.order, uintval)
		return nil
	case reflect.Int16:
		self.align(2)
		binary.Write(&self.data, self.order, int16(v.Int()))
		return nil
	case reflect.Uint16:
		self.align(2)
		binary.Write(&self.data, self.order, uint16(v.Uint()))
		return nil
	case reflect.Int32:
		self.align(4)
		binary.Write(&self.data, self.order, int32(v.Int()))
		return nil
	case reflect.Uint32:
		self.align(4)
		binary.Write(&self.data, self.order, uint32(v.Uint()))
		return nil
	case reflect.Int64:
		self.align(8)
		binary.Write(&self.data, self.order, int64(v.Int()))
		return nil
	case reflect.Uint64:
		self.align(8)
		binary.Write(&self.data, self.order, uint64(v.Uint()))
		return nil
	case reflect.Float64:
		self.align(8)
		binary.Write(&self.data, self.order, float64(v.Float()))
		return nil
	case reflect.String:
		s := v.String()
		// Signatures only use a single byte for the length.
		if v.Type() == typeSignature {
			self.align(1)
			self.data.WriteByte(byte(len(s)))
		} else {
			self.align(4)
			binary.Write(&self.data, self.order, uint32(len(s)))
		}
		self.data.Write([]byte(s))
		self.data.WriteByte(0)
		return nil
	case reflect.Array, reflect.Slice:
		// Marshal array contents to a separate buffer so we
		// can find its length.
		var content encoder
		content.order = self.order
		// Offset alignment by current data and length field
		content.offset = self.data.Len() + 4
		for i := 0; i < v.Len(); i++ {
			if err := content.appendValue(v.Index(i)); err != nil {
				return err
			}
		}
		self.align(4)
		binary.Write(&self.data, self.order, uint32(content.data.Len()))
		self.data.Write(content.data.Bytes())
		return nil
	case reflect.Map:
		// Marshal array contents to a separate buffer so we
		// can find its length.
		var content encoder
		content.order = self.order
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
		binary.Write(&self.data, self.order, uint32(content.data.Len()))
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
		self.align(8)
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


