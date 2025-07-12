#!/bin/bash

# Simple test script to demonstrate the crawler
# Usage: ./test.sh

echo "Building crawler..."
go build -o crawler main.go

if [ $? -eq 0 ]; then
    echo "Build successful!"
    
    echo "Testing help message..."
    ./crawler -h
    
    echo ""
    echo "Example usage:"
    echo "./crawler -url https://example.com -workers 3 -output ./example-output -verbose"
    
    echo ""
    echo "To test with a real website (be respectful!):"
    echo "./crawler -url https://httpbin.org -workers 2 -output ./test-output"
else
    echo "Build failed!"
    exit 1
fi
