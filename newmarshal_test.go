package dbus

import "encoding/binary"
import . "launchpad.net/gocheck"

func (s *S) TestEncoderAlign(c *C) {
	var enc encoder
	enc.data.WriteByte(1)
	enc.align(1)
	c.Check(enc.data.Bytes(), DeepEquals, []byte{1})
	enc.align(2)
	c.Check(enc.data.Bytes(), DeepEquals, []byte{1, 0})
	enc.align(4)
	c.Check(enc.data.Bytes(), DeepEquals, []byte{1, 0, 0, 0})
	enc.align(8)
	c.Check(enc.data.Bytes(), DeepEquals, []byte{1, 0, 0, 0, 0, 0, 0, 0})
}

func (s *S) TestEncoderAppendByte(c *C) {
	var enc encoder
	if err := enc.Append(byte(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("y"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42})
}

func (s *S) TestEncoderAppendBoolean(c *C) {
	var enc encoder
	if err := enc.Append(true); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("b"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{1, 0, 0, 0})
}

func (s *S) TestEncoderAppendInt16(c *C) {
	var enc encoder
	if err := enc.Append(int16(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("n"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0})
}

func (s *S) TestEncoderAppendUint16(c *C) {
	var enc encoder
	if err := enc.Append(uint16(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("q"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0})
}

func (s *S) TestEncoderAppendInt32(c *C) {
	var enc encoder
	if err := enc.Append(int32(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("i"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0, 0, 0})
}

func (s *S) TestEncoderAppendUint32(c *C) {
	var enc encoder
	if err := enc.Append(uint32(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("u"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0, 0, 0})
}

func (s *S) TestEncoderAppendInt64(c *C) {
	var enc encoder
	if err := enc.Append(int64(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("x"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0, 0, 0, 0, 0, 0, 0})
}

func (s *S) TestEncoderAppendUint64(c *C) {
	var enc encoder
	if err := enc.Append(uint64(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("t"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0, 0, 0, 0, 0, 0, 0})
}

func (s *S) TestEncoderAppendFloat64(c *C) {
	var enc encoder
	if err := enc.Append(float64(42.0)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("d"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{0, 0, 0, 0, 0, 0, 69, 64})
}

func (s *S) TestEncoderAppendString(c *C) {
	var enc encoder
	if err := enc.Append("hello"); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("s"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		5, 0, 0, 0,              // Length
		'h', 'e', 'l', 'l', 'o', // "hello"
		0})                      // nul termination
}

func (s *S) TestEncoderAppendObjectPath(c *C) {
	var enc encoder
	if err := enc.Append(ObjectPath("/foo")); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("o"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		4, 0, 0, 0,         // Length
		'/', 'f', 'o', 'o', // ObjectPath("/foo")
		0})                 // nul termination
}

type testObject struct {}
func (f *testObject) GetObjectPath() ObjectPath {
	return ObjectPath("/foo")
}

func (s *S) TestEncoderAppendObject(c *C) {
	var enc encoder
	if err := enc.Append(&testObject{}); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("o"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		4, 0, 0, 0,         // Length
		'/', 'f', 'o', 'o', // ObjectPath("/foo")
		0})                 // nul termination
}

func (s *S) TestEncoderAppendSignature(c *C) {
	var enc encoder
	if err := enc.Append(Signature("a{si}")); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("g"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		5,                       // Length
		'a', '{', 's', 'i', '}', // Signature("a{si}")
		0})                      // nul termination
}

func (s *S) TestEncoderAppendArray(c *C) {
	var enc encoder
	if err := enc.Append([]int32{42, 420}); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("ai"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		8, 0, 0, 0,    // Length
		42, 0, 0, 0,   // int32(42)
		164, 1, 0, 0}) // int32(420)
}

func (s *S) TestEncoderAppendMap(c *C) {
	var enc encoder
	if err := enc.Append(map[string]bool{"true": true}); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("a{sb}"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		20, 0, 0, 0,                       // array content length
		0, 0, 0, 0,                        // padding to 8 bytes
		4, 0, 0, 0, 't', 'r', 'u', 'e', 0, // "true"
		0, 0, 0,                           // padding to 4 bytes
		1, 0, 0, 0})                       // true
}

func (s *S) TestEncoderAppendStruct(c *C) {
	var enc encoder
	type sample struct {
		one int32
		two string
	}
	if err := enc.Append(&sample{42, "hello"}); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("(is)"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		42, 0, 0, 0,
		5, 0, 0, 0, 'h', 'e' , 'l', 'l', 'o', 0})
}

func (s *S) TestEncoderAppendVariant(c *C) {
	var enc encoder
	if err := enc.Append(&Variant{int32(42)}); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("v"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		1, 'i', 0,    // Signature("i")
		0,            // padding to 4 bytes
		42, 0, 0, 0}) // int32(42)
}

