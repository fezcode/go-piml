package piml

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// An Encoder writes PIML values to an output stream.
type Encoder struct {
	w io.Writer
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes the PIML encoding of v to the stream.
func (e *Encoder) Encode(v interface{}) error {
	rv := reflect.ValueOf(v)
	// Start with indent -1 to signify the root.
	return e.encodeValue(rv, -1, false) // false = not in an array
}

// encodeValue is the main recursive marshalling function.
func (e *Encoder) encodeValue(v reflect.Value, indent int, inArray bool) error {
	// Handle nil and empty values
	if !v.IsValid() || isNilOrEmpty(v) {
		if inArray {
			// This case should be handled by encodeSlice
			return nil
		}
		// Not in an array, so it's a value for a key.
		// The key itself is written by encodeStruct, here we just write 'nil'.
		_, err := e.w.Write([]byte(" nil\n"))
		return err
	}

	// This is only used for array items
	var indentStr string
	if indent > 0 {
		indentStr = strings.Repeat("  ", indent)
	}

	// Dereference pointers
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// Dispatch based on type
	switch v.Kind() {
	case reflect.Struct:
		// NEW CHECK: Handle time.Time as a primitive string
		if v.Type() == reflect.TypeOf(time.Time{}) {
			s := v.Interface().(time.Time).Format(time.RFC3339Nano)
			if inArray {
				_, err := e.w.Write([]byte(fmt.Sprintf("%s> %s\n", indentStr, s)))
				return err
			}
			_, err := e.w.Write([]byte(fmt.Sprintf(" %s\n", s)))
			return err
		}

		// If we're marshalling a struct inside an array, we must add the '>'
		if inArray {
			// e.g., > (item)
			// The key (e.g., "item") is metadata, per our spec.
			// We'll use the struct's type name, or "item".
			itemName := v.Type().Name()
			if itemName == "" {
				itemName = "item"
			}
			if _, err := e.w.Write([]byte(fmt.Sprintf("%s> (%s)\n", indentStr, itemName))); err != nil {
				return err
			}
			// Now encode the struct's fields, one level deeper
			return e.encodeStruct(v, indent+1)
		} else {
			// A top-level struct or nested struct field.
			// The keys will be indented *by* encodeStruct.
			// If we are nested (indent > -1), we need a newline first.
			if indent > -1 {
				if _, err := e.w.Write([]byte("\n")); err != nil {
					return err
				}
			}
			return e.encodeStruct(v, indent)
		}

	case reflect.Slice, reflect.Array:
		// We need a newline if we are not in an array
		if !inArray {
			if _, err := e.w.Write([]byte("\n")); err != nil {
				return err
			}
		}
		return e.encodeSlice(v, indent+1) // Slices items are one level deeper

	case reflect.Map:
		// Per our spec, map keys are PIML keys.
		// This is just like a struct.
		if !inArray {
			if _, err := e.w.Write([]byte("\n")); err != nil {
				return err
			}
		}
		return e.encodeMap(v, indent)

	case reflect.String:
		return e.encodeString(v, indent, inArray)

	// Primitives
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s := strconv.FormatInt(v.Int(), 10)
		if inArray {
			_, err := e.w.Write([]byte(fmt.Sprintf("%s> %s\n", indentStr, s)))
			return err
		}
		_, err := e.w.Write([]byte(fmt.Sprintf(" %s\n", s)))
		return err

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		s := strconv.FormatUint(v.Uint(), 10)
		if inArray {
			_, err := e.w.Write([]byte(fmt.Sprintf("%s> %s\n", indentStr, s)))
			return err
		}
		_, err := e.w.Write([]byte(fmt.Sprintf(" %s\n", s)))
		return err

	case reflect.Float32, reflect.Float64:
		s := strconv.FormatFloat(v.Float(), 'f', -1, 64)
		if inArray {
			_, err := e.w.Write([]byte(fmt.Sprintf("%s> %s\n", indentStr, s)))
			return err
		}
		_, err := e.w.Write([]byte(fmt.Sprintf(" %s\n", s)))
		return err

	case reflect.Bool:
		s := strconv.FormatBool(v.Bool())
		if inArray {
			_, err := e.w.Write([]byte(fmt.Sprintf("%s> %s\n", indentStr, s)))
			return err
		}
		_, err := e.w.Write([]byte(fmt.Sprintf(" %s\n", s)))
		return err

	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedType, v.Kind())
	}
}

