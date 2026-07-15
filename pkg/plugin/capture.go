package plugin

// TODO(dev-only): raw response capture for development. Safe to comment out or delete — remove
// this file, capture_test.go, and the single captureResponse call in client.go; nothing else
// references it.
//
// When the MESHIQ_CAPTURE_DIR environment variable is set (see docker-compose.yaml), every
// dataservice response body is written there verbatim as <timestamp>-<n>.json — the same raw
// result-set format as the pkg/jkql/testdata/captures corpus, so a captured file can be used
// as a fixture directly. queries.log in the same directory maps each file to the jKQL, date
// range and repository that produced it (tab-separated), so any capture can be reproduced.
// When the variable is unset (production), the whole feature is one Getenv per query.

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

const captureDirEnv = "MESHIQ_CAPTURE_DIR"

// captureSeq disambiguates files created within the same second (queries run concurrently).
var captureSeq atomic.Int64

// captureResponse writes one response body to the capture directory and indexes it in
// queries.log. Failures are logged and ignored — a dev tool must never affect a query.
func captureResponse(queryModel QueryModel, body []byte) {
	dir := os.Getenv(captureDirEnv)
	if dir == "" {
		return
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.DefaultLogger.Warn("response capture failed", "dir", dir, "error", err)
		return
	}

	name := fmt.Sprintf("%s-%03d.json", time.Now().UTC().Format("20060102-150405"), captureSeq.Add(1))
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
		log.DefaultLogger.Warn("response capture failed", "file", name, "error", err)
		return
	}

	index := fmt.Sprintf("%s\t%s\t%s\t%s\n", name, queryModel.RepositoryID, queryModel.Date, queryModel.JKQL)
	f, err := os.OpenFile(filepath.Join(dir, "queries.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.DefaultLogger.Warn("response capture index failed", "error", err)
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(index)
}
