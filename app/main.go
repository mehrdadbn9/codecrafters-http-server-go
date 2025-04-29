package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	fmt.Println("Logs from your program will appear here!")
	directoryPath := ""
	if len(os.Args) > 2 && os.Args[1] == "--directory" {
		directoryPath = os.Args[2]
	}

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}
		go handleConnection(conn, directoryPath)
	}
}

func supportsGzip(acceptEncoding string) bool {
	if acceptEncoding == "" {
		return false
	}
	encodings := strings.Split(acceptEncoding, ",")
	for _, encoding := range encodings {
		if strings.TrimSpace(encoding) == "gzip" {
			return true
		}
	}
	return false
}

func parseHeaders(reader *bufio.Reader) (map[string]string, error) {
	headers := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			headers[parts[0]] = parts[1]
		}
	}
	return headers, nil
}

func sendResponse(conn net.Conn, statusCode int, statusText string, contentType string, body []byte, supportsGzip bool, closeConnection bool) {
	headers := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, statusText)

	if contentType != "" {
		headers += fmt.Sprintf("Content-Type: %s\r\n", contentType)
	}

	// Add Connection: close header if needed
	if closeConnection {
		headers += "Connection: close\r\n"
	}

	// Gzip compression
	if supportsGzip && len(body) > 0 {
		var compressed bytes.Buffer
		gz := gzip.NewWriter(&compressed)
		gz.Write(body)
		gz.Close()
		body = compressed.Bytes()
		headers += "Content-Encoding: gzip\r\n"
	}

	headers += fmt.Sprintf("Content-Length: %d\r\n", len(body))
	headers += "\r\n"

	conn.Write([]byte(headers))
	conn.Write(body)
}

func handleConnection(conn net.Conn, directoryPath string) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	for {
		requestLine, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		requestLine = strings.TrimSpace(requestLine)
		if requestLine == "" {
			continue
		}

		parts := strings.Split(requestLine, " ")
		if len(parts) < 3 {
			sendResponse(conn, 400, "Bad Request", "", nil, false, false)
			continue
		}
		method := parts[0]
		path := parts[1]

		headers, err := parseHeaders(reader)
		if err != nil {
			break
		}

		// Read body if present
		var body []byte
		if clStr, ok := headers["Content-Length"]; ok {
			cl, _ := strconv.Atoi(clStr)
			body = make([]byte, cl)
			io.ReadFull(reader, body)
		}

		// Determine if connection should close
		closeConn := strings.ToLower(headers["Connection"]) == "close"
		clientSupportsGzip := supportsGzip(headers["Accept-Encoding"])

		switch {
		case path == "/":
			sendResponse(conn, 200, "OK", "", nil, clientSupportsGzip, closeConn)
		case strings.HasPrefix(path, "/echo/"):
			echoString := strings.TrimPrefix(path, "/echo/")
			sendResponse(conn, 200, "OK", "text/plain", []byte(echoString), clientSupportsGzip, closeConn)
		case path == "/user-agent":
			userAgent := headers["User-Agent"]
			sendResponse(conn, 200, "OK", "text/plain", []byte(userAgent), clientSupportsGzip, closeConn)
		case strings.HasPrefix(path, "/files/"):
			filename := strings.TrimPrefix(path, "/files/")
			filePath := filepath.Join(directoryPath, filename)
			if method == "GET" {
				fileData, err := os.ReadFile(filePath)
				if err != nil {
					sendResponse(conn, 404, "Not Found", "", nil, clientSupportsGzip, closeConn)
				} else {
					sendResponse(conn, 200, "OK", "application/octet-stream", fileData, clientSupportsGzip, closeConn)
				}
			} else if method == "POST" {
				err := os.WriteFile(filePath, body, 0644)
				if err != nil {
					sendResponse(conn, 500, "Internal Server Error", "", nil, clientSupportsGzip, closeConn)
				} else {
					sendResponse(conn, 201, "Created", "", nil, clientSupportsGzip, closeConn)
				}
			}
		default:
			sendResponse(conn, 404, "Not Found", "", nil, clientSupportsGzip, closeConn)
		}

		// Terminate connection if requested
		if closeConn {
			break
		}
	}
}