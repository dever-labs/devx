package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Response struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func main() {
	if len(os.Args) < 2 {
		return
	}

	switch os.Args[1] {
	case "describe":
		resp := Response{Name: "echo", Version: "0.1.0"}
		_ = json.NewEncoder(os.Stdout).Encode(resp)
	case "render":
		fmt.Println("{}")
	}
}
