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

	rawCsvPath := filepath.Join(workDir, "raw.csv")
	header, rows, err := readCSVWithHeader(rawCsvPath)
	if err != nil {
		fmt.Printf("Error reading raw.csv: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Read %d rows from raw.csv\n", len(rows))

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

		plainFilename := state.CsvFilename
		if numChunks > 1 {
			base := strings.TrimSuffix(state.CsvFilename, ".csv")
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
			fmt.Printf("Encryption failed: %v\n", err)
			os.Exit(1)
		}

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
	controlFilename := fmt.Sprintf("reconcile-lending-void_itmx_billers-%s.json", uuidv4.String())
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

	// SQL — values are pre-formatted SQL fragments; emit verbatim
	const tableName = "bill_payment_transaction"
	const tableNameVoid = "bill_payment_transaction_void"
	const batchSize = 500
	sqlPath := filepath.Join(outputDir, "insert_void_transactions.sql")
	deleteSqlPath := filepath.Join(outputDir, "delete_void_transactions.sql")
	insertHeader := buildInsertHeader(tableName, sqlColumns)
	insertHeaderVoid := buildInsertHeader(tableNameVoid, sqlColumnsVoid)

	var sqlRowsVals []string
	var sqlRowsValsVoid []string
	var deleteStmts []string
	var deleteStmtsVoid []string
	for _, ref := range orderedRefs {
		row, ok := state.SqlRows[ref]
		if !ok {
			continue
		}
		var vals []string
		for _, col := range sqlColumns {
			vals = append(vals, row[col]) // already SQL-formatted
		}
		sqlRowsVals = append(sqlRowsVals, fmt.Sprintf("(%s)", strings.Join(vals, ", ")))
		deleteStmts = append(deleteStmts, fmt.Sprintf("DELETE FROM \"public\".\"%s\" WHERE \"ref_id\" = '%s';", tableName, ref))

		if rowVoid, ok := state.SqlRowsVoid[ref]; ok {
			var valsVoid []string
			for _, col := range sqlColumnsVoid {
				valsVoid = append(valsVoid, rowVoid[col]) // already SQL-formatted
			}
			sqlRowsValsVoid = append(sqlRowsValsVoid, fmt.Sprintf("(%s)", strings.Join(valsVoid, ", ")))
			// void.ref_id is independent of the main table's ref_id; retrieval_ref_no is the shared link
			deleteStmtsVoid = append(deleteStmtsVoid, fmt.Sprintf("DELETE FROM \"public\".\"%s\" WHERE \"retrieval_ref_no\" = %s;", tableNameVoid, rowVoid["retrieval_ref_no"]))
		}
	}

	fSql, _ := os.Create(sqlPath)
	// Wrapped in a single transaction: bill_payment_transaction_void.transaction_id is
	// resolved via a subquery against bill_payment_transaction (see generate.go), so if the
	// main insert fails or is skipped, this aborts the whole batch instead of the void insert
	// silently writing NULL and failing its own not-null constraint.
	fSql.WriteString("BEGIN;\n")
	for i := 0; i < len(sqlRowsVals); i += batchSize {
		end := i + batchSize
		if end > len(sqlRowsVals) {
			end = len(sqlRowsVals)
		}
		fSql.WriteString(insertHeader + strings.Join(sqlRowsVals[i:end], ",\n") + ";\n")
	}
	for i := 0; i < len(sqlRowsValsVoid); i += batchSize {
		end := i + batchSize
		if end > len(sqlRowsValsVoid) {
			end = len(sqlRowsValsVoid)
		}
		fSql.WriteString(insertHeaderVoid + strings.Join(sqlRowsValsVoid[i:end], ",\n") + ";\n")
	}
	fSql.WriteString("COMMIT;\n")
	fSql.Close()

	fDel, _ := os.Create(deleteSqlPath)
	fDel.WriteString("BEGIN;\n")
	// void rows reference the main table via ref_id, so delete them first
	fDel.WriteString(strings.Join(deleteStmtsVoid, "\n") + "\n")
	fDel.WriteString(strings.Join(deleteStmts, "\n") + "\n")
	fDel.WriteString("COMMIT;\n")
	fDel.Close()

	fmt.Printf("\nOutput → %s\n", outputDir)
	for _, fn := range encryptedFilenames {
		fmt.Printf("  ✓ %s\n", fn)
	}
	fmt.Printf("  ✓ %s\n", controlFilename)
	fmt.Printf("  ✓ insert_void_transactions.sql  (%d bill_payment_transaction rows, %d bill_payment_transaction_void rows)\n", len(sqlRowsVals), len(sqlRowsValsVoid))
	fmt.Printf("  ✓ delete_void_transactions.sql\n")
	fmt.Printf("\nRun 'go run . upload' to push to S3\n")
}

func buildInsertHeader(tableName string, columns []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("INSERT INTO \"public\".\"%s\" (", tableName))
	for i, c := range columns {
		sb.WriteString(fmt.Sprintf("\"%s\"", c))
		if i < len(columns)-1 {
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
