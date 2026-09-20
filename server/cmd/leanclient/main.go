package main

import (
	"fmt"
	"net"
)

func main() {
	// Connect to the server
	conn, err := net.Dial("tcp", "localhost:8080")
	if err != nil {
		fmt.Println("Error connecting to server: ", err)
		return
	}
	defer conn.Close()

	// Send data to the server
	_, err = conn.Write([]byte("{\"token\": \"hello\", \"content\": \"world\"}"))
	if err != nil {
		fmt.Println("Error sending data to server: ", err)
		return
	}

	// Read data back from server
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("Error reading from server: ", err)
		return
	}
	fmt.Printf("Received: %s\n", buf[:n])
}
