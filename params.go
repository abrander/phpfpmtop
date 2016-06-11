package main

import (
	"encoding/binary"
	"io"
)

type (
	// Params is a simple type used for format parameters for FastCGI.
	Params map[string]string
)

func NewParams() Params {
	return make(map[string]string)
}

// Size will predict the needed space for encoding all parameters.
func (p Params) Size() uint16 {
	accum := 0

	for key, value := range p {
		// two bytes for setting length.
		accum += 2

		// ... and the actual values.
		accum += len(key)
		accum += len(value)
	}

	return uint16(accum)
}

// Write the encoded parameters to w.
func (p Params) Write(w io.Writer) error {
	for key, value := range p {
		keyLen := len(key)
		valueLen := len(value)

		if keyLen > 255 || valueLen > 255 {
			continue
		}

		err := binary.Write(w, binary.BigEndian, byte(keyLen))
		if err != nil {
			return err
		}

		err = binary.Write(w, binary.BigEndian, byte(valueLen))
		if err != nil {
			return err
		}

		_, err = w.Write([]byte(key))
		if err != nil {
			return err
		}

		_, err = w.Write([]byte(value))
		if err != nil {
			return err
		}
	}

	return nil
}
