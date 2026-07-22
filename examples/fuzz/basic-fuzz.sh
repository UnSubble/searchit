#!/usr/bin/env bash
# Basic path fuzzing — replace FUZZ in URL with each word
searchit fuzz -u http://example.com/FUZZ -w words.txt

# URL parameter fuzzing
searchit fuzz -u 'http://example.com/search?q=FUZZ' -w words.txt
