package jkql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/experimental"
)

// TestGoldenFrames locks the converter's output for a corpus of real dataservice responses
// (testdata/captures — captured live, then sanitized). Each capture runs through
// BuildDataModel + BuildDataFrame and the resulting frame is compared against its golden file
// in testdata/golden. To regenerate the golden files after an intentional converter change:
//
//	UPDATE_GOLDEN=1 go test ./pkg/jkql -run TestGoldenFrames
func TestGoldenFrames(t *testing.T) {
	update := os.Getenv("UPDATE_GOLDEN") != ""

	files, err := filepath.Glob(filepath.Join("testdata", "captures", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no capture fixtures found in testdata/captures")
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			rs := make(map[string]interface{})
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.UseNumber() // decode exactly like the plugin's parseServiceResponse
			if err := decoder.Decode(&rs); err != nil {
				t.Fatalf("decode capture: %v", err)
			}

			frame := BuildDataFrame(BuildDataModel(rs, nil), nil)
			experimental.CheckGoldenJSONFrame(t, filepath.Join("testdata", "golden"), name, frame, update)
		})
	}
}
