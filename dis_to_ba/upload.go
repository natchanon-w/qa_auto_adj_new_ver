package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cmdUpload(args []string) {
	workDir := resolveWorkDir(args)
	cfg := readConfig()

	outputDir := filepath.Join(workDir, "output")
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		fmt.Printf("Output directory not found: %s\nRun 'finalize' first.\n", outputDir)
		os.Exit(1)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		fmt.Printf("Error reading output dir: %v\n", err)
		os.Exit(1)
	}

	var toUpload []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".encrypted") || strings.HasSuffix(e.Name(), ".json")) {
			toUpload = append(toUpload, e.Name())
		}
	}

	if len(toUpload) == 0 {
		fmt.Println("No .encrypted or .json files found in output/. Run 'finalize' first.")
		os.Exit(1)
	}

	s3Base := fmt.Sprintf("s3://%s/%s", cfg.Bucket, cfg.BasePath)
	fmt.Printf("Bucket  : %s\n", cfg.Bucket)
	fmt.Printf("Path    : %s\n", cfg.BasePath)
	fmt.Printf("Profile : %s\n", cfg.AwsProfile)
	fmt.Println()

	// Clean S3 base path first
	fmt.Printf("Cleaning %s ...\n", s3Base)
	cleanCmd := exec.Command("aws", "s3", "rm", s3Base, "--recursive", "--profile", cfg.AwsProfile)
	cleanCmd.Stdout = os.Stdout
	cleanCmd.Stderr = os.Stderr
	if err := cleanCmd.Run(); err != nil {
		fmt.Printf("(clean warning — path may have been empty: %v)\n", err)
	}
	fmt.Println()

	// Upload each file
	for _, filename := range toUpload {
		localPath := filepath.Join(outputDir, filename)
		s3Path := fmt.Sprintf("s3://%s/%s%s", cfg.Bucket, cfg.BasePath, filename)
		fmt.Printf("Uploading %s\n  → %s\n", filename, s3Path)
		cmd := exec.Command("aws", "s3", "cp", localPath, s3Path, "--profile", cfg.AwsProfile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("\nUpload complete → %s\n", s3Base)
}
