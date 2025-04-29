package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
)

var directory string

func main() {
	// Parse the --directory flag to get the directory where files are stored
	flag.StringVar(&directory, "directory", "", "Directory where files are stored")
	flag.Parse()

	if directory == "" {
		fmt.Println("Directory flag is required")
		os.Exit(1)
	}

	// Validate that the directory exists
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		fmt.Printf("Directory does not exist: %s\n", directory)
		os.Exit(1)
	}

	fmt.Println("Logs from your program will appear here!")

	// Listen on all interfaces on port 4221
	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221:", err)
		os.Exit(1)
	}

	// Accept connections concurrently
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		// Handle the connection in a new goroutine
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set up a buffered reader to handle incoming requests
	reader := bufio.NewReader(conn)

	// Read the request line
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading request line:", err)
		return
	}

	// Split the request line into components: method, path, and version
	parts := strings.Split(requestLine, " ")
	if len(parts) < 2 {
		fmt.Println("Malformed request line:", requestLine)
		return
	}
	method := parts[0]
	path := parts[1]

	// Read and discard headers (if any)
	headers := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading headers:", err)
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			// End of headers
			break
		}

		// Parse headers into a map (key: value)
		colonIndex := strings.Index(line, ":")
		if colonIndex != -1 {
			key := strings.TrimSpace(line[:colonIndex])
			value := strings.TrimSpace(line[colonIndex+1:])
			headers[strings.ToLower(key)] = value
		}
	}

	// Determine the appropriate response based on the request path
	var response string
	if method == "GET" {
		if path == "/" {
			response = "HTTP/1.1 200 OK\r\n\r\n"
		} else if strings.HasPrefix(path, "/echo/") {
			// Extract echo string from URL path
			echoText := strings.TrimPrefix(path, "/echo/")
			body := echoText
			contentLength := len(body)
			response = fmt.Sprintf(
				"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
				contentLength,
				body,
			)
		} else if path == "/user-agent" {
			// Extract User-Agent header value
			userAgent := headers["user-agent"]
			body := userAgent
			contentLength := len(body)
			response = fmt.Sprintf(
				"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s",
				contentLength,
				body,
			)
		} else if strings.HasPrefix(path, "/files/") {
			// Handle /files/{filename}
			filePath := strings.TrimPrefix(path, "/files/")
			filePath = filepath.Join(directory, filePath) // Combine with the directory path

			// Check if the file exists
			content, err := ioutil.ReadFile(filePath)
			if err != nil {
				// File does not exist
				response = "HTTP/1.1 404 Not Found\r\n\r\n"
			} else {
				// File exists, return the file contents
				contentLength := len(content)
				response = fmt.Sprintf(
					"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n",
					contentLength,
				)
				conn.Write([]byte(response)) // Write headers
				conn.Write(content)          // Write file content
				return
			}
		} else {
			// Path not found
			response = "HTTP/1.1 404 Not Found\r\n\r\n"
		}
	} else {
		// Invalid method (other than GET)
		response = "HTTP/1.1 405 Method Not Allowed\r\n\r\n"
	}

	// Send the response back to the client
	_, err = conn.Write([]byte(response))
	if err != nil {
		fmt.Println("Error writing response:", err)
	}
}
