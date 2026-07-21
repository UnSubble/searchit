package html_test

import (
	"reflect"
	"testing"

	"github.com/unsubble/searchit/internal/html"
)

func TestExtractLinks(t *testing.T) {
	input := []byte(`
<!DOCTYPE html>
<html>
<head>
	<link rel="stylesheet" href="/assets/style.css">
	<script src="https://cdn.example.com/app.js"></script>
</head>
<body>
	<a href="/admin/settings">Admin Settings</a>
	<a href="http://otherdomain.com/page">External</a>
	<a href="#fragment-only">Fragment</a>
	<a href="javascript:void(0)">JS link</a>
	<a href="mailto:admin@example.com">Email</a>
	<a href="tel:+12345">Phone</a>
	<img src="/images/logo.png" />
	<form action="/login" method="POST">
		<input type="text" name="user">
	</form>
</body>
</html>
	`)

	expected := []string{
		"/assets/style.css",
		"https://cdn.example.com/app.js",
		"/admin/settings",
		"http://otherdomain.com/page",
		"/images/logo.png",
		"/login",
	}

	got := html.ExtractLinks(input)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ExtractLinks got %v, expected %v", got, expected)
	}
}
