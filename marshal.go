package piml

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
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
	return e.encodeValue(rv, -1, false)
}

func (e *Encoder) write(s string) error {
	_, err := e.w.Write([]byte(s))
	return err
}

func indentOf(level int) string {
	if level <= 0 {
		return ""
	}
	return strings.Repeat("  ", level)
}

// resolve unwraps pointers and interfaces to the concrete value.
func resolve(v reflect.Value) reflect.Value {
	for {
		if v.Kind() == reflect.Ptr && !v.IsNil() {
			v = v.Elem()
			continue
		}
		if v.Kind() == reflect.Interface && !v.IsNil() {
			v = v.Elem()
			continue
		}
		return v
	}
}

// encodeValue is the main recursive marshalling function.
func (e *Encoder) encodeValue(v reflect.Value, indent int, inArray bool) error {
	if !v.IsValid() || isNilOrEmpty(v) {
		if inArray {
			return e.write(fmt.Sprintf("%s> nil\n", indentOf(indent)))
		}
		return e.write(" nil\n")
	}

	v = resolve(v)
	indentStr := indentOf(indent)

	switch v.Kind() {
	case reflect.Struct:
		// time.Time marshals as an RFC3339Nano scalar.
		if v.Type() == reflect.TypeOf(time.Time{}) {
			s := v.Interface().(time.Time).Format(time.RFC3339Nano)
			return e.writeRawScalar(s, indent, inArray)
		}
		if inArray {
			// The label is readability metadata; parsers ignore it.
			itemName := v.Type().Name()
			if itemName == "" {
				itemName = "item"
			}
			if err := e.write(fmt.Sprintf("%s> (%s)\n", indentStr, itemName)); err != nil {
				return err
			}
			return e.encodeStruct(v, indent)
		}
		if indent > -1 {
			if err := e.write("\n"); err != nil {
				return err
			}
		}
		return e.encodeStruct(v, indent)

	case reflect.Slice, reflect.Array:
		if inArray {
			// A nested list: bare '>' with its items one level deeper.
			if err := e.write(fmt.Sprintf("%s>\n", indentStr)); err != nil {
				return err
			}
			return e.encodeSliceItems(v, indent+1)
		}
		if err := e.write("\n"); err != nil {
			return err
		}
		return e.encodeSliceItems(v, indent+1)

	case reflect.Map:
		if inArray {
			if err := e.write(fmt.Sprintf("%s> (item)\n", indentStr)); err != nil {
				return err
			}
			return e.encodeMap(v, indent)
		}
		if err := e.write("\n"); err != nil {
			return err
		}
		return e.encodeMap(v, indent)

	case reflect.String:
		return e.encodeString(v.String(), indent, inArray)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return e.writeRawScalar(strconv.FormatInt(v.Int(), 10), indent, inArray)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return e.writeRawScalar(strconv.FormatUint(v.Uint(), 10), indent, inArray)
	case reflect.Float32, reflect.Float64:
		return e.writeRawScalar(strconv.FormatFloat(v.Float(), 'f', -1, 64), indent, inArray)
	case reflect.Bool:
		return e.writeRawScalar(strconv.FormatBool(v.Bool()), indent, inArray)

	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedType, v.Kind())
	}
}

// writeRawScalar emits a non-string scalar (numbers, booleans, times),
// which is never quoted.
func (e *Encoder) writeRawScalar(s string, indent int, inArray bool) error {
	if inArray {
		return e.write(fmt.Sprintf("%s> %s\n", indentOf(indent), s))
	}
	return e.write(fmt.Sprintf(" %s\n", s))
}

// encodeStruct handles marshalling a Go struct to PIML.
// `indent` is the level of the struct's own key line; fields go one deeper.
func (e *Encoder) encodeStruct(v reflect.Value, indent int) error {
	t := v.Type()
	fieldIndent := indent + 1
	indentStr := indentOf(fieldIndent)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldV := v.Field(i)

		tag := field.Tag.Get("piml")
		if tag == "-" {
			continue
		}

		tagName := tag
		omitempty := false
		if idx := strings.Index(tag, ","); idx != -1 {
			tagName = tag[:idx]
			if strings.Contains(tag[idx+1:], "omitempty") {
				omitempty = true
			}
		}
		if tagName == "" {
			tagName = strings.ToLower(field.Name)
		}
		if strings.ContainsAny(tagName, "()") {
			return fmt.Errorf("piml: key %q may not contain parentheses", tagName)
		}

		if omitempty && isEmptyValue(fieldV) {
			continue
		}

		if err := e.write(fmt.Sprintf("%s(%s)", indentStr, tagName)); err != nil {
			return err
		}
		if err := e.encodeValue(fieldV, fieldIndent, false); err != nil {
			return err
		}
	}
	return nil
}

