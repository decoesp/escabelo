#!/bin/bash

# Test script for Escabelo
set -e

echo "==================================="
echo "Escabelo Test Suite"
echo "==================================="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PORT=9999
DATA_DIR="./test-data-$$"
SERVER_PID=""

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."
    if [ ! -z "$SERVER_PID" ]; then
        kill $SERVER_PID 2>/dev/null || true
        wait $SERVER_PID 2>/dev/null || true
    fi
    rm -rf "$DATA_DIR"
    echo "Cleanup complete"
}

trap cleanup EXIT

# Build
echo "Building..."
make build-all > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Build successful${NC}"
else
    echo -e "${RED}✗ Build failed${NC}"
    exit 1
fi

# Start server
echo ""
echo "Starting server on port $PORT..."
./bin/escabelo -port=$PORT -data-dir=$DATA_DIR > /dev/null 2>&1 &
SERVER_PID=$!
sleep 2

# Check if server is running
if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo -e "${RED}✗ Server failed to start${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Server started (PID: $SERVER_PID)${NC}"

# Helper function to send command
send_cmd() {
    echo -n "$1\r" | nc localhost $PORT 2>/dev/null | tr -d '\r'
}

# Test 1: Write operation
echo ""
echo "Test 1: Write operation"
RESULT=$(send_cmd "write test:1|hello-world")
if [ "$RESULT" = "success" ]; then
    echo -e "${GREEN}✓ Write successful${NC}"
else
    echo -e "${RED}✗ Write failed: $RESULT${NC}"
    exit 1
fi

# Test 2: Read operation
echo ""
echo "Test 2: Read operation"
RESULT=$(send_cmd "read test:1")
if [ "$RESULT" = "hello-world" ]; then
    echo -e "${GREEN}✓ Read successful${NC}"
else
    echo -e "${RED}✗ Read failed: $RESULT${NC}"
    exit 1
fi

# Test 3: Multiple writes
echo ""
echo "Test 3: Multiple writes"
send_cmd "write user:1|Alice" > /dev/null
send_cmd "write user:2|Bob" > /dev/null
send_cmd "write user:3|Charlie" > /dev/null
RESULT=$(send_cmd "read user:2")
if [ "$RESULT" = "Bob" ]; then
    echo -e "${GREEN}✓ Multiple writes successful${NC}"
else
    echo -e "${RED}✗ Multiple writes failed${NC}"
    exit 1
fi

# Test 4: Delete operation
echo ""
echo "Test 4: Delete operation"
RESULT=$(send_cmd "delete user:1")
if [ "$RESULT" = "success" ]; then
    echo -e "${GREEN}✓ Delete successful${NC}"
else
    echo -e "${RED}✗ Delete failed: $RESULT${NC}"
    exit 1
fi

# Test 5: Read deleted key
echo ""
echo "Test 5: Read deleted key"
RESULT=$(send_cmd "read user:1")
if [[ "$RESULT" == *"error"* ]]; then
    echo -e "${GREEN}✓ Deleted key not found (correct)${NC}"
else
    echo -e "${RED}✗ Deleted key still exists${NC}"
    exit 1
fi

# Test 6: Keys command
echo ""
echo "Test 6: Keys command"
RESULT=$(send_cmd "keys")
if [[ "$RESULT" == *"user:2"* ]] && [[ "$RESULT" == *"user:3"* ]]; then
    echo -e "${GREEN}✓ Keys command successful${NC}"
else
    echo -e "${RED}✗ Keys command failed${NC}"
    exit 1
fi

# Test 7: Prefix scan
echo ""
echo "Test 7: Prefix scan"
send_cmd "write prefix:a|value-a" > /dev/null
send_cmd "write prefix:b|value-b" > /dev/null
RESULT=$(send_cmd "reads prefix:")
if [[ "$RESULT" == *"value-a"* ]] && [[ "$RESULT" == *"value-b"* ]]; then
    echo -e "${GREEN}✓ Prefix scan successful${NC}"
else
    echo -e "${RED}✗ Prefix scan failed${NC}"
    exit 1
fi

# Test 8: Status command
echo ""
echo "Test 8: Status command"
RESULT=$(send_cmd "status")
if [[ "$RESULT" == *"writes="* ]] && [[ "$RESULT" == *"reads="* ]]; then
    echo -e "${GREEN}✓ Status command successful${NC}"
    echo "  Status: $RESULT"
else
    echo -e "${RED}✗ Status command failed${NC}"
    exit 1
fi

# Test 9: Large value
echo ""
echo "Test 9: Large value (10KB)"
LARGE_VALUE=$(head -c 10240 < /dev/urandom | base64 | tr -d '\n')
RESULT=$(send_cmd "write large:1|$LARGE_VALUE")
if [ "$RESULT" = "success" ]; then
    RESULT=$(send_cmd "read large:1")
    if [ "$RESULT" = "$LARGE_VALUE" ]; then
        echo -e "${GREEN}✓ Large value successful${NC}"
    else
        echo -e "${RED}✗ Large value read mismatch${NC}"
        exit 1
    fi
else
    echo -e "${RED}✗ Large value write failed${NC}"
    exit 1
fi

# Test 10: Crash recovery
echo ""
echo "Test 10: Crash recovery"
send_cmd "write crash:1|before-crash" > /dev/null
send_cmd "write crash:2|should-survive" > /dev/null

echo "  Killing server..."
kill -9 $SERVER_PID
wait $SERVER_PID 2>/dev/null || true
sleep 1

echo "  Restarting server..."
./bin/escabelo -port=$PORT -data-dir=$DATA_DIR > /dev/null 2>&1 &
SERVER_PID=$!
sleep 2

RESULT=$(send_cmd "read crash:1")
if [ "$RESULT" = "before-crash" ]; then
    RESULT=$(send_cmd "read crash:2")
    if [ "$RESULT" = "should-survive" ]; then
        echo -e "${GREEN}✓ Crash recovery successful${NC}"
    else
        echo -e "${RED}✗ Crash recovery failed (crash:2)${NC}"
        exit 1
    fi
else
    echo -e "${RED}✗ Crash recovery failed (crash:1)${NC}"
    exit 1
fi

# Summary
echo ""
echo "==================================="
echo -e "${GREEN}All tests passed! ✓${NC}"
echo "==================================="
