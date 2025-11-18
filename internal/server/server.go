package server

import (
	"bufio"
	"escabelo/internal/engine"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
)

// Server handles TCP connections
type Server struct {
	engine   *engine.Engine
	listener net.Listener
	addr     string
	wg       sync.WaitGroup
	stopCh   chan struct{}
}

// NewServer creates a new TCP server
func NewServer(addr string, eng *engine.Engine) *Server {
	return &Server{
		engine: eng,
		addr:   addr,
		stopCh: make(chan struct{}),
	}
}

// Start begins listening for connections
func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}

	s.listener = listener
	log.Printf("Server listening on %s", s.addr)

	go s.acceptLoop()
	return nil
}

// acceptLoop accepts incoming connections
func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				log.Printf("Accept error: %v", err)
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection processes a client connection
func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	log.Printf("New connection from %s", conn.RemoteAddr())

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		// Read until \r separator
		line, err := reader.ReadString('\r')
		if err != nil {
			if err != io.EOF {
				log.Printf("Read error: %v", err)
			}
			return
		}

		// Remove \r
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}

		// Parse and execute command
		cmd, err := ParseCommand(line)
		if err != nil {
			s.writeResponse(writer, fmt.Sprintf("error: %v", err))
			continue
		}

		response := s.executeCommand(cmd)
		s.writeResponse(writer, response)
	}
}

// executeCommand executes a parsed command
func (s *Server) executeCommand(cmd *Command) string {
	switch cmd.Type {
	case CmdRead:
		value, found, err := s.engine.Get(cmd.Key)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		if !found {
			return "error"
		}
		return string(value)

	case CmdWrite:
		if err := s.engine.Put(cmd.Key, cmd.Value); err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		return "success"

	case CmdDelete:
		deleted, err := s.engine.Delete(cmd.Key)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		if !deleted {
			return "error"
		}
		return "success"

	case CmdStatus:
		stats := s.engine.GetStats()
		return fmt.Sprintf("well going our operation\nwrites=%d reads=%d deletes=%d flushes=%d memtable_size=%d sst_count=%d wal_size=%d",
			stats.Writes, stats.Reads, stats.Deletes, stats.Flushes, stats.MemTableSize, stats.SSTCount, stats.WALSize)

	case CmdKeys:
		keys, err := s.engine.Keys()
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		if len(keys) == 0 {
			return ""
		}
		return strings.Join(keys, "\r")

	case CmdReads:
		values, err := s.engine.PrefixScan(cmd.Prefix)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		if len(values) == 0 {
			return ""
		}
		strValues := make([]string, len(values))
		for i, v := range values {
			strValues[i] = string(v)
		}
		return strings.Join(strValues, "\r")

	default:
		return "error: unknown command"
	}
}

// writeResponse writes a response to the client
func (s *Server) writeResponse(writer *bufio.Writer, response string) {
	writer.WriteString(response)
	writer.WriteString("\r")
	writer.Flush()
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	close(s.stopCh)

	if s.listener != nil {
		s.listener.Close()
	}

	s.wg.Wait()
	return nil
}
