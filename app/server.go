package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

type EncodingTypes []string

var encodings = EncodingTypes{
	"gzip",
}

func (e EncodingTypes) Exists(encoding string) bool {
	for _, v := range e {
		if v == encoding {
			return true
		}
	}

	return false
}

// TODO: refactoring using the below todos
// TODO: create a handler for each route
// TODO: create option pattern for each handler
// TODO: response for each handler should only receive body, message and status code (the rest should be handled behind the scene)
// TODO: separate each handler into its own file
// TODO: separate Request and Response into its own file
// TODO: compress response body, but as inside the write function of response, or as a separate function?
// TODO: refactor Response.Write function
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
	if contentLength, ok := request.Headers["Content-Length"]; ok {
		length, err := strconv.Atoi(contentLength)
		if err != nil {
			return nil, err
		}
		body := make([]byte, length)
		_, err = io.ReadFull(bufreader, body)
		if err != nil {
			return nil, err
		}
		request.Body = string(body)
	}

	return request, nil
}

func getEncoding(request *Request) string {
	requestedEncodings := strings.Split(request.Headers["Accept-Encoding"], ",")
	for _, encoding := range requestedEncodings {
		if encodings.Exists(strings.TrimSpace(encoding)) {
			return encoding
		}
	}

	return ""
}

type Response struct {
	Headers map[string]string
	Status  string
	Version string
	Message string
	Body    string
}

func (res *Response) Write(conn net.Conn) error {
	writer := bufio.NewWriter(conn)
	defer writer.Flush()

	h := strings.Builder{}

	if encoding := res.Headers["Content-Encoding"]; encoding != "" {
		encodedBody, err := compressBody(res.Body, encoding)
		if err != nil {
			return err
		}
		res.Body = encodedBody
		res.Headers["Content-Length"] = strconv.Itoa(len(encodedBody))
	}

	for key, value := range res.Headers {
		if key == "Content-Encoding" && value == "" {
			continue
		}
		h.Write([]byte(fmt.Sprintf("%s: %s\r\n", key, value)))
	}

	_, err := writer.Write([]byte(fmt.Sprintf("%s %s %s\r\n%s\r\n%s", res.Version, res.Status, res.Message, h.String(), res.Body)))
	return err
}

func compressBody(body string, encoding string) (string, error) {
	var err error
	var encodedBody string
	switch encoding {
	case "gzip":
		encodedBody, err = gzipEncoding(body)
	}

	return encodedBody, err
}

func gzipEncoding(body string) (string, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)

	_, err := zw.Write([]byte(body))
	zw.Close()
	return buf.String(), err
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
			continue
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
						Status:  "500",
						Message: "Internal Server Error",
						Headers: map[string]string{
							"Content-Type": "text/plain",
						},
					}
					break
				}

				resp = Response{
					Version: "HTTP/1.1",
					Status:  "200",
					Message: "OK",
					Headers: map[string]string{
						"Content-Type":   "application/octet-stream",
						"Content-Length": strconv.Itoa(len(data)),
					},
					Body: string(data),
				}
				break
			}
			resp = Response{
				Version: "HTTP/1.1",
				Status:  "404",
				Message: "Not Found",
				Headers: map[string]string{
					"Content-Type": "text/plain",
				},
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
				break
			}

			defer file.Close()

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
				break
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
			break
		}
		resp = Response{
			Version: "HTTP/1.1",
			Status:  "200",
			Message: "OK",
			Headers: map[string]string{
				"Content-Type":     "text/plain",
				"Content-Length":   strconv.Itoa(len(req.Routes[2])),
				"Content-Encoding": getEncoding(req),
			},
			Body: req.Routes[2],
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

	err = resp.Write(conn)
	if err != nil {
		fmt.Println("Error writing response: ", err.Error())
	}
}
