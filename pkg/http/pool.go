package http

import (
	"bufio"
	"net"
	"sync"
)

type pooledConn struct {
	conn   net.Conn
	reader *bufio.Reader
}

type connPool struct {
	mu   sync.Mutex
	idle map[string][]pooledConn
}

func newConnPool() *connPool {
	return &connPool{
		idle: make(map[string][]pooledConn),
	}
}

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

func (p *connPool) put(backend string, conn pooledConn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.idle[backend] = append(p.idle[backend], conn)
}

func (p *connPool) discard(conn *pooledConn) {
	if conn != nil && conn.conn != nil {
		conn.conn.Close()
	}
}
