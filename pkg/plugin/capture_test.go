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

// TestCaptureResponse_MultilineJkqlIndexedAsOneLine pins that a multi-line jKQL query (routine in
// the Monaco editor) still produces exactly one queries.log line per capture — an unescaped
// embedded newline would split one record across lines and corrupt the index.
func TestCaptureResponse_MultilineJkqlIndexedAsOneLine(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(captureDirEnv, dir)

	queryModel := QueryModel{JKQL: "get log\nwhere Severity = 'ERROR'\tfields Message", Date: "100 TO 200", RepositoryID: "Repo$Org"}
	captureResponse(queryModel, []byte(`{}`))

	index, err := os.ReadFile(filepath.Join(dir, "queries.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(index), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("queries.log = %q, want exactly 1 line, got %d", index, len(lines))
	}
	if strings.Count(lines[0], "\t") != 3 {
		t.Errorf("queries.log line = %q, want exactly 3 tab-separated fields after the filename", lines[0])
	}
}

// TestWriteCaptureFile_NeverOverwritesAnExistingFile pins that a filename collision (e.g. a
// process restart within the same wall-clock second as an earlier capture) retries with a new
// name instead of silently clobbering the earlier capture via os.WriteFile.
func TestWriteCaptureFile_NeverOverwritesAnExistingFile(t *testing.T) {
	dir := t.TempDir()

	first, err := writeCaptureFile(dir, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	// Force a same-name collision: the counter is monotonic, so nothing else can produce it
	// naturally within this test, but a colliding file on disk exercises the retry path exactly
	// like a genuine counter collision would.
	captureSeq.Add(-1)
	second, err := writeCaptureFile(dir, []byte("second"))
	if err != nil {
		t.Fatal(err)
	}

	if first == second {
		t.Fatalf("writeCaptureFile returned the same name twice: %q", first)
	}

	gotFirst, err := os.ReadFile(filepath.Join(dir, first))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotFirst) != "first" {
		t.Errorf("first capture = %q, want it untouched by the second write", gotFirst)
	}
}
