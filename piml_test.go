package piml

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// --- Simple Roundtrip ---

type SimpleConfig struct {
	SiteName        string  `piml:"site_name"`
	Port            int     `piml:"port"`
	IsProduction    bool    `piml:"is_production"`
	Version         float64 `piml:"version"`
	InternalCounter int     `piml:"-"` // Should be ignored
}

func TestSimpleRoundtrip(t *testing.T) {
	input := SimpleConfig{
		SiteName:        "PIML Demo",
		Port:            8080,
		IsProduction:    false,
		Version:         1.2,
		InternalCounter: 5, // This should not be marshalled
	}

	expectedPIML := `(site_name) PIML Demo
(port) 8080
(is_production) false
(version) 1.2
`

	// Test Marshal
	data, err := Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if strings.TrimSpace(string(data)) != strings.TrimSpace(expectedPIML) {
		t.Fatalf("Marshal() output mismatch:\nExpected:\n%s\nGot:\n%s", expectedPIML, string(data))
	}

	// Test Unmarshal
	var output SimpleConfig
	if err := Unmarshal(data, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Compare everything except the ignored field
	if input.SiteName != output.SiteName ||
		input.Port != output.Port ||
		input.IsProduction != output.IsProduction ||
		input.Version != output.Version {
		t.Fatalf("Unmarshal() result mismatch:\nExpected:\n%+v\nGot:\n%+v", input, output)
	}

	if output.InternalCounter != 0 {
		t.Fatalf("Unmarshal() did not ignore InternalCounter, expected 0, got %d", output.InternalCounter)
	}
}

// --- Nil Handling (Per Spec) ---

type NilConfig struct {
	Admin    *string        `piml:"admin"`
	Features []string       `piml:"features"`
	Aliases  map[string]int `piml:"aliases"`
	SiteName string         `piml:"site_name"`
}

func TestNilHandling(t *testing.T) {
	t.Run("Marshal", func(t *testing.T) {
		input := NilConfig{
			Admin:    nil,
			Features: []string{}, // Empty slice
			Aliases:  nil,
			SiteName: "Test",
		}
		expectedPIML := `(admin) nil
(features) nil
(aliases) nil
(site_name) Test
`
		data, err := Marshal(input)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		if strings.TrimSpace(string(data)) != strings.TrimSpace(expectedPIML) {
			t.Fatalf("Marshal() output mismatch:\nExpected:\n%s\nGot:\n%s", expectedPIML, string(data))
		}
	})

	t.Run("Unmarshal", func(t *testing.T) {
		pimlData := []byte(`
(admin) nil
(features) nil
(aliases) nil
(site_name) Test
`)
		var output NilConfig
		if err := Unmarshal(pimlData, &output); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}

		if output.Admin != nil {
			t.Errorf("Expected Admin to be nil, got %v", *output.Admin)
		}
		if output.Features != nil {
			t.Errorf("Expected Features to be nil, got %v", output.Features)
		}
		if output.Aliases != nil {
			t.Errorf("Expected Aliases to be nil, got %v", output.Aliases)
		}
		if output.SiteName != "Test" {
			t.Errorf("Expected SiteName 'Test', got %q", output.SiteName)
		}
	})
}

// --- Complex Roundtrip ---

type User struct {
	ID   int    `piml:"id"`
	Name string `piml:"name"`
}

type DBConfig struct {
	Host string `piml:"host"`
	Port int    `piml:"port"`
}

type ComplexConfig struct {
	Description string    `piml:"description"`
	Database    *DBConfig `piml:"database"`
	Admins      []*User   `piml:"admins"`
	Features    []string  `piml:"features"`
}

