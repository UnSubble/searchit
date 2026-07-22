#!/usr/bin/env bash
# Fuzz using a raw HTTP request template
# Placeholders (FUZZ, FOO, etc.) work inside the template file
searchit fuzz --request requests/post-login.http -w passwords.txt
