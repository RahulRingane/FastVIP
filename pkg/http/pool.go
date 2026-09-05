package http

import (
	"bufio"
	"net"
	"sync"
)

// pooledConn represents a connection that can be reused.
type pooledConn struct {
	conn   net.Conn
	reader *bufio.Reader
}

// connPool manages a pool of reusable connections for different backends.
type connPool struct {
	mu   sync.Mutex
	idle map[string][]pooledConn
}

// connBody wraps the response body and manages connection reuse.
func newConnPool() *connPool {
	return &connPool{
		idle: make(map[string][]pooledConn),
	}
}

// get retrieves a pooled connection for the specified backend, if available.
func (p *connPool) get(backend string) *pooledConn {
	p.mu.Lock()
	defer p.mu.Unlock()

	conns := p.idle[backend]

	if len(conns) == 0 {
		return nil
	}

	conn := conns[len(conns)-1]
	p.idle[backend] = conns[:len(conns)-1]

	return &conn
}

// put returns a connection to the pool for reuse.
func (p *connPool) put(backend string, conn pooledConn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.idle[backend] = append(p.idle[backend], conn)
}

// discard closes the connection and removes it from the pool.
func (p *connPool) discard(conn *pooledConn) {
	if conn != nil && conn.conn != nil {
		conn.conn.Close()
	}
}
