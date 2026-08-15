package piml

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Type inference patterns, per PIML spec v1.2.0.
var (
	intPattern   = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	floatPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)\.[0-9]+$`)
)

// A Decoder reads and decodes PIML values from an input byte slice.
type Decoder struct {
	data []byte
}

// rawLine is one significant line of the document. Comment lines are
// dropped at scan time; blank lines are kept because multi-line string
// blocks preserve them.
type rawLine struct {
	indent int    // number of leading spaces
	text   string // content with indentation stripped (empty for blank lines)
	blank  bool
	num    int // 1-based line number, for error messages
}

// parser walks the scanned lines.
type parser struct {
	lines []rawLine
	pos   int
}

// NewDecoder returns a new decoder that reads from data.
func NewDecoder(data []byte) *Decoder {
	return &Decoder{data: data}
}

// Decode reads the PIML document and stores it in the value pointed to by v.
func (d *Decoder) Decode(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return ErrInvalidUnmarshal
	}
	p, err := scan(string(d.data))
	if err != nil {
		return err
	}
	// The document root is an implicit object at indentation level 0.
	return p.decodeObject(rv, 0)
}

// scan splits the input into significant lines. Tabs in indentation are
// rejected; comment lines (first non-space char is an unescaped '#') are
// dropped; a trailing '\r' is stripped from every line.
func scan(input string) (*parser, error) {
	rawLines := strings.Split(input, "\n")
	lines := make([]rawLine, 0, len(rawLines))
	for i, l := range rawLines {
		num := i + 1
		l = strings.TrimSuffix(l, "\r")
		if strings.TrimSpace(l) == "" {
			lines = append(lines, rawLine{blank: true, num: num})
			continue
		}
		indent := 0
		for indent < len(l) {
			if l[indent] == ' ' {
				indent++
				continue
			}
			if l[indent] == '\t' {
				return nil, fmt.Errorf("%w: tabs are not allowed in indentation (line %d)", ErrSyntax, num)
			}
			break
		}
		text := l[indent:]
		if text[0] == '#' {
			continue // full-line comment, at any indentation
		}
		lines = append(lines, rawLine{indent: indent, text: text, num: num})
	}
	return &parser{lines: lines}, nil
}

func (p *parser) peek() *rawLine {
	if p.pos >= len(p.lines) {
		return nil
	}
	return &p.lines[p.pos]
}

func (p *parser) consume() {
	p.pos++
}

// nextContentLine returns the next non-blank line without consuming anything.
func (p *parser) nextContentLine() *rawLine {
	for i := p.pos; i < len(p.lines); i++ {
		if !p.lines[i].blank {
			return &p.lines[i]
		}
	}
	return nil
}

// scalar is the parsed value part of a key or array-item line.
type scalar struct {
	text   string
	quoted bool
	empty  bool
}

// parseScalar interprets everything after "(key)" or ">" on a line:
// quoting, inline comments, and the \# escape.
func parseScalar(rest string) scalar {
	s := strings.TrimSpace(rest)
	if s == "" {
		return scalar{empty: true}
	}

	if s[0] == '"' {
		// A value is quoted only when the quotes cleanly wrap it: the LAST
		// '"' on the line is followed by nothing but whitespace or an
		// inline comment. Anything else falls through and the value is
		// ordinary literal text, quotes included.
		last := strings.LastIndexByte(s, '"')
		if last > 0 {
			tail := s[last+1:]
			trimmed := strings.TrimSpace(tail)
			if trimmed == "" || (strings.HasPrefix(trimmed, "#") && len(tail) > len(trimmed)) {
				return scalar{text: s[1:last], quoted: true}
			}
		}
	}

	if s[0] == '#' {
		// The whole value position is a comment.
		return scalar{empty: true}
	}

	// Inline comment: '#' preceded by whitespace. A '\#' never matches
	// because its preceding character is a backslash.
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			s = strings.TrimRight(s[:i], " \t")
			break
		}
	}
	s = strings.ReplaceAll(s, `\#`, "#")
	if s == "" {
		return scalar{empty: true}
	}
	return scalar{text: s}
}

