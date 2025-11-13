#!/bin/bash

# Quick build test script
cd "$(dirname "$0")"

echo "Building escabelo..."
go build -o bin/escabelo ./cmd/escabelo

if [ $? -eq 0 ]; then
    echo "✓ Server build successful"
else
    echo "✗ Server build failed"
    exit 1
fi

echo "Building bench..."
go build -o bin/bench ./cmd/bench

if [ $? -eq 0 ]; then
    echo "✓ Bench build successful"
else
    echo "✗ Bench build failed"
    exit 1
fi

echo "Building client..."
go build -o bin/client ./cmd/client

if [ $? -eq 0 ]; then
    echo "✓ Client build successful"
else
    echo "✗ Client build failed"
    exit 1
fi

echo ""
echo "All builds successful! ✓"
echo ""
echo "To run:"
echo "  ./bin/escabelo          # Start server"
echo "  ./bin/client            # Interactive client"
echo "  ./bin/bench             # Benchmark"
