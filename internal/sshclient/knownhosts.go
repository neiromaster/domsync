package sshclient

import (
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // hashed known_hosts entries are HMAC-SHA1 by definition
	"encoding/base64"
	"os"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

func knownHostAlgorithms(file, host string, port int) []string {
	data, err := os.ReadFile(file) //nolint:gosec // path is the user-configured known_hosts file
	if err != nil {
		return nil
	}
	target := normalizedHost(host, port)
	algorithms := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || strings.HasPrefix(fields[0], "#") || strings.HasPrefix(fields[0], "@") {
			continue
		}
		if !hostPatternMatches(fields[0], target) {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.Join(fields[1:], " ")))
		if err != nil {
			continue
		}
		if _, ok := seen[key.Type()]; ok {
			continue
		}
		seen[key.Type()] = struct{}{}
		algorithms = append(algorithms, key.Type())
	}
	if len(algorithms) == 0 {
		return nil
	}
	return algorithms
}

func normalizedHost(host string, port int) string {
	if port == 22 {
		return host
	}
	return "[" + host + "]:" + strconv.Itoa(port)
}

func hostPatternMatches(patterns, host string) bool {
	matched := false
	for _, pattern := range strings.Split(patterns, ",") {
		if strings.HasPrefix(pattern, "!") {
			if hostPatternMatch(pattern[1:], host) {
				return false
			}
			continue
		}
		if hostPatternMatch(pattern, host) {
			matched = true
		}
	}
	return matched
}

func hostPatternMatch(pattern, host string) bool {
	if strings.HasPrefix(pattern, "|1|") {
		return hashedHostMatches(pattern, host)
	}
	return wildcardMatch(pattern, host)
}

func wildcardMatch(pattern, value string) bool {
	pi, vi := 0, 0
	star, mark := -1, 0
	for vi < len(value) {
		switch {
		case pi < len(pattern) && (pattern[pi] == '?' || pattern[pi] == value[vi]):
			pi++
			vi++
		case pi < len(pattern) && pattern[pi] == '*':
			star, mark = pi, vi
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			vi = mark
		default:
			return false
		}
	}
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

func hashedHostMatches(pattern, host string) bool {
	parts := strings.Split(pattern, "|")
	if len(parts) != 4 || parts[1] != "1" {
		return false
	}
	salt, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	mac := hmac.New(sha1.New, salt)
	mac.Write([]byte(host))
	return hmac.Equal(mac.Sum(nil), want)
}