// decodeObject decodes key lines at exactly `indent` into a struct, map,
// or empty interface.
func (p *parser) decodeObject(v reflect.Value, indent int) error {
	v = indirect(v, true)
	if !v.IsValid() {
		return fmt.Errorf("piml: cannot unmarshal into invalid value")
	}

	// Schemaless decode: materialize a map[string]interface{}.
	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		m := map[string]interface{}{}
		mv := reflect.ValueOf(&m)
		if err := p.decodeObject(mv, indent); err != nil {
			return err
		}
		v.Set(reflect.ValueOf(m))
		return nil
	}

	isMap := v.Kind() == reflect.Map
	isStruct := v.Kind() == reflect.Struct
	if !isMap && !isStruct {
		return fmt.Errorf("piml: cannot unmarshal object into %s", v.Kind())
	}
	if isMap {
		if v.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("piml: map key must be string")
		}
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
	}

	seen := map[string]bool{}

	for {
		line := p.peek()
		if line == nil {
			return nil
		}
		if line.blank {
			p.consume()
			continue
		}
		if line.indent < indent {
			return nil // end of this object
		}
		if line.indent > indent {
			return fmt.Errorf("%w: unexpected indentation, expected %d spaces, got %d (line %d)", ErrSyntax, indent, line.indent, line.num)
		}

		if line.text[0] != '(' {
			return fmt.Errorf("%w: expected (key), got %q (line %d)", ErrSyntax, line.text, line.num)
		}
		closeParen := strings.IndexByte(line.text, ')')
		if closeParen == -1 {
			return fmt.Errorf("%w: invalid key format, missing ')' (line %d)", ErrSyntax, line.num)
		}
		key := line.text[1:closeParen]
		if key == "" {
			return fmt.Errorf("%w: empty key (line %d)", ErrSyntax, line.num)
		}
		if strings.ContainsRune(key, '(') {
			return fmt.Errorf("%w: key %q contains '(' (line %d)", ErrSyntax, key, line.num)
		}
		if seen[key] {
			return fmt.Errorf("%w: duplicate key %q (line %d)", ErrSyntax, key, line.num)
		}
		seen[key] = true

		val := parseScalar(line.text[closeParen+1:])
		p.consume()

		// A key with an inline value cannot also have a block.
		if !val.empty {
			if next := p.nextContentLine(); next != nil && next.indent > indent {
				return fmt.Errorf("%w: key %q has both a value and an indented block (line %d)", ErrSyntax, key, next.num)
			}
		}

		// Resolve the decode target.
		var target reflect.Value
		found := true
		if isStruct {
			f, ferr := findStructField(v, key)
			if ferr != nil {
				found = false // unknown field: skip it (and its block)
			} else {
				target = f
			}
		} else {
			target = reflect.New(v.Type().Elem())
		}

		if !found {
			if val.empty {
				p.skipBlock(indent)
			}
			continue
		}

		if !val.empty {
			if err := setScalar(target, val); err != nil {
				return fmt.Errorf("piml: error setting field %q: %w", key, err)
			}
		} else {
			if err := p.decodeBlock(target, indent); err != nil {
				return fmt.Errorf("piml: error decoding field %q: %w", key, err)
			}
		}

		if isMap {
			v.SetMapIndex(reflect.ValueOf(key), target.Elem())
		}
	}
}

// decodeBlock handles a bare key (or bare '>'): it inspects the following
// block, decides its type from the first content line, and decodes it.
// keyIndent is the indentation of the owning key/item line.
func (p *parser) decodeBlock(v reflect.Value, keyIndent int) error {
	next := p.nextContentLine()
	if next == nil || next.indent <= keyIndent {
		return setNilValue(v)
	}
	if next.indent != keyIndent+2 {
		return fmt.Errorf("%w: expected %d spaces of indentation, got %d (line %d)", ErrSyntax, keyIndent+2, next.indent, next.num)
	}
	switch {
	case strings.HasPrefix(next.text, `\(`), strings.HasPrefix(next.text, `\>`):
		return p.decodeMultiline(v, keyIndent)
	case next.text[0] == '(':
		return p.decodeObject(v, keyIndent+2)
	case next.text[0] == '>':
		return p.decodeArray(v, keyIndent+2)
	default:
		return p.decodeMultiline(v, keyIndent)
	}
}

