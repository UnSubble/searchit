package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/unsubble/searchit/internal/engine"
	"github.com/unsubble/searchit/internal/output"
)

func TestFuzzOutput_URLOnly(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/admin",
		StatusCode: 200,
		Length:     35,
		Accepted:   true,
		FuzzData:   nil,
	}

	var buf bytes.Buffer
	tf := output.NewTextFormatter(&buf, false, false, false, false)
	if err := tf.Print(res); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "https://host/admin") {
		t.Errorf("expected URL in output, got: %s", out)
	}
	if strings.Contains(out, "Header:") || strings.Contains(out, "Cookie:") || strings.Contains(out, "Body:") || strings.Contains(out, "JSON:") {
		t.Errorf("expected no extra fields for URL-only fuzz, got: %s", out)
	}
}

func TestFuzzOutput_Header(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/api",
		StatusCode: 200,
		Length:     35,
		Accepted:   true,
		FuzzData: &engine.FuzzData{
			Fields: []engine.FuzzField{
				{Location: engine.LocationHeader, Name: "Authorization", Value: "Bearer admin-token"},
			},
		},
	}

	// Text format check
	var textBuf bytes.Buffer
	tf := output.NewTextFormatter(&textBuf, false, false, false, false)
	_ = tf.Print(res)
	textOut := textBuf.String()

	if !strings.Contains(textOut, "Header: Authorization=Bearer admin-token") {
		t.Errorf("expected Header in text output, got: %s", textOut)
	}

	// JSON format check
	var jsonBuf bytes.Buffer
	jf := output.NewJSONFormatter(&jsonBuf, false, false)
	_ = jf.Print(res)
	_ = jf.Close()

	var jsonArr []map[string]interface{}
	if err := json.Unmarshal(jsonBuf.Bytes(), &jsonArr); err != nil {
		t.Fatalf("failed to unmarshal JSON output: %v", err)
	}
	if len(jsonArr) != 1 {
		t.Fatalf("expected 1 result in JSON, got %d", len(jsonArr))
	}

	fuzzVal, ok := jsonArr[0]["fuzz"].([]interface{})
	if !ok || len(fuzzVal) != 1 {
		t.Fatalf("expected 1 fuzz field in JSON, got: %v", jsonArr[0]["fuzz"])
	}
	fieldMap := fuzzVal[0].(map[string]interface{})
	if fieldMap["location"] != "header" || fieldMap["name"] != "Authorization" || fieldMap["value"] != "Bearer admin-token" {
		t.Errorf("unexpected JSON fuzz field structure: %v", fieldMap)
	}
}

func TestFuzzOutput_Cookie(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/profile",
		StatusCode: 200,
		Length:     42,
		Accepted:   true,
		FuzzData: &engine.FuzzData{
			Fields: []engine.FuzzField{
				{Location: engine.LocationCookie, Name: "session", Value: "abcdef123"},
			},
		},
	}

	var buf bytes.Buffer
	tf := output.NewTextFormatter(&buf, false, false, false, false)
	_ = tf.Print(res)
	out := buf.String()

	if !strings.Contains(out, "Cookie: session=abcdef123") {
		t.Errorf("expected Cookie in text output, got: %s", out)
	}
}

func TestFuzzOutput_POSTBody(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/login",
		StatusCode: 200,
		Length:     100,
		Accepted:   true,
		FuzzData: &engine.FuzzData{
			Fields: []engine.FuzzField{
				{Location: engine.LocationBody, Value: "username=admin&password=secret123"},
			},
		},
	}

	var buf bytes.Buffer
	tf := output.NewTextFormatter(&buf, false, false, false, false)
	_ = tf.Print(res)
	out := buf.String()

	if !strings.Contains(out, "Body: username=admin&password=secret123") {
		t.Errorf("expected Body in text output, got: %s", out)
	}
}

