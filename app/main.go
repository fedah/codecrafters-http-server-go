package main

import (
	"errors"
	"fmt"

	"io"
	"net"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	// Carriage Return Line Feed
	CRLF = "\r\n"

	// Response Status Lines
	OK        = "HTTP/1.1 200 OK"
	CREATED   = "HTTP/1.1 201 Created"
	NOT_FOUND = "HTTP/1.1 404 Not Found"

	// Request Headers
	USER_AGENT      = "User-Agent: "
	ACCEPT_ENCODING = "Accept-Encoding: "

	// Response Headers
	CONTENT_TYPE     = "Content-Type: "
	CONTENT_LENGTH   = "Content-Length: "
	CONTENT_ENCODING = "Content-Encoding: "

	// Content-Types
	PLAINTEXT        = "text/plain"
	APP_OCTET_STREAM = "application/octet-stream"
)

const (
	tmpDataDir     = "/tmp/data/codecrafters.io/http-server-tester/"
	serverIp       = "0.0.0.0"
	serverPort     = "4221"
	connBufferSize = 1024
)

// getSocketAdress returns a complete socket address given a ip and a port
func getSocketAdress(ip, port string) string {
	return ip + ":" + port
}

func main() {
	// Instantiate the server tcp listener.
	log.Info("Binding to port 4221")
	listener, err := net.Listen("tcp", getSocketAdress(serverIp, serverPort))
	if err != nil {
		log.Info("Failed to bind to port 4221")
		os.Exit(1)
	}

	// Accept and concurrently handle connections on requests
	connCounter := 0
	log.Info("Accepting client connections")
	for {
		conn, err := listener.Accept()
		log.Info("Accepted connection #", connCounter)
		if err != nil {
			log.Info("Error accepting connection: ", err)
			os.Exit(1)
		}

		go func() {
			defer conn.Close()
			log.Info("Handling request #", connCounter)
			err = handleConnection(conn)
			if err != nil {
				log.Error("Error handling request: ", err.Error())
				os.Exit(1)
			}
		}()
		connCounter++
	}

}

func handleConnection(conn net.Conn) error {
	req, err := NewRequest(conn)
	if err != nil {
		return errors.New("error reading request: " + err.Error())
	}

	var resp string
	path := NewHttpPathWrapper(req.path)

	log.Info("Sending response")
	switch path.main() {
	case "":
		resp = OK + CRLF
		resp += CRLF // make the end of the headers

	case "echo":
		respBody := path.secondary()
		resp = getOKResponseWithBody(respBody, PLAINTEXT, req.getResponseEncoding())

	case "user-agent":
		userAgent := req.getUserAgent()
		resp = getOKResponseWithBody(userAgent, PLAINTEXT, req.getResponseEncoding())

	case "files":
		filename := path.secondary()
		switch req.method {
		case "GET":
			resp = handleGetFileRequest(req, filename)
		case "POST":
			resp = CREATED + CRLF + CRLF
			handlePostFileRequest(req, filename)
		default:
			resp = getNotFoundResponse()
		}

	default:
		log.Info(fmt.Sprintf("Error path \"%s\" not found\n", req.path))
		resp = getNotFoundResponse()
	}

	log.Info("Response: \n", resp)
	conn.Write([]byte(resp))
	return nil
}

func getRequestLine(buffer []byte) (method, path, version string) {
	// The format of a HTTP request line is as follow:
	// -> method path version OR
	requestLine := strings.Split(strings.Split(string(buffer), CRLF)[0], " ")
	method = requestLine[0]
	path, version = "", ""

	log.Info("request line: ", requestLine)
	if len(requestLine) == 3 {
		path = requestLine[1]
		version = requestLine[2]
	} else {
		version = requestLine[1]
	}
	return
}

func getOKResponseWithBody(body, contentType, contentEncoding string) (resp string) {
	// status line
	resp = OK + CRLF

	// responses headers
	resp += CONTENT_TYPE + contentType + CRLF
	resp += CONTENT_LENGTH + strconv.Itoa(len(body)) + CRLF

	// Only include a content encoding header if provided
	if contentEncoding != "" {
		resp += CONTENT_ENCODING + contentEncoding + CRLF
	}
	resp += CRLF // make end of headers

	// response body
	resp += body
	return
}

