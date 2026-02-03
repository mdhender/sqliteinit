// Copyright (c) 2026 Michael D Henderson. All rights reserved.

//go:build mattn

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
	{name: "_foreign_keys", value: "1"},
	{name: "_busy_timeout", value: "5000"},
	{name: "_journal_mode", value: "MEMORY"},
	{name: "_synchronous", value: "OFF"},
	{name: "_txlock", value: "exclusive"},
}

// persistentPragmas are optimized for durable persistent databases.
var persistentPragmas = []pragma{
	{name: "_foreign_keys", value: "1"},
	{name: "_busy_timeout", value: "5000"},
	{name: "_journal_mode", value: "WAL"},
	{name: "_synchronous", value: "NORMAL"},
}

// buildDSN constructs a DSN for github.com/mattn/go-sqlite3.
// mattn uses the syntax: file:path?_foreign_keys=1&_journal_mode=WAL
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
		fmt.Fprintf(&sb, "%s=%s", p.name, p.value)
	}

	return sb.String()
}
