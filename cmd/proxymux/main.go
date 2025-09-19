package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

type Proxy struct {
	LocalPort  string
	RemoteHost string
}

func main() {
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
	for _, p := range proxies {
		go func(pr Proxy) {
			err := proxy(ctx, pr)
			if err != nil {
				fmt.Println("Proxy error:", err)
			}
		}(p)
	}
}

func proxy(ctx context.Context, proxy Proxy) error {
	// Implement proxy logic here
	fmt.Println("Proxying", proxy.LocalPort, "to", proxy.RemoteHost)

	// Start proxying
	go func() {
		<-ctx.Done()
		fmt.Println("Stopping proxy for", proxy.LocalPort)
	}()

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
		go handleConnection(conn, proxy.RemoteHost)
	}
}

func handleConnection(conn net.Conn, remoteHost string) {
	defer conn.Close()

	remoteConn, err := net.Dial("tcp", remoteHost)
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
		port, host := parts[0], parts[1]
		if port == "" || host == "" {
			return nil, fmt.Errorf("invalid proxy argument: %s", arg)
		}
		proxies = append(proxies, Proxy{LocalPort: port, RemoteHost: host})
	}
	return proxies, nil
}