// encodeStruct handles marshalling a Go struct to PIML.
func (e *Encoder) encodeStruct(v reflect.Value, indent int) error {
	t := v.Type()
	// The fields of a struct are indented one level deeper than the struct's key.
	// For the root, indent = -1, so fieldIndent = 0.
	// For a nested struct, indent = 0, so fieldIndent = 1.
	fieldIndent := indent + 1
	var indentStr string
	if fieldIndent > 0 {
		indentStr = strings.Repeat("  ", fieldIndent)
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldV := v.Field(i)

		tag := field.Tag.Get("piml")
		if tag == "-" {
			continue // Skip this field
		}
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}

		// Write the key
		if _, err := e.w.Write([]byte(fmt.Sprintf("%s(%s)", indentStr, tag))); err != nil {
			return err
		}

		// Write the value
		if err := e.encodeValue(fieldV, fieldIndent, false); err != nil {
			return err
		}
	}
	return nil
}

// encodeSlice handles marshalling a Go slice to PIML.
func (e *Encoder) encodeSlice(v reflect.Value, indent int) error {
	if v.Len() == 0 {
		return nil // Handled by isNilOrEmpty in encodeValue
	}

	// We'll peek at the first element
	elemType := v.Type().Elem()
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}

	switch elemType.Kind() {
	case reflect.Struct:
		// List of Objects
		for i := 0; i < v.Len(); i++ {
			elemV := v.Index(i)
			// Pass 'true' for inArray
			if err := e.encodeValue(elemV, indent, true); err != nil {
				return err
			}
		}
	default:
		// List of Primitives
		var indentStr string
		if indent > 0 {
			indentStr = strings.Repeat("  ", indent)
		}
		for i := 0; i < v.Len(); i++ {
			elemV := v.Index(i)
			// Pass 'true' for inArray, but we re-implement the primitive
			// logic here to write the '>'.
			if err := e.writePrimitiveArrayItem(elemV, indentStr); err != nil {
				return err
			}
		}
	}
	return nil
}

// encodeMap handles marshalling a Go map to PIML.
// This is just like a struct.
func (e *Encoder) encodeMap(v reflect.Value, indent int) error {
	// Maps are encoded just like structs.
	fieldIndent := indent + 1
	var indentStr string
	if fieldIndent > 0 {
		indentStr = strings.Repeat("  ", fieldIndent)
	}

	// Note: Map iteration is not stable, so output may vary.
	for _, key := range v.MapKeys() {
		val := v.MapIndex(key)
		keyStr, ok := key.Interface().(string)
		if !ok {
			return errors.New("piml: map keys must be strings")
		}

		// Write the key
		if _, err := e.w.Write([]byte(fmt.Sprintf("%s(%s)", indentStr, keyStr))); err != nil {
			return err
		}
		// Write the value
		if err := e.encodeValue(val, fieldIndent, false); err != nil {
			return err
		}
	}
	return nil
}

// encodeString handles marshalling a string.
// It detects multi-line strings.
func (e *Encoder) encodeString(v reflect.Value, indent int, inArray bool) error {
	s := v.String()
	var indentStr string
	if indent > 0 {
		indentStr = strings.Repeat("  ", indent)
	}

	if strings.Contains(s, "\n") {
		// --- Multi-line String ---
		if inArray {
			// This is tricky. A multi-line string in an array?
			// Let's just use > for the first line
			_, err := e.w.Write([]byte(fmt.Sprintf("%s> ... (multi-line not fully supported in array yet)\n", indentStr)))
			return err
		}
		// Write key (already written by caller), then newline
		if _, err := e.w.Write([]byte("\n")); err != nil {
			return err
		}
		// Write each line indented
		lines := strings.Split(s, "\n")
		lineIndent := indent + 1
		var lineIndentStr string
		if lineIndent > 0 {
			lineIndentStr = strings.Repeat("  ", lineIndent)
		}
		for _, line := range lines {
			// Escape any line that starts with # to prevent it being parsed as a comment.
			if strings.HasPrefix(line, "#") {
				line = `\` + line
			}
			if _, err := e.w.Write([]byte(fmt.Sprintf("%s%s\n", lineIndentStr, line))); err != nil {
				return err
			}
		}
		return nil
	}

	// --- Single-line String ---
	if inArray {
		_, err := e.w.Write([]byte(fmt.Sprintf("%s> %s\n", indentStr, s)))
		return err
	}
	_, err := e.w.Write([]byte(fmt.Sprintf(" %s\n", s)))
	return err
}

// writePrimitiveArrayItem is a helper for encodeSlice
func (e *Encoder) writePrimitiveArrayItem(v reflect.Value, indentStr string) error {
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	var s string
	switch v.Kind() {
	case reflect.String:
		s = v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s = strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		s = strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		s = strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.Bool:
		s = strconv.FormatBool(v.Bool())
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedType, v.Kind())
	}

	_, err := e.w.Write([]byte(fmt.Sprintf("%s> %s\n", indentStr, s)))
	return err
}

// isNilOrEmpty checks if a reflect.Value is nil, or an empty slice/map.
func isNilOrEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	}
	return false
}
