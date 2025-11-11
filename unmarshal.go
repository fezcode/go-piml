package piml

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// A Decoder reads and decodes PIML values from an input byte slice.
type Decoder struct {
	s       *bufio.Scanner
	peekBuf *lineInfo // Buffer for one-line lookahead
}

// lineInfo stores the parsed data from a single line.
type lineInfo struct {
	indent   int    // Number of leading spaces
	key      string // Key (if present)
	value    string // Value (if present)
	lineType lineType
}

// lineType categorizes the parsed line.
type lineType int

const (
	lineBlank       lineType = iota // Empty or comment-only
	lineKeyValue                    // (key) value
	lineKeyOnly                     // (key)
	lineArrayItem                   // > value
	lineSetItem                     // >| value
	lineArrayObject                 // > (item)
	lineMultiLine                   //   value (indented, no key)
)

// NewDecoder returns a new decoder that reads from data.
func NewDecoder(data []byte) *Decoder {
	return &Decoder{
		s: bufio.NewScanner(bytes.NewReader(data)),
	}
}

// Decode reads the next PIML-encoded value from its
// input and stores it in the value pointed to by v.
func (d *Decoder) Decode(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return ErrInvalidUnmarshal
	}
	// We start with -1, as the root has no indentation.
	return d.decodeValue(rv, -1)
}

// peek gets the next line, parses it, and stores it in the buffer.
func (d *Decoder) peek() (*lineInfo, error) {
	if d.peekBuf != nil {
		return d.peekBuf, nil
	}

	for d.s.Scan() {
		fullLine := d.s.Text() // The original, unmodified line

		// 1. Clean comments from the *full* line
		cleanLine := fullLine
		if commentIdx := strings.Index(cleanLine, "#"); commentIdx != -1 {
			cleanLine = cleanLine[:commentIdx]
		}

		// 2. Check for blank lines
		trimmedLine := strings.TrimSpace(cleanLine)
		if trimmedLine == "" {
			continue // Skip blank lines and comment-only lines
		}

		// 3. Calculate indentation
		indent := 0
		for _, r := range cleanLine {
			if r == ' ' {
				indent++
			} else if r == '\t' {
				// Per spec, tabs are not allowed.
				return nil, fmt.Errorf("%w: tabs are not allowed (line: %q)", ErrSyntax, fullLine)
			} else {
				// We found the first non-space char
				break
			}
		}

		// 4. Parse the line based on its *trimmed* content
		li := &lineInfo{indent: indent}
		lineContent := strings.TrimSpace(cleanLine)

		if strings.HasPrefix(lineContent, "> (") {
			// > (item)
			li.lineType = lineArrayObject
			// Key is ignored, per spec
		} else if strings.HasPrefix(lineContent, ">|") {
			// >| value
			li.lineType = lineSetItem
			li.value = strings.TrimSpace(lineContent[2:])
		} else if strings.HasPrefix(lineContent, ">") {
			// > value
			li.lineType = lineArrayItem
			li.value = strings.TrimSpace(lineContent[1:])
		} else if strings.HasPrefix(lineContent, "(") {
			// (key) value  OR (key)
			// It starts with '(', it MUST be a key.
			closeParen := strings.Index(lineContent, ")")
			if closeParen == -1 {
				// It starts with '(' but has no ')'. This is a syntax error.
				return nil, fmt.Errorf("%w: invalid key format, missing ')' (line: %q)", ErrSyntax, fullLine)
			}
			li.key = lineContent[1:closeParen]
			li.value = strings.TrimSpace(lineContent[closeParen+1:])

			if li.value == "" {
				li.lineType = lineKeyOnly
			} else {
				li.lineType = lineKeyValue
			}
		} else {
			// A line with no key... must be multi-line string
			li.lineType = lineMultiLine
			// For multi-line, the value is the *full line*
			// with its indentation preserved, post-comment-stripping.
			li.value = cleanLine
		}

		d.peekBuf = li
		return li, nil
	}
	if err := d.s.Err(); err != nil {
		return nil, err
	}
	// End of file
	return nil, nil
}

