package piml

import (
	"os"
	"reflect"
	"testing"
)

func TestExamplePimlRoundtrip(t *testing.T) {
	data, err := os.ReadFile("example.piml")
	if err != nil {
		t.Fatalf("Failed to read example.piml: %v", err)
	}

	var collection VideoCollection
	if err := Unmarshal(data, &collection); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Marshal it back
	marshaled, err := Marshal(collection)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal it once more
	var collection2 VideoCollection
	if err := Unmarshal(marshaled, &collection2); err != nil {
		t.Fatalf("Second unmarshal failed: %v", err)
	}

	// Compare the two collections
	if !reflect.DeepEqual(collection, collection2) {
		t.Errorf("Roundtrip failed: collections are not equal")
	}
}
