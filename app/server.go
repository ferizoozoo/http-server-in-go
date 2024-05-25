package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

//TODO: refactoring using the below todos
//TODO: create a server struct
//TODO: create a request struct
//TODO: create a response struct
//TODO: create a parserequest func (have a headers map type)
//TODO: create a handler for each route
//TODO: create option pattern for each handler

var directory string

func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	flag.StringVar(&directory, "directory", "", "directory to serve static files from")
	flag.Parse()
	fmt.Println("Logs from your program will appear here!")

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}

		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	reader := bufio.NewReader(conn)

	line, err := reader.ReadString('\r')
	if err != nil {
		fmt.Println("Error reading from connection: ", err.Error())
		os.Exit(1)
	}

	url := strings.Split(line, " ")
	path := url[1]
	method := url[0]

	if path == "/" {
		conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		return
	}

	if path == "/user-agent" {
		var userAgent string
		for {
			line, _, err := reader.ReadLine()
			if err != nil {
				if err == io.EOF {
					break
				}

				fmt.Println("Error reading from connection: ", err.Error())
				os.Exit(1)
			}

			if header := strings.SplitN(string(line), ":", 2); strings.TrimSpace(header[0]) == "User-Agent" {
				userAgent = strings.TrimSpace(header[1])
				break
			}
		}
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(userAgent), userAgent)))
	}

	if strings.Contains(path, "/files") {
		if method == "GET" {
			filename := strings.Split(path, "/")[2]
			filePath := directory + "/" + filename
			if _, err := os.Stat(filePath); err == nil {
				data, err := os.ReadFile(filePath)
				if err != nil {
					conn.Write([]byte(fmt.Sprintf("HTTP/1.1 404 Not Found\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(err.Error()), err.Error())))
					return
				}

				conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n%s", len(data), string(data))))
				return
			}
		} else if method == "POST" {
			filename := strings.Split(path, "/")[2]
			filePath := directory + "/" + filename

			var buffer bytes.Buffer
			body := make([]byte, 1024)
			headerEnded := false

			for {
				n, err := reader.Read(body)
				if err != nil && err != io.EOF {
					fmt.Println("Error reading from connection:", err.Error())
					conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
					return
				}

				if n > 0 {
					buffer.Write(body[:n])
				}

				if !headerEnded {
					content := buffer.Bytes()
					index := bytes.Index(content, []byte("\r\n\r\n"))
					if index != -1 {
						headerEnded = true
						buffer.Reset()
						buffer.Write(content[index+4:])
					}
				}

				if err == io.EOF {
					break
				}
			}

			file, err := os.Create(filePath)
			if err != nil {
				conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
				return
			}
			defer file.Close()

			_, err = file.Write(buffer.Bytes())
			if err != nil {
				conn.Write([]byte("HTTP/1.1 500 Internal Server Error\r\n\r\n"))
				return
			}

			conn.Write([]byte("HTTP/1.1 201 Created\r\n\r\n"))
			return
		}

		conn.Write([]byte("HTTP/1.1 405 Method Not Allowed\r\n\r\n"))
	}

	if strings.Contains(path, "/echo") {
		param := strings.Split(path, "/")[2]
		conn.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n%s", len(param), param)))
		return
	}

	conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))
}
