# go-piml

`go-piml` is a Go package that provides functionality to marshal and unmarshal data to and from the PIML (Parenthesis Intended Markup Language) format. PIML is a human-readable, indentation-based data serialization format designed for configuration files and simple data structures.

Implements **PIML spec v1.2.0** — see the [spec repository](https://github.com/fezcode/piml). Conformance is verified against the shared compliance suite.

## Features

-   **Intuitive Syntax:** Easy-to-read key-value pairs, supporting nested structures.
-   **Go-like Tagging:** Uses `piml:"tag"` struct tags for flexible field mapping.
-   **Primitive Types:** Supports strings, integers, floats, and booleans.
-   **Quoted Strings:** `(zip) "08080"` forces a string value where inference would produce another type.
-   **Schemaless Decoding:** Unmarshals into `map[string]interface{}` / `interface{}` using the spec's type inference rules.
-   **Complex Types:** Handles structs, slices (arrays), maps, nested lists, and multi-line strings inside arrays.
-   **Nil Handling:** `nil` (and bare keys) represent null, empty slices, and empty maps.
-   **Omitempty Support:** Supports the `omitempty` struct tag option to skip fields with zero/empty values during marshalling.
-   **Multi-line Strings:** Base indent is key + 2; deeper indentation is preserved; the end of the value is trimmed.
-   **Comments:** Full-line comments (`#` at line start, any indentation) and inline comments (whitespace + `#`). A `#` glued to text is literal; escape with `\#` where needed.
-   **Strict Indentation:** Exactly 2 spaces per level; tabs and skipped levels are syntax errors, as are duplicate keys.
-   **Time Support:** Marshals and unmarshals `time.Time` values using RFC3339Nano format.

### Breaking changes in v1.3.0 (spec v1.2.0)

-   Inline comments are now stripped from values (`(host) localhost # note` → `"localhost"`).
-   Duplicate keys, tabs, and non-2-space indentation are now errors.
-   A bare `(key)` decodes as `nil` instead of being ignored.
-   Booleans accept only lowercase `true`/`false`.
-   The undocumented `>|` set syntax has been removed.

## PIML Format Overview

PIML uses a simple key-value structure. Keys are enclosed in parentheses `()`, and values follow. Indentation defines nesting.

```piml
(site_name) PIML Demo
(port) 8080
(is_production) false
(version) 1.2

(database)
  (host) localhost
  (port) 5432

(admins)
  > (User)
    (id) 1
    (name) Alice
  > (User)
    (id) 2
    (name) Bob

(features)
  > auth
  > logging
  > metrics

(description)
  This is a sample product description.
  It spans multiple lines.

  With an empty line in between.

(metadata)
(related_ids) nil
```

## Installation

To use `go-piml` in your Go project, simply run:

```bash
go get github.com/fezcode/go-piml
```

## Usage

### Marshalling Go Structs to PIML

Define your Go struct with `piml` tags:

```go
package main

import (
	"fmt"
	"time"
	"github.com/fezcode/go-piml"
)

type User struct {
	ID   int    `piml:"id"`
	Name string `piml:"name"`
}

type Config struct {
	SiteName    string    `piml:"site_name"`
	Port        int       `piml:"port"`
	IsProduction bool      `piml:"is_production"`
	Admins      []*User   `piml:"admins"`
	LastUpdated time.Time `piml:"last_updated"`
	Description string    `piml:"description"`
	OptionalVal string    `piml:"optional_val,omitempty"` // Will be omitted if empty
}

func main() {
	cfg := Config{
		SiteName:    "My Awesome Site",
		Port:        8080,
		IsProduction: true,
		Admins: []*User{
			{ID: 1, Name: "Admin One"},
			{ID: 2, Name: "Admin Two"},
		},
		LastUpdated: time.Date(2023, time.November, 10, 15, 30, 0, 0, time.UTC),
		Description: "This is a multi-line\ndescription for the site.",
		OptionalVal: "", // Empty, so it won't appear in the output
	}

	pimlData, err := piml.Marshal(cfg)
	if err != nil {
		fmt.Println("Error marshalling:", err)
		return
	}
	fmt.Println(string(pimlData))
}
```

Output:

```piml
(site_name) My Awesome Site
(port) 8080
(is_production) true
(admins)
  > (User)
    (id) 1
    (name) Admin One
  > (User)
    (id) 2
    (name) Admin Two
(last_updated) 2023-11-10T15:30:00Z
(description)
  This is a multi-line
  description for the site.
```

### Unmarshalling PIML to Go Structs

```go
package main

import (
	"fmt"
	"time"
	"github.com/fezcode/go-piml"
)

type User struct {
	ID   int    `piml:"id"`
	Name string `piml:"name"`
}

type Config struct {
	SiteName    string    `piml:"site_name"`
	Port        int       `piml:"port"`
	IsProduction bool      `piml:"is_production"`
	Admins      []*User   `piml:"admins"`
	LastUpdated time.Time `piml:"last_updated"`
	Description string    `piml:"description"`
}

func main() {
	pimlData := []byte(`
(site_name) My Awesome Site
(port) 8080
(is_production) true
(admins)
  > (User)
    (id) 1
    (name) Admin One
  > (User)
    (id) 2
    (name) Admin Two
(last_updated) 2023-11-10T15:30:00Z
(description)
  This is a multi-line
  description for the site.
`)

	var cfg Config
	err := piml.Unmarshal(pimlData, &cfg)
	if err != nil {
		fmt.Println("Error unmarshalling:", err)
		return
	}

	fmt.Printf("%+v\n", cfg)
	for _, admin := range cfg.Admins {
		fmt.Printf("Admin: ID=%d, Name=%s\n", admin.ID, admin.Name)
	}
}
```

Output:

```
{SiteName:My Awesome Site Port:8080 IsProduction:true Admins:[0xc0000a4000 0xc0000a4020] LastUpdated:2023-11-10 15:30:00 +0000 UTC Description:This is a multi-line
description for the site.}
Admin: ID=1, Name=Admin One
Admin: ID=2, Name=Admin Two
```

### Schemaless Decoding

Without a struct, PIML decodes into a `map[string]interface{}` using the spec's type inference (int64, float64, bool, nil, string):

```go
var doc map[string]interface{}
err := piml.Unmarshal([]byte(`
(port) 8080
(zip) "08080"
(tags)
  > auth
  > logging
`), &doc)
// doc["port"] == int64(8080), doc["zip"] == "08080"
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
