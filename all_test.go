package http2tcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConn(t *testing.T) {
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw, req, err := Accept(w, r, `123`)
		if err != nil {
			t.Fatal(err)
		}
		defer rw.Close()
		_ = req
	}))

	s.Start()

	conn, err := Dial(s.URL, `123`, ``)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
}
