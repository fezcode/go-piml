package piml

import (
	"bytes"
	"strings"
	"testing"
)

type ComprehensiveOmitemptyStruct struct {
	Name     string            `piml:"name,omitempty"`
	Age      int               `piml:"age,omitempty"`
	Price    float64           `piml:"price,omitempty"`
	IsActive bool              `piml:"is_active,omitempty"`
	Tags     []string          `piml:"tags,omitempty"`
	Meta     map[string]string `piml:"meta,omitempty"`
	Ptr      *int              `piml:"ptr,omitempty"`
	Address  string            `piml:"address"` // Required field
}

func TestOmitempty_AllEmpty(t *testing.T) {
	s := ComprehensiveOmitemptyStruct{
		Address: "123 Main St",
	}

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	err := enc.Encode(s)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	got := buf.String()

	expectedMissing := []string{
		"name", "age", "price", "is_active", "tags", "meta", "ptr",
	}

	for _, field := range expectedMissing {
		if strings.Contains(got, field) {
			t.Errorf("Expected '%s' to be omitted, but it was found in output:\n%s", field, got)
		}
	}

	if !strings.Contains(got, "address") {
		t.Errorf("Expected 'address' to be present, but it was missing.")
	}
}

func TestOmitempty_NoneEmpty(t *testing.T) {
	val := 42
	s := ComprehensiveOmitemptyStruct{
		Name:     "Alice",
		Age:      30,
		Price:    19.99,
		IsActive: true,
		Tags:     []string{"a", "b"},
		Meta:     map[string]string{"key": "value"},
		Ptr:      &val,
		Address:  "123 Main St",
	}

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	err := enc.Encode(s)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	got := buf.String()

	expectedPresent := []string{
		"name", "age", "price", "is_active", "tags", "meta", "ptr", "address",
	}

	for _, field := range expectedPresent {
		if !strings.Contains(got, field) {
			t.Errorf("Expected '%s' to be present, but it was missing in output:\n%s", field, got)
		}
	}
}

func TestOmitempty_Partial(t *testing.T) {
	s := ComprehensiveOmitemptyStruct{
		Name:    "Bob",
		Address: "456 Side St",
		// Age: 0 (omit)
		// Price: 0.0 (omit)
		// IsActive: false (omit)
	}

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	err := enc.Encode(s)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	got := buf.String()

	if !strings.Contains(got, "name") {
		t.Errorf("Expected 'name' to be present.")
	}
	if strings.Contains(got, "age") {
		t.Errorf("Expected 'age' to be omitted.")
	}
}
