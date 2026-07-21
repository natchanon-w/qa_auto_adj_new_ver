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
		fmt.Println("  generate <records> [types]  Generate raw_<type>.csv files and state.json")
		fmt.Println("                               types: comma-separated subset of")
		fmt.Println("                               inbound_promptpay,inbound_actual_account,")
		fmt.Println("                               outbound_promptpay,outbound_actual_account")
		fmt.Println("                               (default: all four, matching a real batch)")
		fmt.Println("  finalize [dir]               Encrypt, verify, build control JSONs and SQL scripts")
		fmt.Println("                               no dir: finalizes the latest work dir")
		fmt.Println("  upload   [dir] [--keep]      Upload output files to S3 (reads config.json)")
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
