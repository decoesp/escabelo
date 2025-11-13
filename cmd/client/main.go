package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
)

var (
	addr = flag.String("addr", "localhost:8080", "Server address")
)

func main() {
	flag.Parse()

	// Connect to server
	conn, err := net.Dial("tcp", *addr)
	if err != nil {
		fmt.Printf("Failed to connect to %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Printf("Connected to %s\n", *addr)
	fmt.Println("Commands: read <key> | write <key>|<value> | delete <key> | status | keys | reads <prefix> | quit")
	fmt.Println()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := scanner.Text()
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if line == "quit" || line == "exit" {
			break
		}

		// Send command
		cmd := line + "\r"
		if _, err := writer.WriteString(cmd); err != nil {
			fmt.Printf("Send error: %v\n", err)
			break
		}
		if err := writer.Flush(); err != nil {
			fmt.Printf("Flush error: %v\n", err)
			break
		}

		// Read response
		response, err := reader.ReadString('\r')
		if err != nil {
			fmt.Printf("Read error: %v\n", err)
			break
		}

		// Remove \r and print
		response = strings.TrimSuffix(response, "\r")
		fmt.Println(response)
		fmt.Println()
	}

	fmt.Println("Goodbye!")
}
