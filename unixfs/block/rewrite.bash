#!/bin/bash

# Find all .go files in the current directory and its subdirectories
# files=$(find . -type f -name '*.go')
files=( node.go )

# Loop over the files and replace the pattern
for file in $files; do
    echo $file
    # Use sed to replace the pattern
    sed -i 's/func Unmarshal\([A-Z][a-z]*\)(bcs \*block.Cursor) (*\([A-Z][a-z]*\), error) {/func Unmarshal\1(bcs *block.Cursor) (*\2, error) {\
        return block.UnmarshalBlock[*\2](bcs, func() block.Block { return &\2{} })\
    }/g' "$file"
done
