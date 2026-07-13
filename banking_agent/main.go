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
		fmt.Println("  generate <times> [type]  Generate raw CSV and state.json")
		fmt.Println("                           type: regular (default) | copay | all")
		fmt.Println("                           all generates both types, each into its own work dir")
		fmt.Println("  finalize [dir]           Encrypt, verify, build control JSON and SQL scripts")
		fmt.Println("  upload   [dir] [--keep]  Upload output files to S3 (reads config.json)")
		fmt.Println("                           --keep skips wiping base_path first —")
		fmt.Println("                           use it to add a second file type (e.g. copay)")
		fmt.Println("                           alongside one already uploaded")
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
