// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

func TestCoverageSecondPartOnly(t *testing.T) {
	line := `127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET / HTTP/1.1" 200 12 "-" "agent"`
	part, err := secondPartOnly(line)
	if err != nil || part == "" || part[0] != '"' {
		t.Fatalf("second part parse failed: %q %v", part, err)
	}
	if _, err := secondPartOnly("missing quotes"); err == nil {
		t.Fatal("missing quote was accepted")
	}
}
