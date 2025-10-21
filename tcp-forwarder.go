package main

import (
	"bytes"
	// "fmt"
	"io"
	"log"
	"net"
	"sync"
)

const (
	// maxBufferSize defines the 10MB limit for the data buffer.
	maxBufferSize = 10 * 1024 * 1024
	// sourceAddr is the address for the client that sends data to be buffered.
	sourceAddr = "127.0.0.1:13337"
	// replayAddr is the address for clients that want to receive the buffered data.
	replayAddr = "127.0.0.1:13338"
	// forwardAddr is the address for clients that want to forward data to the source client.
	// NOTE: This was assumed to be a different port than replayAddr to avoid ambiguity.
	forwardAddr = "127.0.0.1:13339"
)

// safeBuffer is a thread-safe buffer to store data from the source client.
// A mutex is used to prevent race conditions when multiple goroutines
// access the buffer simultaneously.
type safeBuffer struct {
	buf bytes.Buffer
	mux sync.Mutex
}

// Write appends data to the buffer in a thread-safe manner.
func (sb *safeBuffer) Write(p []byte) (n int, err error) {
	sb.mux.Lock()
	defer sb.mux.Unlock()
	return sb.buf.Write(p)
}

// ReadAndReset reads the entire buffer's content and then clears the buffer.
// This is done as an atomic operation to ensure data consistency.
func (sb *safeBuffer) ReadAndReset() []byte {
	sb.mux.Lock()
	defer sb.mux.Unlock()
	data := sb.buf.Bytes()
	sb.buf.Reset()
	return data
}

var (
	// dataBuffer holds all data recorded from the source client.
	dataBuffer safeBuffer

	// sourceClient holds the single, primary client connection.
	// A mutex is needed because the forwarder service will read this
	// while the source service writes to it.
	sourceClient struct {
		conn net.Conn
		mux  sync.Mutex
	}
)

func main() {
	// A WaitGroup is used to keep the main function alive while the listeners
	// run in the background as goroutines.
	var wg sync.WaitGroup
	wg.Add(3)

	log.Println("Starting TCP server...")

	go listenForSource(&wg)
	go listenForReplay(&wg)
	go listenForForwarding(&wg)

	log.Printf("-> Source Listener running on tcp://%s", sourceAddr)
	log.Printf("-> Replay Listener running on tcp://%s", replayAddr)
	log.Printf("-> Forward Listener running on tcp://%s", forwardAddr)

	wg.Wait()
}

// listenForSource listens on 13337 for a single client connection.
// It records everything the client sends into the shared dataBuffer.
func listenForSource(wg *sync.WaitGroup) {
	defer wg.Done()

	listener, err := net.Listen("tcp", sourceAddr)
	if err != nil {
		log.Fatalf("Failed to start source listener: %v", err)
	}
	// We defer closing the listener right after it's opened. Once a client connects,
	// this function will block on io.Copy, and the listener will be closed
	// when the connection is eventually terminated.
	defer listener.Close()

	// Accept only one connection.
	conn, err := listener.Accept()
	if err != nil {
		log.Printf("Failed to accept source connection: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("Source client connected from %s. Ceasing to accept new connections on %s.", conn.RemoteAddr(), sourceAddr)

	// Store the connection globally so other parts of the app can use it.
	sourceClient.mux.Lock()
	sourceClient.conn = conn
	sourceClient.mux.Unlock()

	// Use io.LimitReader to enforce the 10MB buffer limit.
	limitedReader := io.LimitReader(conn, maxBufferSize)

	// Copy data from the client connection directly into our thread-safe buffer.
	// This will block until the connection is closed or the limit is reached.
	written, err := io.Copy(&dataBuffer, limitedReader)
	if err != nil {
		log.Printf("Error reading from source client: %v", err)
	}
	log.Printf("Source client disconnected. Read %d bytes.", written)

	// Clear the global connection reference.
	sourceClient.mux.Lock()
	sourceClient.conn = nil
	sourceClient.mux.Unlock()
}

// listenForReplay listens on 13338 for clients who want to receive the buffer.
// It continues to accept new clients.
func listenForReplay(wg *sync.WaitGroup) {
	defer wg.Done()

	listener, err := net.Listen("tcp", replayAddr)
	if err != nil {
		log.Fatalf("Failed to start replay listener: %v", err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept replay connection: %v", err)
			continue // Keep trying to accept new connections.
		}
		// Handle each client in a new goroutine to allow for concurrent connections.
		go handleReplayConnection(conn)
	}
}

// handleReplayConnection sends the buffer contents to a client and then closes the connection.
func handleReplayConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("Replay client connected from %s. Sending buffer.", conn.RemoteAddr())

	// Atomically get the buffer data and clear it.
	data := dataBuffer.ReadAndReset()

	if len(data) == 0 {
		log.Printf("Buffer is empty. Nothing to send to replay client %s.", conn.RemoteAddr())
		return
	}

	// Write the captured data to the replay client.
	written, err := conn.Write(data)
	if err != nil {
		log.Printf("Error sending buffer to replay client %s: %v", conn.RemoteAddr(), err)
		return
	}
	log.Printf("Sent %d bytes to replay client %s and closed connection.", written, conn.RemoteAddr())
}

// listenForForwarding listens on 13339 and forwards all incoming data to the source client.
// It continues to accept new clients.
func listenForForwarding(wg *sync.WaitGroup) {
	defer wg.Done()

	listener, err := net.Listen("tcp", forwardAddr)
	if err != nil {
		log.Fatalf("Failed to start forward listener: %v", err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept forward connection: %v", err)
			continue
		}
		go handleForwardingConnection(conn)
	}
}

// handleForwardingConnection forwards data from a "forward" client to the main "source" client.
func handleForwardingConnection(conn net.Conn) {
	defer conn.Close()
	log.Printf("Forwarding client connected from %s.", conn.RemoteAddr())

	// Safely get the source client connection.
	sourceClient.mux.Lock()
	sc := sourceClient.conn
	sourceClient.mux.Unlock()

	// If the source client is not connected, we cannot forward data.
	if sc == nil {
		log.Printf("Source client is not connected. Closing connection for forwarding client %s.", conn.RemoteAddr())
		return
	}

	// io.Copy will block and stream data from the forwarding client to the source client.
	copied, err := io.Copy(sc, conn)
	if err != nil {
		log.Printf("Error forwarding data from %s: %v", conn.RemoteAddr(), err)
	}
	log.Printf("Forwarding client %s disconnected after forwarding %d bytes.", conn.RemoteAddr(), copied)
}
