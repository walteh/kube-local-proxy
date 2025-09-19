package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
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

func TestProxyHandlesHTTPS(t *testing.T) {
	t.Parallel()

	var hits int32
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		require.Equal(t, "/hello", r.URL.Path)
		_, err := w.Write([]byte("world"))
		require.NoError(t, err)
	}))
	defer tlsServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	localPort := availablePort(t)
	proxyConfig := Proxy{LocalPort: localPort, RemoteHost: tlsServer.Listener.Addr().String()}

	proxyErr := make(chan error, 1)
	go func() {
		proxyErr <- proxy(ctx, proxyConfig)
	}()

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	defer client.CloseIdleConnections()

	var resp *http.Response
	var err error
	target := fmt.Sprintf("https://127.0.0.1:%s/hello", localPort)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		resp, err = client.Get(target)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NoError(t, err, "failed to reach HTTPS server through proxy")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, []byte("world"), body)
	require.Equal(t, int32(1), atomic.LoadInt32(&hits))

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
