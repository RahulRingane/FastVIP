package http

import (
	"bufio"
	"log"
	"net"
	"sync"
	"time"
)

// pooledConn represents a connection that can be reused.
type pooledConn struct {
	conn     net.Conn
	reader   *bufio.Reader
	idleTime time.Time
}

// connPool manages a pool of reusable connections for different backends.
type connPool struct {
	mu              sync.Mutex
	idle            map[string][]pooledConn
	maxIdle         int
	idleTimeout     time.Duration
	cleanupInterval time.Duration
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

func newConnPool(
	maxIdle int,
	idleTimeout time.Duration,
	cleanupInterval time.Duration,
) *connPool {

	p := &connPool{
		idle:            make(map[string][]pooledConn),
		maxIdle:         maxIdle,
		idleTimeout:     idleTimeout,
		cleanupInterval: cleanupInterval,
		stopCh:          make(chan struct{}),
	}

	p.wg.Add(1)
	go p.cleanupLoop()

	return p
}

// get retrieves a pooled connection for the specified backend, if available.
func (p *connPool) get(backend string) *pooledConn {
	p.mu.Lock()
	defer p.mu.Unlock()

	conns := p.idle[backend]

	if len(conns) == 0 {
		return nil
	}

	for len(conns) > 0 {
		conn := conns[len(conns)-1]
		conns = conns[:len(conns)-1]

		if time.Since(conn.idleTime) > p.idleTimeout {
			conn.conn.Close()
			continue
		}

		p.idle[backend] = conns
		return &conn
	}

	p.idle[backend] = conns
	return nil
}

// put returns a connection to the pool for reuse.
func (p *connPool) put(backend string, conn pooledConn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.idle[backend]) >= p.maxIdle {
		// If the pool is full, close the connection instead of adding it back to the pool.
		conn.conn.Close()
		return
	}

	conn.idleTime = time.Now()

	p.idle[backend] = append(p.idle[backend], conn)
}

func (p *connPool) discard(conn *pooledConn) {
	if conn != nil && conn.conn != nil {
		conn.conn.Close()
	}
}

func (p *connPool) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanup()

		case <-p.stopCh:
			return
		}
	}
}

func (p *connPool) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()

	for backend, conns := range p.idle {
		kept := make([]pooledConn, 0, len(conns))

		for _, conn := range conns {
			if now.Sub(conn.idleTime) > p.idleTimeout {

				log.Printf(
					"closing idle backend connection: backend=%s",
					backend,
				)

				conn.conn.Close()
				continue
			}

			kept = append(kept, conn)
		}

		if len(kept) == 0 {
			delete(p.idle, backend)
		} else {
			p.idle[backend] = kept
		}
	}
}

func (p *connPool) Close() {
	close(p.stopCh)

	p.wg.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	for backend, conns := range p.idle {
		for _, conn := range conns {
			conn.conn.Close()
		}

		delete(p.idle, backend)
	}
}
