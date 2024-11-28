package http2tcp

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	authHeaderType    = `HTTP2TCP`
	httpHeaderUpgrade = `http2tcp/1.0`
)

func auth(r *http.Request, token string) bool {
	a := strings.Fields(r.Header.Get("Authorization"))
	if len(a) == 2 && a[0] == authHeaderType && a[1] == token {
		return true
	}
	return false
}

// type BeforeAccept func(r *http.Request) error

func Accept(w http.ResponseWriter, r *http.Request, token string) (io.ReadWriteCloser, *http.Request, error) {
	if !auth(r, token) {
		w.WriteHeader(401)
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return nil, r, fmt.Errorf(`accept: unauthorized`)
	}

	if upgrade := r.Header.Get(`Upgrade`); upgrade != httpHeaderUpgrade {
		http.Error(w, `upgrade error`, http.StatusBadRequest)
		return nil, r, fmt.Errorf(`upgrade error`)
	}

	w.Header().Add(`Content-Length`, `0`)
	w.Header().Add(`Connection`, `Upgrade`)
	w.Header().Add(`Upgrade`, httpHeaderUpgrade)
	w.WriteHeader(http.StatusSwitchingProtocols)
	local, bio, err := w.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, r, fmt.Errorf(`error hijacking`)
	}

	return &_ReadWriteCloser{
		Reader: bio,
		Writer: local,
		Closer: local,
	}, r, nil
}

type _ReadWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}
