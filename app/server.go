package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

// TODO: refactoring using the below todos
// TODO: create a handler for each route
// TODO: create option pattern for each handler
type Server struct {
	port    string
	ip      string
	rootDir string
}

func NewServer(port, ip, rootDir string) *Server {
	return &Server{
		port,
		ip,
		rootDir,
	}
}

func (s *Server) Start() (net.Listener, error) {
	addr := s.ip + ":" + s.port
	return net.Listen("tcp", addr)
}

type Request struct {
	Headers map[string]string
	Params  map[string]string
	Routes  []string
	Body    string
	Method  string
	Url     string
	Version string
}

func ParseRequest(reader io.Reader) (*Request, error) {
	request := &Request{}
	request.Headers = make(map[string]string)
	bufreader := bufio.NewReader(reader)

	// read request line
	line, err := bufreader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	line = strings.Trim(line, "\r\n")

	requestLine := strings.Split(line, " ")
	request.Method = requestLine[0]
	request.Url = requestLine[1]
	request.Version = requestLine[2]

	// read routes and params
	routesAndParams := strings.Split(request.Url, "?")
	request.Routes = strings.Split(routesAndParams[0], "/")

	if len(routesAndParams) > 1 {
		params := strings.Split(routesAndParams[1], "&")
		for _, param := range params {
			paramParts := strings.Split(param, "=")
			request.Params[paramParts[0]] = paramParts[1]
		}
	}

	// read headers
	for {
		line, err := bufreader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.Trim(line, "\r\n")
		if line == "" {
			break
		}

		header := strings.SplitN(line, ":", 2)
		request.Headers[strings.TrimSpace(header[0])] = strings.TrimSpace(header[1])
	}

	// read body
	body := &strings.Builder{}

	for {
		line, err := bufreader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}

		line = strings.Trim(line, "\r\n")
		body.WriteString(line)
		if err == io.EOF {
			break
		}
	}

	request.Body = body.String()

	return request, nil
}

type Response struct {
	Headers map[string]string
	Status  string
	Version string
	Message string
	Body    string
}

func (res *Response) Write(conn net.Conn) error {
	h := strings.Builder{}

	for key, value := range res.Headers {
		h.Write([]byte(fmt.Sprintf("%s: %s\r\n", key, value)))
	}

	_, err := conn.Write([]byte(fmt.Sprintf("%s %s %s\r\n%s\r\n%s", res.Version, res.Status, res.Message, h.String(), res.Body)))
	return err
}

var server *Server

func main() {
	var directory string
	flag.StringVar(&directory, "directory", "", "directory to serve static files from")
	flag.Parse()

	server = NewServer("4221", "0.0.0.0", directory)
	l, err := server.Start()
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
	req, err := ParseRequest(conn)
	if err != nil {
		fmt.Println("Error reading from connection: ", err.Error())
		os.Exit(1)
	}

	var resp Response

	switch {
	case req.Url == "/":
		resp = Response{
			Version: "HTTP/1.1",
			Status:  "200",
			Message: "OK",
			Headers: map[string]string{
				"Content-Type": "text/html",
			},
			Body: "Hello World!",
		}
	case req.Url == "/user-agent":
		resp = Response{
			Version: "HTTP/1.1",
			Status:  "200",
			Message: "OK",
			Headers: map[string]string{
				"Content-Type":   "text/plain",
				"Content-Length": strconv.Itoa(len(req.Headers["User-Agent"])),
			},
			Body: req.Headers["User-Agent"],
		}
	case strings.Contains(req.Url, "/files"):
		filename := req.Routes[2]
		filePath := server.rootDir + "/" + filename

		if req.Method == "GET" {
			if _, err := os.Stat(filePath); err == nil {
				data, err := os.ReadFile(filePath)
				if err != nil {
					resp = Response{
						Version: "HTTP/1.1",
						Status:  "404",
						Message: "Not Found",
						Headers: map[string]string{
							"Content-Type": "text/plain",
						},
						Body: err.Error(),
					}
				}

				resp = Response{
					Version: "HTTP/1.1",
					Status:  "200",
					Message: "OK",
					Headers: map[string]string{
						"Content-Type":   "application/octet-stream",
						"Content-Length": strconv.Itoa(len(data)),
					},
				}
			}
		} else if req.Method == "POST" {
			file, err := os.Create(filePath)
			if err != nil {
				resp = Response{
					Version: "HTTP/1.1",
					Status:  "500",
					Message: "Internal Server Error",
					Headers: map[string]string{
						"Content-Type": "text/plain",
					},
					Body: err.Error(),
				}

			}

			_, err = file.Write([]byte(req.Body))
			if err != nil {
				resp = Response{
					Version: "HTTP/1.1",
					Status:  "500",
					Message: "Internal Server Error",
					Headers: map[string]string{
						"Content-Type": "text/plain",
					},
					Body: err.Error(),
				}
			}

			resp = Response{
				Version: "HTTP/1.1",
				Status:  "201",
				Message: "Created",
				Headers: map[string]string{
					"Content-Type": "text/plain",
				},
			}
		}
	case strings.Contains(req.Url, "/echo"):
		if len(req.Routes) == 0 {
			resp = Response{
				Version: "HTTP/1.1",
				Status:  "400",
				Message: "Bad Request",
				Headers: map[string]string{
					"Content-Type": "text/plain",
				},
				Body: "No route specified",
			}
		}
		resp = Response{
			Version: "HTTP/1.1",
			Status:  "200",
			Message: "OK",
			Headers: map[string]string{
				"Content-Type":   "text/plain",
				"Content-Length": strconv.Itoa(len(req.Routes[0])),
			},
			Body: req.Routes[0],
		}
	default:
		resp = Response{
			Version: "HTTP/1.1",
			Status:  "404",
			Message: "Not Found",
			Headers: map[string]string{
				"Content-Type": "text/plain",
			},
			Body: "Not Found",
		}

	}

	resp.Write(conn)
}
