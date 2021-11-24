package git

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/net/proxy"
)

type httpProxyDialer struct {
	u *url.URL
}

func (h *httpProxyDialer) Dial(_, addr string) (net.Conn, error) {
	conn, err := net.Dial("tcp", h.u.Host)
	if err != nil {
		return nil, err
	}

	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Host: addr}}
	err = req.Write(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		_ = conn.Close()
		return nil, fmt.Errorf("CONNECT response to %s was not 2xx", addr)
	}

	return conn, nil
}

func httpProxy(u *url.URL, _ proxy.Dialer) (proxy.Dialer, error) {
	return &httpProxyDialer{u}, nil
}