// decodeArray decodes '>' item lines at exactly `indent` into a slice or
// empty interface.
func (p *parser) decodeArray(v reflect.Value, indent int) error {
	v = indirect(v, true)

	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		s := []interface{}{}
		sv := reflect.ValueOf(&s)
		if err := p.decodeArray(sv, indent); err != nil {
			return err
		}
		v.Set(reflect.ValueOf(s))
		return nil
	}

	if v.Kind() != reflect.Slice {
		return fmt.Errorf("piml: cannot unmarshal array into %s", v.Kind())
	}
	v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	elemType := v.Type().Elem()

	for {
		line := p.peek()
		if line == nil {
			return nil
		}
		if line.blank {
			p.consume()
			continue
		}
		if line.indent < indent {
			return nil // end of array
		}
		if line.indent > indent {
			return fmt.Errorf("%w: unexpected indentation in array (line %d)", ErrSyntax, line.num)
		}
		if line.text[0] != '>' {
			return fmt.Errorf("%w: expected array item '>', got %q (line %d)", ErrSyntax, line.text, line.num)
		}

		val := parseScalar(line.text[1:])

		elemPtr := reflect.New(elemType)

		if !val.empty {
			// "> (label)" followed by a deeper block is an object item;
			// the label is metadata and is ignored.
			isLabel := !val.quoted && len(val.text) > 2 &&
				val.text[0] == '(' && strings.IndexByte(val.text, ')') == len(val.text)-1
			if isLabel {
				if next := p.nextContentLineAfter(p.pos + 1); next != nil && next.indent > indent {
					p.consume()
					if err := p.decodeObject(elemPtr, indent+2); err != nil {
						return err
					}
					v.Set(reflect.Append(v, elemPtr.Elem()))
					continue
				}
			}
			p.consume()
			if next := p.nextContentLine(); next != nil && next.indent > indent {
				return fmt.Errorf("%w: array item has both a value and an indented block (line %d)", ErrSyntax, next.num)
			}
			if err := setScalar(elemPtr, val); err != nil {
				return err
			}
		} else {
			p.consume()
			if err := p.decodeBlock(elemPtr, indent); err != nil {
				return err
			}
		}

		v.Set(reflect.Append(v, elemPtr.Elem()))
	}
}

// nextContentLineAfter returns the next non-blank line at or after index i.
func (p *parser) nextContentLineAfter(i int) *rawLine {
	for ; i < len(p.lines); i++ {
		if !p.lines[i].blank {
			return &p.lines[i]
		}
	}
	return nil
}

// decodeMultiline decodes a multi-line string block owned by a key or item
// at keyIndent. The base indent is keyIndent+2; deeper indentation is
// content. Interior blank lines are preserved; leading and trailing
// whitespace of the value is trimmed.
func (p *parser) decodeMultiline(v reflect.Value, keyIndent int) error {
	base := keyIndent + 2
	var parts []string
	pendingBlanks := 0
	started := false

	for {
		line := p.peek()
		if line == nil {
			break
		}
		if line.blank {
			p.consume()
			if started {
				pendingBlanks++
			}
			continue
		}
		if line.indent <= keyIndent {
			break // end of block
		}
		if line.indent < base {
			return fmt.Errorf("%w: multi-line string content indented less than its base (line %d)", ErrSyntax, line.num)
		}
		p.consume()

		content := strings.Repeat(" ", line.indent-base) + line.text
		content = unescapeContentLine(content, !started)

		for ; pendingBlanks > 0; pendingBlanks-- {
			parts = append(parts, "")
		}
		parts = append(parts, content)
		started = true
	}

	result := strings.TrimRight(strings.Join(parts, "\n"), " \t")

	v = indirect(v, true)
	switch {
	case v.Kind() == reflect.String:
		v.SetString(result)
	case v.Kind() == reflect.Interface && v.NumMethod() == 0:
		v.Set(reflect.ValueOf(result))
	default:
		return fmt.Errorf("piml: cannot unmarshal multi-line string into %s", v.Kind())
	}
	return nil
}