// consume moves the scanner past the buffered line.
func (d *Decoder) consume() {
	d.peekBuf = nil
}

// decodeValue is the main recursive unmarshalling function.
func (d *Decoder) decodeValue(v reflect.Value, currentIndent int) error {
	line, err := d.peek()
	if err != nil {
		return err
	}
	if line == nil {
		return nil // End of file
	}

	// We must check indentation *first*.
	// If the line is not indented deeper, it's not part of this value.
	if line.indent <= currentIndent {
		return nil
	}

	// This is the core logic. What type of value is this?
	// We decide based on the *first line* of the value.
	switch line.lineType {
	case lineKeyOnly, lineKeyValue:
		// (key) or (key) value
		// This is the start of an object.
		return d.decodeObject(v, currentIndent)

	case lineArrayItem, lineArrayObject:
		// > value  OR  > (item)
		// This must be a slice.
		return d.decodeSlice(v, currentIndent)

	case lineSetItem:
		// >| value
		return d.decodeSet(v, currentIndent)

	case lineMultiLine:
		//   value
		// This must be a multi-line string.
		return d.decodeMultiLineString(v, currentIndent)

	default:
		// Should be impossible
		return fmt.Errorf("%w: unknown line type %v", ErrSyntax, line.lineType)
	}
}

// decodeObject unmarshals into a struct or map.
func (d *Decoder) decodeObject(v reflect.Value, currentIndent int) error {
	v = indirect(v, true) // forceAlloc=true to create nil struct pointers
	if !v.IsValid() {
		return errors.New("piml: cannot unmarshal into invalid value")
	}

	isMap := v.Kind() == reflect.Map
	isStruct := v.Kind() == reflect.Struct

	if !isMap && !isStruct {
		return fmt.Errorf("piml: cannot unmarshal object into %s", v.Kind())
	}

	if isMap {
		if v.Type().Key().Kind() != reflect.String {
			return errors.New("piml: map key must be string")
		}
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
	}

	for {
		line, err := d.peek()
		if err != nil {
			return err
		}
		if line == nil || line.indent <= currentIndent {
			break // End of this object
		}

		if line.lineType != lineKeyValue && line.lineType != lineKeyOnly {
			// This is a child of the object, it *must* be a key.
			// e.g. Array items (>) are not allowed here.
			return fmt.Errorf("%w: expected (key) or (key) value, got line type %v", ErrSyntax, line.lineType)
		}

		key := line.key

		// Find the target field/map entry
		var targetV reflect.Value
		if isStruct {
			targetV, err = findStructField(v, key)
			if err != nil {
				// Field not found, but we just consume and ignore
				d.consume() // Consume the (key) or (key) value
				// We also need to consume its children if it's (key) only
				if line.lineType == lineKeyOnly {
					d.consumeChildren(line.indent)
				}
				continue
			}
		} else if isMap {
			// This is the new, robust logic
			elemType := v.Type().Elem()
			targetV = reflect.New(elemType)
		} else {
			// Should be impossible
			return errors.New("piml: invalid state in decodeObject")
		}

		// We have our targetV (either a struct field or a map element)
		if line.lineType == lineKeyValue {
			// (key) value
			d.consume() // Consume the line
			if err := d.setPrimitive(targetV, line.value); err != nil {
				return fmt.Errorf("piml: error setting field %q: %w", key, err)
			}
		} else {
			// (key)
			// This is a complex value, recurse
			//
			// !!!!! THIS IS THE FIX !!!!!
			// Consume the (key) line *before* recursing
			d.consume()
			// !!!!! END FIX !!!!!
			//
			if err := d.decodeValue(targetV, line.indent); err != nil {
				return fmt.Errorf("piml: error decoding field %q: %w", key, err)
			}
		}

		// If it was a map, set the value in the map
		if isMap {
			// targetV is a *pointer* to the element type.
			// We need to set the dereferenced element.
			v.SetMapIndex(reflect.ValueOf(key), targetV.Elem())
		}
	}

	return nil
}

