package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestPrintJSONError(t *testing.T) {
	var out, errOut bytes.Buffer
	SetWriters(&out, &errOut)
	defer SetWriters(os.Stdout, os.Stderr)

	if err := PrintJSONError(errors.New("organization 'missing' not found")); err != nil {
		t.Fatalf("PrintJSONError returned error: %v", err)
	}
	var got ErrorOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Success {
		t.Fatal("expected success=false")
	}
	if got.ErrorCode != "ORG_NOT_FOUND" {
		t.Fatalf("expected ORG_NOT_FOUND, got %q", got.ErrorCode)
	}
	if got.Suggestion == "" {
		t.Fatal("expected suggestion")
	}
}
