package server

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SimulatorTestSuite struct {
	suite.Suite
	srv *Server
}

func (s *SimulatorTestSuite) SetupSuite() {
	s.srv = New()
	go func() {
		err := s.srv.Start(":8080", 3*time.Second)
		if err != nil {
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
			conn, err := net.Dial("tcp", ":8080")
			require.NoError(t, err, "Failed to connect to server")
			defer conn.Close()

			_, err = fmt.Fprintf(conn, tt.input+"\n")
			require.NoError(t, err, "Failed to send request")

			start := time.Now()

			response, err := bufio.NewReader(conn).ReadString('\n')
			require.NoError(t, err, "Failed to read response")
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

func TestSimulatorTestSuite(t *testing.T) {
	suite.Run(t, new(SimulatorTestSuite))
}