func TestComplexRoundtrip(t *testing.T) {
	input := ComplexConfig{
		Description: "This is a\nmulti-line description.",
		Database: &DBConfig{
			Host: "localhost",
			Port: 5432,
		},
		Admins: []*User{
			{ID: 1, Name: "Alice"},
			{ID: 2, Name: "Bob"},
		},
		Features: []string{"auth", "logging", "metrics"},
	}

	// Test Marshal -> Unmarshal
	data, err := Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var output ComplexConfig
	if err := Unmarshal(data, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Deep compare
	if !reflect.DeepEqual(input, output) {
		t.Fatalf("Roundtrip failed:\nInput:\n%+v\n\nOutput:\n%+v", input, output)
	}
}

// --- Embedded Structs ---

type BaseSettings struct {
	Timeout int `piml:"timeout"`
}

type AppSettings struct {
	BaseSettings
	AppName string `piml:"app_name"`
}

func TestEmbeddedStructs(t *testing.T) {
	pimlData := []byte(`
(app_name) My-App
(timeout) 30
`)
	var output AppSettings
	if err := Unmarshal(pimlData, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if output.AppName != "My-App" {
		t.Errorf("Expected AppName 'My-App', got %q", output.AppName)
	}
	if output.Timeout != 30 {
		t.Errorf("Expected Timeout 30, got %d", output.Timeout)
	}
}

// --- Keys With Spaces ---

type Profile struct {
	FirstName string `piml:"first name"`
	LastName  string `piml:"last name"`
}

func TestKeysWithSpaces(t *testing.T) {
	pimlData := []byte(`
(first name) John
(last name) Doe
`)
	var output Profile
	if err := Unmarshal(pimlData, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	expected := Profile{FirstName: "John", LastName: "Doe"}
	if output != expected {
		t.Fatalf("Unmarshal() mismatch:\nExpected:\n%+v\nGot:\n%+v", expected, output)
	}

	// Test Marshal
	data, err := Marshal(expected)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	expectedPIML := `(first name) John
(last name) Doe
`
	if strings.TrimSpace(string(data)) != strings.TrimSpace(expectedPIML) {
		t.Fatalf("Marshal() output mismatch:\nExpected:\n%s\nGot:\n%s", expectedPIML, string(data))
	}
}

// --- Comment Handling ---

func TestCommentHandling(t *testing.T) {
	pimlData := []byte(`
# This is a full-line comment
(host) localhost # This is now part of the value
# Another comment
(port) 5432
(description)
  This is a multi-line string. # Comments are allowed here
  # And on their own line.
  Even with weird indentation.
`)
	var output DBConfig
	if err := Unmarshal(pimlData, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if output.Host != "localhost # This is now part of the value" {
		t.Errorf("Expected Host 'localhost # This is now part of the value', got %q", output.Host)
	}
	if output.Port != 5432 {
		t.Errorf("Expected Port 5432, got %d", output.Port)
	}
}

// --- Unmarshal Errors ---

func TestUnmarshalErrors(t *testing.T) {
	t.Run("Invalid key format", func(t *testing.T) {
		pimlData := []byte(`(age 30`)
		var output SimpleConfig
		err := Unmarshal(pimlData, &output)
		if !errors.Is(err, ErrSyntax) {
			t.Fatalf("Expected ErrSyntax, got %v", err)
		}
	})

	t.Run("Type mismatch", func(t *testing.T) {
		pimlData := []byte(`(port) thirty`)
		var output SimpleConfig
		err := Unmarshal(pimlData, &output)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "invalid integer value") {
			t.Fatalf("Expected integer error, got %v", err)
		}
	})

	t.Run("Assign nil to int", func(t *testing.T) {
		pimlData := []byte(`(port) nil`)
		var output SimpleConfig
		err := Unmarshal(pimlData, &output)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "cannot assign nil to non-nillable type") {
			t.Fatalf("Expected nil assignment error, got %v", err)
		}
	})

	t.Run("Assign nil to string", func(t *testing.T) {
		pimlData := []byte(`(site_name) nil`)
		var output SimpleConfig
		err := Unmarshal(pimlData, &output)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !strings.Contains(err.Error(), "cannot assign nil to non-nillable type") {
			t.Fatalf("Expected nil assignment error, got %v", err)
		}
	})

	t.Run("Non-pointer v", func(t *testing.T) {
		pimlData := []byte(`(port) 123`)
		var output SimpleConfig
		err := Unmarshal(pimlData, output) // Not a pointer
		if !errors.Is(err, ErrInvalidUnmarshal) {
			t.Fatalf("Expected ErrInvalidUnmarshal, got %v", err)
		}
	})
}

// --- Deeply Nested Objects ---

type Level3 struct {
	Name string `piml:"name"`
}
type Level2 struct {
	L3 *Level3 `piml:"level3"`
}
type Level1 struct {
	L2 *Level2 `piml:"level2"`
}

func TestDeeplyNestedObject(t *testing.T) {
	input := Level1{
		L2: &Level2{
			L3: &Level3{
				Name: "Deep",
			},
		},
	}

	data, err := Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var output Level1
	if err := Unmarshal(data, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(input, output) {
		t.Fatalf("Roundtrip failed:\nInput:\n%+v\n\nOutput:\n%+v", input, output)
	}
}

// --- Map[string]string ---

type MapConfig struct {
	Headers map[string]string `piml:"headers"`
}

func TestMapStringString(t *testing.T) {
	input := MapConfig{
		Headers: map[string]string{
			"Content-Type": "application/piml",
			"X-Test":       "true",
		},
	}

	data, err := Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var output MapConfig
	if err := Unmarshal(data, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(input, output) {
		t.Fatalf("Roundtrip failed:\nInput:\n%+v\n\nOutput:\n%+v", input, output)
	}
}

// --- List of Pointers to Primitives ---

type PtrListConfig struct {
	Names []*string `piml:"names"`
}

func TestListOfPointersToPrimitives(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	input := PtrListConfig{
		Names: []*string{
			strPtr("hello"),
			strPtr("world"),
		},
	}
	data, err := Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var output PtrListConfig
	if err := Unmarshal(data, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(input, output) {
		t.Fatalf("Roundtrip failed:\nInput:\n%+v\n\nOutput:\n%+v", input, output)
	}
}

// --- JSON/PIML Semantic Equivalency Test ---

// This struct matches testdata/one.json and testdata/one.piml
type ProductConfig struct {
	ID             int         `json:"id" piml:"id"`
	Title          string      `json:"title" piml:"title"`
	IsPublished    bool        `json:"is_published" piml:"is_published"`
	LastUpdated    *string     `json:"last_updated" piml:"last_updated"`
	ProductManager string      `json:"product manager" piml:"product manager"`
	Description    string      `json:"description" piml:"description"`
	Author         *Author     `json:"author" piml:"author"`
	Tags           []string    `json:"tags" piml:"tags"`
	Revisions      []*Revision `json:"revisions" piml:"revisions"`
	Metadata       interface{} `json:"metadata" piml:"metadata"` // Handles {}
	RelatedIDs     []int       `json:"related_ids" piml:"related_ids"`
}
type Author struct {
	Name  string `json:"name" piml:"name"`
	Email string `json:"email" piml:"email"`
}
type Revision struct {
	Timestamp string `json:"timestamp" piml:"timestamp"`
	Notes     string `json:"notes" piml:"notes"`
}

// TestJSONToPIMLSemanticEquivalency proves that our PIML file
// is semantically identical to its JSON counterpart,
// given our spec's `nil` ambiguity.
func TestJSONToPIMLSemanticEquivalency(t *testing.T) {
	// 1. Read the PIML file
	pimlData, err := os.ReadFile("testdata/one.piml")
	if err != nil {
		t.Fatalf("Failed to read testdata/one.piml: %v", err)
	}

	// 2. Unmarshal PIML into our Go struct
	var pimlConfig ProductConfig
	if err := Unmarshal(pimlData, &pimlConfig); err != nil {
		t.Fatalf("Failed to unmarshal PIML: %v", err)
	}

	// 3. Read the JSON file
	jsonData, err := os.ReadFile("testdata/one.json")
	if err != nil {
		t.Fatalf("Failed to read testdata/one.json: %v", err)
	}

	// 4. Unmarshal JSON into its Go struct
	var jsonConfig ProductConfig
	if err := json.Unmarshal(jsonData, &jsonConfig); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// 5. THE CRITICAL TEST
	// We must account for our spec's nil ambiguity.
	// JSON `[]` -> Go `[]string{}` (non-nil, len 0)
	// PIML `nil` -> Go `nil` (nil, len 0)
	// These are not DeepEqual, but are semantically equivalent for us.
	// We'll normalize the PIML struct to match JSON's behavior.

	if pimlConfig.RelatedIDs == nil {
		pimlConfig.RelatedIDs = []int{} // Normalize nil slice
	}
	if pimlConfig.Metadata == nil {
		pimlConfig.Metadata = map[string]interface{}{} // Normalize nil map
	}
	if pimlConfig.Tags == nil {
		pimlConfig.Tags = []string{} // Normalize nil slice
	}
	if pimlConfig.Revisions == nil {
		pimlConfig.Revisions = []*Revision{} // Normalize nil slice
	}

	// 6. Now, they must be identical.
	if !reflect.DeepEqual(pimlConfig, jsonConfig) {
		t.Logf("JSON Config: %+v\n", jsonConfig)
		t.Logf("PIML Config: %+v\n", pimlConfig)

		// Print a more detailed diff
		if !reflect.DeepEqual(pimlConfig.RelatedIDs, jsonConfig.RelatedIDs) {
			t.Errorf("RelatedIDs mismatch: JSON=%v (nil:%t) vs PIML=%v (nil:%t)",
				jsonConfig.RelatedIDs, jsonConfig.RelatedIDs == nil,
				pimlConfig.RelatedIDs, pimlConfig.RelatedIDs == nil)
		}
		if !reflect.DeepEqual(pimlConfig.Metadata, jsonConfig.Metadata) {
			t.Errorf("Metadata mismatch: JSON=%v (nil:%t) vs PIML=%v (nil:%t)",
				jsonConfig.Metadata, jsonConfig.Metadata == nil,
				pimlConfig.Metadata, pimlConfig.Metadata == nil)
		}

		t.Fatalf("Semantic equivalency failed. PIML struct does not match JSON struct after normalization.")
	}
}

func TestStructTest(t *testing.T) {
	type Address struct {
		Line1 string `piml:"line1"`
		Line2 string `piml:"line2"`
		City  string `piml:"city"`
		State string `piml:"state"`
	}

	type Phone struct {
		Number  string `piml:"number"`
		Country string `piml:"country"`
	}

	type User struct {
		Name           string    `piml:"name"`
		Address        Address   `piml:"address"`
		Phone          Phone     `piml:"phone"`
		Gender         string    `piml:"gender"`
		Alive          bool      `piml:"alive"`
		Nicknames      []string  `piml:"nicknames"`
		BirthYear      int       `piml:"birth_year"`
		BirthMonth     int       `piml:"birth_month"`
		BirthDay       int       `piml:"birth_day"`
		BirthTimestamp time.Time `piml:"birth_timestamp"`
		Children       []*User   `piml:"children"`
	}

	user := User{
		Name: "Alex King",
		Address: Address{
			Line1: "Line1",
			Line2: "Line2",
			City:  "City",
			State: "State",
		},
		Phone: Phone{
			Number:  "999-999-99-99",
			Country: "USA",
		},
		Gender:         "FEMALE",
		Alive:          false,
		Nicknames:      []string{"Nick1", "Nick2"},
		BirthYear:      1999,
		BirthMonth:     12,
		BirthDay:       10,
		BirthTimestamp: time.Now(),
		Children:       make([]*User, 0),
	}

	user.Children = append(user.Children, &User{
		Name:           "Child 1",
		Address:        Address{},
		Phone:          Phone{},
		Gender:         "MALE",
		Alive:          true,
		Nicknames:      nil,
		BirthYear:      1,
		BirthMonth:     2,
		BirthDay:       3,
		BirthTimestamp: time.Now(),
		Children:       nil,
	})

	userPiml, err := Marshal(user)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var newUser User
	if err := Unmarshal(userPiml, &newUser); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	fmt.Println(string(userPiml))

	if !reflect.DeepEqual(user, newUser) {
		t.Logf("user    : %+v\n", user)
		t.Logf("newUser : %+v\n", newUser)

		if user.BirthTimestamp.Equal(newUser.BirthTimestamp) {
			t.Logf("times are parsed correctly\n")
		} else {
			t.Logf("times are not parsed correctly\n")
		}

		if len(user.Children) != len(newUser.Children) {
			t.Fatalf("Children length are not parsed correctly\n")
		} else {
			t.Logf("Children length are parsed correctly\n")
		}

		if user.Children[0].Address.City != newUser.Children[0].Address.City {
			t.Fatalf("Children address city are not parsed correctly\n")
		} else {
			t.Logf("Children address city are parsed correctly\n")
		}

		if user.Children[0].Alive != newUser.Children[0].Alive {
			t.Fatalf("Alive is not parsed correctly\n")
		} else {
			t.Logf("Alive is parsed correctly\n")
		}

		if len(user.Children[0].Children) != len(newUser.Children[0].Children) {
			t.Logf("user    : %+v\n", user.Children[0].Children)
			t.Logf("newUser : %+v\n", newUser.Children[0].Children)
			t.Fatalf("Children are not parsed correctly\n")
		} else {
			t.Logf("Children parsed correctly\n")
		}

	}
}

func TestMultineStringCorrectly(t *testing.T) {
	type User struct {
		ID   int    `piml:"id"`
		Name string `piml:"name"`
	}

	type Config struct {
		SiteName     string    `piml:"Site Name"`
		Port         int       `piml:"port"`
		IsProduction bool      `piml:"is_production"`
		Admins       []*User   `piml:"admins"`
		LastUpdated  time.Time `piml:"last_updated"`
		Description  string    `piml:"description"`
	}

	pimlData := []byte(`
(Site Name) My Awesome Site
(port) 8080
(is_production) true
(admins)
  > (User) # This is used for metadata only
    (id) 1
    (name) Admin One
  > (User)
    (id) 2
    (name) Admin Two
(last_updated) 2023-11-10T15:30:00Z
(description)
  This is a multi-line
  description for the site.
  
  as well
  
  
  close
`)

	var cfg Config
	err := Unmarshal(pimlData, &cfg)
	if err != nil {
		t.Fatalf("Error unmarshalling: %v", err)
		return
	}

	expectedDesc := "This is a multi-line\ndescription for the site.\n\nas well\n\n\nclose"
	if cfg.Description != expectedDesc {
		// Use %q to make whitespace differences obvious.
		t.Fatalf("Description mismatch:\nExpected:\n%q\nGot:\n%q", expectedDesc, cfg.Description)
	}

	t.Logf("%+v\n", cfg)
	for _, admin := range cfg.Admins {
		if !strings.HasPrefix(admin.Name, "Admin") {
			t.Fatalf("Admin Name does not start with 'Admin'")
		}
	}
}

func TestMultilineWithError(t *testing.T) {
	type Config struct {
		SomeKey     string `piml:"some key"`
		Description string `piml:"description"`
	}

	pimlData := []byte(`
(some key) XXXX
(description)
  This is a multi-line
  description for the site.

  as well
`)

	var cfg Config
	err := Unmarshal(pimlData, &cfg)
	if err == nil {
		t.Fatalf("No error on unmarshalling, should be some errors due to newlines: %v", err)
	}
}

func TestComments(t *testing.T) {
	type Config struct {
		SomeKey     string `piml:"some key"`
		Description string `piml:"description"`
	}

	pimlData := []byte(`
(some key) XXXX
(description)
  This is a multi-line
  description for the site.
  # this is a comment and should be ignored
  \# this line should start with a hash
  as well
`)

	var cfg Config
	err := Unmarshal(pimlData, &cfg)
	if err != nil {
		t.Fatalf("Error on unmarshalling: %v", err)
	}

	expectedDesc := "This is a multi-line\ndescription for the site.\n# this line should start with a hash\nas well"
	if cfg.Description != expectedDesc {
		t.Fatalf("Description mismatch:\nExpected:\n%q\nGot:\n%q", expectedDesc, cfg.Description)
	}
}

func TestEscapedHashRoundtrip(t *testing.T) {
	type Note struct {
		Content string `piml:"content"`
	}

	input := Note{
		Content: "First line\n# Second line starts with a hash\nThird line.",
	}

	// Marshal the struct to PIML.
	// The marshaller should escape the line starting with #.
	pimlData, err := Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Unmarshal the PIML back into a new struct.
	// The unmarshaller should correctly handle the escaped hash.
	var output Note
	if err := Unmarshal(pimlData, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify that the roundtrip was successful.
	if !reflect.DeepEqual(input, output) {
		t.Fatalf("Roundtrip failed for escaped hash:\nInput:\n%+v\n\nOutput:\n%+v", input, output)
	}
}
