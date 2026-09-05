package http

import (
	"bufio"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

type RoundTripper interface {
	RoundTrip(r *http.Request, backend string) (*http.Response, error)
}

type Transport struct {
	dialTimeout time.Duration
	pool        *connPool
}

func NewTransport() *Transport {
	return &Transport{
		dialTimeout: 10 * time.Second,
		pool:        newConnPool(),
	}
}

type connBody struct {
	body     io.ReadCloser
	conn     *pooledConn
	backend  string
	pool     *connPool
	reusable bool
}

func (c *connBody) Read(p []byte) (int, error) {
	return c.body.Read(p)
}

func (c *connBody) Close() error {

	_, err := io.Copy(io.Discard, c.body)

	closeErr := c.body.Close()

	if err != nil {
		c.conn.conn.Close()
		return err
	}

	if closeErr != nil {
		c.conn.conn.Close()
		return closeErr
	}

	if c.reusable {
		log.Printf("PUT connection into pool: %s", c.backend)
		c.pool.put(c.backend, *c.conn)
	} else {
		c.conn.conn.Close()
	}

	return nil
}

func (t *Transport) RoundTrip(
	r *http.Request,
	backend string,
) (*http.Response, error) {

	pc := t.pool.get(backend)
	reused := pc != nil

	if pc == nil {

		log.Printf("DIAL new backend connection: %s", backend)
		conn, err := net.DialTimeout(
			"tcp",
			backend,
			t.dialTimeout,
		)
		if err != nil {
			return nil, err
		}

		pc = &pooledConn{
			conn:   conn,
			reader: bufio.NewReader(conn),
		}
	} else {
		log.Printf("REUSE backend connection: %s", backend)
	}

	resp, err := t.roundTripConn(r, backend, pc)

	if err == nil {
		return resp, nil
	}

	// The pooled connection may be stale.
	if reused {
		t.pool.discard(pc)

		conn, err := net.DialTimeout(
			"tcp",
			backend,
			t.dialTimeout,
		)
		if err != nil {
			return nil, err
		}

		pc = &pooledConn{
			conn:   conn,
			reader: bufio.NewReader(conn),
		}

		resp, err := t.roundTripConn(r, backend, pc)
		if err != nil {
			pc.conn.Close()
			return nil, err
		}

		return resp, nil
	}

	pc.conn.Close()
	return nil, err
}

func (t *Transport) roundTripConn(
	r *http.Request,
	backend string,
	pc *pooledConn,
) (*http.Response, error) {

	req := r.Clone(r.Context())

	req.URL.Scheme = "http"
	req.URL.Host = backend
	req.RequestURI = ""

	if err := req.Write(pc.conn); err != nil {
		return nil, err
	}

	resp, err := http.ReadResponse(pc.reader, req)
	if err != nil {
		return nil, err
	}

	log.Printf(
		"backend response: status=%d close=%v headers=%v",
		resp.StatusCode,
		resp.Close,
		resp.Header,
	)

	resp.Body = &connBody{
		body:     resp.Body,
		conn:     pc,
		backend:  backend,
		pool:     t.pool,
		reusable: !resp.Close,
	}

	return resp, nil
}
