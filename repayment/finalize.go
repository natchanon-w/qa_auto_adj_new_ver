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

	if state.Repayment == nil && state.Crossbank == nil {
		fmt.Println("state.json has no datasets — nothing to finalize. Run 'generate' first.")
		os.Exit(1)
	}

	outputDir := filepath.Join(workDir, "output")
	os.MkdirAll(outputDir, os.ModePerm)

	pubKeyPath := filepath.Join(baseDir(), "public.pgp")
	privKeyPath := filepath.Join(baseDir(), "private.pgp")

	var summaries []string

	// datasetRun pairs a dataset with the label used to tag each of its rows in the combined
	// SQL scripts below — repayment and crossbank rows share one insert/delete script (both
	// target repayment_transactions), distinguished only by a "-- dataset: <label> | ref_id:
	// <ref>" comment above each row/statement.
	type datasetRun struct {
		ds    *CsvDataset
		label string // "repayment" | "crossbank"
	}
	var runs []datasetRun
	if state.Repayment != nil {
		runs = append(runs, datasetRun{ds: state.Repayment, label: "repayment"})
	}
	if state.Crossbank != nil {
		runs = append(runs, datasetRun{ds: state.Crossbank, label: "crossbank"})
	}

	type labeledRow struct {
		label string
		ref   string
		vals  string // already-joined, SQL-formatted column values
	}
	var rows []labeledRow
	for _, run := range runs {
		refs, err := processDataset(run.ds, workDir, outputDir, pubKeyPath, privKeyPath, &summaries)
		if err != nil {
			fmt.Printf("Error finalizing %s dataset: %v\n", run.label, err)
			os.Exit(1)
		}
		for _, ref := range refs {
			row, ok := state.SqlRows[ref]
			if !ok {
				continue
			}
			var vals []string
			for _, col := range sqlColumns {
				vals = append(vals, row[col]) // already SQL-formatted
			}
			rows = append(rows, labeledRow{label: run.label, ref: ref, vals: strings.Join(vals, ", ")})
		}
	}

	// SQL — values are pre-formatted SQL fragments; emit verbatim. One combined insert/delete
	// script covers every row generated this run; each row/statement carries its own
	// "-- dataset: ... | ref_id: ..." comment so crossbank vs normal repayment rows (and
	// which CSV row each maps to) are identifiable at a glance.
	const tableName = "repayment_transactions"
	const batchSize = 500
	insertHeader := buildInsertHeader(tableName)

	fSql, _ := os.Create(filepath.Join(outputDir, "insert_repayment_transactions.sql"))
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		fSql.WriteString(insertHeader + "\n")
		for j := i; j < end; j++ {
			r := rows[j]
			fSql.WriteString(fmt.Sprintf("-- dataset: %s | ref_id: %s\n", r.label, r.ref))
			sep := ","
			if j == end-1 {
				sep = ";"
			}
			fSql.WriteString(fmt.Sprintf("(%s)%s\n", r.vals, sep))
		}
	}
	fSql.Close()

	fDel, _ := os.Create(filepath.Join(outputDir, "delete_repayment_transactions.sql"))
	for _, r := range rows {
		fDel.WriteString(fmt.Sprintf("-- dataset: %s | ref_id: %s\n", r.label, r.ref))
		fDel.WriteString(fmt.Sprintf("DELETE FROM \"public\".\"%s\" WHERE \"ref_id\" = '%s';\n", tableName, r.ref))
	}
	fDel.Close()

	summaries = append(summaries, fmt.Sprintf("insert_repayment_transactions.sql  (%d rows)", len(rows)))
	summaries = append(summaries, "delete_repayment_transactions.sql")

	fmt.Printf("\nOutput → %s\n", outputDir)
	for _, s := range summaries {
		fmt.Printf("  ✓ %s\n", s)
	}
	fmt.Printf("\nRun 'go run . upload' to push to S3\n")
}

// processDataset encrypts+verifies one dataset's raw CSV, writes its control JSON, appends
// human-readable summary lines to *summaries, and returns the CSV's reconcile_ref_id values
// in file order (deduped) so the caller can look them up in state.SqlRows.
func processDataset(ds *CsvDataset, workDir, outputDir, pubKeyPath, privKeyPath string, summaries *[]string) ([]string, error) {
	rawCsvPath := filepath.Join(workDir, ds.RawFilename)
	header, rows, err := readCSVWithHeader(rawCsvPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ds.RawFilename, err)
	}
	fmt.Printf("Read %d rows from %s\n", len(rows), ds.RawFilename)

	refColIdx := indexOf(header, "reconcile_ref_id")
	if refColIdx < 0 {
		return nil, fmt.Errorf("column 'reconcile_ref_id' not found in %s header", ds.RawFilename)
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

		plainFilename := ds.CsvFilename
		if numChunks > 1 {
			base := strings.TrimSuffix(ds.CsvFilename, ".csv")
			plainFilename = fmt.Sprintf("%s_%d.csv", base, i+1)
		}
		encFilename := plainFilename + ".encrypted"
		plainPath := filepath.Join(outputDir, plainFilename)
		encPath := filepath.Join(outputDir, encFilename)

		f, err := os.Create(plainPath)
		if err != nil {
			panic(err)
		}
		f.WriteString(strings.Join(header, "|") + "\n")
		for _, row := range chunk {
			f.WriteString(strings.Join(row, "|") + "\n")
		}
		f.Close()

		fmt.Printf("[%d/%d] Encrypting %s ...\n", i+1, numChunks, plainFilename)
		if err := encryptFile(plainPath, encPath, pubKeyPath); err != nil {
			return nil, fmt.Errorf("encryption failed: %w", err)
		}

		fmt.Printf("[%d/%d] Verifying with private.pgp ...\n", i+1, numChunks)
		decBytes, err := decryptToBytes(encPath, privKeyPath)
		if err != nil {
			return nil, fmt.Errorf("decryption verification failed: %w", err)
		}
		tmpPath := encPath + ".verify_tmp"
		os.WriteFile(tmpPath, decBytes, 0644)
		origSum := sha256File(plainPath)
		decSum := sha256File(tmpPath)
		os.Remove(tmpPath)
		if origSum != decSum {
			return nil, fmt.Errorf("checksum mismatch for %s:\n  original:  %s\n  decrypted: %s", plainFilename, origSum, decSum)
		}
		fmt.Printf("[%d/%d] Verification passed (SHA256: %s)\n", i+1, numChunks, origSum[:16]+"...")

		encryptedFilenames = append(encryptedFilenames, encFilename)
		totalRows = append(totalRows, len(chunk))
		checksums = append(checksums, origSum)
	}

	// Control JSON
	uuidv4, _ := uuid.NewRandom()
	uuidHex := strings.ReplaceAll(uuidv4.String(), "-", "")
	controlFilename := fmt.Sprintf("reconcile-lending-%s-%s.json", ds.CtrlPrefix, uuidHex)
	controlPath := filepath.Join(outputDir, controlFilename)

	schemaList := make([]map[string]string, len(header))
	for i, h := range header {
		schemaList[i] = map[string]string{"name": h, "type": "string"}
	}
	controlData := map[string]interface{}{
		"project":             "reconcile",
		"dataset":             "VFS-DCB",
		"table":               "sams_vfs_dcb",
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

	for _, fn := range encryptedFilenames {
		*summaries = append(*summaries, fn)
	}
	*summaries = append(*summaries, controlFilename)

	return orderedRefs, nil
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
