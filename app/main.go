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
	CRLF = "\r\n"
	// Response Status Lines
	OK        = "HTTP/1.1 200 OK"
	NOT_FOUND = "HTTP/1.1 404 Not Found"

	// Response Headers
	CONTENT_TYPE   = "Content-Type: "
	CONTENT_LENGTH = "Content-Length: "
	USER_AGENT     = "User-Agent: "

	// Content-Types
	PLAINTEXT        = "text/plain"
	APP_OCTET_STREAM = "application/octet-stream"
)

const (
	tmpDataDir = "/tmp/data/codecrafters.io/http-server-tester/"
)

func main() {
	log.Info("Binding to port 4221")
	listener, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		log.Info("Failed to bind to port 4221")
		os.Exit(1)
	}

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
	buffer := make([]byte, 1024)
	_, err := conn.Read(buffer)
	if err != nil {
		return errors.New("error reading request: " + err.Error())
	}

	var resp string
	// method, path, version := getRequestLine(buffer)
	_, path, _ := getRequestLine(buffer)

	// log.Info(method + " " + path + " " + version)
	pathSubstrings := strings.Split(path, "/")
	rootpath := pathSubstrings[1]

	log.Info("Sending response")
	switch rootpath {
	case "":
		resp = OK + CRLF
		resp += CRLF // make the end of the headers

	case "echo":
		var body string
		if len(pathSubstrings) > 2 {
			body = pathSubstrings[2]
		}
		resp = getOKResponseWithBody(body, PLAINTEXT)

	case "user-agent":
		headers := getRequestHeaders(buffer)
		userAgent := headers[strings.ToLower(USER_AGENT)]
		resp = getOKResponseWithBody(userAgent, PLAINTEXT)

	case "files":
		// set response to not found by default.
		resp = getNotFoundResponse()
		// process request / provided file.
		filename := pathSubstrings[2]
		if file, err := os.Open(tmpDataDir + filename); err == nil {
			if content, err := io.ReadAll(file); err != nil {
				log.Error("couldn't read file content: ", err)
				resp = getNotFoundResponse()
			} else {
				resp = getOKResponseWithBody(string(content), APP_OCTET_STREAM)
			}
		} else {
			log.Errorf("couldn't open file %s: %s", filename, err)
		}
	default:
		log.Info(fmt.Sprintf("Error path \"%s\" not found\n", path))
		resp = getNotFoundResponse()
	}

	log.Info("Response: \n", resp)
	conn.Write([]byte(resp))
	return nil
}

func getRequestLine(buffer []byte) (method, path, version string) {
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

func getOKResponseWithBody(body, contentType string) (resp string) {
	resp = OK + CRLF // status line
	// headers
	resp += CONTENT_TYPE + contentType + CRLF
	resp += CONTENT_LENGTH + strconv.Itoa(len(body)) + CRLF
	resp += CRLF // make end of headers
	// body
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
	lines := strings.SplitAfter(strings.Split(string(buffer), CRLF+CRLF)[0], CRLF)[1:]
	for _, line := range lines {
		name := strings.Split(line, ": ")[0]
		value := strings.Split(line, ": ")[1]

		headers[strings.ToLower(name)+": "] = value
	}

	return headers
}