func (s *S) TestEncoderAppendAlignment(c *C) {
	var enc encoder
	if err := enc.Append(byte(42), int16(42), true, int32(42), int64(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, Signature("ynbix"))
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		42,                       // byte(42)
		0,                        // padding to 2 bytes
		42, 0,                    // int16(42)
		1, 0, 0, 0,               // true
		42, 0, 0, 0,              // int32(42)
		0, 0, 0, 0,               // padding to 8 bytes
		42, 0, 0, 0, 0, 0, 0, 0}) // int64(42)
}


func (s *S) TestDecoderDecodeByte(c *C) {
	dec := newDecoder("yy", []byte{42, 100}, binary.LittleEndian)
	var value1 byte
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, byte(42))
	c.Check(value2, Equals, byte(100))
	c.Check(dec.dataOffset, Equals, 2)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeBool(c *C) {
	dec := newDecoder("bb", []byte{0, 0, 0, 0, 1, 0, 0, 0}, binary.LittleEndian)
	var value1 bool
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, false)
	c.Check(value2, Equals, true)
	c.Check(dec.dataOffset, Equals, 8)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeInt16(c *C) {
	dec := newDecoder("nn", []byte{42, 0, 100, 0}, binary.LittleEndian)
	var value1 int16
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, int16(42))
	c.Check(value2, Equals, int16(100))
	c.Check(dec.dataOffset, Equals, 4)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeUint16(c *C) {
	dec := newDecoder("qq", []byte{42, 0, 100, 0}, binary.LittleEndian)
	var value1 uint16
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, uint16(42))
	c.Check(value2, Equals, uint16(100))
	c.Check(dec.dataOffset, Equals, 4)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeInt32(c *C) {
	dec := newDecoder("ii", []byte{42, 0, 0, 0, 100, 0, 0, 0}, binary.LittleEndian)
	var value1 int32
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, int32(42))
	c.Check(value2, Equals, int32(100))
	c.Check(dec.dataOffset, Equals, 8)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeUint32(c *C) {
	dec := newDecoder("uu", []byte{42, 0, 0, 0, 100, 0, 0, 0}, binary.LittleEndian)
	var value1 uint32
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, uint32(42))
	c.Check(value2, Equals, uint32(100))
	c.Check(dec.dataOffset, Equals, 8)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeInt64(c *C) {
	dec := newDecoder("xx", []byte{42, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0}, binary.LittleEndian)
	var value1 int64
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, int64(42))
	c.Check(value2, Equals, int64(100))
	c.Check(dec.dataOffset, Equals, 16)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeUint64(c *C) {
	dec := newDecoder("tt", []byte{42, 0, 0, 0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0}, binary.LittleEndian)
	var value1 uint64
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, uint64(42))
	c.Check(value2, Equals, uint64(100))
	c.Check(dec.dataOffset, Equals, 16)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeFloat64(c *C) {
	dec := newDecoder("dd", []byte{0, 0, 0, 0, 0, 0, 69, 64, 0, 0, 0, 0, 0, 0, 89, 64}, binary.LittleEndian)
	var value1 float64
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, float64(42))
	c.Check(value2, Equals, float64(100))
	c.Check(dec.dataOffset, Equals, 16)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeString(c *C) {
	dec := newDecoder("ss", []byte{
		5, 0, 0, 0,                  // len("hello")
		'h', 'e', 'l', 'l', 'o', 0,  // "hello"
		0, 0,                        // padding
		5, 0, 0, 0,                  // len("world")
		'w', 'o', 'r', 'l', 'd', 0}, // "world"
		binary.LittleEndian)
	var value1 string
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, "hello")
	c.Check(value2, Equals, "world")
	c.Check(dec.dataOffset, Equals, 22)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeObjectPath(c *C) {
	dec := newDecoder("oo", []byte{
		4, 0, 0, 0,             // len("/foo")
		'/', 'f', 'o', 'o', 0,  // ObjectPath("/foo")
		0, 0, 0,                // padding
		4, 0, 0, 0,             // len("/bar")
		'/', 'b', 'a', 'r', 0}, // ObjectPath("/bar")
		binary.LittleEndian)
	var value1 ObjectPath
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, ObjectPath("/foo"))
	c.Check(value2, Equals, ObjectPath("/bar"))
	c.Check(dec.dataOffset, Equals, 21)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeSignature(c *C) {
	dec := newDecoder("gg", []byte{
		8,                                         // len("a{s(iv)}")
		'a', '{', 's', '(', 'i', 'v', ')', '}', 0, // Signature("a{s(iv)}")
		4,                                         // len("asvi")
		'a', 's', 'v', 'i', 0},                    // Signature("asvi")
		binary.LittleEndian)
	var value1 Signature
	var value2 interface{}
	if err := dec.Decode(&value1, &value2); err != nil {
		c.Error(err)
	}
	c.Check(value1, Equals, Signature("a{s(iv)}"))
	c.Check(value2, Equals, Signature("asvi"))
	c.Check(dec.dataOffset, Equals, 16)
	c.Check(dec.sigOffset, Equals, 2)
}

