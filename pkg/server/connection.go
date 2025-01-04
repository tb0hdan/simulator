package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ConnState int

const (
	// StateNew represents a new connection that is expected to
	// send a request immediately. Connections begin at this
	// state and then transition to either StateActive or
	// StateClosed.
	StateNew ConnState = iota

	// StateActive represents a connection that has read 1 or more
	// bytes of a request. The Server.ConnState hook for
	// StateActive fires before the request has entered a handler
	// and doesn't fire again until the request has been
	// handled. After the request is handled, the state
	// transitions to StateClosed, StateHijacked, or StateIdle.
	// For HTTP/2, StateActive fires on the transition from zero
	// to one active request, and only transitions away once all
	// active requests are complete. That means that ConnState
	// cannot be used to do per-request work; ConnState only notes
	// the overall state of the connection.
	StateActive

	// StateIdle represents a connection that has finished
	// handling a request and is in the keep-alive state, waiting
	// for a new request. Connections transition from StateIdle
	// to either StateActive or StateClosed.
	StateIdle

	// StateClosed represents a closed connection.
	// This is a terminal state. Hijacked connections do not
	// transition to StateClosed.
	StateClosed
)

type conn struct {
	// server is the server on which the connection arrived.
	// Immutable; never nil.
	server *Server

	// cancelCtx cancels the connection-level context.
	// cancelCtx context.CancelFunc

	// rwc is the underlying network connection.
	// This is never wrapped by other types and is the value given out
	// to CloseNotifier callers. It is usually of type *net.TCPConn or
	// *tls.Conn.
	rwc net.Conn

	curState atomic.Uint64 // packed (unixtime<<8|uint8(ConnState))

	mu sync.Mutex
}

func (c *conn) getState() (state ConnState, unixSec int64) {
	packedState := c.curState.Load()
	return ConnState(packedState & 0xff), int64(packedState >> 8)
}

func (c *conn) setState(state ConnState) {
	srv := c.server
	switch state {
	case StateNew:
		srv.trackConn(c, true)
	case StateClosed:
		srv.trackConn(c, false)
	}
	if state > 0xff || state < 0 {
		panic("internal error")
	}
	packedState := uint64(time.Now().Unix()<<8) | uint64(state)
	c.curState.Store(packedState)
	return
}

func (c *conn) handleConnection() {
	defer c.setState(StateClosed)

	scanner := bufio.NewScanner(c.rwc)
	for scanner.Scan() {
		request := scanner.Text()
		response := c.handleRequestWrapped(request)
		fmt.Fprintf(c.rwc, "%s\n", response)
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading from connection:", err)
	}
}

func (c *conn) handleRequestWrapped(request string) string {
	if !c.server.inShutdown.Load() {
		return c.handleRequest(request)
	}
	handleCh := make(chan string)

	go func() {
		handleCh <- c.handleRequest(request)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), c.server.gracePeriod)
	defer cancel()
	select {
	case <-ctx.Done():
		return "RESPONSE|REJECTED|Cancelled"
	case result := <-handleCh:
		return result
	}
}

func (c *conn) handleRequest(request string) string {
	parts := strings.Split(request, "|")
	if len(parts) != 2 || parts[0] != "PAYMENT" {
		return "RESPONSE|REJECTED|Invalid request"
	}

	amount, err := strconv.Atoi(parts[1])
	if err != nil {
		return "RESPONSE|REJECTED|Invalid amount"
	}

	if amount > 100 {
		processingTime := amount
		if amount > 10000 {
			processingTime = 10000
		}
		time.Sleep(time.Duration(processingTime) * time.Millisecond)
	}

	return "RESPONSE|ACCEPTED|Transaction processed"
}
