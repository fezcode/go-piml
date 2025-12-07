package piml

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type ComplianceTestCase struct {
	Name string          `json:"name"`
	Piml string          `json:"piml"`
	Json json.RawMessage `json:"json"`
	Note string          `json:"note"`
}

func TestCompliance(t *testing.T) {
	data, err := os.ReadFile("../piml/tests/compliance.json")
	if err != nil {
		t.Fatalf("Failed to read compliance.json: %v", err)
	}

	var tests []ComplianceTestCase
	if err := json.Unmarshal(data, &tests); err != nil {
		t.Fatalf("Failed to parse compliance.json: %v", err)
	}

	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			var v interface{}
			var err error

			switch tc.Name {
			case "Simple String":
				s := struct {
					Key string `piml:"key" json:"key"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			case "Empty Array":
				s := struct {
					List []string `piml:"list" json:"list"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			case "Empty Object":
				s := struct {
					Map map[string]string `piml:"map" json:"map"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			case "Integer":
				s := struct {
					Val int `piml:"val" json:"val"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			case "String looking like int":
				s := struct {
					Val int `piml:"val" json:"val"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			case "Nested List":
				s := struct {
					List []string `piml:"list" json:"list"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			case "Nested Object":
				type Child struct {
					Child string `piml:"child" json:"child"`
				}
				s := struct {
					Parent Child `piml:"parent" json:"parent"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			case "List of Objects":
				type Item struct {
					Name string `piml:"name" json:"name"`
				}
				s := struct {
					Users []Item `piml:"users" json:"users"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			case "Multiline String", "Multiline String with Blank Lines":
				s := struct {
					Desc string `piml:"desc" json:"desc"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			case "Keys with spaces":
				s := struct {
					MyKey string `piml:"my key" json:"my key"`
				}{}
				err = Unmarshal([]byte(tc.Piml), &s)
				v = s
			default:
				t.Fatalf("Unknown test case: %s", tc.Name)
			}

			if err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Marshal Go struct to JSON
			gotJSONBytes, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("JSON Marshal failed: %v", err)
			}

			// Normalize JSONs (compact)
			var expectedObj interface{}
			if err := json.Unmarshal(tc.Json, &expectedObj); err != nil {
				t.Fatalf("Failed to parse expected JSON: %v", err)
			}
			
			// Re-marshal to get canonical JSON string (to match gotJSONBytes)
			expectedJSONBytes, _ := json.Marshal(expectedObj)

			// We need to decode gotJSONBytes back to object to compare generic structure
			// because field order might differ, etc.
			var gotObj interface{}
			if err := json.Unmarshal(gotJSONBytes, &gotObj); err != nil {
				t.Fatalf("Failed to parse got JSON: %v", err)
			}

			if !reflect.DeepEqual(gotObj, expectedObj) {
				t.Errorf("Mismatch!\nExpected: %s\nGot:      %s", string(expectedJSONBytes), string(gotJSONBytes))
			}
		})
	}
}