func TestFuzzOutput_JSONBody(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/login",
		StatusCode: 200,
		Length:     100,
		Accepted:   true,
		FuzzData: &engine.FuzzData{
			Fields: []engine.FuzzField{
				{Location: engine.LocationJSON, Value: `{"user":"admin","password":"secret123"}`},
			},
		},
	}

	var buf bytes.Buffer
	tf := output.NewTextFormatter(&buf, false, false, false, false)
	_ = tf.Print(res)
	out := buf.String()

	if !strings.Contains(out, `JSON: {"user":"admin","password":"secret123"}`) {
		t.Errorf("expected JSON in text output, got: %s", out)
	}
}

func TestFuzzOutput_MultipleLocations(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/api?id=test",
		StatusCode: 200,
		Length:     150,
		Accepted:   true,
		FuzzData: &engine.FuzzData{
			Fields: []engine.FuzzField{
				{Location: engine.LocationHeader, Name: "Authorization", Value: "Bearer admin-token"},
				{Location: engine.LocationCookie, Name: "session", Value: "abcdef"},
			},
		},
	}

	var buf bytes.Buffer
	tf := output.NewTextFormatter(&buf, false, false, false, false)
	_ = tf.Print(res)
	out := buf.String()

	if !strings.Contains(out, "Header: Authorization=Bearer admin-token") {
		t.Errorf("missing Header line in output: %s", out)
	}
	if !strings.Contains(out, "Cookie: session=abcdef") {
		t.Errorf("missing Cookie line in output: %s", out)
	}
}

func TestFuzzOutput_MultiplePlaceholdersInOneLocation(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/api",
		StatusCode: 200,
		Length:     150,
		Accepted:   true,
		FuzzData: &engine.FuzzData{
			Fields: []engine.FuzzField{
				{Location: engine.LocationHeader, Name: "Authorization", Value: "Bearer admin:secret"},
			},
		},
	}

	var buf bytes.Buffer
	tf := output.NewTextFormatter(&buf, false, false, false, false)
	_ = tf.Print(res)
	out := buf.String()

	if !strings.Contains(out, "Header: Authorization=Bearer admin:secret") {
		t.Errorf("missing multiple placeholder header line: %s", out)
	}
}

func TestFuzzOutput_NDJSONSchemaConsistency(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/api",
		StatusCode: 200,
		Length:     100,
		Accepted:   true,
		FuzzData: &engine.FuzzData{
			Fields: []engine.FuzzField{
				{Location: engine.LocationHeader, Name: "X-Fuzz", Value: "val1"},
				{Location: engine.LocationBody, Value: "payload1"},
			},
		},
	}

	var buf bytes.Buffer
	ndf := output.NewNDJSONFormatter(&buf, false, false)
	if err := ndf.Print(res); err != nil {
		t.Fatalf("NDJSON Print failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse NDJSON line: %v", err)
	}

	fuzzSlice, ok := parsed["fuzz"].([]interface{})
	if !ok || len(fuzzSlice) != 2 {
		t.Fatalf("expected 2 fuzz fields in NDJSON schema, got: %v", parsed["fuzz"])
	}
}

func TestFuzzOutput_OmitFuzzWhenNil(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/admin",
		StatusCode: 200,
		Length:     35,
		Accepted:   true,
		FuzzData:   nil,
	}

	// JSON check
	var jsonBuf bytes.Buffer
	jf := output.NewJSONFormatter(&jsonBuf, false, false)
	_ = jf.Print(res)
	_ = jf.Close()

	if strings.Contains(jsonBuf.String(), `"fuzz"`) {
		t.Errorf("expected 'fuzz' key to be completely omitted, got: %s", jsonBuf.String())
	}

	// NDJSON check
	var ndjsonBuf bytes.Buffer
	ndf := output.NewNDJSONFormatter(&ndjsonBuf, false, false)
	_ = ndf.Print(res)

	if strings.Contains(ndjsonBuf.String(), `"fuzz"`) {
		t.Errorf("expected 'fuzz' key to be completely omitted in NDJSON, got: %s", ndjsonBuf.String())
	}
}
