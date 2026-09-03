package http

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"time"
)

type RoundTripper interface {
	RoundTrip(r *http.Request, backend string) (*http.Response, error)
}

type Transport struct {
	dialTimeout time.Duration
}

func NewTransport() *Transport {
	return &Transport{
		dialTimeout: 10 * time.Second,
	}
}

type connBody struct {
	body io.ReadCloser
	conn net.Conn
}

func (c *connBody) Read(p []byte) (int, error) {
	return c.body.Read(p)
}

func (c *connBody) Close() error {
	c.body.Close()
	return c.conn.Close()
}

func (t *Transport) RoundTrip(
	r *http.Request,
	backend string,
) (*http.Response, error) {

	conn, err := net.DialTimeout(
		"tcp",
		backend,
		t.dialTimeout,
	)
	if err != nil {
		return nil, err
	}

	req := r.Clone(r.Context())

	req.URL.Scheme = "http"
	req.URL.Host = backend

	// RequestURI is used by the server for incoming requests.
	// For an outgoing request it must be empty.
	req.RequestURI = ""

	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)

	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		conn.Close()
		return nil, err
	}

	resp.Body = &connBody{
		body: resp.Body,
		conn: conn,
	}

	return resp, nil
}
