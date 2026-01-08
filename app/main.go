package main

import (
	"bytes"
	"compress/gzip"
	"sync"

	"io"
	"net"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	// Carriage Return Line Feed
	CRLF  = "\r\n"
	BLANK = ""

	// Response Status Lines
	OK        = "HTTP/1.1 200 OK"
	CREATED   = "HTTP/1.1 201 Created"
	NOT_FOUND = "HTTP/1.1 404 Not Found"

	// Request Headers
	USER_AGENT      = "User-Agent: "
	ACCEPT_ENCODING = "Accept-Encoding: "
	CONNECTION      = "Connection: "

	// Response Headers
	CONTENT_TYPE     = "Content-Type: "
	CONTENT_LENGTH   = "Content-Length: "
	CONTENT_ENCODING = "Content-Encoding: "

	// Content-Types
	PLAINTEXT        = "text/plain"
	APP_OCTET_STREAM = "application/octet-stream"

	// Connection Header Values
	CONNECTION_CLOSE = "close"

	// Content Encoding Shemes
	GZIP string = "gzip"
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
			// defer conn.Close()
			log.Info("Handling Connection #", connCounter)
			err = handleConnection(conn)
			if err != nil {
				log.Error("Error handling request: ", err.Error())
				os.Exit(1)
			}
		}()
		connCounter++
	}
}

// GetRequest returns a request struct if the connection has any pending/incoming request on the wire
// that has not been manifested yet. The keepConnAlive flag informs whether the connections should be close after handling the next request.
// In which case, the connection handler will stop accepting next requests after manifesting this current one.
func GetRequest(conn net.Conn, buffer []byte, wg *sync.WaitGroup) (req *request, keepConnAlive bool) {
	req = NewRequest(conn, buffer, wg)
	keepConnAlive = !req.shouldCloseConnectionAfterReq()
	return req, keepConnAlive
}

func handleConnection(conn net.Conn) error {
	var req *request
	keepConnAlive := true
	wg := &sync.WaitGroup{}
	reqCount := 0

	for keepConnAlive {
		buffer := make([]byte, connBufferSize)
		_, err := conn.Read(buffer)

		if err != nil {
			continue
		}

		req, keepConnAlive = GetRequest(conn, buffer, wg)
		if req != nil {
			reqCount++
			log.Infof("Handling request #%d KeepConnAlive: %t", reqCount, keepConnAlive)
			go handleRequest(req)
		}

		if req.shouldCloseConnectionAfterReq() {
			break
		}
	}

	wg.Wait()
	return nil
}

func handleRequest(req *request) error {
	// Increase connection wait group count
	req.connWg.Add(1)
	var resp string
	path := NewHttpPathWrapper(req.path)

	log.Info("Sending response")
	switch path.main() {
	case "":
		resp = getOKResponseWithBody(req, BLANK, BLANK)

	case "echo":
		respBody := path.secondary()
		resp = getOKResponseWithBody(req, respBody, PLAINTEXT)

	case "user-agent":
		userAgent := req.getUserAgent()
		resp = getOKResponseWithBody(req, userAgent, PLAINTEXT)

	case "files":
		filename := path.secondary()
		switch req.method {
		case "GET":
			resp = handleGetFileRequest(req, filename)
		case "POST":
			resp = CREATED + CRLF + CRLF
			handlePostFileRequest(req, filename)
		default:
			resp = getNotFoundResponse(req)
		}

	default:
		log.Infof("Error path \"%s\" not found\n", req.path)
		resp = getNotFoundResponse(req)
	}

	// Write response on wire
	log.Info("Response: \n", resp)
	req.conn.Write([]byte(resp))
	// Make goroutine as done
	req.connWg.Done()

	// Close connection if instructed by request headers
	if req.shouldCloseConnectionAfterReq() {
		req.connWg.Wait()
		req.conn.Close()
	}
	return nil
}

func getRequestLineParts(buffer []byte) (method, path, version string) {
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

func getOKResponseWithBody(req *request, respBody, contentType string) (resp string) {
	contentEncoding := req.getResponseEncoding()
	closeConnection := req.shouldCloseConnectionAfterReq()

	// status line
	resp = OK + CRLF

	// Only include a content encoding header if provided
	if contentEncoding != "" {
		resp += CONTENT_ENCODING + contentEncoding + CRLF
		// Enocde the body
		var buffer bytes.Buffer
		w := gzip.NewWriter(&buffer)
		w.Write([]byte(respBody))
		w.Close()

		encodedBody := buffer.Bytes()
		log.Printf("Encoded Body: %v %v", encodedBody, buffer)
		respBody = string(encodedBody)
	}

	// responses headers
	if contentType != "" {
		resp += CONTENT_TYPE + contentType + CRLF
	}

	if respBody != "" {
		resp += CONTENT_LENGTH + strconv.Itoa(len(respBody)) + CRLF
	}

	if closeConnection {
		resp += CONNECTION + CONNECTION_CLOSE + CRLF
	}

	// make end of headers
	resp += CRLF

	// response body
	resp += respBody
	return
}

func getNotFoundResponse(req *request) (resp string) {
	resp = NOT_FOUND + CRLF
	if req.shouldCloseConnectionAfterReq() {
		resp += CONNECTION + CONNECTION_CLOSE + CRLF
	}
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
	resp = getNotFoundResponse(req)
	// process request / provided file.
	if file, err := os.Open(tmpDataDir + filename); err == nil {
		if content, err := io.ReadAll(file); err != nil {
			log.Error("couldn't read file content: ", err)
			resp = getNotFoundResponse(req)
		} else {
			resp = getOKResponseWithBody(req, string(content), APP_OCTET_STREAM)
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
	conn   net.Conn
	connWg *sync.WaitGroup

	method  string
	path    string
	version string

	headers map[string]string
	body    []byte
}

func NewRequest(conn net.Conn, buffer []byte, wg *sync.WaitGroup) *request {
	// log.Infof("Request Buffer : %v", string(buffer))

	method, path, version := getRequestLineParts(buffer)
	headers := getRequestHeaders(buffer)
	body := getRequestBody(buffer)

	return &request{
		conn:   conn,
		connWg: wg,

		method:  method,
		path:    path,
		version: version,

		headers: headers,
		body:    body,
	}
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

// shouldCloseConnectionAfterReq flags whether the underlying tcp connection
// should be closed after handling request based on request CONNECTION header value
func (r *request) shouldCloseConnectionAfterReq() bool {
	value, _ := r.getHeader(CONNECTION)
	return value == CONNECTION_CLOSE
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
	GZIP: nil,
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