// encodeSliceItems writes the '>' items of a slice at the given level.
func (e *Encoder) encodeSliceItems(v reflect.Value, indent int) error {
	for i := 0; i < v.Len(); i++ {
		if err := e.encodeValue(v.Index(i), indent, true); err != nil {
			return err
		}
	}
	return nil
}

// encodeMap handles marshalling a Go map to PIML, with sorted keys for
// deterministic output. `indent` is the level of the map's own key line.
func (e *Encoder) encodeMap(v reflect.Value, indent int) error {
	fieldIndent := indent + 1
	indentStr := indentOf(fieldIndent)

	keys := make([]string, 0, v.Len())
	for _, key := range v.MapKeys() {
		keyStr, ok := key.Interface().(string)
		if !ok {
			return errors.New("piml: map keys must be strings")
		}
		keys = append(keys, keyStr)
	}
	sort.Strings(keys)

	for _, keyStr := range keys {
		if keyStr == "" || strings.ContainsAny(keyStr, "()") {
			return fmt.Errorf("piml: key %q may not be empty or contain parentheses", keyStr)
		}
		if err := e.write(fmt.Sprintf("%s(%s)", indentStr, keyStr)); err != nil {
			return err
		}
		if err := e.encodeValue(v.MapIndex(keyStr2Value(keyStr, v)), fieldIndent, false); err != nil {
			return err
		}
	}
	return nil
}

// keyStr2Value builds the reflect key value for a map lookup.
func keyStr2Value(keyStr string, m reflect.Value) reflect.Value {
	return reflect.ValueOf(keyStr).Convert(m.Type().Key())
}

// encodeString handles marshalling a string, quoting or block-forming it
// so that it round-trips under the v1.2.0 parsing rules.
func (e *Encoder) encodeString(s string, indent int, inArray bool) error {
	indentStr := indentOf(indent)

	if strings.Contains(s, "\n") {
		// --- Multi-line string block ---
		if inArray {
			if err := e.write(fmt.Sprintf("%s>\n", indentStr)); err != nil {
				return err
			}
		} else {
			if err := e.write("\n"); err != nil {
				return err
			}
		}
		lineIndentStr := indentOf(indent + 1)
		for i, line := range strings.Split(s, "\n") {
			if strings.TrimSpace(line) == "" {
				if err := e.write("\n"); err != nil {
					return err
				}
				continue
			}
			if err := e.write(fmt.Sprintf("%s%s\n", lineIndentStr, escapeContentLine(line, i == 0))); err != nil {
				return err
			}
		}
		return nil
	}

	// --- Single-line string ---
	out := s
	if needsQuoting(s) {
		out = `"` + s + `"`
	}
	if inArray {
		return e.write(fmt.Sprintf("%s> %s\n", indentStr, out))
	}
	return e.write(fmt.Sprintf(" %s\n", out))
}

// escapeContentLine applies the positional escapes a multi-line content
// line needs to survive parsing: a leading '#' always, and a leading '('
// or '>' on the first line only.
func escapeContentLine(line string, firstLine bool) string {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	rest := line[i:]
	if strings.HasPrefix(rest, "#") {
		return line[:i] + `\` + rest
	}
	if firstLine && (strings.HasPrefix(rest, "(") || strings.HasPrefix(rest, ">")) {
		return line[:i] + `\` + rest
	}
	return line
}

// needsQuoting reports whether a single-line string value must be quoted
// to parse back as the same string.
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	if s != strings.TrimSpace(s) {
		return true // leading/trailing whitespace would be trimmed
	}
	switch s {
	case "nil", "true", "false":
		return true
	}
	if intPattern.MatchString(s) || floatPattern.MatchString(s) {
		return true
	}
	if s[0] == '"' || s[0] == '#' {
		return true
	}
	if strings.Contains(s, `\#`) {
		return true // would be unescaped to '#'
	}
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			return true // would start an inline comment
		}
	}
	return false
}

// isNilOrEmpty checks if a reflect.Value is nil, or an empty slice/map.
func isNilOrEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return true
		}
		return isNilOrEmpty(v.Elem())
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	}
	return false
}

// isEmptyValue checks if a value is "empty" according to Go's struct tag rules for omitempty.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}
