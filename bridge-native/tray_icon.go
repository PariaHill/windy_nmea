package main

import _ "embed"

//go:embed assets/icon.ico
var trayIcon []byte

func trayIconBytes() []byte {
	return trayIcon
}
