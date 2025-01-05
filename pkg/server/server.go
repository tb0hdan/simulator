package server

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	shutdownPollIntervalMax = 500 * time.Millisecond
)

type Server struct {
	mu            sync.Mutex
	activeConn    map[*conn]struct{}
	inShutdown    atomic.Bool
	listeners     map[*net.Listener]struct{}
	listenerGroup sync.WaitGroup
	gracePeriod   time.Duration
}

func (s *Server) closeListenersLocked() error {
	var err error
	for ln := range s.listeners {
		if cerr := (*ln).Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}

func (s *Server) closeIdleConns() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	quiescent := true
	for c := range s.activeConn {
		st, unixSec := c.getState()
		// Issue 22682: treat StateNew connections as if
		// they're idle if we haven't read the first request's
		// header in over 5 seconds.
		if st == StateNew && unixSec < time.Now().Unix()-5 {
			st = StateIdle
		}
		if st != StateIdle || unixSec == 0 {
			// Assume unixSec == 0 means it's a very new
			// connection, without state set yet.
			quiescent = false
			continue
		}
		c.rwc.Close()
		delete(s.activeConn, c)
	}
	return quiescent
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.inShutdown.Store(true)

	s.mu.Lock()
	lnerr := s.closeListenersLocked()

	s.mu.Unlock()
	s.listenerGroup.Wait()

	pollIntervalBase := time.Millisecond
	nextPollInterval := func() time.Duration {
		// Add 10% jitter.
		interval := pollIntervalBase + time.Duration(rand.Intn(int(pollIntervalBase/10)))
		// Double and clamp for next time.
		pollIntervalBase *= 2
		if pollIntervalBase > shutdownPollIntervalMax {
			pollIntervalBase = shutdownPollIntervalMax
		}
		return interval
	}

	timer := time.NewTimer(nextPollInterval())
	defer timer.Stop()
	for {
		if s.closeIdleConns() {
			return lnerr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			timer.Reset(nextPollInterval())
		}
	}
}

func (s *Server) newConn(rwc net.Conn) *conn {
	c := &conn{
		server: s,
		rwc:    rwc,
	}

	return c
}

func (s *Server) trackListener(ln *net.Listener, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listeners == nil {
		s.listeners = make(map[*net.Listener]struct{})
	}
	if add {
		if s.shuttingDown() {
			return false
		}
		s.listeners[ln] = struct{}{}
		s.listenerGroup.Add(1)
	} else {
		delete(s.listeners, ln)
		s.listenerGroup.Done()
	}
	return true
}

func (s *Server) trackConn(c *conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConn == nil {
		s.activeConn = make(map[*conn]struct{})
	}
	if add {
		s.activeConn[c] = struct{}{}
	} else {
		delete(s.activeConn, c)
	}
}

func (s *Server) shuttingDown() bool {
	return s.inShutdown.Load()
}

func (s *Server) Start(bindAddr string, gracePeriod time.Duration) error {
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	// set grace period
	s.gracePeriod = gracePeriod

	if !s.trackListener(&listener, true) {
		return ErrServerClosed
	}
	defer s.trackListener(&listener, false)

	for {
		rw, err := listener.Accept()
		if err != nil {
			if s.shuttingDown() {
				return ErrServerClosed
			}
			fmt.Println("Error accepting connection:", err)
			continue
		}
		c := s.newConn(rw)
		c.setState(StateNew)
		go c.handleConnection()
	}
}

func New() *Server {
	return &Server{}
}
