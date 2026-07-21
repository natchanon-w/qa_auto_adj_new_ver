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
const batchSize = 500

func cmdFinalize(args []string) {
	workDir := resolveWorkDir(args)
	finalizeOne(workDir)
}

func finalizeOne(workDir string) {
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

	outputDir := filepath.Join(workDir, "output")
	os.MkdirAll(outputDir, os.ModePerm)

	pubKeyPath := filepath.Join(baseDir(), "public.pgp")
	privKeyPath := filepath.Join(baseDir(), "private.pgp")
	now := time.Now()

	insertSqlPath := filepath.Join(outputDir, "insert_transfer_offus_transactions.sql")
	updateSqlPath := filepath.Join(outputDir, "update_transfer_offus_transactions.sql")
	deleteSqlPath := filepath.Join(outputDir, "delete_transfer_offus_transactions.sql")
	fInsert, _ := os.Create(insertSqlPath)
	fUpdate, _ := os.Create(updateSqlPath)
	fDelete, _ := os.Create(deleteSqlPath)
	defer fInsert.Close()
	defer fUpdate.Close()
	defer fDelete.Close()

	var controlFilenames []string
	var encryptedFilenames []string
	insertRows, deleteRows := 0, 0

	for _, key := range typesOrder {
		ts, ok := state.Types[key]
		if !ok {
			continue
		}

		rawCsvPath := filepath.Join(workDir, ts.RawCsvFilename)
		header, rows, err := readCSVWithHeader(rawCsvPath)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", ts.RawCsvFilename, err)
			os.Exit(1)
		}
		fmt.Printf("[%s] Read %d rows from %s\n", key, len(rows), ts.RawCsvFilename)

		refColIdx := indexOf(header, "transaction_reference_id")
		if refColIdx < 0 {
			fmt.Printf("[%s] Column 'transaction_reference_id' not found in header\n", key)
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

		// Encrypt CSV (chunked if needed) + verify
		numChunks := 1
		if len(rows) > 0 {
			numChunks = (len(rows) + maxRowsPerFile - 1) / maxRowsPerFile
		}
		var typeEncFilenames []string
		var typeTotalRows []int
		var typeChecksums []string

		for i := 0; i < numChunks; i++ {
			start := i * maxRowsPerFile
			end := (i + 1) * maxRowsPerFile
			if end > len(rows) {
				end = len(rows)
			}
			chunk := rows[start:end]

			plainFilename := ts.CsvFilename
			if numChunks > 1 {
				base := strings.TrimSuffix(ts.CsvFilename, ".csv")
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

			fmt.Printf("[%s] Encrypting %s ...\n", key, plainFilename)
			if err := encryptFile(plainPath, encPath, pubKeyPath); err != nil {
				fmt.Printf("Encryption failed: %v\n", err)
				os.Exit(1)
			}
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
				fmt.Printf("Checksum mismatch for %s\n", plainFilename)
				os.Exit(1)
			}
			fmt.Printf("[%s] Verified (SHA256: %s)\n", key, origSum[:16]+"...")

			typeEncFilenames = append(typeEncFilenames, encFilename)
			typeTotalRows = append(typeTotalRows, len(chunk))
			typeChecksums = append(typeChecksums, origSum)
		}
		encryptedFilenames = append(encryptedFilenames, typeEncFilenames...)

		// Control JSON — matches real production reconcile-spending format
		// (verified against actual S3 downloads for transfer-off-us):
		// dataset "spending", table "transfer_off_us_<type>_auto_adjust", sha256, compression "PGP".
		uuidv4, _ := uuid.NewRandom()
		controlFilename := fmt.Sprintf("reconcile-spending-%s_auto_adjust-%s.json", ts.ControlSlug, uuidv4.String())
		controlPath := filepath.Join(outputDir, controlFilename)
		schemaList := make([]map[string]interface{}, len(header))
		for i, h := range header {
			schemaList[i] = map[string]interface{}{"name": h, "type": "string", "nullable": h != "reconcile_status"}
		}
		controlData := map[string]interface{}{
			"project":             "reconcile",
			"dataset":             "spending",
			"table":               ts.ControlSlug + "_auto_adjust",
			"sharding":            now.Format("2006-01-02"),
			"schema":              schemaList,
			"filename":            typeEncFilenames,
			"total_row":           typeTotalRows,
			"head_row":            1,
			"tail_row":            0,
			"file_check_sum":      typeChecksums,
			"check_sum_algorithm": "sha256",
			"format": map[string]string{
				"type":        "csv",
				"delimiter":   "|",
				"compression": "PGP",
			},
		}
		fJson, _ := os.Create(controlPath)
		enc := json.NewEncoder(fJson)
		enc.SetIndent("", "  ")
		enc.Encode(controlData)
		fJson.Close()
		controlFilenames = append(controlFilenames, controlFilename)

		// SQL — appended per type into the shared insert/update/delete files,
		// each type targets its own source table.
		insertHeader := buildInsertHeader(ts.Table, ts.SqlColumns)
		var sqlRowsVals []string
		var quotedRefs []string
		for _, ref := range orderedRefs {
			row, ok := ts.SqlRows[ref]
			if !ok {
				continue
			}
			var vals []string
			for _, col := range ts.SqlColumns {
				vals = append(vals, row[col])
			}
			sqlRowsVals = append(sqlRowsVals, fmt.Sprintf("(%s)", strings.Join(vals, ", ")))
			quotedRefs = append(quotedRefs, fmt.Sprintf("'%s'", ref))
		}
		for i := 0; i < len(sqlRowsVals); i += batchSize {
			end := i + batchSize
			if end > len(sqlRowsVals) {
				end = len(sqlRowsVals)
			}
			fInsert.WriteString(insertHeader + strings.Join(sqlRowsVals[i:end], ",\n") + ";\n")
		}
		insertRows += len(sqlRowsVals)

		for i := 0; i < len(quotedRefs); i += batchSize {
			end := i + batchSize
			if end > len(quotedRefs) {
				end = len(quotedRefs)
			}
			batch := strings.Join(quotedRefs[i:end], ", ")
			fUpdate.WriteString(fmt.Sprintf("UPDATE \"public\".\"%s\" SET \"%s\" = 'PROCESSING' WHERE \"%s\" IN (%s);\n", ts.Table, ts.StatusColumn, ts.RefColumn, batch))
			fDelete.WriteString(fmt.Sprintf("DELETE FROM \"public\".\"%s\" WHERE \"%s\" IN (%s);\n", ts.Table, ts.RefColumn, batch))
		}
		deleteRows += len(quotedRefs)
	}

	fmt.Printf("\nOutput → %s\n", outputDir)
	for _, fn := range encryptedFilenames {
		fmt.Printf("  ✓ %s\n", fn)
	}
	for _, fn := range controlFilenames {
		fmt.Printf("  ✓ %s\n", fn)
	}
	fmt.Printf("  ✓ insert_transfer_offus_transactions.sql  (%d rows)\n", insertRows)
	fmt.Printf("  ✓ update_transfer_offus_transactions.sql  (%d refs)\n", deleteRows)
	fmt.Printf("  ✓ delete_transfer_offus_transactions.sql  (%d refs)\n", deleteRows)
	fmt.Printf("\nRun 'go run . upload' to push to S3\n")
}

func buildInsertHeader(tableName string, cols []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("INSERT INTO \"public\".\"%s\" (", tableName))
	for i, c := range cols {
		sb.WriteString(fmt.Sprintf("\"%s\"", c))
		if i < len(cols)-1 {
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
