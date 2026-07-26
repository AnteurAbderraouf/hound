package main

import (
	"fmt"
	"os"
)

const version = "0.0.1"

func main() {
	fmt.Printf("hound v%s — retro-terminal DNS monitor for home networks\n", version)
	fmt.Println("early dev — see https://github.com/AnteurAbderraouf/hound")
	os.Exit(0)
}
