package http

import (
	"net"
	"sync"
)

type connPool struct {
	mu   sync.Mutex
	idle map[string][]net.Conn
}

func newConnPool() *connPool {
	return &connPool{
		idle: make(map[string][]net.Conn),
	}
}

func (p *connPool) get(backend string) net.Conn {
	p.mu.Lock()
	defer p.mu.Unlock()

	conns := p.idle[backend]

	if len(conns) == 0 {
		return nil
	}

	conn := conns[len(conns)-1]
	p.idle[backend] = conns[:len(conns)-1]

	return conn
}

func (p *connPool) put(backend string, conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.idle[backend] = append(p.idle[backend], conn)
}

func (p *connPool) discard(conn net.Conn) {
	if conn != nil {
		conn.Close()
	}
}
