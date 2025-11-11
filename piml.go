package piml

import (
	"bytes"
	"errors"
)

// Standard error types for the PIML package
var (
	ErrSyntax           = errors.New("piml: syntax error")
	ErrInvalidUnmarshal = errors.New("piml: Unmarshal(nil) or Unmarshal(non-pointer)")
	ErrUnsupportedType  = errors.New("piml: unsupported type for marshalling")
)

//const SimpleTimeFormat = "2006-01-02 15:04:05"
//const CustomLayout = "2006-01-02 15:04:05.000000 -0700"

// Unmarshal parses the PIML-encoded data and stores the result
// in the value pointed to by v.
//
// Unmarshal uses reflection to map PIML keys to struct fields,
// using the `piml:"..."` struct tag.
func Unmarshal(data []byte, v interface{}) error {
	d := NewDecoder(data)
	return d.Decode(v)
}

// Marshal returns the PIML encoding of v.
//
// Marshal uses reflection to map struct fields to PIML keys,
// using the `piml:"..."` struct tag.
//
// Per the PIML spec we defined:
// - Go nil pointers will be marshalled to `nil`.
// - Go empty slices (`[]string{}`) will be marshalled to `nil`.
// - Go empty maps (`map[string]int{}`) will be marshalled to `nil`.
func Marshal(v interface{}) ([]byte, error) {
	var b bytes.Buffer
	e := NewEncoder(&b)
	// Start with indent level -1 to signify the root.
	// This ensures top-level struct fields have indent 0.
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
