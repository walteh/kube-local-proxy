package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
)

type Proxy struct {
	LocalPort  string
	RemoteHost string
	RemotePort string
}

func main() {
	fmt.Println("Starting proxymux with arguments:", os.Args)

	if len(os.Args) < 2 {
		fmt.Println("Usage: proxymux <port>:<host>:<port> <port>:<host>:<port>")
		os.Exit(1)
	}

	proxies, err := parseProxyArgs(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ctx := context.Background()

	// look for signal and end context
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	go func() {
		<-signalChan
		ctx.Done()
	}()

	for _, p := range proxies {
		go func(pr Proxy) {
			err := proxy(ctx, pr)
			if err != nil {
				fmt.Println("Proxy error:", err)
			}
		}(p)
	}

	<-ctx.Done()

	fmt.Println("Proxymux stopped")
}

func proxy(ctx context.Context, proxy Proxy) error {
	// Implement proxy logic here
	fmt.Println("Proxying", proxy.LocalPort, "to", proxy.RemoteHost)

	// Start proxying
	go func() {
		<-ctx.Done()
		fmt.Println("Stopping proxy for", proxy.LocalPort)
	}()

	// make sure the remote is reachable
	if _, err := net.Dial("tcp", net.JoinHostPort(proxy.RemoteHost, proxy.RemotePort)); err != nil {
		return fmt.Errorf("failed to connect to remote host: %w", err)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", proxy.LocalPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	defer lis.Close()
	go func() {
		<-ctx.Done()
		lis.Close()
	}()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		conn, err := lis.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Println("Failed to accept connection:", err)
			continue
		}
		go handleConnection(conn, proxy.RemoteHost, proxy.RemotePort)
	}
}

func handleConnection(conn net.Conn, remoteHost string, remotePort string) {
	defer conn.Close()

	remoteConn, err := net.Dial("tcp", net.JoinHostPort(remoteHost, remotePort))
	if err != nil {
		fmt.Println("Failed to connect to remote host:", err)
		return
	}
	defer remoteConn.Close()

	go func() {
		io.Copy(conn, remoteConn)
		conn.Close()
	}()
	io.Copy(remoteConn, conn)
}

func parseProxyArgs(args []string) ([]Proxy, error) {
	var proxies []Proxy
	for _, arg := range args {
		parts := strings.SplitN(arg, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid proxy argument: %s", arg)
		}
		localPort, host := parts[0], parts[1]
		if localPort == "" || host == "" {
			return nil, fmt.Errorf("invalid proxy argument: %s", arg)
		}

		hostname, port, err := net.SplitHostPort(host)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy argument: %s", arg)
		}

		if port == "" {
			port = "80"
		}

		if hostname == "" {
			return nil, fmt.Errorf("invalid proxy argument: %s", arg)
		}

		proxies = append(proxies, Proxy{LocalPort: localPort, RemoteHost: hostname, RemotePort: port})
	}
	return proxies, nil
}
