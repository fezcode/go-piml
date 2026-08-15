package piml

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// The compliance suite lives in the piml spec repository and is shared by
// every implementation. It is expected as a sibling checkout; the tests
// skip when it is not present.
const complianceSuitePath = "../piml/tests/compliance.json"

type complianceSuite struct {
	Spec   string           `json:"spec"`
	Tests  []complianceCase `json:"tests"`
	Errors []complianceCase `json:"errors"`
}

type complianceCase struct {
	Name string          `json:"name"`
	Piml string          `json:"piml"`
	Json json.RawMessage `json:"json"`
	Note string          `json:"note"`
}

func loadComplianceSuite(t *testing.T) *complianceSuite {
	t.Helper()
	data, err := os.ReadFile(complianceSuitePath)
	if err != nil {
		t.Skipf("compliance suite not available: %v", err)
	}
	var suite complianceSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatalf("failed to parse compliance.json: %v", err)
	}
	return &suite
}

// normalize round-trips a value through encoding/json so that numeric
// types and map ordering are comparable.
func normalize(t *testing.T, v interface{}) interface{} {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var out interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	return out
}

func TestCompliance(t *testing.T) {
	suite := loadComplianceSuite(t)
	if len(suite.Tests) == 0 {
		t.Fatal("compliance suite has no tests")
	}

	for _, tc := range suite.Tests {
		t.Run(tc.Name, func(t *testing.T) {
			var expected interface{}
			if err := json.Unmarshal(tc.Json, &expected); err != nil {
				t.Fatalf("failed to parse expected JSON: %v", err)
			}

			// Schemaless decode.
			var got map[string]interface{}
			if err := Unmarshal([]byte(tc.Piml), &got); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if gotN := normalize(t, got); !reflect.DeepEqual(gotN, expected) {
				t.Fatalf("parse mismatch\nexpected: %#v\ngot:      %#v", expected, gotN)
			}

			// Round-trip: marshal the decoded value and decode again.
			out, err := Marshal(got)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			var again map[string]interface{}
			if err := Unmarshal(out, &again); err != nil {
				t.Fatalf("Unmarshal of marshalled output failed: %v\noutput:\n%s", err, out)
			}
			if againN := normalize(t, again); !reflect.DeepEqual(againN, expected) {
				t.Fatalf("round-trip mismatch\nexpected: %#v\ngot:      %#v\noutput:\n%s", expected, againN, out)
			}
		})
	}
}

func TestComplianceErrors(t *testing.T) {
	suite := loadComplianceSuite(t)
	if len(suite.Errors) == 0 {
		t.Fatal("compliance suite has no error cases")
	}

	for _, tc := range suite.Errors {
		t.Run(tc.Name, func(t *testing.T) {
			var got map[string]interface{}
			if err := Unmarshal([]byte(tc.Piml), &got); err == nil {
				t.Fatalf("expected a parse error, got: %#v", got)
			}
		})
	}
}
