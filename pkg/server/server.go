package server

import (
	"context"
	"log"
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
	logger        *log.Logger
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
	for activeConnection := range s.activeConn {
		state, unixSec := activeConnection.getState()
		// Issue 22682: treat StateNew connections as if
		// they're idle if we haven't read the first request's
		// header in over 5 seconds.
		if state == StateNew && unixSec < time.Now().Unix()-5 {
			state = StateIdle
		}
		if state != StateIdle || unixSec == 0 {
			// Assume unixSec == 0 means it's a very new
			// connection, without state set yet.
			quiescent = false
			continue
		}
		activeConnection.rwc.Close()
		delete(s.activeConn, activeConnection)
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
		interval := pollIntervalBase + time.Duration(rand.Intn(int(pollIntervalBase/10))) //nolint:gosec,mnd
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
	return &conn{
		server:   s,
		rwc:      rwc,
		curState: atomic.Uint64{},
	}
}

func (s *Server) trackListener(listener *net.Listener, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listeners == nil {
		s.listeners = make(map[*net.Listener]struct{})
	}
	if add {
		if s.shuttingDown() {
			return false
		}
		s.listeners[listener] = struct{}{}
		s.listenerGroup.Add(1)
	} else {
		delete(s.listeners, listener)
		s.listenerGroup.Done()
	}
	return true
}

func (s *Server) trackConn(connection *conn, add bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeConn == nil {
		s.activeConn = make(map[*conn]struct{})
	}
	if add {
		s.activeConn[connection] = struct{}{}
	} else {
		delete(s.activeConn, connection)
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
		connection, err := listener.Accept()
		if err != nil {
			if s.shuttingDown() {
				return ErrServerClosed
			}
			s.logger.Printf("Error accepting connection: %v", err)
			continue
		}
		c := s.newConn(connection)
		c.setState(StateNew)
		go c.handleConnection()
	}
}

func New(logger *log.Logger) *Server {
	return &Server{
		mu:            sync.Mutex{},
		activeConn:    make(map[*conn]struct{}),
		inShutdown:    atomic.Bool{},
		listeners:     make(map[*net.Listener]struct{}),
		listenerGroup: sync.WaitGroup{},
		// This will be overridden by the Start method
		gracePeriod: 0,
		logger:      logger,
	}
}
