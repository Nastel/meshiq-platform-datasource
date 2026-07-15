package plugin

// TODO(dev-only): tests for the dev-only response capture — delete together with capture.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureResponse_WritesBodyAndIndex(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(captureDirEnv, dir)

	body := []byte(`{"row-count": 1, "status": "SUCCESS"}`)
	queryModel := QueryModel{JKQL: "get log", Date: "100 TO 200", RepositoryID: "Repo$Org"}
	captureResponse(queryModel, body)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var captured string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			captured = e.Name()
		}
	}
	if captured == "" {
		t.Fatalf("no capture file written, dir has %v", entries)
	}

	got, err := os.ReadFile(filepath.Join(dir, captured))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Errorf("capture = %s, want the body verbatim", got)
	}

	index, err := os.ReadFile(filepath.Join(dir, "queries.log"))
	if err != nil {
		t.Fatal(err)
	}
	line := string(index)
	for _, want := range []string{captured, "get log", "100 TO 200", "Repo$Org"} {
		if !strings.Contains(line, want) {
			t.Errorf("queries.log = %q, want it to contain %q", line, want)
		}
	}
}

func TestCaptureResponse_NoOpWhenUnset(t *testing.T) {
	t.Setenv(captureDirEnv, "")
	// Must not write anywhere or fail; just returns.
	captureResponse(QueryModel{JKQL: "get log"}, []byte(`{}`))
}
