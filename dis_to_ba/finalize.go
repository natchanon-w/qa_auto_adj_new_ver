package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxRowsPerFile = 100000

func cmdFinalize(args []string) {
	workDir := resolveWorkDir(args)

	// Load state
	stateBytes, err := os.ReadFile(filepath.Join(workDir, "state.json"))
	if err != nil {
		fmt.Printf("Error reading state.json: %v\n", err)
		os.Exit(1)
	}
	var state StateFile
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		fmt.Printf("Error parsing state.json: %v\n", err)
		os.Exit(1)
	}

	cfg := readConfig()
	pathParts := strings.Split(strings.Trim(cfg.BasePath, "/"), "/")
	csvDateStr := pathParts[len(pathParts)-1]
	csvDate, err := time.Parse("2006-01-02", csvDateStr)
	if err != nil {
		fmt.Printf("Cannot parse date from base_path %q: %v\n", cfg.BasePath, err)
		os.Exit(1)
	}
	csvFilename := fmt.Sprintf("DISBURSE_TO_BA_RECONCILE_UNMATCHED_%s.csv", csvDate.Format("20060102"))
	if state.CsvFilename != csvFilename {
		fmt.Printf("Warning: config base_path date does not match generated filename — using config date (%s)\n", csvDate.Format("2006-01-02"))
	}

	// Read raw.csv (user may have edited it)
	rawCsvPath := filepath.Join(workDir, "raw.csv")
	header, rows, err := readCSVWithHeader(rawCsvPath)
	if err != nil {
		fmt.Printf("Error reading raw.csv: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Read %d rows from raw.csv\n", len(rows))

	// Collect reconcile_ref_ids in CSV order (deduped)
	refColIdx := indexOf(header, "reconcile_ref_id")
	if refColIdx < 0 {
		fmt.Println("Column 'reconcile_ref_id' not found in CSV header")
		os.Exit(1)
	}
	seen := make(map[string]bool)
	var orderedRefs []string
	for _, row := range rows {
		if refColIdx < len(row) {
			ref := row[refColIdx]
			if ref != "" && !seen[ref] {
				seen[ref] = true
				orderedRefs = append(orderedRefs, ref)
			}
		}
	}

	outputDir := filepath.Join(workDir, "output")
	os.MkdirAll(outputDir, os.ModePerm)

	pubKeyPath := filepath.Join(baseDir(), "public.pgp")
	privKeyPath := filepath.Join(baseDir(), "private.pgp")

	now := time.Now()
	numChunks := 1
	if len(rows) > 0 {
		numChunks = (len(rows) + maxRowsPerFile - 1) / maxRowsPerFile
	}

	var encryptedFilenames []string
	var totalRows []int
	var checksums []string

	for i := 0; i < numChunks; i++ {
		start := i * maxRowsPerFile
		end := (i + 1) * maxRowsPerFile
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]

		plainFilename := csvFilename
		if numChunks > 1 {
			base := strings.TrimSuffix(csvFilename, ".csv")
			plainFilename = fmt.Sprintf("%s_%d.csv", base, i+1)
		}
		encFilename := plainFilename + ".encrypted"
		plainPath := filepath.Join(outputDir, plainFilename)
		encPath := filepath.Join(outputDir, encFilename)

		// Write plain chunk to output/
		f, err := os.Create(plainPath)
		if err != nil {
			panic(err)
		}
		f.WriteString(strings.Join(header, "|") + "\n")
		for _, row := range chunk {
			f.WriteString(strings.Join(row, "|") + "\n")
		}
		f.Close()

		// Encrypt
		fmt.Printf("[%d/%d] Encrypting %s ...\n", i+1, numChunks, plainFilename)
		if err := encryptFile(plainPath, encPath, pubKeyPath); err != nil {
			fmt.Printf("Encryption failed: %v\n", err)
			os.Exit(1)
		}

		// Verify: decrypt and compare SHA256
		fmt.Printf("[%d/%d] Verifying with private.pgp ...\n", i+1, numChunks)
		decBytes, err := decryptToBytes(encPath, privKeyPath)
		if err != nil {
			fmt.Printf("Decryption verification failed: %v\n", err)
			os.Exit(1)
		}
		tmpPath := encPath + ".verify_tmp"
		os.WriteFile(tmpPath, decBytes, 0644)
		origSum := sha256File(plainPath)
		decSum := sha256File(tmpPath)
		os.Remove(tmpPath)
		if origSum != decSum {
			fmt.Printf("Checksum mismatch for %s:\n  original:  %s\n  decrypted: %s\n", plainFilename, origSum, decSum)
			os.Exit(1)
		}
		fmt.Printf("[%d/%d] Verification passed (SHA256: %s)\n", i+1, numChunks, origSum[:16]+"...")

		encryptedFilenames = append(encryptedFilenames, encFilename)
		totalRows = append(totalRows, len(chunk))
		checksums = append(checksums, origSum)
	}

	// Control JSON
	uuidv4, _ := uuid.NewRandom()
	controlFilename := fmt.Sprintf("reconcile-lending-disburse_to_ba-%s.json", uuidv4.String())
	controlPath := filepath.Join(outputDir, controlFilename)

	schemaList := make([]map[string]string, len(header))
	for i, h := range header {
		schemaList[i] = map[string]string{"name": h, "type": "string"}
	}
	controlData := map[string]interface{}{
		"project":             "reconcile",
		"dataset":             "VFS-DCB",
		"table":               "disburse_to_ba",
		"sharding":            now.Format("2006-01-02"),
		"schema":              schemaList,
		"filename":            encryptedFilenames,
		"total_row":           totalRows,
		"head_row":            1,
		"tail_row":            0,
		"file_check_sum":      checksums,
		"check_sum_algorithm": "sha256",
		"format": map[string]string{
			"type":        "csv",
			"delimiter":   "|",
			"compression": "",
		},
	}
	fJson, _ := os.Create(controlPath)
	enc := json.NewEncoder(fJson)
	enc.SetIndent("", "  ")
	enc.Encode(controlData)
	fJson.Close()

	// SQL — only for refs present in the (possibly edited) CSV
	sqlPath := filepath.Join(outputDir, "insert_disburse_to_transactions.sql")
	deleteSqlPath := filepath.Join(outputDir, "delete_disburse_to_transactions.sql")
	const tableName = "bill_payment_transaction"
	const batchSize = 500

	insertHeader := buildInsertHeader(tableName)

	var sqlRowsVals []string
	var sqlRefs []string
	for _, ref := range orderedRefs {
		row, ok := state.SqlRows[ref]
		if !ok {
			continue
		}
		var vals []string
		for _, col := range sqlColumns {
			val := row[col]
			if val == "" {
				vals = append(vals, "NULL")
			} else {
				vals = append(vals, fmt.Sprintf("'%s'", strings.ReplaceAll(val, "'", "''")))
			}
		}
		sqlRowsVals = append(sqlRowsVals, fmt.Sprintf("(%s)", strings.Join(vals, ", ")))
		sqlRefs = append(sqlRefs, fmt.Sprintf("'%s'", ref))
	}

	fSql, _ := os.Create(sqlPath)
	for i := 0; i < len(sqlRowsVals); i += batchSize {
		end := i + batchSize
		if end > len(sqlRowsVals) {
			end = len(sqlRowsVals)
		}
		fSql.WriteString(insertHeader + strings.Join(sqlRowsVals[i:end], ",\n") + ";\n")
	}
	fSql.Close()

	fDel, _ := os.Create(deleteSqlPath)
	for i := 0; i < len(sqlRefs); i += batchSize {
		end := i + batchSize
		if end > len(sqlRefs) {
			end = len(sqlRefs)
		}
		fDel.WriteString(fmt.Sprintf("DELETE FROM \"public\".\"%s\" WHERE \"ref_id\" IN (%s);\n",
			tableName, strings.Join(sqlRefs[i:end], ", ")))
	}
	fDel.Close()

	fmt.Printf("\nOutput → %s\n", outputDir)
	for _, fn := range encryptedFilenames {
		fmt.Printf("  ✓ %s\n", fn)
	}
	fmt.Printf("  ✓ %s\n", controlFilename)
	fmt.Printf("  ✓ insert_disburse_to_transactions.sql  (%d rows)\n", len(sqlRowsVals))
	fmt.Printf("  ✓ delete_disburse_to_transactions.sql\n")
	fmt.Printf("\nRun 'go run . upload' to push to S3\n")
}

func buildInsertHeader(tableName string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("INSERT INTO \"public\".\"%s\" (", tableName))
	for i, c := range sqlColumns {
		sb.WriteString(fmt.Sprintf("\"%s\"", c))
		if i < len(sqlColumns)-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString(") VALUES ")
	return sb.String()
}

func readCSVWithHeader(path string) ([]string, [][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var header []string
	var rows [][]string
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			header = strings.Split(line, "|")
			continue
		}
		if line == "" {
			continue
		}
		rows = append(rows, strings.Split(line, "|"))
	}
	return header, rows, scanner.Err()
}

func indexOf(slice []string, val string) int {
	for i, s := range slice {
		if s == val {
			return i
		}
	}
	return -1
}
