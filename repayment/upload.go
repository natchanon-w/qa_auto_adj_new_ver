package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// crossbankFileMarker identifies output files belonging to the crossbank dataset by name —
// both REPAY_CROSS_BANK_*.csv/.encrypted and reconcile-lending-repayment_cross_bank_unmatched-*.json
// contain this substring. Everything else in output/ is treated as the regular repayment dataset.
const crossbankFileMarker = "CROSS_BANK"

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

	var repaymentFiles, crossbankFiles []string
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".encrypted") || strings.HasSuffix(e.Name(), ".json")) {
			continue
		}
		if strings.Contains(strings.ToUpper(e.Name()), crossbankFileMarker) {
			crossbankFiles = append(crossbankFiles, e.Name())
		} else {
			repaymentFiles = append(repaymentFiles, e.Name())
		}
	}

	if len(repaymentFiles) == 0 && len(crossbankFiles) == 0 {
		fmt.Println("No .encrypted or .json files found in output/. Run 'finalize' first.")
		os.Exit(1)
	}

	if len(crossbankFiles) > 0 && cfg.CrossbankBasePath == "" {
		fmt.Println("Found crossbank output files but config.json is missing \"crossbank_base_path\" for the active env.")
		os.Exit(1)
	}

	if len(repaymentFiles) > 0 {
		uploadGroup(cfg, "repayment", cfg.BasePath, outputDir, repaymentFiles)
	}
	if len(crossbankFiles) > 0 {
		if len(repaymentFiles) > 0 {
			fmt.Println()
		}
		uploadGroup(cfg, "crossbank", cfg.CrossbankBasePath, outputDir, crossbankFiles)
	}
}

// uploadGroup cleans basePath on S3 then uploads every file in files (from outputDir) into it.
func uploadGroup(cfg Config, label, basePath, outputDir string, files []string) {
	s3Base := fmt.Sprintf("s3://%s/%s", cfg.Bucket, basePath)
	fmt.Printf("[%s]\n", label)
	fmt.Printf("Bucket  : %s\n", cfg.Bucket)
	fmt.Printf("Path    : %s\n", basePath)
	fmt.Printf("Profile : %s\n", cfg.AwsProfile)
	if cfg.Region != "" {
		fmt.Printf("Region  : %s\n", cfg.Region)
	}
	fmt.Println()

	// regionArgs is appended to every aws invocation so requests hit the bucket's
	// own regional endpoint — without it, buckets in regions like ap-southeast-7
	// fail with IllegalLocationConstraintException.
	regionArgs := []string{}
	if cfg.Region != "" {
		regionArgs = []string{"--region", cfg.Region}
	}

	// Clean the S3 base path first
	fmt.Printf("Cleaning %s ...\n", s3Base)
	cleanArgs := append([]string{"s3", "rm", s3Base, "--recursive", "--profile", cfg.AwsProfile}, regionArgs...)
	cleanCmd := exec.Command("aws", cleanArgs...)
	cleanCmd.Stdout = os.Stdout
	cleanCmd.Stderr = os.Stderr
	if err := cleanCmd.Run(); err != nil {
		fmt.Printf("(clean warning — path may have been empty: %v)\n", err)
	}
	fmt.Println()

	// Upload each file
	for _, filename := range files {
		localPath := filepath.Join(outputDir, filename)
		s3Path := fmt.Sprintf("s3://%s/%s%s", cfg.Bucket, basePath, filename)
		fmt.Printf("Uploading %s\n  → %s\n", filename, s3Path)
		cpArgs := append([]string{"s3", "cp", localPath, s3Path, "--profile", cfg.AwsProfile}, regionArgs...)
		cmd := exec.Command("aws", cpArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Upload failed: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("\nUpload complete → %s\n", s3Base)
}
