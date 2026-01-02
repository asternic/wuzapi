package main

import (
	_ "unsafe"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
)

//go:linkname ClientSendNode go.mau.fi/whatsmeow.(*Client).sendNode
func ClientSendNode(c *whatsmeow.Client, node waBinary.Node) error
