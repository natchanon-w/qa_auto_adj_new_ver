package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . <command> [work-dir]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  generate <times>  Generate raw CSV and state.json")
		fmt.Println("  finalize [dir]    Encrypt, verify, build control JSON and SQL scripts")
		fmt.Println("  upload   [dir]    Upload output files to S3 (reads config.json)")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate":
		cmdGenerate(os.Args[2:])
	case "finalize":
		cmdFinalize(os.Args[2:])
	case "upload":
		cmdUpload(os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