// decodeSlice unmarshals into a Go slice.
func (d *Decoder) decodeSlice(v reflect.Value, currentIndent int) error {
	v = indirect(v, false)
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("piml: cannot unmarshal array into %s", v.Kind())
	}

	// Clear the slice
	v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	elemType := v.Type().Elem()

	for {
		line, err := d.peek()
		if err != nil {
			return err
		}
		if line == nil || line.indent <= currentIndent {
			break // End of array
		}

		// Allocate a new element
		// We pass a pointer to the element type to decodeValue/setPrimitive
		elemVPtr := reflect.New(elemType)

		if line.lineType == lineArrayObject {
			// > (item)
			// This is a list of objects.
			d.consume() // Consume the '> (item)' line. It's just metadata.
			// Now we decode the object *inside* the list item.
			if err := d.decodeValue(elemVPtr, line.indent); err != nil {
				return err
			}
		} else if line.lineType == lineArrayItem {
			// > value
			d.consume() // Consume the line
			if err := d.setPrimitive(elemVPtr, line.value); err != nil {
				return err
			}
		} else {
			// This line is not an array item, so we're done.
			// This can happen if the array is followed by a key
			// at the same indent level.
			break
		}

		// Append the new element (dereferenced from the pointer)
		v.Set(reflect.Append(v, elemVPtr.Elem()))
	}

	return nil
}

// decodeSet unmarshals into a Go map[string]struct{}.
func (d *Decoder) decodeSet(v reflect.Value, currentIndent int) error {
	v = indirect(v, false)

	// We'll treat sets as map[string]struct{} or map[string]bool
	if v.Kind() != reflect.Map || v.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("piml: sets must be unmarshalled into map[string]struct{} or map[string]bool")
	}

	if v.IsNil() {
		v.Set(reflect.MakeMap(v.Type()))
	}

	elemType := v.Type().Elem()
	var setValue reflect.Value
	if elemType.Kind() == reflect.Struct && elemType.NumField() == 0 {
		// map[string]struct{}
		setValue = reflect.New(elemType).Elem()
	} else if elemType.Kind() == reflect.Bool {
		// map[string]bool
		setValue = reflect.ValueOf(true)
	} else {
		return fmt.Errorf("piml: set must be map[string]struct{} or map[string]bool, not %s", v.Type())
	}

	for {
		line, err := d.peek()
		if err != nil {
			return err
		}
		if line == nil || line.indent <= currentIndent {
			break // End of set
		}

		if line.lineType != lineSetItem {
			// This line is not a set item, we're done.
			break
		}

		d.consume()
		keyV := reflect.ValueOf(line.value)
		v.SetMapIndex(keyV, setValue)
	}

	return nil
}

// decodeMultiLineString unmarshals a multi-line string.
func (d *Decoder) decodeMultiLineString(v reflect.Value, currentIndent int) error {
	v = indirect(v, true) // true = force allocation
	if v.Kind() != reflect.String {
		return fmt.Errorf("piml: cannot unmarshal multi-line string into %s", v.Kind())
	}

	var b strings.Builder
	var baseIndent = -1 // -1 means not set yet

	for {
		line, err := d.peek()
		if err != nil {
			return err
		}
		if line == nil || line.indent <= currentIndent {
			break // End of multi-line string
		}

		if line.lineType != lineMultiLine {
			// Not a multi-line string line, we're done.
			break
		}

		d.consume()

		// The line.value is the full, indented line.
		content := line.value

		// Set the base indentation from the first line
		if baseIndent == -1 {
			baseIndent = line.indent
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			// Add the content, stripping the base indent
			if len(content) > baseIndent {
				b.WriteString(content[baseIndent:])
			} else {
				// It's possible the line is just whitespace
				b.WriteString(strings.TrimSpace(content))
			}
		} else {
			b.WriteString("\n")
			// Strip the *base* indent from subsequent lines
			if strings.HasPrefix(content, strings.Repeat(" ", baseIndent)) {
				b.WriteString(content[baseIndent:])
			} else {
				// If a line is not indented, or less indented,
				// just strip all leading space.
				b.WriteString(strings.TrimSpace(content))
			}
		}
	}

	v.SetString(b.String())
	return nil
}