// unescapeContentLine removes the positional escapes from a multi-line
// content line: \# anywhere a line starts (comment escape), and \( / \>
// on the first line of a block (type-determination escapes).
func unescapeContentLine(content string, firstLine bool) string {
	i := 0
	for i < len(content) && content[i] == ' ' {
		i++
	}
	rest := content[i:]
	if strings.HasPrefix(rest, `\#`) {
		return content[:i] + rest[1:]
	}
	if firstLine && (strings.HasPrefix(rest, `\(`) || strings.HasPrefix(rest, `\>`)) {
		return content[:i] + rest[1:]
	}
	return content
}

// skipBlock consumes all lines belonging to a block deeper than `indent`.
func (p *parser) skipBlock(indent int) {
	for {
		line := p.peek()
		if line == nil {
			return
		}
		if line.blank || line.indent > indent {
			p.consume()
			continue
		}
		return
	}
}

// setScalar stores a parsed scalar into v, honoring the target type in
// schema mode and the spec's inference rules for interface{} targets.
func setScalar(v reflect.Value, val scalar) error {
	// nil first: it needs the original (possibly pointer) value.
	if !val.quoted && val.text == "nil" {
		return setNilValue(v)
	}

	v = indirect(v, true)

	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		v.Set(reflect.ValueOf(inferValue(val)))
		return nil
	}

	if val.quoted {
		// A quoted value is a string; it only fits string-shaped targets.
		switch {
		case v.Kind() == reflect.String:
			v.SetString(val.text)
			return nil
		case v.Type() == reflect.TypeOf(time.Time{}):
			t, err := time.Parse(time.RFC3339Nano, val.text)
			if err != nil {
				return fmt.Errorf("piml: invalid time format: %w", err)
			}
			v.Set(reflect.ValueOf(t))
			return nil
		default:
			return fmt.Errorf("piml: cannot unmarshal quoted string into %s", v.Kind())
		}
	}

	valueStr := val.text
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
		switch valueStr {
		case "true":
			v.SetBool(true)
		case "false":
			v.SetBool(false)
		default:
			return fmt.Errorf("piml: invalid boolean value: %q (only lowercase true/false)", valueStr)
		}
	case reflect.Struct:
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

// inferValue applies the spec's schemaless type inference.
func inferValue(val scalar) interface{} {
	if val.quoted {
		return val.text
	}
	s := val.text
	switch {
	case s == "true":
		return true
	case s == "false":
		return false
	case intPattern.MatchString(s):
		i, _ := strconv.ParseInt(s, 10, 64)
		return i
	case floatPattern.MatchString(s):
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	return s
}

// setNilValue sets v to nil, erroring for non-nillable targets.
func setNilValue(v reflect.Value) error {
	if !v.CanSet() && v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface:
		if !v.IsNil() {
			v.Set(reflect.Zero(v.Type()))
		}
		return nil
	default:
		return fmt.Errorf("piml: cannot assign nil to non-nillable type %s", v.Type())
	}
}

// indirect dereferences pointers until it gets a non-pointer.
// If forceAlloc is true, it will allocate new pointers.
func indirect(v reflect.Value, forceAlloc bool) reflect.Value {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if forceAlloc {
				v.Set(reflect.New(v.Type().Elem()))
			} else {
				return v
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

		tag := fieldT.Tag.Get("piml")
		if tag == "-" {
			continue
		}

		tagName := tag
		if idx := strings.Index(tag, ","); idx != -1 {
			tagName = tag[:idx]
		}

		if tagName == key {
			return fieldV, nil
		}

		if tag == "" && strings.ToLower(fieldT.Name) == key {
			return fieldV, nil
		}

		if fieldT.Anonymous && fieldT.Type.Kind() == reflect.Struct {
			if f, err := findStructField(fieldV, key); err == nil {
				return f, nil
			}
		}
	}
	return reflect.Value{}, fmt.Errorf("field %q not found", key)
}
