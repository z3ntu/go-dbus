package dbus

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	typeHasObjectPath = reflect.TypeOf((*HasObjectPath)(nil)).Elem()
	typeVariant = reflect.TypeOf(Variant{})
	typeSignature = reflect.TypeOf(Signature(""))
	typeBlankInterface = reflect.TypeOf((*interface{})(nil)).Elem()
)


type Signature string

func SignatureOf(t reflect.Type) (Signature, error) {
	if t.AssignableTo(typeHasObjectPath) {
		return Signature("o"), nil
	}
	switch t.Kind() {
	case reflect.Uint8:
		return Signature("y"), nil
	case reflect.Bool:
		return Signature("b"), nil
	case reflect.Int16:
		return Signature("n"), nil
	case reflect.Uint16:
		return Signature("q"), nil
	case reflect.Int32:
		return Signature("i"), nil
	case reflect.Uint32:
		return Signature("u"), nil
	case reflect.Int64:
		return Signature("x"), nil
	case reflect.Uint64:
		return Signature("t"), nil
	case reflect.Float64:
		return Signature("d"), nil
	case reflect.String:
		if t == typeSignature {
			return Signature("g"), nil
		}
		return Signature("s"), nil
	case reflect.Array, reflect.Slice:
		valueSig, err := SignatureOf(t.Elem())
		if err != nil {
			return Signature(""), err
		}
		return Signature("a") + valueSig, nil
	case reflect.Map:
		keySig, err := SignatureOf(t.Key())
		if err != nil {
			return Signature(""), err
		}
		valueSig, err := SignatureOf(t.Elem())
		if err != nil {
			return Signature(""), err
		}
		return Signature("a{") + keySig + valueSig + Signature("}"), nil
	case reflect.Struct:
		// Special case the variant structure
		if t == typeVariant {
			return Signature("v"), nil
		}

		sig := Signature("(")
		for i := 0; i != t.NumField(); i++ {
			fieldSig, err := SignatureOf(t.Field(i).Type)
			if err != nil {
				return Signature(""), err
			}
			sig += fieldSig
		}
		sig += Signature(")")
		return sig, nil
	case reflect.Ptr:
		// dereference pointers
		sig, err := SignatureOf(t.Elem())
		return sig, err
	}
	return Signature(""), errors.New("Can not determine signature for " + t.String())
}


type ObjectPath string

type HasObjectPath interface {
	GetObjectPath() ObjectPath
}

func (o ObjectPath) GetObjectPath() ObjectPath {
	return o
}

type Variant struct {
	Value interface{}
}

func (v *Variant) GetVariantSignature() (Signature, error) {
	return SignatureOf(reflect.TypeOf(v.Value))
}


type Error struct {
	Name string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprint(e.Name, ": ", e.Message)
}
