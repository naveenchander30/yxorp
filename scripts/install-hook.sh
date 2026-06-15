#!/bin/bash

# Script to install the pre-commit hook

HOOK_SRC="scripts/pre-commit"
HOOK_DEST=".git/hooks/pre-commit"

if [ ! -d ".git" ]; then
    echo "ERROR: Not in root of git repository"
    exit 1
fi

if [ ! -f "$HOOK_SRC" ]; then
    echo "ERROR: Hook source file not found at $HOOK_SRC"
    exit 1
fi

echo "Installing Git pre-commit hook..."
cp "$HOOK_SRC" "$HOOK_DEST"

if [ $? -eq 0 ]; then
    chmod +x "$HOOK_DEST"
    echo "✅ Success: Git pre-commit hook installed successfully at $HOOK_DEST"
    exit 0
else
    echo "❌ Error: Failed to copy pre-commit hook to $HOOK_DEST"
    exit 1
fi
