package plugin

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func TestAttachRawResponse_SmallResponseKeptWhole(t *testing.T) {
	frame := data.NewFrame("")
	response := ServiceResponse{"status": "SUCCESS", "row-count": json.Number("1")}

	attachRawResponse(frame, response)

	custom, ok := frame.Meta.Custom.(map[string]interface{})
	if !ok {
		t.Fatalf("Meta.Custom = %#v, want map[string]interface{}", frame.Meta.Custom)
	}
	raw, ok := custom["rawResponse"].(json.RawMessage)
	if !ok {
		t.Fatalf("rawResponse = %#v, want json.RawMessage", custom["rawResponse"])
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("rawResponse doesn't decode: %v", err)
	}
	if decoded["status"] != "SUCCESS" {
		t.Errorf("decoded status = %v, want SUCCESS", decoded["status"])
	}
	if _, hasNotice := custom["rawResponseNotice"]; hasNotice {
		t.Error("small response should not carry a truncation notice")
	}
}

func TestAttachRawResponse_LargeResponseTruncatedWithNotice(t *testing.T) {
	frame := data.NewFrame("")
	// A single field whose value alone exceeds maxRawResponseBytes once marshaled.
	response := ServiceResponse{"big": strings.Repeat("x", maxRawResponseBytes*2)}

	attachRawResponse(frame, response)

	custom, ok := frame.Meta.Custom.(map[string]interface{})
	if !ok {
		t.Fatalf("Meta.Custom = %#v, want map[string]interface{}", frame.Meta.Custom)
	}
	raw, ok := custom["rawResponse"].(json.RawMessage)
	if !ok {
		t.Fatalf("rawResponse = %#v, want json.RawMessage", custom["rawResponse"])
	}
	if len(raw) != maxRawResponseBytes {
		t.Errorf("truncated rawResponse length = %d, want %d", len(raw), maxRawResponseBytes)
	}
	if _, hasNotice := custom["rawResponseNotice"]; !hasNotice {
		t.Error("truncated response should carry a truncation notice")
	}
}
