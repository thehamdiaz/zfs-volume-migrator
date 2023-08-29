#!/bin/bash

data_per_file=$((5 * 1024 * 1024 * 1024 / 10))  # 3GB divided by 10 files
output_dir="json_data_files"

mkdir -p "$output_dir"

for ((i=1; i<=10; i++)); do
    filename="$output_dir/data_$i.json"
    dd if=/dev/urandom bs=$data_per_file count=1 | jq -sRc . > "$filename"
done
