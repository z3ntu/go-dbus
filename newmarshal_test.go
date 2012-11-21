package dbus

import "bytes"
import "testing"

func checkContent(t *testing.T, enc *encoder, expectedSig string, expectedData []byte) {
	if enc.signature != expectedSig {
		t.Error("Bad signature, expected:", expectedSig, " actual:", enc.signature)
	}
	if !bytes.Equal(enc.data.Bytes(), expectedData) {
		t.Error("Bad data, expected:", expectedData, " actual:", enc.data.Bytes())
	}
}

func TestAlign_NM(t *testing.T) {
	var enc encoder
	enc.data.WriteByte(1)
	enc.align(1)
	checkContent(t, &enc, "", []byte{1})
	enc.align(2)
	checkContent(t, &enc, "", []byte{1, 0})
	enc.align(4)
	checkContent(t, &enc, "", []byte{1, 0, 0, 0})
	enc.align(8)
	checkContent(t, &enc, "", []byte{1, 0, 0, 0, 0, 0, 0, 0})
}

func TestAppendByte_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(byte(42)); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "y", []byte{42})
}

func TestAppendBoolean_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(true); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "b", []byte{1, 0, 0, 0})
}

func TestAppendInt16_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(int16(42)); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "n", []byte{42, 0})
}

func TestAppendUint16_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(uint16(42)); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "q", []byte{42, 0})
}

func TestAppendInt32_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(int32(42)); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "i", []byte{42, 0, 0, 0})
}

func TestAppendUint32_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(uint32(42)); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "u", []byte{42, 0, 0, 0})
}

func TestAppendInt64_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(int64(42)); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "x", []byte{42, 0, 0, 0, 0, 0, 0, 0})
}

func TestAppendUint64_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(uint64(42)); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "t", []byte{42, 0, 0, 0, 0, 0, 0, 0})
}

func TestAppendFloat64_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(float64(42.0)); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "d", []byte{0, 0, 0, 0, 0, 0, 69, 64})
}

func TestAppendString_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append("hello"); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "s", []byte{5, 0, 0, 0, 'h', 'e', 'l', 'l', 'o', 0})
}

func TestAppendArray_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append([]int32{42, 420}); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "ai", []byte{8, 0, 0, 0, 42, 0, 0, 0, 164, 1, 0, 0})
}

func TestAppendMap_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(map[string]bool{"true": true}); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "a{sb}", []byte{
		20, 0, 0, 0,                       // array content length
		0, 0, 0, 0,                        // padding to 8 bytes
		4, 0, 0, 0, 't', 'r', 'u', 'e', 0, // "true"
		0, 0, 0,                           // padding to 4 bytes
		1, 0, 0, 0})                       // true
}

func TestAppendStruct_NM(t *testing.T) {
	var enc encoder
	type sample struct {
		one int32
		two string
	}
	if err := enc.Append(&sample{42, "hello"}); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "(is)", []byte{
		42, 0, 0, 0,
		5, 0, 0, 0, 'h', 'e' , 'l', 'l', 'o', 0})
}

func TestAppendAlignment_NM(t *testing.T) {
	var enc encoder
	if err := enc.Append(byte(42), int16(42), true, int32(42), int64(42)); err != nil {
		t.Error(err)
	}
	checkContent(t, &enc, "ynbix", []byte{
		42,                       // byte(42)
		0,                        // padding to 2 bytes
		42, 0,                    // int16(42)
		1, 0, 0, 0,               // true
		42, 0, 0, 0,              // int32(42)
		0, 0, 0, 0,               // padding to 8 bytes
		42, 0, 0, 0, 0, 0, 0, 0}) // int64(42)
}
