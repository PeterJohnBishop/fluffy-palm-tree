package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// generate a random 32 bit string for master key
func main() {
	b := make([]byte, 32)
	rand.Read(b)
	fmt.Println(hex.EncodeToString(b))
}
