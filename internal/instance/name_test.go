package instance

import "testing"

func TestNameFromURL(t *testing.T) {
	cases := map[string]string{
		"":                               "default",
		"not a url at all":               "default",
		"https://psazuse.jfrog.io":       "psazuse",
		"https://psazuse.jfrog.io/":      "psazuse",
		"https://jfrog-a.cloudlab.rocks": "jfrog-a",
		"http://10.0.0.1":                "10.0.0.1",
		"http://10.0.0.1/artifactory":    "10.0.0.1",
		"https://myhost:8081":            "myhost",
		"https://myhost":                 "myhost",
	}
	for in, want := range cases {
		if got := NameFromURL(in); got != want {
			t.Errorf("NameFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}
