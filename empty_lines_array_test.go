package piml

import (
	"os"
	"testing"
)

type Video struct {
	Name string `piml:"name"`
	Url  string `piml:"url"`
}

type VideoCollection struct {
	Videos []Video `piml:"videos"`
}

func TestExamplePimlWithEmptyLines(t *testing.T) {
	data, err := os.ReadFile("example.piml")
	if err != nil {
		t.Fatalf("Failed to read example.piml: %v", err)
	}

	var collection VideoCollection
	if err := Unmarshal(data, &collection); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	expectedCount := 9
	if len(collection.Videos) != expectedCount {
		t.Errorf("Expected %d videos, got %d", expectedCount, len(collection.Videos))
	}

	// Verify a few items to ensure data integrity
	if collection.Videos[0].Name != "Tears by Health" {
		t.Errorf("First video name mismatch. Got: %s", collection.Videos[0].Name)
	}
	if collection.Videos[1].Name != "Even Though - Morcheeba" {
		t.Errorf("Second video name mismatch. Got: %s", collection.Videos[1].Name)
	}
}

func TestExamplePimlWithMultipleEmptyLines(t *testing.T) {
	pimlContent := `
(items)
  > (item)
    (name) Item 1


  > (item)
    (name) Item 2
`
	type SimpleItem struct {
		Name string `piml:"name"`
	}
	type Container struct {
		Items []SimpleItem `piml:"items"`
	}

	var c Container
	if err := Unmarshal([]byte(pimlContent), &c); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(c.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(c.Items))
	}
	if c.Items[0].Name != "Item 1" {
		t.Errorf("Item 1 mismatch: %s", c.Items[0].Name)
	}
	if c.Items[1].Name != "Item 2" {
		t.Errorf("Item 2 mismatch: %s", c.Items[1].Name)
	}
}
