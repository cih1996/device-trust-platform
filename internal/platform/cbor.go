package platform

import (
	"encoding/binary"
	"errors"
)

// A minimal CBOR (RFC 8949) decoder for App Attest objects: unsigned and
// negative integers, byte and text strings, arrays and maps of definite
// length. Anything else is rejected.

const cborMaxDepth = 8

type cborDecoder struct {
	data []byte
	pos  int
}

func decodeCBOR(data []byte) (any, error) {
	d := &cborDecoder{data: data}
	v, err := d.value(0)
	if err != nil {
		return nil, err
	}
	if d.pos != len(data) {
		return nil, errors.New("cbor: trailing data")
	}
	return v, nil
}

func (d *cborDecoder) head() (major byte, arg uint64, err error) {
	if d.pos >= len(d.data) {
		return 0, 0, errors.New("cbor: truncated")
	}
	b := d.data[d.pos]
	d.pos++
	major, info := b>>5, b&31
	size := 0
	switch {
	case info < 24:
		return major, uint64(info), nil
	case info == 24:
		size = 1
	case info == 25:
		size = 2
	case info == 26:
		size = 4
	case info == 27:
		size = 8
	default:
		return 0, 0, errors.New("cbor: indefinite or reserved length")
	}
	if len(d.data)-d.pos < size {
		return 0, 0, errors.New("cbor: truncated")
	}
	buf := make([]byte, 8)
	copy(buf[8-size:], d.data[d.pos:d.pos+size])
	d.pos += size
	return major, binary.BigEndian.Uint64(buf), nil
}

func (d *cborDecoder) value(depth int) (any, error) {
	if depth > cborMaxDepth {
		return nil, errors.New("cbor: nesting too deep")
	}
	major, arg, err := d.head()
	if err != nil {
		return nil, err
	}
	switch major {
	case 0:
		return arg, nil
	case 1:
		return -1 - int64(arg), nil
	case 2, 3:
		if arg > uint64(len(d.data)-d.pos) {
			return nil, errors.New("cbor: truncated string")
		}
		b := d.data[d.pos : d.pos+int(arg)]
		d.pos += int(arg)
		if major == 3 {
			return string(b), nil
		}
		return b, nil
	case 4:
		if arg > uint64(len(d.data)-d.pos) {
			return nil, errors.New("cbor: array too long")
		}
		out := make([]any, 0, arg)
		for i := uint64(0); i < arg; i++ {
			v, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case 5:
		if arg > uint64(len(d.data)-d.pos) {
			return nil, errors.New("cbor: map too long")
		}
		out := make(map[string]any, arg)
		for i := uint64(0); i < arg; i++ {
			k, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			key, ok := k.(string)
			if !ok {
				return nil, errors.New("cbor: non-text map key")
			}
			if _, dup := out[key]; dup {
				return nil, errors.New("cbor: duplicate map key")
			}
			if out[key], err = d.value(depth + 1); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return nil, errors.New("cbor: unsupported type")
}

// cborBytes and friends read typed fields from a decoded map.
func cborBytes(m map[string]any, key string) ([]byte, error) {
	b, ok := m[key].([]byte)
	if !ok {
		return nil, errors.New("cbor: missing byte string " + key)
	}
	return b, nil
}
