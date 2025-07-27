#!/bin/bash

# Script to check IPThing server status reliably

# Check for PID file first
if [ -f ipthing.pid ]; then
    pid=$(cat ipthing.pid)
    if kill -0 "$pid" 2>/dev/null; then
        echo "IPThing server is running (PID: $pid)"
        exit 0
    else
        echo "IPThing server is not running (stale PID file: $pid)"
        rm -f ipthing.pid
    fi
fi

# Look for running processes
go_pids=$(pgrep -f "go run \\." 2>/dev/null || true)
binary_pids=$(pgrep -f "/ipthing" 2>/dev/null || true)

if [ -n "$go_pids" ] || [ -n "$binary_pids" ]; then
    echo "IPThing server is running (no PID file)"
    if [ -n "$go_pids" ]; then
        echo "  Go run processes: $go_pids"
    fi
    if [ -n "$binary_pids" ]; then
        echo "  Binary processes: $binary_pids"
    fi
else
    echo "IPThing server is not running"
fi
