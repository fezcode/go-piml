package piml

import (
	"testing"
)

type UnmarshalOmitemptyStruct struct {
	Name string `piml:"name,omitempty"`
	Age  int    `piml:"age,omitempty"`
}

func TestUnmarshalOmitempty(t *testing.T) {
	data := []byte(`
(name) Alice
(age) 30
`)

	var s UnmarshalOmitemptyStruct
	err := Unmarshal(data, &s)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if s.Name != "Alice" {
		t.Errorf("Expected Name='Alice', got '%s'. The tag 'name,omitempty' might be confusing the unmarshaller.", s.Name)
	}
	if s.Age != 30 {
		t.Errorf("Expected Age=30, got %d. The tag 'age,omitempty' might be confusing the unmarshaller.", s.Age)
	}
}