// setPrimitive sets a primitive value (string, int, etc.)
func (d *Decoder) setPrimitive(v reflect.Value, valueStr string) error {
	// 1. Handle "nil" first.
	if valueStr == "nil" {
		if !v.CanSet() {
			v = v.Elem()
		}
		// Check if the target type can be nil
		switch v.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface:
			if !v.IsNil() {
				v.Set(reflect.Zero(v.Type())) // Set to nil
			}
			return nil
		default:
			// Trying to assign nil to a non-nillable type
			return fmt.Errorf("piml: cannot assign nil to non-nillable type %s", v.Type())
		}
	}

	// 2. Dereference pointer
	v = indirect(v, true) // true = force allocation

	// 3. Set value based on kind
	switch v.Kind() {
	case reflect.String:
		v.SetString(valueStr)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			return fmt.Errorf("piml: invalid integer value: %w", err)
		}
		if v.OverflowInt(i) {
			return fmt.Errorf("piml: integer overflow: %s", valueStr)
		}
		v.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := strconv.ParseUint(valueStr, 10, 64)
		if err != nil {
			return fmt.Errorf("piml: invalid unsigned integer value: %w", err)
		}
		if v.OverflowUint(i) {
			return fmt.Errorf("piml: integer overflow: %s", valueStr)
		}
		v.SetUint(i)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return fmt.Errorf("piml: invalid float value: %w", err)
		}
		if v.OverflowFloat(f) {
			return fmt.Errorf("piml: float overflow: %s", valueStr)
		}
		v.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(valueStr)
		if err != nil {
			return fmt.Errorf("piml: invalid boolean value: %w", err)
		}
		v.SetBool(b)
	case reflect.Struct: // <-- NEW CASE
		if v.Type() == reflect.TypeOf(time.Time{}) {
			t, err := time.Parse(time.RFC3339Nano, valueStr)
			if err != nil {
				return fmt.Errorf("piml: invalid time format: %w", err)
			}
			v.Set(reflect.ValueOf(t))
		} else {
			return fmt.Errorf("piml: cannot unmarshal primitive into %s", v.Kind())
		}
	default:
		return fmt.Errorf("piml: cannot unmarshal primitive into %s", v.Kind())
	}
	return nil
}

// indirect dereferences pointers until it gets a non-pointer.
// If forceAlloc is true, it will allocate new pointers.
func indirect(v reflect.Value, forceAlloc bool) reflect.Value {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if forceAlloc {
				v.Set(reflect.New(v.Type().Elem()))
			} else {
				return v // Return the nil pointer
			}
		}
		v = v.Elem()
	}
	return v
}

// findStructField finds a field in a struct by its piml tag.
func findStructField(v reflect.Value, key string) (reflect.Value, error) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		fieldT := t.Field(i)
		fieldV := v.Field(i)

		// 1. Check tag
		tag := fieldT.Tag.Get("piml")
		if tag == key {
			return fieldV, nil
		}

		// 2. Check default name (if no tag)
		if tag == "" && strings.ToLower(fieldT.Name) == key {
			return fieldV, nil
		}

		// 3. Recurse into anonymous/embedded structs *regardless* of tag
		if fieldT.Anonymous && fieldT.Type.Kind() == reflect.Struct {
			if f, err := findStructField(fieldV, key); err == nil {
				return f, nil // Found in embedded struct
			}
		}
	}
	return reflect.Value{}, fmt.Errorf("field %q not found", key)
}

// consumeChildren peeks and consumes all lines that are
// indented more than the given indent.
func (d *Decoder) consumeChildren(currentIndent int) {
	for {
		line, err := d.peek()
		if err != nil || line == nil {
			return // EOF or error
		}

		if line.indent > currentIndent {
			d.consume()
		} else {
			return // We found a line at our level or above
		}
	}
}
