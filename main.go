package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"

	"golang.org/x/net/proxy"
)

func handleConnection(clientConn net.Conn, tlsConfig *tls.Config, socks5Proxy string) {
	defer clientConn.Close()
	println("Connection established")
	// 使用SNI获取客户端的目标主机
	tlsConn := tls.Server(clientConn, tlsConfig)
	// Perform TLS handshake
	err := tlsConn.Handshake()
	if err != nil {
		fmt.Println("TLS handshake error:", err)
		return
	}
	state := tlsConn.ConnectionState()
	sniData := state.ServerName
	if sniData == "" {
		fmt.Printf("Failed to get SNI: %s\n", clientConn.RemoteAddr().String())
		return
	}
	parts := strings.SplitN(sniData, "-", 2)

	targetAddress := ""
	// Check if there are at least two parts
	if len(parts) == 2 {
		// Access the first and second parts
		targetPort := parts[0]
		targetHost := parts[1]
		targetAddress = targetHost + ":" + targetPort
		fmt.Println("Target host:", targetHost)
		fmt.Println("Target port:", targetPort)
	} else {
		fmt.Println("Could not split the sni into two parts.")
		return
	}
	fmt.Printf("Target Host: %s\n", targetAddress)

	// 使用Socks5代理
	if socks5Proxy != "" {
		proxyURL, err := url.Parse(socks5Proxy)
		if err != nil {
			fmt.Println("Error parsing SOCKS5 URL:", err)
			return
		}
		// 从 URL 中获取用户名和密码（如果存在）
		var username, password string
		if proxyURL.User != nil {
			username = proxyURL.User.Username()
			password, _ = proxyURL.User.Password()
		}

		// 创建 SOCKS5 代理 Dialer，包括认证信息（如果存在）
		var proxyAuth *proxy.Auth
		if username != "" && password != "" {
			proxyAuth = &proxy.Auth{
				User:     username,
				Password: password,
			}
		}
		dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, proxyAuth, proxy.Direct)
		if err != nil {
			fmt.Println("Failed to create SOCKS5 dialer:", err)
			return
		}
		serverConn, err := dialer.Dial("tcp", targetAddress)
		if err != nil {
			fmt.Println("Failed to connect to target with socks5:", err)
			return
		}
		defer serverConn.Close()
		transferData(serverConn, tlsConn)

	} else {
		serverConn, err := net.Dial("tcp", targetAddress)
		if err != nil {
			fmt.Printf("Failed to connect to %s: %v\n", targetAddress, err)
			return
		}
		defer serverConn.Close()
		transferData(serverConn, tlsConn)
	}

}

func transferData(serverConn net.Conn, tlsConn *tls.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(serverConn, tlsConn)
	}()

	go func() {
		defer wg.Done()
		io.Copy(tlsConn, serverConn)
	}()

	wg.Wait()
}

func main() {

	address := flag.String("address", "0.0.0.0:44521", "Specify the server address")
	tlsCertPath := flag.String("tlscert", "", "Specify the tls certificate file path")
	tlsKeyPath := flag.String("tlskey", "", "Specify the tls key file path")
	socks5Proxy := flag.String("proxy", "", "Specify the socks5 proxy url: socks5://username:password@your_socks5_proxy_host:1080")
	flag.Parse()
	if *address == "" {
		fmt.Println("Error: address is required.")
		flag.Usage()
		os.Exit(1)
	}

	if *tlsCertPath == "" {
		fmt.Println("Error: tlsCertPath is required.")
		flag.Usage()
		os.Exit(1)
	}
	if *tlsKeyPath == "" {
		fmt.Println("Error: tlsKeyPath is required.")
		flag.Usage()
		os.Exit(1)
	}

	// Parse command-line arguments
	flag.Parse()

	// 解析命令行参数
	// 加载服务器证书和私钥
	cert, err := tls.LoadX509KeyPair(*tlsCertPath, *tlsKeyPath) // 替换为实际证书和私钥文件的路径
	if err != nil {
		fmt.Printf("Failed to load certificate: %v\n", err)
		return
	}

	// 配置TLS
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// 监听在本地端口
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		fmt.Printf("Failed to listen: %v\n", err)
		return
	}
	defer listener.Close()
	fmt.Printf("Listening on %s\n", *address)

	for {
		// 等待并接受客户端连接
		clientConn, err := listener.Accept()
		if err != nil {
			fmt.Printf("Failed to accept connection: %v\n", err)
			continue
		}

		// 在新的goroutine中处理连接
		go handleConnection(clientConn, tlsConfig, *socks5Proxy)
	}
}
