package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

/*
https://medium.com/@diasmashikovnasa/tcp-listeners-and-servers-in-golang-a-beginners-guide-9032a9e2eeeb
https://okanexe.medium.com/the-complete-guide-to-tcp-ip-connections-in-golang-1216dae27b5a
https://oneuptime.com/blog/post/2026-01-25-concurrent-tcp-server-10k-connections-go/view
*/

const maxConnections = 10000
const readTimeoutSeconds = 30
const writeTimeoutSeconds = 10
const bufferSizeBytes = 1024 // 1KB buffers - make bigger if needed

func main() {
	// Create a TCP lisener
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal("Error creating listener:", err)
	}
	defer listener.Close()

	// Context for coordinating shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Semaphore to limit connections
	sem := make(chan struct{}, maxConnections)
	var wg sync.WaitGroup

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Unknown signal received")
		cancel()
		listener.Close() // Unblock Accept()
	}()

	fmt.Println("Server is running on port 8080")

	for {
		// Accept incoming connections
		conn, err := listener.Accept() // blocks until connection arrives
		if err != nil {
			// Check if graceful shutdown
			select {
			case <-ctx.Done():
				log.Println("Waiting for connections to close...")
				wg.Wait()
				log.Println("Server stopped")
				return
			default:
				log.Printf("Error accepting connection: %v", err)
				continue
			}
		}

		// Acquire semaphore (blocks if over limit)
		sem <- struct{}{}
		wg.Add(1)

		// Handle client connection
		go func(ctx context.Context, conn net.Conn) {
			defer wg.Done()
			defer func() { <-sem }() // Release slot when done
			handleConnection(ctx, conn)
		}(ctx, conn)
	}
}

// Pool of buffers to save resources by reusing streams
var bufferPool = sync.Pool{
	New: func() any {
		// 1KB buffers - make bigger if needed
		return new([bufferSizeBytes]byte)
	},
}

func handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close() // Just before this function exits, the connection will be closed

	// Get a buffer from the pool
	buf := bufferPool.Get().(*[bufferSizeBytes]byte)
	defer bufferPool.Put(buf) // Return the buffer when done

	for {
		// Check if shutdown was requested
		select {
		case <-ctx.Done():
			return
		default: // Pass
		}

		// Enforce timeout for reading
		conn.SetReadDeadline(time.Now().Add(readTimeoutSeconds * time.Second))

		// Read data from the connection
		n, err := conn.Read(buf[:]) // n is number of bytes read
		if err != nil {             // Timeout or connection closed
			log.Println("Error reading data from connection:", err)
			return
		}

		// Process data and generate response (placeholder)
		fmt.Printf("Received: %s\n", buf[:n])
		response := []byte(buf[:n])

		// Enforce timeout for writing
		conn.SetWriteDeadline(time.Now().Add(writeTimeoutSeconds * time.Second))

		// Send response to the client
		_, err = conn.Write(response)
		if err != nil { // Timeout or connection closed
			log.Println("Error sending response to connection:", err)
			return
		}
	}

}
