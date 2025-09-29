package main

import (
	"fmt"
	"net"
	"os"
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
)

func main() {
	fmt.Println("Binding to port 4221")
	listener, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	fmt.Println("Accepting client connection")
	conn, err := listener.Accept()
	if err != nil {
		fmt.Println("Error accepting connection: ", err.Error())
		os.Exit(1)
	}

	fmt.Println("Getting request path")
	buffer := make([]byte, 1024)
	_, err = conn.Read(buffer)
	if err != nil {
		fmt.Println("Error reading request: ", err.Error())
		os.Exit(1)
	}

	var resp string
	// method, path, version := getRequestLine(buffer)
	_, path, _ := getRequestLine(buffer)

	// fmt.Println(method + " " + path + " " + version)
	rootpath := strings.Split(path, "/")[1]

	fmt.Println("Sending response")
	switch rootpath {
	case "":
		resp = OK + CRLF
	case "echo":
		resp = OK + CRLF
		resp += CONTENT_TYPE + "text/plain" + CRLF
		if len(path) > 1 {
			phrase := strings.Split(path, "/")[2]
			resp += fmt.Sprintf("%s%v%s", CONTENT_LENGTH, len(phrase), CRLF) + CRLF
			resp += phrase
			break
		}
	default:
		fmt.Printf("Error path \"%s\" not found\n", path)
		resp = NOT_FOUND + CRLF
	}

	resp += CRLF
	fmt.Println(resp)
	conn.Write([]byte(resp))
}

func getRequestLine(buffer []byte) (method, path, version string) {
	requestLine := strings.Split(strings.Split(string(buffer), CRLF)[0], " ")
	method = requestLine[0]
	path, version = "", ""

	fmt.Println(requestLine)
	if len(requestLine) == 3 {
		path = requestLine[1]
		version = requestLine[2]
	} else {
		version = requestLine[1]
	}
	return
}
