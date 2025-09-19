package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseProxyArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		want      []Proxy
		wantError bool
	}{
		{
			name: "valid entries",
			args: []string{"8080:example.com:80", "9000:localhost:9001"},
			want: []Proxy{
				{LocalPort: "8080", RemoteHost: "example.com:80"},
				{LocalPort: "9000", RemoteHost: "localhost:9001"},
			},
		},
		{
			name:      "missing separator",
			args:      []string{"invalid"},
			wantError: true,
		},
		{
			name:      "empty port",
			args:      []string{":example.com:80"},
			wantError: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			proxies, err := parseProxyArgs(tc.args)
			if tc.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, proxies)
		})
	}
}

func TestProxyForwardsData(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	remoteListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer remoteListener.Close()

	remoteAddr := remoteListener.Addr().String()
	remoteResult := make(chan error, 1)

	go func() {
		conn, err := remoteListener.Accept()
		if err != nil {
			remoteResult <- err
			return
		}
		defer conn.Close()

		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			remoteResult <- err
			return
		}
		if string(buf) != "ping" {
			remoteResult <- fmt.Errorf("unexpected payload: %q", buf)
			return
		}
		if _, err := conn.Write([]byte("pong")); err != nil {
			remoteResult <- err
			return
		}
		remoteResult <- nil
	}()

	localPort := availablePort(t)
	proxyConfig := Proxy{LocalPort: localPort, RemoteHost: remoteAddr}

	proxyErr := make(chan error, 1)
	go func() {
		proxyErr <- proxy(ctx, proxyConfig)
	}()

	var conn net.Conn
	dialAddr := fmt.Sprintf("127.0.0.1:%s", localPort)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, err = net.Dial("tcp", dialAddr)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err, "failed to dial proxy")
	defer conn.Close()

	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)

	buf := make([]byte, 4)
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	require.Equal(t, []byte("pong"), buf)

	require.NoError(t, <-remoteResult)

	cancel()

	select {
	case err := <-proxyErr:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("proxy did not exit after cancellation")
	}
}

func availablePort(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer lis.Close()

	port := lis.Addr().(*net.TCPAddr).Port
	return strconv.Itoa(port)
}
