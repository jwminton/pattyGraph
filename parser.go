// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// splits at the timestamp/request border. The request is the first quoted field
func secondPartOnly(line string) (string, error) {
	// Find the index of the first quote (start of the request)
	quoteIndex := strings.Index(line, "\"")
	if quoteIndex == -1 {
		return "", fmt.Errorf("log logLine does not contain a valid request")
	}

	// Split into two substrings
	//firstPart := strings.TrimSpace(logLine[:quoteIndex])  // From start to before the first quote
	secondPart := strings.TrimSpace(line[quoteIndex:]) // From the first quote onward

	return secondPart, nil
}

func findQuoteIndexes(line string) ([5]int, error) {
	var indexes [5]int
	count := 0

	for i := 0; i < len(line); i++ {
		if line[i] == '"' {
			if count < 5 {
				indexes[count] = i
				count++
				if count == 5 {
					break
				}
			}
		}
	}

	if count < 5 {
		return [5]int{}, fmt.Errorf("not enough quotes found, expected 5, got %d", count)
	}

	return indexes, nil
}

func findUserAgentCloseQuote(line string, userAgentOpenQuote int) (int, error) {
	end := strings.TrimRightFunc(line, unicode.IsSpace)
	if len(end) <= userAgentOpenQuote+1 || end[len(end)-1] != '"' {
		return 0, fmt.Errorf("log line missing user-agent closing quote")
	}

	if strings.HasSuffix(end, ` "-"`) {
		userAgentCloseQuote := len(end) - len(` "-"`) - 1
		if userAgentCloseQuote > userAgentOpenQuote {
			return userAgentCloseQuote, nil
		}
	}
	return len(end) - 1, nil
}

func splitLogLinePartsIntoCurrent() error {
	quoteIndex := strings.IndexByte(currentLine.logLine, '"')
	if quoteIndex == -1 {
		return fmt.Errorf("log line missing opening quote")
	}
	line := currentLine.logLine[quoteIndex:]
	quoteIndexes, err := findQuoteIndexes(line)
	if err != nil {
		return err
	}
	// Parse directly into fields
	//out.logLine = fullLine
	currentLine.request = line[quoteIndexes[0]+1 : quoteIndexes[1]]
	currentLine.respCode = line[quoteIndexes[1]+2 : quoteIndexes[1]+5]

	byteStr := line[quoteIndexes[1]+6 : quoteIndexes[2]-1]
	bytesVal, err := strconv.Atoi(byteStr)
	if err != nil {
		return fmt.Errorf("bad byte count: %w", err)
	}
	currentLine.bytesValue = bytesVal

	currentLine.referer = line[quoteIndexes[2]+1 : quoteIndexes[3]]
	userAgentCloseQuote, err := findUserAgentCloseQuote(line, quoteIndexes[4])
	if err != nil {
		return err
	}
	currentLine.userAgent = line[quoteIndexes[4]+1 : userAgentCloseQuote]

	return nil
}

// structured for efficiency in overall log line parsing
// splits out the 3 quoted strings and two ints in between as 5 returned strings.
func splitLogLineParts(fullLine string) (string, string, string, string, string, error) {
	quoteIndex := strings.Index(fullLine, "\"")
	if quoteIndex == -1 {
		return "", "", "", "", "", fmt.Errorf("log logLine does not contain a valid request")
	}

	// Split into two substrings
	//firstPart := strings.TrimSpace(logLine[:quoteIndex])  // From start to before the first quote
	line := strings.TrimSpace(fullLine[quoteIndex:]) // From the first quote onward

	// ^"GET /requested/url HTTP/1.1" 200 1234 "referer_url/text" "user agent text"
	// The first 5 double quotes are guaranteed to be present and delineate the request,
	// referer, and user-agent opening quote. The user-agent closing quote is found from
	// the right side so quote bytes inside user-agent content do not affect parsing.
	// Fields after the user-agent are ignored.
	// use these facts to avoid regex parsing
	quoteIndexes, err := findQuoteIndexes(line)
	if err != nil {
		return "", "", "", "", "", err
	}
	// The returned int slice did this:
	// ^"GET /requested/url HTTP/1.1" 200 1234 "referer_url/text" "user agent text"
	//  0                           1          2                3 4
	// Above are the quotesIndexes legend, no need to mentally juggle
	// messy but avoids regex and backtrack parsing
	request := line[quoteIndexes[0]+1 : quoteIndexes[1]]
	// response code is always 3 chars with a single space on either side
	resp := line[quoteIndexes[1]+2 : quoteIndexes[1]+5]
	// bytes returned bounds is a fixed distance from the request string end quote
	// and a fixed distance from the user agent start quote
	bytes := line[quoteIndexes[1]+6 : quoteIndexes[2]-1]
	referer := line[quoteIndexes[2]+1 : quoteIndexes[3]]
	userAgentCloseQuote, err := findUserAgentCloseQuote(line, quoteIndexes[4])
	if err != nil {
		return "", "", "", "", "", err
	}
	agent := line[quoteIndexes[4]+1 : userAgentCloseQuote]
	// no backtrack, no regex, log logLine parsing is an easy predictable pattern
	return request, resp, bytes, referer, agent, nil
}

// Executed once a cycle for status display only
func extractTimestamp(s string) (*time.Time, error) {
	start := strings.IndexByte(s, '[')
	if start == -1 || start+21 >= len(s) {
		return &time.Time{}, nil
	}
	// Layout expects 20 characters: "02/Jan/2006:15:04:05"
	timestampStr := s[start+1 : start+21] // safe slice
	t, err := time.ParseInLocation("02/Jan/2006:15:04:05", timestampStr, time.Local)
	return &t, err
}

// fast fields scratch areas. Each of these is call site specific to prevent clobbering while reusing the backing stores
// Each should have one usage each. Callers resize and reassign back into these slots!
var uaFieldsBuf []string = make([]string, 0, 50)
var refFieldsBuf []string = make([]string, 0, 50)
var botsFieldsBuf []string = make([]string, 0, 50)
var reqFieldsBuf []string = make([]string, 0, 50)
