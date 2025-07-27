#!/bin/bash

# Script to reliably stop IPThing server processes

echo "Stopping IPThing server..."

# Function to stop processes gracefully
stop_processes() {
    local pids="$1"
    local description="$2"

    if [ -n "$pids" ]; then
        echo "Found $description processes: $pids"
        for pid in $pids; do
            if kill -0 "$pid" 2>/dev/null; then
                echo "Stopping process $pid..."
                kill -TERM "$pid" 2>/dev/null || true
            fi
        done

        # Wait a bit for graceful shutdown
        sleep 2

        # Force kill if still running
        for pid in $pids; do
            if kill -0 "$pid" 2>/dev/null; then
                echo "Force killing process $pid..."
                kill -KILL "$pid" 2>/dev/null || true
            fi
        done
    fi
}

# Check for PID file first
if [ -f ipthing.pid ]; then
    pid=$(cat ipthing.pid)
    if kill -0 "$pid" 2>/dev/null; then
        echo "Stopping IPThing server (PID: $pid)..."
        kill -TERM "$pid" 2>/dev/null || true
        sleep 2
        if kill -0 "$pid" 2>/dev/null; then
            echo "Process still running, sending SIGKILL..."
            kill -KILL "$pid" 2>/dev/null || true
        fi
        rm -f ipthing.pid
        echo "Server stopped."
        exit 0
    else
        echo "No server running with PID: $pid"
        rm -f ipthing.pid
    fi
fi

# Look for running processes
echo "No PID file found. Searching for running processes..."

found_processes=0

# Stop go run processes
go_pids=$(pgrep -f "go run \\." 2>/dev/null || true)
if [ -n "$go_pids" ]; then
    stop_processes "$go_pids" "go run"
    found_processes=1
fi

# Stop binary processes
binary_pids=$(pgrep -f "/ipthing" 2>/dev/null || true)
if [ -n "$binary_pids" ]; then
    stop_processes "$binary_pids" "ipthing binary"
    found_processes=1
fi

if [ $found_processes -eq 0 ]; then
    echo "No running ipthing processes found."
else
    echo "All IPThing processes stopped."
fi