func (s *S) TestDecoderDecodeArray(c *C) {
	dec := newDecoder("ai", []byte{
		8, 0, 0, 0,    // array length
		42, 0, 0, 0,   // int32(42)
		100, 0, 0, 0}, // int32(100)
		binary.LittleEndian)
	// Decode as an array
	var value1 [2]int32
	if err := dec.Decode(&value1); err != nil {
		c.Error("Decode as array:", err)
	}
	c.Check(dec.dataOffset, Equals, 12)
	c.Check(dec.sigOffset, Equals, 2)
	c.Check(value1[0], Equals, int32(42))
	c.Check(value1[1], Equals, int32(100))

	// Decode as a slice
	dec.dataOffset = 0
	dec.sigOffset = 0
	var value2 []int32
	if err := dec.Decode(&value2); err != nil {
		c.Error("Decode as slice:", err)
	}
	c.Check(value2, DeepEquals, []int32{42, 100})

	// Decode as blank interface
	dec.dataOffset = 0
	dec.sigOffset = 0
	var value3 interface{}
	if err := dec.Decode(&value3); err != nil {
		c.Error("Decode as interface:", err)
	}
	c.Check(value3, DeepEquals, []interface{}{int32(42), int32(100)})
}

func (s *S) TestDecoderDecodeStruct(c *C) {
	dec := newDecoder("(si)", []byte{
		5, 0, 0, 0,                 // len("hello")
                'h', 'e', 'l', 'l', 'o', 0, // "hello"
		0, 0,                       // padding
                42, 0, 0, 0},               // int32(42)
		binary.LittleEndian)

	type Dummy struct {
		S string
		I int32
	}
	// Decode as structure
	var value1 Dummy
	if err := dec.Decode(&value1); err != nil {
		c.Error("Decode as structure:", err)
	}
	c.Check(dec.dataOffset, Equals, 16)
	c.Check(dec.sigOffset, Equals, 4)
	c.Check(value1, DeepEquals, Dummy{"hello", 42})

	// Decode as pointer to structure
	dec.dataOffset = 0
	dec.sigOffset = 0
	var value2 *Dummy
	if err := dec.Decode(&value2); err != nil {
		c.Error("Decode as structure pointer:", err)
	}
	c.Check(value2, DeepEquals, &Dummy{"hello", 42})

	// Decode as blank interface
	dec.dataOffset = 0
	dec.sigOffset = 0
	var value3 interface{}
	if err := dec.Decode(&value3); err != nil {
		c.Error("Decode as interface:", err)
	}
	c.Check(value3, DeepEquals, []interface{}{"hello", int32(42)})
}

func (s *S) TestDecoderDecodeVariant(c *C) {
	dec := newDecoder("v", []byte{
		1,            // len("i")
		'i', 0,       // Signature("i")
		0,            // padding
                42, 0, 0, 0}, // int32(42)
		binary.LittleEndian)

	var value1 Variant
	if err := dec.Decode(&value1); err != nil {
		c.Error("Decode as Variant:", err)
	}
	c.Check(dec.dataOffset, Equals, 8)
	c.Check(dec.sigOffset, Equals, 1)
	c.Check(value1, DeepEquals, Variant{int32(42)})

	// Decode as pointer to Variant
	dec.dataOffset = 0
	dec.sigOffset = 0
	var value2 *Variant
	if err := dec.Decode(&value2); err != nil {
		c.Error("Decode as *Variant:", err)
	}
	c.Check(value2, DeepEquals, &Variant{int32(42)})

	// Decode as pointer to blank interface
	dec.dataOffset = 0
	dec.sigOffset = 0
	var value3 interface{}
	if err := dec.Decode(&value3); err != nil {
		c.Error("Decode as interface:", err)
	}
	c.Check(value3, DeepEquals, &Variant{int32(42)})
}
