// Copyright (c) 2026 Michael D Henderson. All rights reserved.

//go:build !mattn

package sqliteinit

import (
	"fmt"
	"strings"
)

// pragma represents a SQLite pragma setting.
type pragma struct {
	name  string
	value string
}

// memoryPragmas are optimized for in-memory databases.
var memoryPragmas = []pragma{
	{name: "foreign_keys", value: "ON"},
	{name: "busy_timeout", value: "5000"},
	{name: "journal_mode", value: "MEMORY"},
	{name: "synchronous", value: "OFF"},
	{name: "temp_store", value: "MEMORY"},
	{name: "locking_mode", value: "EXCLUSIVE"},
}

// persistentPragmas are optimized for durable persistent databases.
var persistentPragmas = []pragma{
	{name: "foreign_keys", value: "ON"},
	{name: "busy_timeout", value: "5000"},
	{name: "journal_mode", value: "WAL"},
	{name: "synchronous", value: "NORMAL"},
	{name: "temp_store", value: "FILE"},
	{name: "locking_mode", value: "NORMAL"},
}

// buildDSN constructs a DSN for modernc.org/sqlite.
// modernc uses the syntax: file:path?_pragma=name(value)&_pragma=name2(value2)
func buildDSN(path string, pragmas []pragma) string {
	var sb strings.Builder

	if path == ":memory:" {
		sb.WriteString("file::memory:?cache=shared")
	} else {
		sb.WriteString("file:")
		sb.WriteString(path)
	}

	for i, p := range pragmas {
		if path == ":memory:" || i > 0 {
			sb.WriteString("&")
		} else {
			sb.WriteString("?")
		}
		fmt.Fprintf(&sb, "_pragma=%s(%s)", p.name, p.value)
	}

	return sb.String()
}
