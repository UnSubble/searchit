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
		Origin:     "fuzz",
		IsFuzz:     true,
		FuzzData:   nil,
	}

	var buf bytes.Buffer
	tf := output.NewTextFormatter(&buf, false, false, false, false)
	if err := tf.Print(res); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	expected := "[+] 200 - 35 B\n  URL\n    https://host/admin\n\n"
	if buf.String() != expected {
		t.Errorf("expected:\n%q\ngot:\n%q", expected, buf.String())
	}
}

func TestFuzzOutput_Header(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/api",
		StatusCode: 200,
		Length:     35,
		Accepted:   true,
		Origin:     "fuzz",
		IsFuzz:     true,
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

	expectedText := "[+] 200 - 35 B\n  URL\n    https://host/api\n  Header\n    Authorization: Bearer admin-token\n\n"
	if textOut != expectedText {
		t.Errorf("expected:\n%q\ngot:\n%q", expectedText, textOut)
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
		Origin:     "fuzz",
		IsFuzz:     true,
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

	expected := "[+] 200 - 42 B\n  URL\n    https://host/profile\n  Cookie\n    session=abcdef123\n\n"
	if out != expected {
		t.Errorf("expected:\n%q\ngot:\n%q", expected, out)
	}
}

func TestFuzzOutput_POSTBody(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/login",
		StatusCode: 200,
		Length:     100,
		Accepted:   true,
		Origin:     "fuzz",
		IsFuzz:     true,
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

	expected := "[+] 200 - 100 B\n  URL\n    https://host/login\n  Body\n    username=admin&password=secret123\n\n"
	if out != expected {
		t.Errorf("expected:\n%q\ngot:\n%q", expected, out)
	}
}

func TestFuzzOutput_JSONBody(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/login",
		StatusCode: 200,
		Length:     100,
		Accepted:   true,
		Origin:     "fuzz",
		IsFuzz:     true,
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

	expected := "[+] 200 - 100 B\n  URL\n    https://host/login\n  Body\n    {\"user\":\"admin\",\"password\":\"secret123\"}\n\n"
	if out != expected {
		t.Errorf("expected:\n%q\ngot:\n%q", expected, out)
	}
}

func TestFuzzOutput_MultipleLocations(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/api?id=test",
		StatusCode: 200,
		Length:     150,
		Accepted:   true,
		Origin:     "fuzz",
		IsFuzz:     true,
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

	expected := "[+] 200 - 150 B\n  URL\n    https://host/api?id=test\n  Header\n    Authorization: Bearer admin-token\n  Cookie\n    session=abcdef\n\n"
	if out != expected {
		t.Errorf("expected:\n%q\ngot:\n%q", expected, out)
	}
}

func TestFuzzOutput_MultipleHeaders(t *testing.T) {
	res := engine.Result{
		URL:        "https://futurevera.thm",
		StatusCode: 200,
		Length:     512,
		Accepted:   true,
		Origin:     "fuzz",
		IsFuzz:     true,
		FuzzData: &engine.FuzzData{
			Fields: []engine.FuzzField{
				{Location: engine.LocationHeader, Name: "Host", Value: "admin.futurevera.thm"},
				{Location: engine.LocationHeader, Name: "Authorization", Value: "Bearer eyJhb..."},
			},
		},
	}

	var buf bytes.Buffer
	tf := output.NewTextFormatter(&buf, false, false, false, false)
	_ = tf.Print(res)
	out := buf.String()

	expected := "[+] 200 - 512 B\n  URL\n    https://futurevera.thm\n  Header\n    Host: admin.futurevera.thm\n    Authorization: Bearer eyJhb...\n\n"
	if out != expected {
		t.Errorf("expected:\n%q\ngot:\n%q", expected, out)
	}
}

func TestFuzzOutput_QuietMode(t *testing.T) {
	res := engine.Result{
		URL:        "https://futurevera.thm",
		StatusCode: 200,
		Length:     4605,
		Accepted:   true,
		Origin:     "fuzz",
		IsFuzz:     true,
		FuzzData: &engine.FuzzData{
			Fields: []engine.FuzzField{
				{Location: engine.LocationHeader, Name: "Host", Value: "admin.futurevera.thm"},
			},
		},
	}

	var buf bytes.Buffer
	tf := output.NewTextFormatter(&buf, true, false, false, false)
	if err := tf.Print(res); err != nil {
		t.Fatalf("Print failed: %v", err)
	}

	expected := "https://futurevera.thm\n"
	if buf.String() != expected {
		t.Errorf("expected quiet mode output %q, got %q", expected, buf.String())
	}
}

func TestFuzzOutput_NDJSONSchemaConsistency(t *testing.T) {
	res := engine.Result{
		URL:        "https://host/api",
		StatusCode: 200,
		Length:     100,
		Accepted:   true,
		Origin:     "fuzz",
		IsFuzz:     true,
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
		Origin:     "fuzz",
		IsFuzz:     true,
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
