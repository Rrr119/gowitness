package favicon

import "testing"

func TestExtractURLPrefersIconLinkAndResolves(t *testing.T) {
	document := `<html><head>
		<link rel="stylesheet" href="/assets/app.css">
		<link rel="shortcut icon" href="/static/favicon-32.png?v=2">
		<link rel="apple-touch-icon" href="/apple.png">
	</head><body>ok</body></html>`

	got := ExtractURL(document, "https://target.example/login?x=1")
	if want := "https://target.example/static/favicon-32.png?v=2"; got != want {
		t.Fatalf("ExtractURL() = %q, want %q", got, want)
	}
}

func TestExtractURLDefaultsToFaviconICO(t *testing.T) {
	got := ExtractURL("<html><head><title>x</title></head></html>", "http://target.example:8080/path")
	if want := "http://target.example:8080/favicon.ico"; got != want {
		t.Fatalf("ExtractURL() = %q, want %q", got, want)
	}
}

func TestNormalizeURLRemovesCacheBusterAndWWW(t *testing.T) {
	got := NormalizeURL("HTTPS://WWW.Example.COM/favicon.ico?ver=123#x")
	if want := "https://example.com/favicon.ico"; got != want {
		t.Fatalf("NormalizeURL() = %q, want %q", got, want)
	}
}
