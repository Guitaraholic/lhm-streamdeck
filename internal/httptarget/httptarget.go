// Package httptarget normalises user-typed host/port pairs into base URLs
// for the HTTP-backed source kinds (SparkDash, CLI Proxy).
package httptarget

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Base builds an http(s) origin from a host/port pair.
//
//	443        → https://host        (HTTPS reverse proxy)
//	80         → http://host         (HTTP, typically redirected to HTTPS)
//	defaultPort and every other port → http://host:port (direct listener)
//
// A pasted https:// URL in host is honoured; if the port is still the
// source's defaultPort, it becomes 443.
func Base(host string, port, defaultPort int) string {
	host = strings.TrimSpace(host)
	scheme := "http"

	if i := strings.Index(host, "://"); i >= 0 {
		if u, err := url.Parse(host); err == nil && u.Host != "" {
			if u.Scheme == "http" || u.Scheme == "https" {
				scheme = u.Scheme
			}
			if h := u.Hostname(); h != "" {
				host = h
			}
			if up := u.Port(); up != "" {
				if n, err := strconv.Atoi(up); err == nil {
					port = n
				}
			} else if scheme == "https" && (port <= 0 || port == defaultPort) {
				port = 443
			}
		} else {
			host = strings.TrimPrefix(host, "https://")
			host = strings.TrimPrefix(host, "http://")
			host = strings.TrimRight(host, "/")
		}
	}

	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 || port > 65535 {
		port = defaultPort
	}
	if port == 443 {
		scheme = "https"
	}

	omitPort := (scheme == "https" && port == 443) || (scheme == "http" && port == 80)
	if omitPort {
		return scheme + "://" + host
	}
	return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port))
}
