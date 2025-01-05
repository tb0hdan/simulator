package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const ServerAddr = "localhost:8080"

type SimulatorTestSuite struct {
	suite.Suite
	srv *Server
}

func (s *SimulatorTestSuite) SetupSuite() {
	s.srv = New(log.New(os.Stdout, "", log.LstdFlags))
	go func() {
		if err := s.srv.Start(ServerAddr, 3*time.Second); !errors.Is(err, ErrServerClosed) {
			fmt.Println("Failed to start server:", err)
			os.Exit(1)
		}
	}()

	// wait of the server to be ready
	time.Sleep(time.Second)
}

func (s *SimulatorTestSuite) TearDownSuite() {
	// wait for the server to shutdown
	time.Sleep(time.Second)
	s.srv.Shutdown(context.Background())
}

func (s *SimulatorTestSuite) TestSchemeSimulator() {
	tests := []struct {
		name           string
		input          string
		expectedOutput string
		minDuration    time.Duration
		maxDuration    time.Duration
	}{
		{
			name:           "Valid Request",
			input:          "PAYMENT|10",
			expectedOutput: "RESPONSE|ACCEPTED|Transaction processed",
			maxDuration:    50 * time.Millisecond,
		},
		{
			name:           "Valid Request with Delay",
			input:          "PAYMENT|101",
			expectedOutput: "RESPONSE|ACCEPTED|Transaction processed",
			minDuration:    101 * time.Millisecond,
			maxDuration:    151 * time.Millisecond,
		},

		{
			name:           "Invalid Request Format",
			input:          "INVALID|100",
			expectedOutput: "RESPONSE|REJECTED|Invalid request",
			maxDuration:    10 * time.Millisecond,
		},
		{
			name:           "Large Amount",
			input:          "PAYMENT|20000",
			expectedOutput: "RESPONSE|ACCEPTED|Transaction processed",
			minDuration:    10 * time.Second,
			maxDuration:    10*time.Second + 50*time.Millisecond,
		},
	}

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			conn, err := net.Dial("tcp", ServerAddr)
			assert.NoError(t, err, "Failed to connect to server")
			defer conn.Close()

			_, err = fmt.Fprintf(conn, tt.input+"\n")
			assert.NoError(t, err, "Failed to send request")

			start := time.Now()

			response, err := bufio.NewReader(conn).ReadString('\n')
			assert.NoError(t, err, "Failed to read response")
			duration := time.Since(start)

			response = strings.TrimSpace(response)

			assert.Equal(t, tt.expectedOutput, response, "Unexpected response")

			if tt.minDuration > 0 {
				assert.GreaterOrEqual(t, duration, tt.minDuration, "Response time was shorter than expected")
			}

			if tt.maxDuration > 0 {
				assert.LessOrEqual(t, duration, tt.maxDuration, "Response time was longer than expected")
			}
		})
	}
}

func (s *SimulatorTestSuite) TestShutdown() {
	conn, err := net.Dial("tcp", ServerAddr)
	assert.NoError(s.T(), err, "Failed to connect to server")
	defer conn.Close()

	go func() {
		_, err = fmt.Fprintf(conn, "PAYMENT|20000\n")
		assert.NoError(s.T(), err, "Failed to send request")
	}()

	err = s.srv.Shutdown(context.Background())

	assert.NoError(s.T(), err, "Failed to shutdown server")
	time.Sleep(10 * time.Second)
	response, err := bufio.NewReader(conn).ReadString('\n')
	assert.NoError(s.T(), err, "Failed to read response")
	assert.Equal(s.T(), "RESPONSE|REJECTED|Cancelled", strings.TrimSpace(response), "Unexpected response")
}

func TestSimulatorTestSuite(t *testing.T) {
	suite.Run(t, new(SimulatorTestSuite))
}
