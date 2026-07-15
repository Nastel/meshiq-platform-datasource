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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

const captureDirEnv = "MESHIQ_CAPTURE_DIR"

// captureSeq disambiguates files created within the same second (queries run concurrently).
// Seeded from the current nanosecond time rather than 0, so a restart within the same wall-clock
// second as an earlier capture doesn't collide with its filename (O_EXCL in writeCaptureFile
// would refuse the overwrite anyway; the seed avoids the pointless retry loop).
var captureSeq atomic.Int64

func init() {
	captureSeq.Store(time.Now().UnixNano())
}

// tsvEscaper replaces the characters that would corrupt queries.log's one-record-per-line,
// tab-separated format: an embedded tab would shift columns, an embedded newline would split one
// record across multiple lines. jKQL queries are routinely multi-line in the Monaco editor.
var tsvEscaper = strings.NewReplacer("\t", "\\t", "\n", "\\n", "\r", "\\r")

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

	name, err := writeCaptureFile(dir, body)
	if err != nil {
		log.DefaultLogger.Warn("response capture failed", "error", err)
		return
	}

	index := fmt.Sprintf("%s\t%s\t%s\t%s\n",
		name, tsvEscaper.Replace(queryModel.RepositoryID), tsvEscaper.Replace(queryModel.Date), tsvEscaper.Replace(queryModel.JKQL))
	f, err := os.OpenFile(filepath.Join(dir, "queries.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.DefaultLogger.Warn("response capture index failed", "error", err)
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(index)
}

// writeCaptureFile creates a new, never-before-seen capture file and writes body to it,
// returning its name. O_EXCL guarantees the file didn't already exist — on the rare collision
// (two captures landing on the same counter value) it retries with the next one instead of
// silently overwriting an earlier capture.
func writeCaptureFile(dir string, body []byte) (string, error) {
	for {
		name := fmt.Sprintf("%s-%03d.json", time.Now().UTC().Format("20060102-150405"), captureSeq.Add(1))
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, writeErr := f.Write(body)
		closeErr := f.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return name, nil
	}
}
