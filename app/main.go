package main

import (
	"errors"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	CRLF = "\r\n"
	// Response Status Lines
	OK        = "HTTP/1.1 200 OK"
	NOT_FOUND = "HTTP/1.1 404 Not Found"

	// Response Headers
	CONTENT_TYPE   = "Content-Type: "
	CONTENT_LENGTH = "Content-Length: "
	USER_AGENT     = "User-Agent: "
)

var logger = log.Default()

func main() {
	logger.Println("Binding to port 4221")
	listener, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		logger.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	connCounter := 0
	logger.Println("Accepting client connections")
	for {
		conn, err := listener.Accept()
		logger.Printf("Accepted connection #%d", connCounter)
		if err != nil {
			logger.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}

		go func() {
			logger.Printf("Handling request #%d", connCounter)
			err = handleConnection(conn)
			if err != nil {
				logger.Fatal("Error handling request: ", err.Error())
			}
		}()
		connCounter++
	}

}

func handleConnection(conn net.Conn) error {
	buffer := make([]byte, 1024)
	_, err := conn.Read(buffer)
	if err != nil {
		return errors.New("error reading request: " + err.Error())
	}

	var resp string
	// method, path, version := getRequestLine(buffer)
	_, path, _ := getRequestLine(buffer)

	// logger.Println(method + " " + path + " " + version)
	pathSubstrings := strings.Split(path, "/")
	rootpath := pathSubstrings[1]

	logger.Println("Sending response")
	switch rootpath {
	case "":
		resp = OK + CRLF

	case "echo":
		var body string
		if len(pathSubstrings) > 2 {
			body = pathSubstrings[2]
		}
		resp = buildOKResponseWithBody(body)

	case "user-agent":
		headers := getRequestHeaders(buffer)
		userAgent := headers[strings.ToLower(USER_AGENT)]
		resp = buildOKResponseWithBody(userAgent)

	default:
		logger.Printf("Error path \"%s\" not found\n", path)
		resp = NOT_FOUND + CRLF
	}

	resp += CRLF
	logger.Println(resp)
	conn.Write([]byte(resp))
	return nil
}

func getRequestLine(buffer []byte) (method, path, version string) {
	requestLine := strings.Split(strings.Split(string(buffer), CRLF)[0], " ")
	method = requestLine[0]
	path, version = "", ""

	logger.Println(requestLine)
	if len(requestLine) == 3 {
		path = requestLine[1]
		version = requestLine[2]
	} else {
		version = requestLine[1]
	}
	return
}

func buildOKResponseWithBody(body string) (resp string) {
	resp = OK + CRLF
	resp += CONTENT_TYPE + "text/plain" + CRLF
	resp += CONTENT_LENGTH + strconv.Itoa(len(body)) + CRLF + CRLF
	resp += body
	return
}

func getRequestHeaders(buffer []byte) map[string]string {
	headers := make(map[string]string)
	lines := strings.SplitAfter(strings.Split(string(buffer), CRLF+CRLF)[0], CRLF)[1:]
	for _, line := range lines {
		name := strings.Split(line, ": ")[0]
		value := strings.Split(line, ": ")[1]

		headers[strings.ToLower(name)+": "] = value
	}

	return headers
}
