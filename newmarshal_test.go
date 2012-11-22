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
	c.Check(enc.signature, Equals, "y")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42})
}

func (s *S) TestEncoderAppendBoolean(c *C) {
	var enc encoder
	if err := enc.Append(true); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "b")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{1, 0, 0, 0})
}

func (s *S) TestEncoderAppendInt16(c *C) {
	var enc encoder
	if err := enc.Append(int16(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "n")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0})
}

func (s *S) TestEncoderAppendUint16(c *C) {
	var enc encoder
	if err := enc.Append(uint16(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "q")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0})
}

func (s *S) TestEncoderAppendInt32(c *C) {
	var enc encoder
	if err := enc.Append(int32(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "i")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0, 0, 0})
}

func (s *S) TestEncoderAppendUint32(c *C) {
	var enc encoder
	if err := enc.Append(uint32(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "u")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0, 0, 0})
}

func (s *S) TestEncoderAppendInt64(c *C) {
	var enc encoder
	if err := enc.Append(int64(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "x")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0, 0, 0, 0, 0, 0, 0})
}

func (s *S) TestEncoderAppendUint64(c *C) {
	var enc encoder
	if err := enc.Append(uint64(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "t")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{42, 0, 0, 0, 0, 0, 0, 0})
}

func (s *S) TestEncoderAppendFloat64(c *C) {
	var enc encoder
	if err := enc.Append(float64(42.0)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "d")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{0, 0, 0, 0, 0, 0, 69, 64})
}

func (s *S) TestEncoderAppendString(c *C) {
	var enc encoder
	if err := enc.Append("hello"); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "s")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		5, 0, 0, 0,              // Length
		'h', 'e', 'l', 'l', 'o', // "hello"
		0})                      // nul termination
}

func (s *S) TestEncoderAppendArray(c *C) {
	var enc encoder
	if err := enc.Append([]int32{42, 420}); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "ai")
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
	c.Check(enc.signature, Equals, "a{sb}")
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
	c.Check(enc.signature, Equals, "(is)")
	c.Check(enc.data.Bytes(), DeepEquals, []byte{
		42, 0, 0, 0,
		5, 0, 0, 0, 'h', 'e' , 'l', 'l', 'o', 0})
}

func (s *S) TestEncoderAppendAlignment(c *C) {
	var enc encoder
	if err := enc.Append(byte(42), int16(42), true, int32(42), int64(42)); err != nil {
		c.Error(err)
	}
	c.Check(enc.signature, Equals, "ynbix")
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

func (s *S) TestDecoderArray(c *C) {
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
