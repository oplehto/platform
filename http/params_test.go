package http

import (
	"net/http"
	"net/url"
	"testing"
)

func TestDecodeParams(t *testing.T) {
	params := url.Values{}
	params.Set("i", "2")
	params.Set("s", "hello, world")
	u := url.URL{
		Scheme:   "http",
		Host:     "localhost",
		RawQuery: params.Encode(),
	}
	req, _ := http.NewRequest("GET", u.String(), nil)

	config := struct {
		I int
		S string
	}{}
	if err := DecodeParams(&config, req); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if got, want := config.I, 2; got != want {
		t.Errorf("unexpected value: got=%d want=%d", got, want)
	}
	if got, want := config.S, "hello, world"; got != want {
		t.Errorf("unexpected value: got=%v want=%v", got, want)
	}
}