func getNotFoundResponse() (resp string) {
	resp = NOT_FOUND + CRLF
	resp += CRLF // make the end of the headers
	return
}

func getRequestHeaders(buffer []byte) map[string]string {
	headers := make(map[string]string)
	lines := strings.Split(strings.Split(string(buffer), CRLF+CRLF)[0], CRLF)[1:]
	for _, line := range lines {
		if line == "" {
			continue
		}
		name := strings.Split(line, ": ")[0]
		value := strings.Split(line, ": ")[1]

		headers[strings.ToLower(name)+": "] = value
	}

	return headers
}

func getRequestBody(buffer []byte) []byte {
	body := strings.Split(string(buffer), CRLF+CRLF)[1]
	return []byte(body)
}

func handleGetFileRequest(req *request, filename string) (resp string) {
	// set response to not found by default.
	resp = getNotFoundResponse()
	// process request / provided file.
	if file, err := os.Open(tmpDataDir + filename); err == nil {
		if content, err := io.ReadAll(file); err != nil {
			log.Error("couldn't read file content: ", err)
			resp = getNotFoundResponse()
		} else {
			resp = getOKResponseWithBody(string(content), APP_OCTET_STREAM, req.getResponseEncoding())
		}
	} else {
		log.Errorf("couldn't open file %s: %s", filename, err)
	}
	return
}

func handlePostFileRequest(req *request, filename string) error {
	content := req.body[:req.getContentLength()]
	os.WriteFile(tmpDataDir+filename, content, os.ModePerm)
	return nil
}

type httpPathWrapper struct {
	path     string
	contents []string
}

func NewHttpPathWrapper(path string) *httpPathWrapper {
	// Return early if the path that is empty
	if len(path) == 0 {
		return nil
	}

	// Since the http path always starts with /...
	// contents[0] = "" when path is not empty.
	// Thus we drop the first element in the slice, for ease of use.
	contents := strings.Split(path, "/")
	contents = contents[1:]
	return &httpPathWrapper{
		path:     path,
		contents: contents,
	}
}

func (pw *httpPathWrapper) main() string {
	if pw == nil || len(pw.contents) < 1 {
		return ""
	}

	return pw.contents[0]
}

func (pw *httpPathWrapper) secondary() string {
	if pw == nil || len(pw.contents) < 2 {
		return ""
	}

	return strings.Join(pw.contents[1:], "/")
}

type request struct {
	conn net.Conn

	method  string
	path    string
	version string

	headers map[string]string
	body    []byte
}

func NewRequest(conn net.Conn) (*request, error) {
	buffer := make([]byte, connBufferSize)
	_, err := conn.Read(buffer)
	if err != nil {
		return nil, errors.New("error reading request: " + err.Error())
	}

	method, path, version := getRequestLine(buffer)
	headers := getRequestHeaders(buffer)
	body := getRequestBody(buffer)

	return &request{
		conn:    conn,
		method:  method,
		path:    path,
		version: version,
		headers: headers,
		body:    body,
	}, nil
}

func (r *request) getHeader(headerName string) (string, bool) {
	value, found := r.headers[strings.ToLower(headerName)]
	return value, found
}

func (r *request) getContentType() string {
	value, _ := r.getHeader(CONTENT_TYPE)
	return value
}

func (r *request) getUserAgent() string {
	value, _ := r.getHeader(USER_AGENT)
	return value
}

func (r *request) getClientEncodingSchemes() []string {
	// The typical key/value pair for encoding request header looks like:
	// Accept-Encoding: encoding-1, encoding-2, encoding-3
	value, found := r.getHeader(ACCEPT_ENCODING)
	if !found {
		return nil
	}
	return strings.Split(value, ", ")
}

var serverSupportedEncodingSchemes = map[string]any{
	"gzip": nil,
}

func (r *request) getResponseEncoding() string {
	schemes := r.getClientEncodingSchemes()
	for _, scheme := range schemes {
		if _, ok := serverSupportedEncodingSchemes[scheme]; ok {
			return scheme
		}
	}
	return ""
}

func (r *request) getContentLength() int {
	value, _ := r.getHeader(CONTENT_LENGTH)
	length, _ := strconv.Atoi(value)
	return length
}
