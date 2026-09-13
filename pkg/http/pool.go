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
	closeOnce       sync.Once
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

	conn := conns[len(conns)-1]
	conns = conns[:len(conns)-1]

	if len(conns) == 0 {
		delete(p.idle, backend)
	} else {
		p.idle[backend] = conns
	}

	return &conn
}

// put returns a connection to the pool for reuse.
func (p *connPool) put(backend string, conn pooledConn) {
	if conn.conn == nil {
		return
	}

	p.mu.Lock()

	if p.maxIdle <= 0 || len(p.idle[backend]) >= p.maxIdle {
		p.mu.Unlock()

		conn.conn.Close()
		return
	}

	conn.idleTime = time.Now()
	p.idle[backend] = append(p.idle[backend], conn)

	p.mu.Unlock()
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

	now := time.Now()
	var expired []pooledConn

	for backend, conns := range p.idle {
		kept := make([]pooledConn, 0, len(conns))

		for _, conn := range conns {
			if now.Sub(conn.idleTime) > p.idleTimeout {
				log.Printf(
					"closing idle backend connection: backend=%s",
					backend,
				)

				expired = append(expired, conn)
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

	p.mu.Unlock()

	// Close connections outside the mutex.
	for _, conn := range expired {
		conn.conn.Close()
	}
}

// Close closes the connection pool and all its connections.
func (p *connPool) Close() {
	p.closeOnce.Do(func() {
		close(p.stopCh)
		p.wg.Wait()

		p.mu.Lock()

		var connsToClose []pooledConn

		for backend, conns := range p.idle {
			connsToClose = append(connsToClose, conns...)
			delete(p.idle, backend)
		}

		p.mu.Unlock()

		// Close connections outside the mutex.
		for _, conn := range connsToClose {
			conn.conn.Close()
		}
	})
}
