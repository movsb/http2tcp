package http2tcp

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
)

func Dial(server string, token string, userAgent string) (io.ReadWriteCloser, error) {
	req, err := http.NewRequest(http.MethodGet, server, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(`Connection`, `upgrade`)
	req.Header.Add(`Upgrade`, httpHeaderUpgrade)
	req.Header.Add(`Authorization`, fmt.Sprintf(`%s %s`, authHeaderType, token))

	if userAgent != `` {
		req.Header.Add(`User-Agent`, userAgent)
	}

	hc := &http.Client{
		Transport: &http.Transport{
			TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
		},
	}

	rsp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}

	if rsp.StatusCode != http.StatusSwitchingProtocols {
		rsp.Body.Close()
		buf := bytes.NewBuffer(nil)
		rsp.Write(buf)
		return nil, fmt.Errorf("statusCode != 101:\n%s", buf.String())
	}

	// TODO 严格判断 Upgrade 协议头和 Connection。
	// https://blog.twofei.com/1485/

	return rsp.Body.(io.ReadWriteCloser), nil
}
