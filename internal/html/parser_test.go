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

func BenchmarkExtractLinks(b *testing.B) {
	input := []byte(`
<!DOCTYPE html>
<html>
<head>
	<link rel="stylesheet" href="/assets/style.css" type="text/css" media="all" id="main-css" data-version="1.0">
	<script src="https://cdn.example.com/app.js" type="text/javascript" async defer charset="utf-8" id="app-js"></script>
</head>
<body>
	<a href="/admin/settings" class="btn btn-primary nav-link active" id="admin-link" data-toggle="modal" data-target="#settings" aria-label="Settings" role="button" tabindex="0">Admin Settings</a>
	<a href="http://otherdomain.com/page" class="external-link" rel="noopener noreferrer" target="_blank" data-tracker="outbound">External</a>
	<img src="/images/logo.png" alt="Company Logo" class="logo img-responsive" id="main-logo" width="200" height="100" loading="lazy" decoding="async" />
	<form action="/login" method="POST" class="login-form form-horizontal" id="loginForm" name="login" autocomplete="on" novalidate onsubmit="return validate()">
	</form>
</body>
</html>
	`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		html.ExtractLinks(input)
	}
}
